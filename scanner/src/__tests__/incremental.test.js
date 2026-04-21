/*
Copyright 2025 The ClamAV Operator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

'use strict';

const { describe, it, before, after, beforeEach } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('fs').promises;
const os = require('os');
const path = require('path');

describe('incremental', () => {
  let tmpDir;

  before(async () => {
    tmpDir = await fs.mkdtemp(path.join(os.tmpdir(), 'clamav-incr-test-'));
  });

  after(async () => {
    try {
      const entries = await fs.readdir(tmpDir);
      await Promise.all(entries.map((e) => fs.unlink(path.join(tmpDir, e)).catch(() => {})));
      await fs.rmdir(tmpDir);
    } catch { /* ignore */ }
  });

  beforeEach(() => {
    // Force incremental mode for tests; reset module caches for isolation
    process.env.INCREMENTAL_ENABLED = 'true';
    process.env.SCAN_STRATEGY = 'incremental';
    process.env.RESULTS_DIR = tmpDir;
    process.env.NODE_NAME = 'test-node';
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../logger')];
    delete require.cache[require.resolve('../incremental')];
  });

  // ── Basic strategy decisions ──────────────────────────────────────────────

  it('shouldScanFile returns full_scan when strategy is full', () => {
    const { shouldScanFile } = require('../incremental');
    const fakeStats = { mtimeMs: Date.now(), size: 1024 };

    const result = shouldScanFile('/tmp/test.txt', fakeStats, 'full');
    assert.equal(result.shouldScan, true);
    assert.equal(result.reason, 'full_scan');
  });

  it('shouldScanFile detects new files', () => {
    const { shouldScanFile } = require('../incremental');
    const fakeStats = { mtimeMs: Date.now(), size: 1024 };

    const result = shouldScanFile('/tmp/new-file-unique-xyz.txt', fakeStats, 'incremental');
    assert.equal(result.shouldScan, true);
    assert.equal(result.reason, 'new_file');
  });

  it('shouldScanFile skips unchanged files', () => {
    const { shouldScanFile, updateCache } = require('../incremental');
    const fakeStats = { mtimeMs: 1000000, size: 512 };

    updateCache('/tmp/cached.txt', fakeStats, 'clean');

    const result = shouldScanFile('/tmp/cached.txt', fakeStats, 'incremental');
    assert.equal(result.shouldScan, false);
    assert.equal(result.reason, 'unchanged');
  });

  it('shouldScanFile rescans modified files (mtime changed)', () => {
    const { shouldScanFile, updateCache } = require('../incremental');
    const oldStats = { mtimeMs: 1000000, size: 512 };
    const newStats = { mtimeMs: 2000000, size: 512 };

    updateCache('/tmp/modified-mtime.txt', oldStats, 'clean');

    const result = shouldScanFile('/tmp/modified-mtime.txt', newStats, 'incremental');
    assert.equal(result.shouldScan, true);
    assert.equal(result.reason, 'modified');
  });

  it('shouldScanFile rescans modified files (size changed)', () => {
    const { shouldScanFile, updateCache } = require('../incremental');
    const oldStats = { mtimeMs: 1000000, size: 512 };
    const newStats = { mtimeMs: 1000000, size: 1024 }; // same mtime, different size

    updateCache('/tmp/modified-size.txt', oldStats, 'clean');

    const result = shouldScanFile('/tmp/modified-size.txt', newStats, 'incremental');
    assert.equal(result.shouldScan, true);
    assert.equal(result.reason, 'modified');
  });

  it('shouldScanFile rescans when max_age_exceeded', async () => {
    // Set 1-hour max age limit
    process.env.MAX_FILE_AGE_HOURS = '1';
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../logger')];
    delete require.cache[require.resolve('../incremental')];

    const { loadCache, shouldScanFile } = require('../incremental');

    // Inject a cache file with lastScanned = 2 hours ago (7200 seconds)
    const twoHoursAgo = Math.floor(Date.now() / 1000) - 7200;
    const cacheFile = path.join(tmpDir, 'test-node_scan_cache.json');
    await fs.writeFile(cacheFile, JSON.stringify({
      version: 2,
      lastScanDate: new Date().toISOString(),
      node: 'test-node',
      totalFiles: 1,
      files: {
        '/tmp/age-test.txt': {
          modTime: 1000000,
          size: 100,
          lastScanned: twoHoursAgo,
          scanResult: 'clean',
        },
      },
    }));

    // Load the manually-crafted cache
    await loadCache();

    const fakeStats = { mtimeMs: 1000000, size: 100 };
    const result = shouldScanFile('/tmp/age-test.txt', fakeStats, 'incremental');
    assert.equal(result.shouldScan, true);
    assert.equal(result.reason, 'max_age_exceeded');

    delete process.env.MAX_FILE_AGE_HOURS;
  });

  it('getIncrementalStats returns a snapshot', () => {
    const { getIncrementalStats } = require('../incremental');
    const stats = getIncrementalStats();

    assert.equal(typeof stats.filesSkipped, 'number');
    assert.equal(typeof stats.cacheHits, 'number');
    assert.equal(typeof stats.cacheMisses, 'number');
    assert.equal(typeof stats.newFiles, 'number');
    assert.equal(typeof stats.modifiedFiles, 'number');
  });

  it('updateCache stores entries that shouldScanFile can read', () => {
    const { shouldScanFile, updateCache } = require('../incremental');
    const fakeStats = { mtimeMs: 5000000, size: 999 };

    updateCache('/tmp/stored-file.txt', fakeStats, 'clean');

    // Same stats → should be skipped
    const r = shouldScanFile('/tmp/stored-file.txt', fakeStats, 'incremental');
    assert.equal(r.shouldScan, false);
    assert.equal(r.reason, 'unchanged');
  });

  // ── Cache persistence ─────────────────────────────────────────────────────

  it('saveCache writes a JSON file and loadCache reads it back', async () => {
    const { saveCache, loadCache, updateCache } = require('../incremental');
    const fakeStats = { mtimeMs: 9999999, size: 42 };

    updateCache('/tmp/persist-test.txt', fakeStats, 'infected');
    await saveCache();

    // Check file was written
    const cacheFile = path.join(tmpDir, 'test-node_scan_cache.json');
    const raw = await fs.readFile(cacheFile, 'utf-8');
    const parsed = JSON.parse(raw);
    assert.equal(parsed.version, 3, 'cache version should be 3');
    assert.ok(parsed.files['/tmp/persist-test.txt'], 'cached file should be present');
    assert.equal(parsed.files['/tmp/persist-test.txt'].scanResult, 'infected');

    // Now load the cache in a fresh module instance
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../logger')];
    delete require.cache[require.resolve('../incremental')];

    const { loadCache: lc2, shouldScanFile: ssf2 } = require('../incremental');
    await lc2();

    // The file with same stats should now be skipped (loaded from cache)
    const result = ssf2('/tmp/persist-test.txt', fakeStats, 'incremental');
    assert.equal(result.shouldScan, false);
    assert.equal(result.reason, 'unchanged');
  });

  it('loadCache handles missing cache file gracefully', async () => {
    // Remove cache file if it exists
    const cacheFile = path.join(tmpDir, 'test-node_scan_cache.json');
    try { await fs.unlink(cacheFile); } catch { /* ok */ }

    const { loadCache } = require('../incremental');
    // Should not throw on missing cache
    await assert.doesNotReject(() => loadCache());
  });

  it('loadCache handles corrupted cache file gracefully', async () => {
    const cacheFile = path.join(tmpDir, 'test-node_scan_cache.json');
    await fs.writeFile(cacheFile, 'this is not valid JSON!!!!');

    const { loadCache } = require('../incremental');
    await assert.doesNotReject(() => loadCache());
  });

  // ── Smart strategy ────────────────────────────────────────────────────────

  it('resolveEffectiveStrategy returns "incremental" when strategy is incremental', async () => {
    const { resolveEffectiveStrategy } = require('../incremental');
    const strategy = await resolveEffectiveStrategy();
    assert.equal(strategy, 'incremental');
  });

  it('resolveEffectiveStrategy returns "full" when not enabled', async () => {
    process.env.INCREMENTAL_ENABLED = 'false';
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../incremental')];

    const { resolveEffectiveStrategy } = require('../incremental');
    const strategy = await resolveEffectiveStrategy();
    assert.equal(strategy, 'full');
  });

  it('resolveEffectiveStrategy smart: triggers full scan after interval', async () => {
    process.env.SCAN_STRATEGY = 'smart';
    process.env.FULL_SCAN_INTERVAL = '3';
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../incremental')];

    const { resolveEffectiveStrategy } = require('../incremental');

    // Write a counter file that simulates 3 incremental scans already done
    const markerFile = path.join(tmpDir, 'test-node_smart_counter.txt');
    await fs.writeFile(markerFile, '3');

    const strategy = await resolveEffectiveStrategy();
    assert.equal(strategy, 'full', 'should trigger full scan after 3 incremental runs');

    // Counter should have been reset to 0
    const counterContent = await fs.readFile(markerFile, 'utf-8');
    assert.equal(counterContent.trim(), '0', 'counter should be reset to 0 after full scan');

    delete process.env.FULL_SCAN_INTERVAL;
  });

  it('resolveEffectiveStrategy smart: increments counter on incremental scan', async () => {
    process.env.SCAN_STRATEGY = 'smart';
    process.env.FULL_SCAN_INTERVAL = '5';
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../incremental')];

    const { resolveEffectiveStrategy } = require('../incremental');

    // Start with counter = 1
    const markerFile = path.join(tmpDir, 'test-node_smart_counter.txt');
    await fs.writeFile(markerFile, '1');

    const strategy = await resolveEffectiveStrategy();
    assert.equal(strategy, 'incremental', 'should return incremental when below interval');

    // Counter should have been incremented to 2
    const counterContent = await fs.readFile(markerFile, 'utf-8');
    assert.equal(counterContent.trim(), '2', 'counter should be incremented');

    delete process.env.FULL_SCAN_INTERVAL;
  });
});
