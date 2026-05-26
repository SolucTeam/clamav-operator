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
const os = require('os');
const { spawn } = require('child_process');
const readline = require('readline');

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
// STANDALONE MODE
//
// Strategy: load the ClamAV signature DB exactly once per scan job.
//
//   Phase 1 — Walk: traverse all scan paths in JS, apply exclude + incremental
//             filters, collect accepted files into a list.
//   Phase 2 — Scan: write the list to a temp file, spawn a single
//             `clamscan --file-list` subprocess. The DB is loaded once
//             regardless of the number of files, eliminating the N × 800 Mi
//             memory spike that occurred when N concurrent subprocesses each
//             loaded the DB simultaneously.
//   Phase 3 — Cache: update the incremental cache for every file that was
//             submitted for scanning.
//
// Memory profile:
//   Peak (DB decompression at startup) : ~800 Mi  (one-time)
//   Steady-state during scan           : ~400 Mi  (one-time)
//   vs. old approach with MaxConcurrent=N: N × 800 Mi peak → OOMKill
// =============================================================================

/**
 * Phase 1: Recursively walk dirPath, applying exclude + incremental filters.
 * Appends accepted file paths to filesToScan and their Stats to fileStatsMap.
 *
 * @param {string}             dirPath
 * @param {string[]}           filesToScan    — accumulator
 * @param {Map<string,object>} fileStatsMap   — path → fs.Stats
 * @param {string}             effectiveStrategy
 */
async function walkDirectory(dirPath, filesToScan, fileStatsMap, effectiveStrategy) {
  let entries;
  try {
    entries = await fs.readdir(dirPath, { withFileTypes: true });
  } catch (err) {
    logger.error('Directory read error', { directory: dirPath, error: err.message });
    return;
  }

  for (const entry of entries) {
    const fullPath = path.join(dirPath, entry.name);

    if (shouldExclude(fullPath)) continue;

    if (entry.isDirectory()) {
      await walkDirectory(fullPath, filesToScan, fileStatsMap, effectiveStrategy);
      continue;
    }

    if (!entry.isFile()) continue;

    let fileStats;
    try {
      fileStats = await fs.stat(fullPath);
    } catch (err) {
      stats.errors++;
      logger.debug('stat error — skipping', { file: fullPath, error: err.message });
      continue;
    }

    if (fileStats.size === 0) {
      stats.filesSkipped++;
      continue;
    }

    if (fileStats.size > CONFIG.maxFileSize) {
      stats.filesSkipped++;
      logger.debug('File too large — skipping', {
        file: fullPath,
        size: fileStats.size,
        maxFileSize: CONFIG.maxFileSize,
      });
      continue;
    }

    // Incremental check — skip files unchanged since last scan
    if (INCREMENTAL_CONFIG.enabled) {
      const { shouldScan } = shouldScanFile(fullPath, fileStats, effectiveStrategy);
      if (!shouldScan) {
        stats.filesSkipped++;
        continue;
      }
    }

    filesToScan.push(fullPath);
    fileStatsMap.set(fullPath, fileStats);
  }
}

/**
 * Phase 2: Spawn a single `clamscan --file-list` subprocess.
 * Streams stdout line-by-line; does not buffer the full output in memory.
 * Updates stats, results, and incremental cache in place.
 *
 * @param {object}             scanner      — { clamscanPath, dbPath }
 * @param {string[]}           filesToScan
 * @param {Map<string,object>} fileStatsMap
 * @param {object}             results      — { infected: [], errors: [] }
 */
