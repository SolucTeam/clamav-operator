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
const {
  shouldScanFile,
  updateCache,
  getIncrementalStats,
} = require('./incremental');

// =============================================================================
// Global scan statistics (populated during the run)
// =============================================================================

const stats = {
  filesScanned: 0,
  filesInfected: 0,
  filesSkipped: 0,
  errors: 0,
  startTime: Date.now(),
};

function getStats() {
  return { ...stats };
}

// =============================================================================
// Exclusion filter
// =============================================================================

function shouldExclude(filePath) {
  return CONFIG.excludePatterns.some((re) => re.test(filePath));
}

// =============================================================================
// Scan a single file
// =============================================================================

/**
 * @param {import('clamscan')} clamscan
 * @param {string}             filePath
 * @param {string}             effectiveStrategy
 */
async function scanFile(clamscan, filePath, effectiveStrategy) {
  if (shouldExclude(filePath)) {
    stats.filesSkipped++;
    return { skipped: true, reason: 'excluded' };
  }

  let fileStats;
  try {
    fileStats = await fs.stat(filePath);
  } catch (err) {
    stats.errors++;
    return { error: true, file: filePath, message: err.message };
  }

  if (!fileStats.isFile()) {
    stats.filesSkipped++;
    return { skipped: true, reason: 'not_regular_file' };
  }

  if (fileStats.size === 0) {
    stats.filesSkipped++;
    return { skipped: true, reason: 'empty_file' };
  }

  if (fileStats.size > CONFIG.maxFileSize) {
    stats.filesSkipped++;
    logger.debug('Fichier trop volumineux — ignoré', {
      file: filePath,
      size: fileStats.size,
      maxFileSize: CONFIG.maxFileSize,
    });
    return { skipped: true, reason: 'too_large' };
  }

  // ── Incremental check ──────────────────────────────────────────────────
  if (INCREMENTAL_CONFIG.enabled) {
    const { shouldScan, reason } = shouldScanFile(filePath, fileStats, effectiveStrategy);
    if (!shouldScan) {
      return { skipped: true, reason, incremental: true };
    }
  }

  // ── Actual ClamAV scan ─────────────────────────────────────────────────
  try {
    const { file, isInfected, viruses } = await clamscan.isInfected(filePath);
    stats.filesScanned++;

    if (INCREMENTAL_CONFIG.enabled) {
      updateCache(filePath, fileStats, isInfected ? 'infected' : 'clean');
    }

    if (isInfected) {
      stats.filesInfected++;
      logger.warn('Fichier infecté détecté', {
        alert: 'INFECTED_FILE',
        file_path: file,
        virus_names: viruses,
        file_size: fileStats.size,
      });
      return { infected: true, file, viruses };
    }

    return { infected: false, file };
  } catch (err) {
    stats.errors++;
    logger.error('Erreur lors du scan', { file: filePath, error: err.message });
    return { error: true, file: filePath, message: err.message };
  }
}

// =============================================================================
// Recursively scan a directory
// =============================================================================

/**
 * @param {import('clamscan')} clamscan
 * @param {string}             dirPath
 * @param {object}             results         — { infected: [], errors: [] }
 * @param {string}             effectiveStrategy
 */
async function scanDirectory(clamscan, dirPath, results, effectiveStrategy) {
  let entries;
  try {
    entries = await fs.readdir(dirPath, { withFileTypes: true });
  } catch (err) {
    logger.error('Erreur répertoire', { directory: dirPath, error: err.message });
    return;
  }

  const files = [];
  const dirs = [];

  for (const entry of entries) {
    const fullPath = path.join(dirPath, entry.name);
    if (shouldExclude(fullPath)) continue;
    if (entry.isDirectory()) dirs.push(fullPath);
    else if (entry.isFile()) files.push(fullPath);
  }

  // Scan files in batches of maxConcurrent
  for (let i = 0; i < files.length; i += CONFIG.maxConcurrent) {
    const batch = files.slice(i, i + CONFIG.maxConcurrent);
    const batchResults = await Promise.all(
      batch.map((f) => scanFile(clamscan, f, effectiveStrategy))
    );

    for (const result of batchResults) {
      if (result.infected) {
        results.infected.push({ file: result.file, viruses: result.viruses });
      } else if (result.error) {
        results.errors.push({ file: result.file, error: result.message });
      }
    }

    // Progress logging every 500 files
    const incremental = getIncrementalStats();
    const total = stats.filesScanned + incremental.filesSkipped;
    if (total > 0 && total % 500 === 0) {
      logger.info('Progression', {
        scanned: stats.filesScanned,
        skipped_incremental: incremental.filesSkipped,
        skipped_other: stats.filesSkipped,
        infected: stats.filesInfected,
        errors: stats.errors,
      });
    }
  }

  // Recurse into subdirectories
  for (const subDir of dirs) {
    await scanDirectory(clamscan, subDir, results, effectiveStrategy);
  }
}

module.exports = { scanDirectory, getStats };
