# Scanning Modes

## Standalone vs Remote

| Mode | How it works | When to use |
|------|-------------|-------------|
| **Standalone** (default) | `clamscan` binary + signatures embedded in the scanner image | No ClamAV service to maintain; works air-gapped |
| **Remote** | Scanner connects to a central `clamd` daemon | Already have a managed ClamAV service |

Set the mode in your ScanPolicy or NodeScan:

```yaml
spec:
  scanMode: standalone   # or "remote"
  clamavHost: ""         # required for remote mode
  clamavPort: 3310       # required for remote mode
```

## Scan Strategies

Configure via `spec.strategy` on `NodeScan`, `ClusterScan`, or `ScanPolicy`:

### `full`

Scans every file on every run. Slowest but most thorough.

```yaml
spec:
  strategy: full
```

### `incremental`

Skips files whose `mtime` + `size` haven't changed since the last run. The cache is stored on the node at `/var/log/clamav-scans/<node>_scan_cache.json`.

**Cache invalidation:** when ClamAV signatures are updated (new `daily.cvd`), the cache fingerprint changes and the next scan automatically runs as a full scan — no stale results.

```yaml
spec:
  strategy: incremental
  incrementalConfig:
    maxFileAgeHours: 168    # re-scan even unchanged files after 7 days
    skipUnchangedFiles: true
```

### `smart`

Alternates automatically: N incremental scans, then 1 full scan. Best balance for production.

```yaml
spec:
  strategy: smart
  incrementalConfig:
    fullScanInterval: 7     # full scan every 7 runs (≈ weekly if daily scans)
```

## Paths and Exclusions

```yaml
spec:
  paths:
    - /var/lib
    - /opt/app
    - /home
  excludePatterns:
    - ".*\\.log$"
    - ".*/tmp/.*"
    - ".*/proc/.*"
```

> **Note:** The paths `/proc`, `/sys`, and `/dev` are blocked by admission webhook and cannot be scanned.

## Resource Profiles

Set `spec.priority` to control the scanner Job's CPU/memory requests:

| Priority | CPU request | Memory request | CPU limit | Memory limit |
|----------|-------------|----------------|-----------|--------------|
| `high`   | 500m        | 512Mi          | 2000m     | 2Gi          |
| `default`| 200m        | 256Mi          | 1000m     | 1Gi          |
| `low`    | 50m         | 128Mi          | 500m      | 512Mi        |

Or specify resources explicitly via `spec.resources` or a `ScanPolicy`.
