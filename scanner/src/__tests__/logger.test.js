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

describe('logger', () => {
  let capturedLines;
  let originalConsoleLog;

  // Capture console.log output before each test
  beforeEach(() => {
    capturedLines = [];
    originalConsoleLog = console.log;
    console.log = (...args) => capturedLines.push(args.join(' '));
  });

  after(() => {
    // Restore console.log after all tests
    if (originalConsoleLog) console.log = originalConsoleLog;
  });

  function parseLastLog() {
    assert.ok(capturedLines.length > 0, 'Expected at least one log line');
    return JSON.parse(capturedLines[capturedLines.length - 1]);
  }

  it('log() emits valid JSON', () => {
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../logger')];
    const logger = require('../logger');

    logger.info('test message');

    const entry = parseLastLog();
    assert.ok(entry.timestamp, 'should have timestamp');
    assert.ok(entry.level, 'should have level');
    assert.ok(entry.service, 'should have service');
    assert.ok(entry.message, 'should have message');
    console.log = originalConsoleLog;
  });

  it('info() sets level to INFO', () => {
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../logger')];
    const logger = require('../logger');

    logger.info('info message');

    const entry = parseLastLog();
    assert.equal(entry.level, 'INFO');
    assert.equal(entry.message, 'info message');
    console.log = originalConsoleLog;
  });

  it('warn() sets level to WARN', () => {
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../logger')];
    const logger = require('../logger');

    logger.warn('warn message');

    const entry = parseLastLog();
    assert.equal(entry.level, 'WARN');
    assert.equal(entry.message, 'warn message');
    console.log = originalConsoleLog;
  });

  it('error() sets level to ERROR', () => {
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../logger')];
    const logger = require('../logger');

    logger.error('error message');

    const entry = parseLastLog();
    assert.equal(entry.level, 'ERROR');
    assert.equal(entry.message, 'error message');
    console.log = originalConsoleLog;
  });

  it('debug() sets level to DEBUG', () => {
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../logger')];
    const logger = require('../logger');

    logger.debug('debug message');

    const entry = parseLastLog();
    assert.equal(entry.level, 'DEBUG');
    assert.equal(entry.message, 'debug message');
    console.log = originalConsoleLog;
  });

  it('service field is always "clamav-scanner"', () => {
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../logger')];
    const logger = require('../logger');

    logger.info('any message');

    const entry = parseLastLog();
    assert.equal(entry.service, 'clamav-scanner');
    console.log = originalConsoleLog;
  });

  it('spreads extra data fields into the log entry', () => {
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../logger')];
    const logger = require('../logger');

    logger.warn('infected file', {
      alert: 'INFECTED_FILE',
      file_path: '/etc/passwd',
      virus_names: ['Eicar-Test-Signature'],
    });

    const entry = parseLastLog();
    assert.equal(entry.level, 'WARN');
    assert.equal(entry.alert, 'INFECTED_FILE');
    assert.equal(entry.file_path, '/etc/passwd');
    assert.deepEqual(entry.virus_names, ['Eicar-Test-Signature']);
    console.log = originalConsoleLog;
  });

  it('timestamp is a valid ISO-8601 string', () => {
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../logger')];
    const logger = require('../logger');

    logger.info('timestamp test');

    const entry = parseLastLog();
    const d = new Date(entry.timestamp);
    assert.ok(!isNaN(d.getTime()), 'timestamp should be a valid date');
    console.log = originalConsoleLog;
  });

  it('NODE_NAME env var is reflected in node_name field', () => {
    process.env.NODE_NAME = 'worker-node-42';
    delete require.cache[require.resolve('../config')];
    delete require.cache[require.resolve('../logger')];
    const logger = require('../logger');

    logger.info('node name test');

    const entry = parseLastLog();
    assert.equal(entry.node_name, 'worker-node-42');

    delete process.env.NODE_NAME;
    console.log = originalConsoleLog;
  });
});