async function scanWithClamscan(scanner, filesToScan, fileStatsMap, results) {
  if (filesToScan.length === 0) {
    logger.info('No files to scan after filtering');
    return;
  }

  // Write file list to a temp file — clamscan reads one path per line.
  const fileListPath = path.join(os.tmpdir(), `clamav-scan-list-${Date.now()}.txt`);
  await fs.writeFile(fileListPath, filesToScan.join('\n') + '\n');

  logger.info('Launching clamscan', {
    files_to_scan: filesToScan.length,
    db_path: scanner.dbPath,
  });

  const infectedPaths = new Set();
  const errorPaths    = new Set();

  try {
    await new Promise((resolve, reject) => {
      const child = spawn(scanner.clamscanPath, [
        `--database=${scanner.dbPath}`,
        `--file-list=${fileListPath}`,
        // Output only infected / error lines — suppresses "OK" lines for large scans.
        '--infected',
        '--no-summary',
        // Prevent clamscan from hanging on pathological archives.
        `--max-filesize=${CONFIG.maxFileSize}`,
        '--max-scansize=419430400', // 400 MB per archive
        '--max-recursion=16',
      ]);

      const rl = readline.createInterface({ input: child.stdout, crlfDelay: Infinity });

      rl.on('line', (line) => {
        const trimmed = line.trim();
        if (!trimmed) return;

        // Infected: "/path/to/file: VirusName FOUND"
        const foundMatch = trimmed.match(/^(.+): (.+) FOUND$/);
        if (foundMatch) {
          const [, filePath, virusName] = foundMatch;
          infectedPaths.add(filePath);
          stats.filesInfected++;
          results.infected.push({ file: filePath, viruses: [virusName] });
          logger.warn('Infected file detected', {
            alert: 'INFECTED_FILE',
            file_path: filePath,
            virus_names: [virusName],
          });
          return;
        }

        // Error: "/path/to/file: Some message ERROR"
        const errorMatch = trimmed.match(/^(.+): (.+) ERROR$/);
        if (errorMatch) {
          const [, filePath, errorMsg] = errorMatch;
          errorPaths.add(filePath);
          stats.errors++;
          results.errors.push({ file: filePath, error: errorMsg });
        }
      });

      child.stderr.on('data', (data) => {
        const text = data.toString().trim();
        if (text) logger.debug('clamscan stderr', { output: text.slice(0, 500) });
      });

      child.on('error', reject);
      child.on('close', (code) => {
        // clamscan exit codes: 0 = all clean, 1 = infected found, 2 = error/crash
        if (code === 0 || code === 1) {
          resolve();
        } else {
          reject(new Error(`clamscan exited with code ${code}`));
        }
      });
    });
  } finally {
    await fs.unlink(fileListPath).catch(() => {});
  }

  // Every submitted file that did not error counts as scanned.
  stats.filesScanned = filesToScan.length - errorPaths.size;

  // Phase 3: update incremental cache for all submitted files.
  // A file is clean if it is not in infectedPaths or errorPaths.
  if (INCREMENTAL_CONFIG.enabled) {
    for (const filePath of filesToScan) {
      if (errorPaths.has(filePath)) continue;
      const fileStats = fileStatsMap.get(filePath);
      if (!fileStats) continue;
      const status = infectedPaths.has(filePath) ? 'infected' : 'clean';
      updateCache(filePath, fileStats, status);
    }
  }

  logger.info('Scan progress', {
    scanned: stats.filesScanned,
    skipped_incremental: getIncrementalStats().filesSkipped,
    skipped_other: stats.filesSkipped,
    infected: stats.filesInfected,
    errors: stats.errors,
  });
}

// =============================================================================
// REMOTE MODE (unchanged)
//
// Uses NodeClam / clamdscan per file over TCP. The clamd daemon owns the DB
// in memory — no subprocess is spawned, no DB is loaded client-side.
// maxConcurrent controls TCP connection concurrency to the remote daemon.
// =============================================================================

async function scanFileRemote(clamscan, filePath, effectiveStrategy, results) {
  if (shouldExclude(filePath)) {
    stats.filesSkipped++;
    return;
  }

  let fileStats;
  try {
    fileStats = await fs.stat(filePath);
  } catch (err) {
    stats.errors++;
    results.errors.push({ file: filePath, error: err.message });
    return;
  }

  if (!fileStats.isFile() || fileStats.size === 0) {
    stats.filesSkipped++;
    return;
  }

  if (fileStats.size > CONFIG.maxFileSize) {
    stats.filesSkipped++;
    return;
  }

  if (INCREMENTAL_CONFIG.enabled) {
    const { shouldScan } = shouldScanFile(filePath, fileStats, effectiveStrategy);
    if (!shouldScan) {
      stats.filesSkipped++;
      return;
    }
  }

  try {
    const { file, isInfected, viruses } = await clamscan.isInfected(filePath);
    stats.filesScanned++;

    if (INCREMENTAL_CONFIG.enabled) {
      updateCache(filePath, fileStats, isInfected ? 'infected' : 'clean');
    }

    if (isInfected) {
      stats.filesInfected++;
      results.infected.push({ file, viruses });
      logger.warn('Infected file detected', {
        alert: 'INFECTED_FILE',
        file_path: file,
        virus_names: viruses,
      });
    }
  } catch (err) {
    stats.errors++;
    logger.error('Error scanning file', { file: filePath, error: err.message });
    results.errors.push({ file: filePath, error: err.message });
  }
}

