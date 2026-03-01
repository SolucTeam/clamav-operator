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

describe('report', () => {
  let tmpDir;

  before(async () => {
    tmpDir = await fs.mkdtemp(path.join(os.tmpdir(), 'clamav-report-test-'));
  });

  after(async () => {
    // Clean up temp dir
    try {
      const entries = await fs.readdir(tmpDir);
      await Promise.all(entries.map((e) => fs.unlink(path.join(tmpDir, e))));
      await fs.rmdir(tmpDir);
    } catch { /* ignore */ }
  });

  beforeEach(async () => {
    // Clean tmpDir between tests to avoid file count interference
    try {
      const existing = await fs.readdir(tmpDir);
      await Promise.all(existing.map((e) => fs.unlink(path.join(tmpDir, e)).catch(() => {})));
    } catch { /* ignore */ }

    // Reset module cache and set env vars
    process.env.RESULTS_DIR = tmpDir;
    process.env.NODE_NAME = 'test-node';
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../logger')];
    delete require.cache[require.resolve('../report')];
  });

  it('generates a JSON report file', async () => {
    const { generateReport } = require('../report');
    const results = { infected: [], errors: [] };
    const stats = { filesScanned: 100, filesInfected: 0, filesSkipped: 5, errors: 0, startTime: Date.now() - 5000 };
    const incStats = { filesSkipped: 0, cacheHits: 0, cacheMisses: 0, newFiles: 0, modifiedFiles: 0 };

    await generateReport(results, stats, incStats, 'full');

    const files = await fs.readdir(tmpDir);
    const jsonFiles = files.filter((f) => f.endsWith('.json'));
    assert.equal(jsonFiles.length, 1, 'should have created exactly one JSON file');
  });

  it('generates a text summary file', async () => {
    const { generateReport } = require('../report');
    const results = { infected: [], errors: [] };
    const stats = { filesScanned: 50, filesInfected: 0, filesSkipped: 2, errors: 0, startTime: Date.now() - 2000 };
    const incStats = { filesSkipped: 0, cacheHits: 0, cacheMisses: 0, newFiles: 0, modifiedFiles: 0 };

    await generateReport(results, stats, incStats, 'incremental');

    const files = await fs.readdir(tmpDir);
    const txtFiles = files.filter((f) => f.endsWith('.txt'));
    assert.equal(txtFiles.length, 1, 'should have created exactly one TXT file');
  });

  it('text summary STATUS=CLEAN when no infections', async () => {
    const { generateReport } = require('../report');
    const results = { infected: [], errors: [] };
    const stats = { filesScanned: 200, filesInfected: 0, filesSkipped: 0, errors: 0, startTime: Date.now() - 10000 };
    const incStats = { filesSkipped: 0, cacheHits: 0, cacheMisses: 0, newFiles: 0, modifiedFiles: 0 };

    await generateReport(results, stats, incStats, 'full');

    const files = await fs.readdir(tmpDir);
    const txtFile = files.find((f) => f.endsWith('.txt'));
    const content = await fs.readFile(path.join(tmpDir, txtFile), 'utf-8');
    assert.ok(content.includes('STATUS=CLEAN'), 'should report CLEAN status');
  });

  it('text summary STATUS=INFECTED when infections found', async () => {
    const { generateReport } = require('../report');
    const results = {
      infected: [{ file: '/tmp/malware.exe', viruses: ['Eicar-Test-Signature'] }],
      errors: [],
    };
    const stats = { filesScanned: 50, filesInfected: 1, filesSkipped: 0, errors: 0, startTime: Date.now() - 3000 };
    const incStats = { filesSkipped: 0, cacheHits: 0, cacheMisses: 0, newFiles: 0, modifiedFiles: 0 };

    await generateReport(results, stats, incStats, 'full');

    const files = await fs.readdir(tmpDir);
    const txtFile = files.find((f) => f.endsWith('.txt'));
    const content = await fs.readFile(path.join(tmpDir, txtFile), 'utf-8');
    assert.ok(content.includes('STATUS=INFECTED'), 'should report INFECTED status');
  });

  it('JSON report contains expected fields', async () => {
    const { generateReport } = require('../report');
    const results = {
      infected: [{ file: '/proc/bad', viruses: ['TestVirus'] }],
      errors: [],
    };
    const stats = { filesScanned: 42, filesInfected: 1, filesSkipped: 3, errors: 0, startTime: Date.now() - 8000 };
    const incStats = { filesSkipped: 0, cacheHits: 0, cacheMisses: 0, newFiles: 0, modifiedFiles: 0 };

    const report = await generateReport(results, stats, incStats, 'full');

    assert.equal(report.node, 'test-node');
    assert.ok(report.scanDate, 'should have scanDate');
    assert.ok(typeof report.duration === 'number', 'duration should be a number');
    assert.equal(report.strategy, 'full');
    assert.deepEqual(report.statistics, {
      filesScanned: 42,
      filesInfected: 1,
      filesSkipped: 3,
      errors: 0,
    });
    assert.equal(report.infected.length, 1);
    assert.equal(report.infected[0].file, '/proc/bad');
  });

  it('caps errors array at 100 entries in JSON report', async () => {
    const { generateReport } = require('../report');
    const manyErrors = Array.from({ length: 150 }, (_, i) => ({
      file: `/tmp/error-${i}.txt`,
      error: 'permission denied',
    }));
    const results = { infected: [], errors: manyErrors };
    const stats = { filesScanned: 0, filesInfected: 0, filesSkipped: 0, errors: 150, startTime: Date.now() };
    const incStats = { filesSkipped: 0, cacheHits: 0, cacheMisses: 0, newFiles: 0, modifiedFiles: 0 };

    const report = await generateReport(results, stats, incStats, 'full');

    assert.equal(report.errors.length, 100, 'errors should be capped at 100');
  });

  it('JSON report includes incremental stats when enabled', async () => {
    process.env.INCREMENTAL_ENABLED = 'true';
    process.env.SCAN_STRATEGY = 'incremental';
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../report')];

    const { generateReport } = require('../report');
    const results = { infected: [], errors: [] };
    const stats = { filesScanned: 30, filesInfected: 0, filesSkipped: 0, errors: 0, startTime: Date.now() - 1000 };
    const incStats = { filesSkipped: 70, cacheHits: 65, cacheMisses: 5, newFiles: 3, modifiedFiles: 2 };

    const report = await generateReport(results, stats, incStats, 'incremental');

    assert.equal(report.incremental.enabled, true);
    assert.equal(report.incremental.filesSkipped, 70);
    assert.equal(report.incremental.cacheHits, 65);

    delete process.env.INCREMENTAL_ENABLED;
    delete process.env.SCAN_STRATEGY;
  });
});
