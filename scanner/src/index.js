#!/usr/bin/env node
/*
Copyright 2025 The ClamAV Operator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

ClamAV Node Scanner – Standalone & Remote modes
================================================
This is the main entry-point for the scanner container.  It is executed as a
Kubernetes Job by the clamav-operator controller.

Supported modes (set via SCAN_MODE env var):
  • standalone — uses the local clamscan binary; signatures must be present in
                 the image (air-gap) or updated via freshclam at boot.
  • remote    — connects to a central clamd service (legacy behaviour).

Incremental scanning is supported in both modes:
  • full        — scan every file every time.
  • incremental — only scan new / modified files since the last run.
  • smart       — alternate between incremental and full scans automatically.
*/

'use strict';

const fs = require('fs').promises;
const { CONFIG, INCREMENTAL_CONFIG } = require('./config');
const logger = require('./logger');
const { initScanner } = require('./init-scanner');
const { scanDirectory, getStats } = require('./scanner');
const { generateReport } = require('./report');
const {
  loadCache,
  saveCache,
  resolveEffectiveStrategy,
  getIncrementalStats,
} = require('./incremental');

// =============================================================================
// MAIN
// =============================================================================

async function main() {
  logger.info('Starting ClamAV scan', {
    node: CONFIG.nodeName,
    mode: CONFIG.scanMode,
    paths: CONFIG.pathsToScan,
    incremental_enabled: INCREMENTAL_CONFIG.enabled,
    incremental_strategy: INCREMENTAL_CONFIG.strategy,
    update_signatures: CONFIG.updateSignatures,
  });

  const results = { infected: [], errors: [] };

  try {
    // ── Ensure results directory exists ────────────────────────────────────
    await fs.mkdir(CONFIG.resultsDir, { recursive: true }).catch(() => {});

    // ── Load incremental cache (if any) ───────────────────────────────────
    await loadCache();

    // ── Determine effective strategy for this run ─────────────────────────
    const effectiveStrategy = await resolveEffectiveStrategy();
    logger.info('Effective strategy', { strategy: effectiveStrategy });

    // ── Init ClamAV scanner (standalone or remote) ────────────────────────
    const clamscan = await initScanner();

    // ── Walk & scan every configured path ─────────────────────────────────
    for (const scanPath of CONFIG.pathsToScan) {
      try {
        await fs.access(scanPath);
        logger.info('Scanning path', { path: scanPath });
        await scanDirectory(clamscan, scanPath, results, effectiveStrategy);
      } catch {
        logger.warn('Path not found — skipping', { path: scanPath });
      }
    }

    // ── Save incremental cache for next run ───────────────────────────────
    await saveCache();

    // ── Generate report files ─────────────────────────────────────────────
    const stats = getStats();
    const incrementalStats = getIncrementalStats();
    await generateReport(results, stats, incrementalStats, effectiveStrategy);

    // ── Final log line — parsed by the Go operator ────────────────────────
    // IMPORTANT: the `type` field is the machine-readable signal consumed by
    // the operator's parseJobResults function. Do NOT rename or remove it.
    logger.info('Scan completed successfully', {
      type: 'scan_complete',
      duration: Math.round((Date.now() - stats.startTime) / 1000),
      files_scanned: stats.filesScanned,
      files_infected: stats.filesInfected,
      files_skipped: stats.filesSkipped,
      errors_count: stats.errors,
      status: results.infected.length > 0 ? 'INFECTED' : 'CLEAN',
      // Incremental stats — consumed by parseJobResults to populate
      // NodeScan.Status.StrategyUsed / FilesSkippedIncremental / CacheHitRate
      strategy: effectiveStrategy,
      files_skipped_incremental: incrementalStats.filesSkipped,
      cache_hits: incrementalStats.cacheHits,
      cache_misses: incrementalStats.cacheMisses,
    });

    process.exit(0); // always 0 — scan outcome is carried in the report/logs, not the exit code
  } catch (error) {
    logger.error('Fatal error', { error: error.message, stack: error.stack });
    process.exit(1);
  }
}

// ── Graceful shutdown ─────────────────────────────────────────────────────────
async function shutdown(signal) {
  logger.info(`${signal} received — graceful shutdown`);
  // Best-effort: persist whatever incremental cache we have so the next run
  // can skip already-scanned files even if this run was interrupted.
  await saveCache().catch(() => {});
  process.exit(0);
}

process.on('SIGTERM', () => shutdown('SIGTERM'));
process.on('SIGINT',  () => shutdown('SIGINT'));

main();
