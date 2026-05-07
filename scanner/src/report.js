/*
Copyright 2025 The ClamAV Operator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

'use strict';

const fs = require('fs').promises;
const path = require('path');

const { CONFIG, INCREMENTAL_CONFIG, REPORT_CONFIG } = require('./config');
const logger = require('./logger');

/**
 * Returns the total size in bytes of all files directly inside dirPath.
 * Used to expose hostPath disk usage as a Prometheus metric.
 * @param {string} dirPath
 * @returns {Promise<number>}
 */
async function getDirSizeBytes(dirPath) {
  try {
    const entries = await fs.readdir(dirPath);
    let total = 0;
    for (const entry of entries) {
      try {
        const stat = await fs.stat(path.join(dirPath, entry));
        if (stat.isFile()) total += stat.size;
      } catch { /* skip unreadable entries */ }
    }
    return total;
  } catch {
    return 0;
  }
}

/**
 * Returns the size in bytes of a single file, or 0 if it does not exist.
 * @param {string} filePath
 * @returns {Promise<number>}
 */
async function getFileSizeBytes(filePath) {
  try {
    const stat = await fs.stat(filePath);
    return stat.size;
  } catch {
    return 0;
  }
}

/**
 * Generate JSON report + short text summary and write them to RESULTS_DIR.
 *
 * The text summary is used by the operator controller to quickly determine
 * the scan outcome (STATUS=CLEAN|INFECTED) without parsing JSON.
 *
 * @param {object} results          – { infected: [], errors: [] }
 * @param {object} stats            – from scanner.getStats()
 * @param {object} incrementalStats – from incremental.getIncrementalStats()
 * @param {string} effectiveStrategy
 * @returns {{ report: object, reportsRotated: number }}
 */
async function generateReport(results, stats, incrementalStats, effectiveStrategy) {
  const duration = Math.round((Date.now() - stats.startTime) / 1000);
  const dateStr = new Date().toISOString().replace(/[:.]/g, '-');

  const report = {
    node: CONFIG.nodeName,
    scanMode: CONFIG.scanMode,
    scanDate: new Date().toISOString(),
    duration,
    strategy: effectiveStrategy,
    incremental: INCREMENTAL_CONFIG.enabled
      ? {
          enabled: true,
          filesSkipped: incrementalStats.filesSkipped,
          cacheHits: incrementalStats.cacheHits,
          cacheMisses: incrementalStats.cacheMisses,
          newFiles: incrementalStats.newFiles,
          modifiedFiles: incrementalStats.modifiedFiles,
        }
      : { enabled: false },
    statistics: {
      filesScanned: stats.filesScanned,
      filesInfected: stats.filesInfected,
      filesSkipped: stats.filesSkipped,
      errors: stats.errors,
    },
    infected: results.infected,
    // Cap errors in the report to avoid oversized payloads
    errors: results.errors.slice(0, 100),
  };

  // ── Write JSON report ──────────────────────────────────────────────────
  const reportPath = path.join(
    CONFIG.resultsDir,
    `${CONFIG.nodeName}_scan_${dateStr}.json`
  );
  await fs.writeFile(reportPath, JSON.stringify(report, null, 2));

  // ── Write short text summary ───────────────────────────────────────────
  const summaryPath = path.join(
    CONFIG.resultsDir,
    `${CONFIG.nodeName}_summary_${dateStr}.txt`
  );
  const summaryLines = [
    `STATUS=${results.infected.length > 0 ? 'INFECTED' : 'CLEAN'}`,
    `NODE=${CONFIG.nodeName}`,
    `MODE=${CONFIG.scanMode}`,
    `STRATEGY=${effectiveStrategy}`,
    `SCANNED=${stats.filesScanned}`,
    `INFECTED=${stats.filesInfected}`,
    `SKIPPED=${stats.filesSkipped}`,
    `ERRORS=${stats.errors}`,
    `DURATION=${duration}s`,
  ];
  await fs.writeFile(summaryPath, summaryLines.join('\n'));

  logger.info('Report generated', { reportPath, summaryPath });

  // ── Rotate old reports ─────────────────────────────────────────────────
  // Keep only the N most recent scan reports per node to prevent unbounded
  // disk growth on the hostPath. JSON reports and their matching .txt
  // summaries are rotated together. Rotation is best-effort: failures are
  // logged but never fatal.
  const reportsRotated = await rotateReports();

  return { report, reportsRotated };
}

/**
 * Delete the oldest scan reports beyond the configured retention limit.
 * Operates only on files matching this node's naming pattern so reports
 * from other nodes sharing the same hostPath directory are untouched.
 */
async function rotateReports() {
  const limit = REPORT_CONFIG.maxReports;
  if (limit <= 0) return; // rotation disabled

  try {
    const entries = await fs.readdir(CONFIG.resultsDir);

    // Collect JSON reports for this node only (pattern: {nodeName}_scan_*.json)
    const prefix = `${CONFIG.nodeName}_scan_`;
    const jsonReports = entries
      .filter((f) => f.startsWith(prefix) && f.endsWith('.json'))
      .sort(); // ISO date in filename → lexicographic = chronological

    const excess = jsonReports.length - limit;
    if (excess <= 0) return;

    // Delete the oldest `excess` reports (and their matching .txt summaries)
    const toDelete = jsonReports.slice(0, excess);
    for (const jsonFile of toDelete) {
      const jsonPath = path.join(CONFIG.resultsDir, jsonFile);
      // Derive matching summary filename: {nodeName}_summary_{dateStr}.txt
      const dateStr = jsonFile.slice(prefix.length, -'.json'.length);
      const summaryFile = `${CONFIG.nodeName}_summary_${dateStr}.txt`;
      const summaryPath = path.join(CONFIG.resultsDir, summaryFile);

      await fs.unlink(jsonPath).catch((err) =>
        logger.warn('Failed to delete old report', { file: jsonPath, error: err.message })
      );
      await fs.unlink(summaryPath).catch((err) =>
        logger.warn('Failed to delete old summary', { file: summaryPath, error: err.message })
      );
    }

    logger.info('Report rotation complete', {
      deleted: toDelete.length,
      kept: limit,
      limit,
    });
    return toDelete.length;
  } catch (err) {
    logger.warn('Report rotation failed', { error: err.message });
    return 0;
  }
}

module.exports = { generateReport, rotateReports, getDirSizeBytes, getFileSizeBytes };
