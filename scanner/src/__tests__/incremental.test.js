const { describe, it, beforeEach } = require('node:test');
const assert = require('node:assert/strict');

describe('incremental', () => {
  beforeEach(() => {
    // Force incremental mode for tests
    process.env.INCREMENTAL_ENABLED = 'true';
    process.env.SCAN_STRATEGY = 'incremental';
    // Clear module caches
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../incremental')];
  });

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

    const result = shouldScanFile('/tmp/new-file.txt', fakeStats, 'incremental');
    assert.equal(result.shouldScan, true);
    assert.equal(result.reason, 'new_file');
  });

  it('shouldScanFile skips unchanged files', () => {
    const { shouldScanFile, updateCache } = require('../incremental');
    const fakeStats = { mtimeMs: 1000000, size: 512 };

    // Simulate previous scan
    updateCache('/tmp/cached.txt', fakeStats, 'clean');

    const result = shouldScanFile('/tmp/cached.txt', fakeStats, 'incremental');
    assert.equal(result.shouldScan, false);
    assert.equal(result.reason, 'unchanged');
  });

  it('shouldScanFile rescans modified files', () => {
    const { shouldScanFile, updateCache } = require('../incremental');
    const oldStats = { mtimeMs: 1000000, size: 512 };
    const newStats = { mtimeMs: 2000000, size: 1024 };

    updateCache('/tmp/modified.txt', oldStats, 'clean');

    const result = shouldScanFile('/tmp/modified.txt', newStats, 'incremental');
    assert.equal(result.shouldScan, true);
    assert.equal(result.reason, 'modified');
  });

  it('getIncrementalStats returns a snapshot', () => {
    const { getIncrementalStats } = require('../incremental');
    const stats = getIncrementalStats();

    assert.equal(typeof stats.filesSkipped, 'number');
    assert.equal(typeof stats.cacheHits, 'number');
    assert.equal(typeof stats.newFiles, 'number');
    assert.equal(typeof stats.modifiedFiles, 'number');
  });
});
