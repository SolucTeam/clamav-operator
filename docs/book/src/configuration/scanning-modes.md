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

Skips files whose `mtime` + `size` haven't changed since the last run. The cache is stored on the node hostPath at `/var/log/clamav-scans/<node>_scan_cache.json` and persists between scan Jobs.

**Cache invalidation:** when ClamAV signatures are updated (new `daily.cvd`), the cache fingerprint changes and the next scan automatically runs as a full scan — no stale results.

**Cache pruning:** entries for files that have been deleted from the node filesystem are automatically removed when the cache is saved, preventing unbounded cache growth over time.

> **⚠️ Important — `maxFileAgeHours` pitfall:** with a daily scan schedule, setting `maxFileAgeHours: 24` (the default) causes every cached file to be re-scanned on every run, effectively identical to a full scan. Set this to at least `168` (7 days) to get real incremental benefit.

```yaml
spec:
  strategy: incremental
  incrementalConfig:
    maxFileAgeHours: 168    # re-scan even unchanged files after 7 days (not 24!)
    skipUnchangedFiles: true
```

### `smart`

Alternates automatically: N incremental scans, then 1 full scan. Best balance for production.

The run counter is stored on the node at `/var/log/clamav-scans/<node>_smart_counter.txt` and persists between Jobs on the same node.

> **Note:** The `smart` strategy requires `maxFileAgeHours` to be set higher than your scan interval (e.g. `168` for daily scans) — otherwise `maxFileAgeHours` expiry will cause every file to be re-scanned regardless of the counter, making incremental runs behave like full scans.

```yaml
spec:
  strategy: smart
  incrementalConfig:
    fullScanInterval: 10    # full scan every 10 runs (≈ every 10 days if daily scans)
    maxFileAgeHours: 168    # must be > scan interval to avoid neutralising incremental
```

## Report Retention

Each scan produces two files on the node hostPath (`/var/log/clamav-scans/`):

- `<node>_scan_<timestamp>.json` — full JSON report (scan stats, infected files, errors)
- `<node>_summary_<timestamp>.txt` — short text summary parsed by the operator

Without rotation these files accumulate indefinitely. By default the scanner keeps the **30 most recent** reports per node and deletes older ones after each scan. Configure via Helm:

```yaml
scanner:
  incremental:
    maxScanReports: 30   # set to 0 to disable rotation (not recommended)
```

Or via the `MAX_SCAN_REPORTS` environment variable on the scanner Job.

The following files are **never** rotated (they are overwritten, not accumulated):
- `<node>_scan_cache.json` — incremental cache
- `<node>_smart_counter.txt` — smart strategy run counter

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
