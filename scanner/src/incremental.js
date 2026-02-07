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
const { INCREMENTAL_CONFIG } = require('./config');
const logger = require('./logger');

// =============================================================================
// In-memory scan cache
//
// The cache maps absolute file paths → { modTime, size, lastScanned, scanResult }.
// It is populated during the current run and can be loaded/saved from a
// JSON file so the next Job (on the same node) can skip unchanged files.
// =============================================================================

let SCAN_CACHE = {};

const CACHE_FILE = path.join(
  process.env.RESULTS_DIR || '/results',
  `${process.env.NODE_NAME || 'unknown'}_scan_cache.json`
);

// ── Stats ────────────────────────────────────────────────────────────────────

const incrementalStats = {
  filesSkipped: 0,
  cacheHits: 0,
  cacheMisses: 0,
  newFiles: 0,
  modifiedFiles: 0,
};

function getIncrementalStats() {
  return { ...incrementalStats };
}

// ── Cache persistence ────────────────────────────────────────────────────────

async function loadCache() {
  if (!INCREMENTAL_CONFIG.enabled) return;

  try {
    const raw = await fs.readFile(CACHE_FILE, 'utf-8');
    const data = JSON.parse(raw);
    if (data && typeof data === 'object' && data.files) {
      SCAN_CACHE = data.files;
      logger.info('Cache incrémental chargé', {
        entries: Object.keys(SCAN_CACHE).length,
        cacheVersion: data.version || 'unknown',
        lastScanDate: data.lastScanDate || 'unknown',
      });
    }
  } catch {
    logger.info('Pas de cache précédent trouvé — scan complet');
    SCAN_CACHE = {};
  }
}

async function saveCache() {
  if (!INCREMENTAL_CONFIG.enabled) return;

  const payload = {
    version: 2,
    lastScanDate: new Date().toISOString(),
    node: process.env.NODE_NAME || 'unknown',
    totalFiles: Object.keys(SCAN_CACHE).length,
    files: SCAN_CACHE,
  };

  try {
    await fs.writeFile(CACHE_FILE, JSON.stringify(payload));
    logger.info('Cache incrémental sauvegardé', { entries: payload.totalFiles });
  } catch (err) {
    logger.warn('Impossible de sauvegarder le cache', { error: err.message });
  }
}

// ── Determine effective strategy for this run ────────────────────────────────

/**
 * For the "smart" strategy we alternate between incremental and full scans.
 * A small marker file tracks how many incremental runs have happened since
 * the last full scan.
 */
async function resolveEffectiveStrategy() {
  if (!INCREMENTAL_CONFIG.enabled) return 'full';
  if (INCREMENTAL_CONFIG.strategy !== 'smart') return INCREMENTAL_CONFIG.strategy;

  // Smart: count incremental runs since last full
  const markerFile = path.join(
    process.env.RESULTS_DIR || '/results',
    `${process.env.NODE_NAME || 'unknown'}_smart_counter.txt`
  );

  let counter = 0;
  try {
    const raw = await fs.readFile(markerFile, 'utf-8');
    counter = parseInt(raw.trim(), 10) || 0;
  } catch {
    /* first run */
  }

  if (counter >= INCREMENTAL_CONFIG.fullScanInterval) {
    // Time for a full scan — reset counter
    await fs.writeFile(markerFile, '0').catch(() => {});
    logger.info('Smart strategy: full scan triggered', {
      counter,
      interval: INCREMENTAL_CONFIG.fullScanInterval,
    });
    return 'full';
  }

  // Increment counter for next run
  await fs.writeFile(markerFile, String(counter + 1)).catch(() => {});
  logger.info('Smart strategy: incremental scan', {
    counter: counter + 1,
    nextFullAt: INCREMENTAL_CONFIG.fullScanInterval,
  });
  return 'incremental';
}

// ── Should we scan a given file? ─────────────────────────────────────────────

/**
 * Returns { shouldScan: boolean, reason: string }.
 *
 * @param {string}   filePath
 * @param {import('fs').Stats} fileStats
 * @param {string}   effectiveStrategy  – "full" or "incremental"
 */
function shouldScanFile(filePath, fileStats, effectiveStrategy) {
  if (effectiveStrategy === 'full') {
    return { shouldScan: true, reason: 'full_scan' };
  }

  const cached = SCAN_CACHE[filePath];

  if (!cached) {
    incrementalStats.newFiles++;
    incrementalStats.cacheMisses++;
    return { shouldScan: true, reason: 'new_file' };
  }

  incrementalStats.cacheHits++;

  const fileMtime = Math.floor(fileStats.mtimeMs / 1000);

  if (fileMtime > cached.modTime || fileStats.size !== cached.size) {
    incrementalStats.modifiedFiles++;
    return { shouldScan: true, reason: 'modified' };
  }

  // Check max age — rescan even if unchanged after N hours
  if (INCREMENTAL_CONFIG.maxFileAgeHours > 0 && cached.lastScanned) {
    const ageHours = (Date.now() / 1000 - cached.lastScanned) / 3600;
    if (ageHours > INCREMENTAL_CONFIG.maxFileAgeHours) {
      return { shouldScan: true, reason: 'max_age_exceeded' };
    }
  }

  incrementalStats.filesSkipped++;
  return { shouldScan: false, reason: 'unchanged' };
}

// ── Update cache after scanning a file ───────────────────────────────────────

function updateCache(filePath, fileStats, scanResult) {
  SCAN_CACHE[filePath] = {
    modTime: Math.floor(fileStats.mtimeMs / 1000),
    size: fileStats.size,
    lastScanned: Math.floor(Date.now() / 1000),
    scanResult, // 'clean' | 'infected'
  };
}

module.exports = {
  loadCache,
  saveCache,
  resolveEffectiveStrategy,
  shouldScanFile,
  updateCache,
  getIncrementalStats,
};
