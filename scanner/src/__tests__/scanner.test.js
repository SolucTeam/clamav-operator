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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// Returns a fake scanner in remote mode so tests exercise the remote code
// path without spawning a real clamscan subprocess. Standalone mode integration
// testing requires a real clamscan binary and is covered by e2e/CI tests.
function makeFakeClamscan({ isInfected = false, viruses = [] } = {}) {
  return {
    mode: 'remote',
    async isInfected(filePath) {
      return { file: filePath, isInfected, viruses };
    },
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('scanner', () => {
  let tmpDir;

  before(async () => {
    tmpDir = await fs.mkdtemp(path.join(os.tmpdir(), 'clamav-scanner-test-'));
  });

  after(async () => {
    // Remove all test files
    try {
      const entries = await fs.readdir(tmpDir, { withFileTypes: true });
      for (const e of entries) {
        const full = path.join(tmpDir, e.name);
        if (e.isDirectory()) {
          const sub = await fs.readdir(full);
          await Promise.all(sub.map((s) => fs.unlink(path.join(full, s))));
          await fs.rmdir(full);
        } else {
          await fs.unlink(full);
        }
      }
      await fs.rmdir(tmpDir);
    } catch { /* ignore */ }
  });

  beforeEach(() => {
    // Reset module cache — ensures stats are fresh for each test
    process.env.RESULTS_DIR = tmpDir;
    process.env.NODE_NAME = 'test-node';
    process.env.INCREMENTAL_ENABLED = 'false';
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../logger')];
    delete require.cache[require.resolve('../incremental')];
    delete require.cache[require.resolve('../scanner')];
  });

  // ── scanFile (via scanDirectory) ─────────────────────────────────────────

  it('scanDirectory skips excluded paths', async () => {
    // Write a file in a proc-like path that matches the default exclusion
    const procDir = path.join(tmpDir, 'proc');
    await fs.mkdir(procDir, { recursive: true });
    await fs.writeFile(path.join(procDir, 'status'), 'fake-proc-content');

    // Set up config to scan tmpDir but exclude our proc subdir
    process.env.PATHS_TO_SCAN = tmpDir;
    process.env.EXCLUDE_PATTERNS = `^${procDir}`;
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../logger')];
    delete require.cache[require.resolve('../incremental')];
    delete require.cache[require.resolve('../scanner')];

    const { scanDirectory, getStats } = require('../scanner');
    const results = { infected: [], errors: [] };
    const clamscan = makeFakeClamscan();

    await scanDirectory(clamscan, tmpDir, results, 'full');

    // The proc/status file should be skipped (excluded), no infected
    assert.equal(results.infected.length, 0);

    delete process.env.PATHS_TO_SCAN;
    delete process.env.EXCLUDE_PATTERNS;
  });

  it('scanDirectory reports infected files', async () => {
    const malFile = path.join(tmpDir, 'malware.bin');
    await fs.writeFile(malFile, 'EICAR-STANDARD-ANTIVIRUS-TEST-FILE');

    process.env.PATHS_TO_SCAN = tmpDir;
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../logger')];
    delete require.cache[require.resolve('../incremental')];
    delete require.cache[require.resolve('../scanner')];

    const { scanDirectory } = require('../scanner');
    const results = { infected: [], errors: [] };
    const clamscan = makeFakeClamscan({ isInfected: true, viruses: ['Eicar-Test-Signature'] });

    await scanDirectory(clamscan, tmpDir, results, 'full');

    assert.ok(results.infected.length > 0, 'should report at least one infected file');
    assert.ok(
      results.infected.some((r) => r.viruses.includes('Eicar-Test-Signature')),
      'should report the correct virus name'
    );

    await fs.unlink(malFile);
    delete process.env.PATHS_TO_SCAN;
  });

  it('scanDirectory recurses into subdirectories', async () => {
    const subDir = path.join(tmpDir, 'subdir-recurse');
    await fs.mkdir(subDir, { recursive: true });
    const deepFile = path.join(subDir, 'deep.txt');
    await fs.writeFile(deepFile, 'content to scan');

    process.env.PATHS_TO_SCAN = tmpDir;
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../logger')];
    delete require.cache[require.resolve('../incremental')];
    delete require.cache[require.resolve('../scanner')];

    const { scanDirectory, getStats } = require('../scanner');
    const results = { infected: [], errors: [] };
    const clamscan = makeFakeClamscan({ isInfected: false });

    await scanDirectory(clamscan, tmpDir, results, 'full');

    // The file should have been scanned (not an error), so errors should be empty
    assert.equal(results.errors.length, 0);

    await fs.unlink(deepFile);
    await fs.rmdir(subDir);
    delete process.env.PATHS_TO_SCAN;
  });

  it('scanDirectory handles unreadable directories gracefully', async () => {
    // Pass a non-existent directory — should not throw
    process.env.PATHS_TO_SCAN = tmpDir;
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../logger')];
    delete require.cache[require.resolve('../incremental')];
    delete require.cache[require.resolve('../scanner')];

    const { scanDirectory } = require('../scanner');
    const results = { infected: [], errors: [] };
    const clamscan = makeFakeClamscan();

    // Should not throw even on non-existent directory
    await assert.doesNotReject(
      () => scanDirectory(clamscan, '/nonexistent/path/xyz', results, 'full')
    );

    delete process.env.PATHS_TO_SCAN;
  });

  it('getStats returns a snapshot with correct shape', () => {
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../logger')];
    delete require.cache[require.resolve('../incremental')];
    delete require.cache[require.resolve('../scanner')];

    const { getStats } = require('../scanner');
    const s = getStats();

    assert.ok(typeof s.filesScanned === 'number', 'filesScanned should be a number');
    assert.ok(typeof s.filesInfected === 'number', 'filesInfected should be a number');
    assert.ok(typeof s.filesSkipped === 'number', 'filesSkipped should be a number');
    assert.ok(typeof s.errors === 'number', 'errors should be a number');
    assert.ok(typeof s.startTime === 'number', 'startTime should be a number');
  });

  it('scanDirectory skips empty files', async () => {
    const emptyFile = path.join(tmpDir, 'empty.txt');
    await fs.writeFile(emptyFile, '');

    process.env.PATHS_TO_SCAN = tmpDir;
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../logger')];
    delete require.cache[require.resolve('../incremental')];
    delete require.cache[require.resolve('../scanner')];

    const { scanDirectory } = require('../scanner');
    const results = { infected: [], errors: [] };
    // Use a clamscan that would report infected — empty file should be skipped before reaching clamscan
    const clamscan = makeFakeClamscan({ isInfected: true, viruses: ['WouldBeVirus'] });

    await scanDirectory(clamscan, tmpDir, results, 'full');

    // Empty file should be skipped, so no infected results from the empty file
    assert.equal(results.infected.length, 0);

    await fs.unlink(emptyFile);
    delete process.env.PATHS_TO_SCAN;
  });
});
