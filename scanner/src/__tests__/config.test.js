const { describe, it } = require('node:test');
const assert = require('node:assert/strict');

describe('config', () => {
  it('loads default CONFIG values', () => {
    // Reset module cache to pick up current env
    delete require.cache[require.resolve('../config')];
    const { CONFIG, INCREMENTAL_CONFIG } = require('../config');

    assert.equal(CONFIG.scanMode, 'standalone');
    assert.equal(CONFIG.clamscanPath, '/usr/bin/clamscan');
    assert.equal(CONFIG.clamavDbPath, '/var/lib/clamav');
    assert.equal(CONFIG.maxConcurrent, 5);
    assert.equal(CONFIG.maxFileSize, 104857600);
    assert.ok(Array.isArray(CONFIG.pathsToScan));
    assert.ok(CONFIG.pathsToScan.length > 0);
    assert.ok(Array.isArray(CONFIG.excludePatterns));
  });

  it('respects SCAN_MODE=remote env', () => {
    process.env.SCAN_MODE = 'remote';
    process.env.CLAMAV_HOST = 'clamd.test.svc';
    delete require.cache[require.resolve('../config')];
    const { CONFIG } = require('../config');

    assert.equal(CONFIG.scanMode, 'remote');
    assert.equal(CONFIG.clamavHost, 'clamd.test.svc');

    // cleanup
    delete process.env.SCAN_MODE;
    delete process.env.CLAMAV_HOST;
  });

  it('parses incremental config', () => {
    process.env.INCREMENTAL_ENABLED = 'true';
    process.env.SCAN_STRATEGY = 'smart';
    process.env.FULL_SCAN_INTERVAL = '5';
    delete require.cache[require.resolve('../config')];
    const { INCREMENTAL_CONFIG } = require('../config');

    assert.equal(INCREMENTAL_CONFIG.enabled, true);
    assert.equal(INCREMENTAL_CONFIG.strategy, 'smart');
    assert.equal(INCREMENTAL_CONFIG.fullScanInterval, 5);

    delete process.env.INCREMENTAL_ENABLED;
    delete process.env.SCAN_STRATEGY;
    delete process.env.FULL_SCAN_INTERVAL;
  });
});
