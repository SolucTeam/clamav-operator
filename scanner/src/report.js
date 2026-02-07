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

const { CONFIG, INCREMENTAL_CONFIG } = require('./config');
const logger = require('./logger');

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
 * @returns {object} The report object (for logging)
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

  logger.info('Rapport généré', { reportPath, summaryPath });
  return report;
}

module.exports = { generateReport };
