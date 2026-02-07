/*
Copyright 2025 The ClamAV Operator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

'use strict';

const { CONFIG } = require('./config');

/**
 * Structured JSON logger.
 * Output is consumed by the Go operator which parses JSON log lines from
 * the scanner pod to extract metrics and infected-file alerts.
 */
class Logger {
  log(level, message, data = {}) {
    const entry = {
      timestamp: new Date().toISOString(),
      level,
      service: 'clamav-scanner',
      node_name: CONFIG.nodeName,
      scan_mode: CONFIG.scanMode,
      message,
      ...data,
    };
    console.log(JSON.stringify(entry));
  }

  info(message, data) {
    this.log('INFO', message, data);
  }
  warn(message, data) {
    this.log('WARN', message, data);
  }
  error(message, data) {
    this.log('ERROR', message, data);
  }
  debug(message, data) {
    this.log('DEBUG', message, data);
  }
}

module.exports = new Logger();
