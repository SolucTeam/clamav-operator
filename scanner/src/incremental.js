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

// =============================================================================
// In-memory scan cache
//
// The cache maps absolute file paths → { modTime, size, lastScanned, scanResult }.
// It is populated during the current run and can be loaded/saved from a
// JSON file so the next Job (on the same node) can skip unchanged files.
//
// SIGNATURE FINGERPRINTING
// ────────────────────────
// When ClamAV signatures are updated (daily.cvd, main.cvd, etc.) the cached
// "clean" results for previously-scanned files are no longer trustworthy — a
// new signature might detect malware in a file that was previously clean.
//
// To handle this, the cache stores a "signature fingerprint": the combined
// mtime+size of each signature file in CONFIG.clamavDbPath. On load, if the
// current fingerprint differs from the stored one, the cache is discarded and
// the scan runs as a full scan, guaranteeing every file is re-evaluated against
// the latest signatures.
// =============================================================================

let SCAN_CACHE = {};

// Tracks every file path encountered during the current scan walk.
// Populated by shouldScanFile() (skipped files) and updateCache() (scanned files).
// Used by saveCache() to prune stale entries for files that no longer exist
// on the node filesystem — prevents unbounded cache growth over time.
const SEEN_FILES = new Set();

const CACHE_FILE = path.join(
  process.env.RESULTS_DIR || '/results',
  `${process.env.NODE_NAME || 'unknown'}_scan_cache.json`
);

// ── Signature fingerprint ────────────────────────────────────────────────────

/**
 * Computes a lightweight fingerprint of the ClamAV signature files in dbPath.
 * Uses mtime (seconds) + size of each recognised signature file.
 * Returns a stable string, or null if the directory cannot be read.
 *
 * @param {string} dbPath  Path to the ClamAV database directory.
 * @returns {Promise<string|null>}
 */
async function computeSignatureFingerprint(dbPath) {
  const sigExtensions = ['.cvd', '.cld'];
  try {
    const entries = await fs.readdir(dbPath);
    const sigFiles = entries
      .filter((f) => sigExtensions.includes(path.extname(f)))
      .sort(); // deterministic order

    if (sigFiles.length === 0) return null;

    const parts = await Promise.all(
      sigFiles.map(async (f) => {
        try {
          const stat = await fs.stat(path.join(dbPath, f));
          return `${f}:${Math.floor(stat.mtimeMs / 1000)}:${stat.size}`;
        } catch {
          return `${f}:missing`;
        }
      })
    );
    return parts.join('|');
  } catch (err) {
    logger.warn('Could not compute signature fingerprint', { dbPath, error: err.message });
    return null;
  }
}

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

  // Compute the current signature fingerprint before loading the cache.
  // If it differs from what the cache recorded, we must discard the cache so
  // every file is re-scanned against the updated signatures.
  const currentFingerprint = await computeSignatureFingerprint(CONFIG.clamavDbPath);

  try {
    const raw = await fs.readFile(CACHE_FILE, 'utf-8');
    const data = JSON.parse(raw);
    if (data && typeof data === 'object' && data.files) {

      // ── Signature-change invalidation ────────────────────────────────────
      if (
        currentFingerprint !== null &&
        data.signatureFingerprint &&
        data.signatureFingerprint !== currentFingerprint
      ) {
        logger.warn(
          'ClamAV signatures have been updated since the last scan — ' +
          'discarding incremental cache to ensure all files are re-scanned',
          {
            storedFingerprint: data.signatureFingerprint,
            currentFingerprint,
          }
        );
        SCAN_CACHE = {};
        return; // force full scan
      }

      SCAN_CACHE = data.files;
      logger.info('Incremental cache loaded', {
        entries: Object.keys(SCAN_CACHE).length,
        cacheVersion: data.version || 'unknown',
        lastScanDate: data.lastScanDate || 'unknown',
        signatureFingerprint: data.signatureFingerprint || 'none',
      });
    }
  } catch {
    logger.info('No previous cache found — full scan');
    SCAN_CACHE = {};
  }
}

async function saveCache() {
  if (!INCREMENTAL_CONFIG.enabled) return;

  // Persist the current signature fingerprint alongside the cache so the next
  // run can detect if signatures were updated between scans.
  const signatureFingerprint = await computeSignatureFingerprint(CONFIG.clamavDbPath);

  // Prune stale entries — keep only files that were encountered during this
  // scan walk (either scanned or skipped-as-unchanged). Files deleted from the
  // node filesystem since the last run will not be in SEEN_FILES and are
  // dropped here, preventing unbounded cache growth over time.
  // Exception: if SEEN_FILES is empty the walk was likely aborted early
  // (e.g. graceful shutdown) — in that case skip pruning to avoid wiping a
  // valid cache based on an incomplete run.
  let prunedCache = SCAN_CACHE;
  if (SEEN_FILES.size > 0) {
    const before = Object.keys(SCAN_CACHE).length;
    prunedCache = Object.fromEntries(
      Object.entries(SCAN_CACHE).filter(([filePath]) => SEEN_FILES.has(filePath))
    );
    const pruned = before - Object.keys(prunedCache).length;
    if (pruned > 0) {
      logger.info('Pruned stale cache entries for deleted files', { pruned, before, after: Object.keys(prunedCache).length });
    }
  }

  const payload = {
    version: 3,
    lastScanDate: new Date().toISOString(),
    node: process.env.NODE_NAME || 'unknown',
    totalFiles: Object.keys(prunedCache).length,
    signatureFingerprint: signatureFingerprint || '',
    files: prunedCache,
  };

  const tmpFile = `${CACHE_FILE}.tmp`;
  try {
    await fs.writeFile(tmpFile, JSON.stringify(payload));
    await fs.rename(tmpFile, CACHE_FILE);
    logger.info('Incremental cache saved', {
      entries: payload.totalFiles,
      signatureFingerprint: payload.signatureFingerprint || 'unavailable',
    });
  } catch (err) {
    logger.warn('Failed to save cache', { error: err.message });
    // Best-effort cleanup of tmp file
    await fs.unlink(tmpFile).catch(() => {});
  }
}

// ── Determine effective strategy for this run ────────────────────────────────

/**
 * For the "smart" strategy we alternate between incremental and full scans.
 * A small marker file tracks how many incremental runs have happened since
 * the last full scan.
 */
async function resolveEffectiveStrategy() {
  // ForceFullScan: per-scan override that bypasses any incremental strategy.
  // Set by the operator when NodeScan.Spec.ForceFullScan is true.
  if (process.env.FORCE_FULL_SCAN === 'true') return 'full';
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
  // Mark as seen so saveCache() keeps this entry (file still exists on disk).
  SEEN_FILES.add(filePath);
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
  // Mark as seen so saveCache() keeps this entry.
  SEEN_FILES.add(filePath);
}

module.exports = {
  loadCache,
  saveCache,
  resolveEffectiveStrategy,
  shouldScanFile,
  updateCache,
  getIncrementalStats,
  computeSignatureFingerprint, // exported for testing
};