async function scanDirectoryRemote(clamscan, dirPath, results, effectiveStrategy) {
  let entries;
  try {
    entries = await fs.readdir(dirPath, { withFileTypes: true });
  } catch (err) {
    logger.error('Directory read error', { directory: dirPath, error: err.message });
    return;
  }

  const files = [];
  const dirs  = [];

  for (const entry of entries) {
    const fullPath = path.join(dirPath, entry.name);
    if (shouldExclude(fullPath)) continue;
    if (entry.isDirectory()) dirs.push(fullPath);
    else if (entry.isFile()) files.push(fullPath);
  }

  for (let i = 0; i < files.length; i += CONFIG.maxConcurrent) {
    const batch = files.slice(i, i + CONFIG.maxConcurrent);
    await Promise.all(batch.map((f) => scanFileRemote(clamscan, f, effectiveStrategy, results)));

    const incremental = getIncrementalStats();
    const total = stats.filesScanned + incremental.filesSkipped;
    if (total > 0 && total % 500 === 0) {
      logger.info('Scan progress', {
        scanned: stats.filesScanned,
        skipped_incremental: incremental.filesSkipped,
        skipped_other: stats.filesSkipped,
        infected: stats.filesInfected,
        errors: stats.errors,
      });
    }
  }

  for (const subDir of dirs) {
    await scanDirectoryRemote(clamscan, subDir, results, effectiveStrategy);
  }
}

// =============================================================================
// Public API
// =============================================================================

/**
 * Scan all paths in a single operation.
 *
 * Standalone: walks all paths in JS (Phase 1), then runs ONE clamscan
 *             --file-list subprocess (Phase 2), then updates cache (Phase 3).
 * Remote:     per-file concurrent clamdscan calls over TCP (unchanged).
 *
 * @param {object}   scanner          — returned by initScanner()
 * @param {string[]} pathsToScan
 * @param {object}   results          — { infected: [], errors: [] }
 * @param {string}   effectiveStrategy
 */
async function scanPaths(scanner, pathsToScan, results, effectiveStrategy) {
  if (scanner.mode === 'standalone') {
    const filesToScan  = [];
    const fileStatsMap = new Map();

    for (const scanPath of pathsToScan) {
      try {
        await fs.access(scanPath);
        logger.info('Walking path', { path: scanPath });
        await walkDirectory(scanPath, filesToScan, fileStatsMap, effectiveStrategy);
      } catch {
        logger.warn('Path not found — skipping', { path: scanPath });
      }
    }

    await scanWithClamscan(scanner, filesToScan, fileStatsMap, results);
  } else {
    for (const scanPath of pathsToScan) {
      try {
        await fs.access(scanPath);
        logger.info('Scanning path', { path: scanPath });
        await scanDirectoryRemote(scanner, scanPath, results, effectiveStrategy);
      } catch {
        logger.warn('Path not found — skipping', { path: scanPath });
      }
    }
  }
}

/**
 * Scan a single directory.
 * Kept for backward compatibility — tests and one-off callers use this.
 * In standalone mode delegates to scanPaths (single clamscan invocation).
 */
async function scanDirectory(scanner, dirPath, results, effectiveStrategy) {
  if (scanner.mode === 'standalone') {
    return scanPaths(scanner, [dirPath], results, effectiveStrategy);
  }
  return scanDirectoryRemote(scanner, dirPath, results, effectiveStrategy);
}

module.exports = { scanPaths, scanDirectory, getStats };
