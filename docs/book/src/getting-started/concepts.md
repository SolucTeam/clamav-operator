# Concepts

## CRD Model

The operator introduces four custom resources:

| Resource | Scope | Purpose |
|----------|-------|---------|
| `NodeScan` | Namespaced | Scan a single node — creates and owns a Kubernetes Job |
| `ClusterScan` | Namespaced | Scan all (or selected) nodes concurrently — creates and owns NodeScans |
| `ScanSchedule` | Namespaced | Trigger recurring ClusterScans on a cron schedule |
| `ScanPolicy` | Namespaced | Reusable scan configuration: paths, exclusions, resources, notifications |

## Ownership Chain

```
ScanSchedule
  └── ClusterScan  (owned, garbage-collected on delete)
        └── NodeScan ×N  (one per node, owned)
              └── Job  (one per NodeScan, owned)
                    └── Pod  (runs clamscan on the node)
```

Every object in the chain sets an `ownerReference` on the one below. Deleting a `ClusterScan` cascades to all its `NodeScan` objects and their `Job` objects automatically.

## Reconciliation Flow

1. **ScanSchedule** reconciler fires when the next cron slot is reached. It creates a `ClusterScan` and records `LastScheduleTime`.
2. **ClusterScan** reconciler lists nodes matching `spec.nodeSelector`, creates one `NodeScan` per node (respecting `spec.concurrent`).
3. **NodeScan** reconciler creates a `Job` that runs the scanner container on the target node (via `spec.nodeName`). It watches the Job to completion, parses the JSON results from pod logs, and writes `FilesScanned`, `FilesInfected`, `Phase` back to status.
4. Notifications are dispatched asynchronously — they never block the reconcile loop.
5. When all `NodeScan` objects reach a terminal phase, the `ClusterScan` transitions to `Completed` or `Failed`.

## Scan Phases

**NodeScan phases:**

| Phase | Meaning |
|-------|---------|
| `Pending` | CR created, Job not yet created |
| `Running` | Job is running on the node |
| `Completed` | Scan finished, results parsed |
| `Failed` | Job failed or results could not be parsed |

**ClusterScan phases:**

| Phase | Meaning |
|-------|---------|
| `Pending` | No NodeScans created yet |
| `Running` | At least one NodeScan is Running |
| `Completed` | All NodeScans reached a terminal phase |
| `Failed` | One or more NodeScans failed |

## Incremental Scanning

The scanner supports three strategies controlled by `spec.strategy`:

| Strategy | Behaviour |
|----------|-----------|
| `full` | Scan every file on every run |
| `incremental` | Skip files whose `mtime` + `size` haven't changed since the last scan |
| `smart` | Alternate automatically: N incremental runs, then 1 full scan |

The incremental cache is stored on the node at `/var/log/clamav-scans/<node>_scan_cache.json`. It is invalidated automatically when ClamAV signatures are updated (fingerprint-based detection).
