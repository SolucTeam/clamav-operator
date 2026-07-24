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

// Parse an integer env var, falling back to def when unset OR non-numeric.
// A plain parseInt would yield NaN for garbage values, and NaN propagates
// silently (e.g. Math.max(1, NaN) === NaN; a NaN batch step makes the
// remote-mode scan loop exit immediately, scanning ZERO files while the Job
// still reports success). Unlike `parseInt(x) || def`, this keeps a legit 0.
const intEnv = (value, def) => {
  const n = parseInt(value, 10);
  return Number.isNaN(n) ? def : n;
};

const CONFIG = {
  // ── Node / runtime ──────────────────────────────────────────────────────
  nodeName: process.env.NODE_NAME || 'unknown',
  hostRoot: process.env.HOST_ROOT || '/host',
  resultsDir: process.env.RESULTS_DIR || '/results',
  pathsToScan: (process.env.PATHS_TO_SCAN || '/host/var/lib,/host/opt')
    .split(',')
    .map((p) => p.trim())
    .filter(Boolean),
  // Remote mode only: controls the number of concurrent TCP connections to
  // the clamd daemon. Ignored in standalone mode — the scanner spawns a
  // single clamscan --file-list subprocess regardless of this value, so
  // concurrency is not a memory concern (DB loaded exactly once per job).
  maxConcurrent: Math.max(1, intEnv(process.env.MAX_CONCURRENT, 1)),
  fileTimeout: intEnv(process.env.FILE_TIMEOUT, 300000),
  maxFileSize: intEnv(process.env.MAX_FILE_SIZE, 104857600),

  // ── Scan mode ───────────────────────────────────────────────────────────
  // "standalone"  → local clamscan binary, zero network dependency
  // "remote"      → connect to a central clamd service (legacy)
  scanMode: process.env.SCAN_MODE || 'standalone',

  // ── Standalone-mode paths ───────────────────────────────────────────────
  clamscanPath: process.env.CLAMSCAN_PATH || '/usr/bin/clamscan',
  clamavDbPath: process.env.CLAMAV_DB_PATH || '/var/lib/clamav',

  // ── Remote-mode settings ────────────────────────────────────────────────
  clamavHost: process.env.CLAMAV_HOST,
  clamavPort: intEnv(process.env.CLAMAV_PORT, 3310),
  connectTimeout: intEnv(process.env.CONNECT_TIMEOUT, 60000),

  // ── Update signatures at boot ───────────────────────────────────────────
  // When UPDATE_SIGNATURES=true the container will run freshclam before
  // scanning.  In air-gap (false) signatures must already be in the image.
  updateSignatures: process.env.UPDATE_SIGNATURES === 'true',

  // ── File exclusion patterns ─────────────────────────────────────────────
  // System paths are always excluded regardless of user config.
  // EXCLUDE_PATTERNS carries additional user-defined regex strings (JSON array)
  // set by the operator from NodeScan.Spec.ExcludePatterns / ScanPolicy.
  excludePatterns: (() => {
    const system = [
      /\/proc\//,
      /\/sys\//,
      /\/dev\//,
      /\/run\//,
      /\.sock$/,
      /\.pid$/,
    ];
    // Convert a glob pattern (e.g. "*.tmp", "/var/lib/docker/*") to a RegExp.
    // The Go admission webhook accepts BOTH globs and regexes
    // (validation.go isRegexPattern), but this scanner previously compiled
    // every pattern as a JS regex: globs like "*.tmp" threw SyntaxError
    // ("nothing to repeat") and were silently dropped — files the user asked
    // to exclude were scanned anyway.
    const globToRegExp = (glob) => {
      // Strip a leading '/' so absolute globs like "/var/lib/docker/*" also
      // match the host-mounted form "/host/var/lib/docker/...".
      const body = glob.startsWith('/') ? glob.slice(1) : glob;
      let re = '';
      for (const ch of body) {
        if (ch === '*') re += '[^/]*';
        else if (ch === '?') re += '[^/]';
        else re += ch.replace(/[.+^${}()|[\]\\]/g, '\\$&');
      }
      // (^|/) anchors the match to a path-component boundary; $ anchors the end.
      // "*.tmp"              → /(^|\/)[^/]*\.tmp$/  (any .tmp file)
      // "/var/lib/docker/*"  → /(^|\/)var\/lib\/docker\/[^/]*$/
      return new RegExp(`(^|/)${re}$`);
    };
    try {
      const raw = process.env.EXCLUDE_PATTERNS;
      if (raw && raw !== '[]') {
        const parsed = JSON.parse(raw);
        const custom = [];
        for (const p of parsed) {
          try {
            custom.push(new RegExp(p));
          } catch (regexErr) {
            // Not a valid JS regex — most likely a glob accepted by the Go
            // webhook. Fall back to glob→regex conversion before giving up.
            try {
              custom.push(globToRegExp(p));
            } catch {
              // The Go webhook validates patterns with RE2 (regexp.Compile); JS
              // uses a different engine, so a pattern valid in Go may be invalid
              // here. Log to stderr so the operator can surface the issue, then
              // skip the offending pattern rather than dropping all exclusions.
              process.stderr.write(
                JSON.stringify({
                  level: 'error',
                  message: 'Invalid exclude pattern in EXCLUDE_PATTERNS — skipping',
                  pattern: p,
                  error: regexErr.message,
                }) + '\n'
              );
            }
          }
        }
        return [...system, ...custom];
      }
    } catch (parseErr) {
      // Malformed JSON: fall back to system defaults to avoid scanning nothing.
      process.stderr.write(
        JSON.stringify({
          level: 'error',
          message: 'EXCLUDE_PATTERNS is not valid JSON — using system defaults only',
          error: parseErr.message,
        }) + '\n'
      );
    }
    return system;
  })(),
};

// =============================================================================
// INCREMENTAL SCAN CONFIGURATION
// =============================================================================

const INCREMENTAL_CONFIG = {
  enabled: process.env.INCREMENTAL_ENABLED === 'true',
  // "full" | "incremental" | "smart"
  strategy: process.env.SCAN_STRATEGY || 'full',
  // 168h (7 days) — aligned with the operator default. A 24h fallback broke
  // daily scans: all cache entries expired before the next run and every
  // "incremental" scan silently became a full scan.
  maxFileAgeHours: intEnv(process.env.MAX_FILE_AGE_HOURS, 168),
  skipUnchangedFiles: process.env.SKIP_UNCHANGED_FILES !== 'false',
  // For "smart" strategy: run a full scan every N incremental runs
  fullScanInterval: intEnv(process.env.FULL_SCAN_INTERVAL, 10),
};

// =============================================================================
// REPORT RETENTION CONFIGURATION
// =============================================================================

const REPORT_CONFIG = {
  // Maximum number of JSON scan reports to keep per node on the hostPath.
  // Oldest reports are deleted after each scan to prevent unbounded disk growth.
  // Set to 0 to disable rotation (keep all reports).
  maxReports: intEnv(process.env.MAX_SCAN_REPORTS, 30),
};

module.exports = { CONFIG, INCREMENTAL_CONFIG, REPORT_CONFIG };
