# NodeScan

A `NodeScan` targets a single Kubernetes node and creates a Job that runs `clamscan` on it.

## Example

```yaml
apiVersion: clamav.io/v1alpha1
kind: NodeScan
metadata:
  name: scan-node-1
  namespace: clamav-system
spec:
  nodeName: "worker-1"
  priority: low
  strategy: smart
  paths:
    - /var/lib
    - /opt
  excludePatterns:
    - ".*\\.log$"
  maxConcurrent: 3
  fileTimeout: 30000        # 30 s per file (ms)
  maxFileSize: 104857600    # 100 MB
  resources:
    requests: { cpu: 200m, memory: 256Mi }
    limits:   { cpu: 1000m, memory: 1Gi }
  incrementalConfig:
    fullScanInterval: 7
    maxFileAgeHours: 168
    skipUnchangedFiles: true
```

## Spec Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `nodeName` | string | **required** | Kubernetes node name to scan |
| `priority` | string | `default` | `high`, `default`, or `low` — sets resource profile |
| `strategy` | string | `full` | `full`, `incremental`, or `smart` |
| `scanPolicy` | string | — | Name of a `ScanPolicy` to inherit config from |
| `paths` | []string | `["/"]` | Paths to scan on the node |
| `excludePatterns` | []string | — | Regex patterns to exclude |
| `maxConcurrent` | int32 | `5` | Max concurrent file scans |
| `fileTimeout` | int64 | `30000` | Per-file timeout in milliseconds |
| `maxFileSize` | int64 | `104857600` | Max file size in bytes (100 MB) |
| `resources` | ResourceRequirements | — | Override the priority-based defaults |
| `scanMode` | string | `standalone` | `standalone` or `remote` |
| `forceFullScan` | bool | `false` | Force full scan even if incremental is enabled |
| `ttlSecondsAfterFinished` | int32 | `86400` | Job TTL after completion |

## Status Fields

| Field | Description |
|-------|-------------|
| `phase` | `Pending`, `Running`, `Completed`, or `Failed` |
| `filesScanned` | Total files scanned |
| `filesInfected` | Files with detected threats |
| `filesSkipped` | Files skipped (errors or exclusions) |
| `filesSkippedIncremental` | Files skipped by incremental logic |
| `strategyUsed` | Actual strategy used (`full` or `incremental`) |
| `cacheHitRate` | Percentage of cache hits (incremental only) |
| `startTime` | Job start time |
| `completionTime` | Job completion time |
| `jobRef` | Reference to the created Job |
| `resultsPartial` | `true` if result parsing failed partially |
