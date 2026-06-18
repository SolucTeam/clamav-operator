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
const { scanPaths, getStats } = require('./scanner');
const { generateReport, getDirSizeBytes, getFileSizeBytes } = require('./report');
const {
  loadCache,
  saveCache,
  resolveEffectiveStrategy,
  getIncrementalStats,
  getCacheStats,
} = require('./incremental');

// Cache file path — mirrors the path built in incremental.js so we can stat it.
const path = require('path');
const CACHE_FILE_PATH = path.join(
  process.env.RESULTS_DIR || '/results',
  `${process.env.NODE_NAME || 'unknown'}_scan_cache.json`
);

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

    // ── Scan all configured paths ──────────────────────────────────────────
    // Standalone: walks all paths then runs a single clamscan --file-list
    // subprocess (DB loaded once). Remote: per-file concurrent clamdscan calls.
    await scanPaths(clamscan, CONFIG.pathsToScan, results, effectiveStrategy);

    // ── Save incremental cache for next run ───────────────────────────────
    const cacheEntriesPruned = await saveCache();
    // Read cache stats AFTER saveCache() so counts reflect post-pruning state.
    const cacheStats = getCacheStats();

    // ── Generate report files ─────────────────────────────────────────────
    const stats = getStats();
    const incrementalStats = getIncrementalStats();
    const { reportsRotated } = await generateReport(results, stats, incrementalStats, effectiveStrategy);

    // ── Signature database freshness ─────────────────────────────────────
    // Find the most recently modified ClamAV signature file in the db path.
    // Candidates: daily.cvd, daily.cld, main.cvd, main.cld, bytecode.cvd, etc.
    // The mtime of the newest file is the effective "signatures last updated" time.
    let signatureDbMtimeSeconds = 0;
    try {
      const sigFiles = ['daily.cvd', 'daily.cld', 'main.cvd', 'main.cld', 'bytecode.cvd', 'bytecode.cld'];
      const mtimes = await Promise.all(
        sigFiles.map(f =>
          fs.stat(`${CONFIG.clamavDbPath}/${f}`)
            .then(s => s.mtimeMs / 1000)
            .catch(() => 0)
        )
      );
      signatureDbMtimeSeconds = Math.max(...mtimes);
    } catch {
      signatureDbMtimeSeconds = 0;
    }

    // ── Disk usage stats (hostPath: /var/log/clamav-scans on the node) ───
    // Collected after rotation so sizes reflect the retained-only files.
    const [resultsDirBytes, cacheFileBytes] = await Promise.all([
      getDirSizeBytes(CONFIG.resultsDir),
      getFileSizeBytes(CACHE_FILE_PATH),
    ]);

    // ── Capture process resource usage ───────────────────────────────────
    // These reflect only the Node.js orchestrator process (not child processes
    // such as clamscan). In standalone mode, clamscan's CPU and memory are
    // tracked by cAdvisor (container_cpu_usage_seconds_total,
    // container_memory_working_set_bytes). In remote mode, the Node.js process
    // IS the primary consumer, so these values are more representative.
    const cpuUsage = process.cpuUsage();          // {user, system} in microseconds
    const memUsage = process.memoryUsage();       // {rss, heapTotal, heapUsed, ...} in bytes

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
      // Maintenance stats — consumed by parseJobResults to emit Prometheus metrics
      reports_rotated: reportsRotated,
      cache_entries_pruned: cacheEntriesPruned,
      // Disk usage of the hostPath results directory (/var/log/clamav-scans on node)
      // and the incremental cache file — both exposed as Prometheus gauges.
      results_dir_bytes: resultsDirBytes,
      cache_file_bytes: cacheFileBytes,
      // Logical cache stats (post-pruning): number of files tracked and their
      // total size in bytes. Distinct from cache_file_bytes (JSON file on disk).
      cache_tracked_files: cacheStats.trackedFiles,
      cache_tracked_bytes: cacheStats.trackedBytes,
      // Cache age in seconds (time since the cache file was last written).
      // -1 = no valid cache was loaded (first scan, invalidated, or corrupted).
      cache_age_seconds: cacheStats.cacheAgeSeconds,
      // Reason the cache was discarded: 'first_scan' | 'signature_change' |
      // 'corrupted' | null (null = cache loaded OK, no invalidation).
      cache_invalidation_reason: cacheStats.invalidationReason,
      // Unix mtime of the most recently modified ClamAV signature file.
      // Used by the operator to compute clamav_signature_db_age_seconds.
      // 0 = signatures not found or could not be stat'd.
      signature_db_mtime_seconds: signatureDbMtimeSeconds,
      // Node.js process resource usage at scan completion (scanner v0.7+).
      // NOTE: in standalone mode these reflect only the orchestrator wrapper, not
      // clamscan. Use cAdvisor container_* metrics for full container accounting.
      memory_rss_bytes: memUsage.rss,
      cpu_user_seconds: Math.round(cpuUsage.user / 1e4) / 100, // µs → s (2 decimal places)
    });

    process.exit(0); // always 0 — scan outcome is carried in the report/logs, not the exit code
  } catch (error) {
    logger.error('Fatal error', { error: error.message, stack: error.stack });
    process.exit(1);
  }
}

// ── Graceful shutdown ─────────────────────────────────────────────────────────
async function shutdown(signal, exitCode) {
  logger.warn(`${signal} received — scan interrupted, persisting cache and exiting`, {
    exit_code: exitCode,
  });
  // Best-effort: persist whatever incremental cache we have so the next run
  // can skip already-scanned files even if this run was interrupted.
  await saveCache().catch(() => {});
  // Exit NON-zero (128+signal, the standard shell convention). Exiting 0 here
  // made an interrupted scan count as a Succeeded pod: the Job (and therefore
  // the NodeScan) was reported Completed even though the filesystem was only
  // partially scanned — a false "clean" signal for a security tool. With a
  // non-zero exit the Job records the failure and the NodeScan ends up Failed
  // with an explicit exit code instead.
  process.exit(exitCode);
}

process.on('SIGTERM', () => shutdown('SIGTERM', 143)); // 128 + 15
process.on('SIGINT',  () => shutdown('SIGINT',  130)); // 128 + 2

main();
