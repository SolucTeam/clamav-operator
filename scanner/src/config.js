/*
Copyright 2025 The ClamAV Operator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

'use strict';

// =============================================================================
// CONFIGURATION – driven by environment variables set by the operator Job
// =============================================================================

const CONFIG = {
  // ── Node / runtime ──────────────────────────────────────────────────────
  nodeName: process.env.NODE_NAME || 'unknown',
  hostRoot: process.env.HOST_ROOT || '/host',
  resultsDir: process.env.RESULTS_DIR || '/results',
  pathsToScan: (process.env.PATHS_TO_SCAN || '/host/var/lib,/host/opt')
    .split(',')
    .map((p) => p.trim())
    .filter(Boolean),
  maxConcurrent: parseInt(process.env.MAX_CONCURRENT || '5', 10),
  fileTimeout: parseInt(process.env.FILE_TIMEOUT || '300000', 10),
  maxFileSize: parseInt(process.env.MAX_FILE_SIZE || '104857600', 10),

  // ── Scan mode ───────────────────────────────────────────────────────────
  // "standalone"  → local clamscan binary, zero network dependency
  // "remote"      → connect to a central clamd service (legacy)
  scanMode: process.env.SCAN_MODE || 'standalone',

  // ── Standalone-mode paths ───────────────────────────────────────────────
  clamscanPath: process.env.CLAMSCAN_PATH || '/usr/bin/clamscan',
  clamavDbPath: process.env.CLAMAV_DB_PATH || '/var/lib/clamav',

  // ── Remote-mode settings ────────────────────────────────────────────────
  clamavHost: process.env.CLAMAV_HOST,
  clamavPort: parseInt(process.env.CLAMAV_PORT || '3310', 10),
  connectTimeout: parseInt(process.env.CONNECT_TIMEOUT || '60000', 10),

  // ── Update signatures at boot ───────────────────────────────────────────
  // When UPDATE_SIGNATURES=true the container will run freshclam before
  // scanning.  In air-gap (false) signatures must already be in the image.
  updateSignatures: process.env.UPDATE_SIGNATURES === 'true',

  // ── File exclusion patterns ─────────────────────────────────────────────
  excludePatterns: [
    /\/proc\//,
    /\/sys\//,
    /\/dev\//,
    /\/run\//,
    /\.sock$/,
    /\.pid$/,
  ],
};

// =============================================================================
// INCREMENTAL SCAN CONFIGURATION
// =============================================================================

const INCREMENTAL_CONFIG = {
  enabled: process.env.INCREMENTAL_ENABLED === 'true',
  // "full" | "incremental" | "smart"
  strategy: process.env.SCAN_STRATEGY || 'full',
  maxFileAgeHours: parseInt(process.env.MAX_FILE_AGE_HOURS || '24', 10),
  skipUnchangedFiles: process.env.SKIP_UNCHANGED_FILES !== 'false',
  // For "smart" strategy: run a full scan every N incremental runs
  fullScanInterval: parseInt(process.env.FULL_SCAN_INTERVAL || '10', 10),
};

module.exports = { CONFIG, INCREMENTAL_CONFIG };
