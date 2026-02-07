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
  logger.info('Initialisation du scanner', {
    mode: CONFIG.scanMode,
    clamscan_path: CONFIG.clamscanPath,
    clamav_db: CONFIG.clamavDbPath,
    remote_host: CONFIG.clamavHost,
    update_signatures: CONFIG.updateSignatures,
  });

  if (CONFIG.scanMode === 'standalone') {
    return initStandaloneScanner();
  }
  return initRemoteScanner();
}

// =============================================================================
// Standalone mode — local clamscan binary, zero network dependency
// =============================================================================

async function initStandaloneScanner() {
  logger.info('Mode standalone — utilisation de clamscan local');

  // 1. Verify binary
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

  // 4. Build NodeClam in clamscan-only mode
  const clamscan = await new NodeClam().init({
    removeInfected: false,
    quarantineInfected: false,
    debugMode: false,
    clamscan: {
      path: CONFIG.clamscanPath,
      db: CONFIG.clamavDbPath,
      scanArchives: true,
      active: true,
    },
    clamdscan: {
      active: false,
    },
    preference: 'clamscan',
  });

  const version = await clamscan.getVersion();
  logger.info('Scanner standalone initialisé', { version });
  return clamscan;
}

// =============================================================================
// Remote mode — connect to an external clamd daemon (legacy behaviour)
// =============================================================================

async function initRemoteScanner() {
  if (!CONFIG.clamavHost) {
    throw new Error('CLAMAV_HOST is required for remote mode');
  }

  logger.info('Mode remote — connexion à clamd distant', {
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
  logger.info('Connexion clamd établie', { version });
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
  logger.info('Mise à jour des signatures via freshclam…');
  try {
    const { stdout, stderr } = await execFileAsync('freshclam', [
      '--datadir', CONFIG.clamavDbPath,
      '--stdout',
    ], { timeout: 300_000 });

    if (stdout) logger.debug('freshclam stdout', { output: stdout.slice(0, 500) });
    if (stderr) logger.warn('freshclam stderr', { output: stderr.slice(0, 500) });
    logger.info('Signatures mises à jour avec succès');
  } catch (err) {
    // Exit code 1 = "already up-to-date"
    if (err.code === 1) {
      logger.info('Signatures déjà à jour');
    } else {
      logger.error('Échec freshclam — les signatures existantes seront utilisées', {
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
