/*
Copyright 2025 The ClamAV Operator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

'use strict';

const NodeClam = require('clamscan');
const fs = require('fs').promises;
const path = require('path');
const { execFile } = require('child_process');
const { promisify } = require('util');

const { CONFIG } = require('./config');
const logger = require('./logger');

const execFileAsync = promisify(execFile);

// =============================================================================
// Public entry-point — returns a ready-to-use NodeClam instance
// =============================================================================

async function initScanner() {
  const baseInfo = {
    mode: CONFIG.scanMode,
    clamscan_path: CONFIG.clamscanPath,
    clamav_db: CONFIG.clamavDbPath,
    update_signatures: CONFIG.updateSignatures,
  };
  // remote_host is only meaningful in remote mode — omit it in standalone to
  // avoid misleading logs that suggest the scanner is trying to reach clamd.
  if (CONFIG.scanMode === 'remote') {
    baseInfo.remote_host = CONFIG.clamavHost;
    baseInfo.remote_port = CONFIG.clamavPort;
  }
  logger.info('Initializing scanner', baseInfo);

  if (CONFIG.scanMode === 'standalone') {
    return initStandaloneScanner();
  }
  return initRemoteScanner();
}

// =============================================================================
// Standalone mode — local clamscan binary, zero network dependency
// =============================================================================

async function initStandaloneScanner() {
  logger.info('Standalone mode — using local clamscan binary');

  // 1. Verify binary exists
  try {
    await fs.access(CONFIG.clamscanPath);
  } catch {
    throw new Error(`clamscan not found at ${CONFIG.clamscanPath}`);
  }

  // 2. Optional: run freshclam to update signatures before scanning
  if (CONFIG.updateSignatures) {
    await updateSignatures();
  }

  // 3. Verify at least one signature database is present
  await verifySignatures();

  // 4. Return a plain config object — scanner.js spawns a single clamscan
  //    --file-list subprocess directly via child_process. The DB is loaded
  //    exactly once per scan job (inside that subprocess) instead of once per
  //    file, eliminating the N × 800 Mi OOMKill risk of the old approach.
  //    NodeClam is not used in standalone mode.
  return {
    mode: 'standalone',
    clamscanPath: CONFIG.clamscanPath,
    dbPath: CONFIG.clamavDbPath,
  };
}

// =============================================================================
// Remote mode — connect to an external clamd daemon (legacy behaviour)
// =============================================================================

async function initRemoteScanner() {
  if (!CONFIG.clamavHost) {
    throw new Error('CLAMAV_HOST is required for remote mode');
  }

  logger.info('Remote mode — connecting to remote clamd', {
    host: CONFIG.clamavHost,
    port: CONFIG.clamavPort,
  });

  const clamscan = await new NodeClam().init({
    removeInfected: false,
    quarantineInfected: false,
    debugMode: false,
    clamdscan: {
      socket: false,
      host: CONFIG.clamavHost,
      port: CONFIG.clamavPort,
      timeout: CONFIG.fileTimeout,
      localFallback: false,
      active: true,
    },
    preference: 'clamdscan',
  });

  await clamscan.ping();
  const version = await clamscan.getVersion();
  logger.info('clamd connection established', { version });
  return clamscan;
}

// =============================================================================
// Helpers
// =============================================================================

/**
 * Run freshclam to pull the latest virus definitions.
 * Called only when UPDATE_SIGNATURES=true (i.e. NOT in air-gap mode).
 */
async function updateSignatures() {
  logger.info('Updating signatures via freshclam...');
  try {
    const { stdout, stderr } = await execFileAsync('freshclam', [
      '--datadir', CONFIG.clamavDbPath,
      '--stdout',
    ], { timeout: 300_000 });

    if (stdout) logger.debug('freshclam stdout', { output: stdout.slice(0, 500) });
    if (stderr) logger.warn('freshclam stderr', { output: stderr.slice(0, 500) });
    logger.info('Signatures updated successfully');
  } catch (err) {
    // Exit code 1 = "already up-to-date"
    if (err.code === 1) {
      logger.info('Signatures already up-to-date');
    } else {
      logger.error('freshclam failed — existing signatures will be used', {
        error: err.message,
      });
      // Non-fatal: continue with whatever signatures are already present
    }
  }
}

/**
 * Verify at least one ClamAV signature database file exists.
 */
async function verifySignatures() {
  const dbFiles = ['main.cvd', 'main.cld', 'daily.cvd', 'daily.cld', 'bytecode.cvd', 'bytecode.cld'];
  let found = false;

  for (const file of dbFiles) {
    try {
      await fs.access(path.join(CONFIG.clamavDbPath, file));
      found = true;
      logger.debug('Signature database found', { file });
      break;
    } catch {
      /* try next */
    }
  }

  if (!found) {
    throw new Error(
      `No ClamAV signatures found in ${CONFIG.clamavDbPath}. ` +
      'Either bake them into the image (air-gap) or set UPDATE_SIGNATURES=true.'
    );
  }
}

module.exports = { initScanner };
