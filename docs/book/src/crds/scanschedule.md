# ScanSchedule

A `ScanSchedule` triggers recurring `ClusterScan` objects on a cron schedule. Scheduling is drift-free: the next run is always anchored to the cron grid, not to `time.Now()`.

## Example

```yaml
apiVersion: clamav.io/v1beta1
kind: ScanSchedule
metadata:
  name: nightly-scan
  namespace: clamav-system
spec:
  schedule: "0 2 * * *"    # every day at 02:00 UTC
  concurrent: 3
  priority: low
  suspend: false
  clusterScanTemplate:
    nodeScanTemplate:
      strategy: smart
      paths:
        - /var/lib
        - /opt
      incrementalConfig:
        fullScanInterval: 7
```

## Spec Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `schedule` | string | **required** | Cron expression (UTC) |
| `concurrent` | int32 | `3` | Passed to each ClusterScan |
| `priority` | string | `default` | Passed to each ClusterScan |
| `suspend` | bool | `false` | Pause scheduling without deleting the resource |
| `scanPolicy` | string | — | Name of a `ScanPolicy` to inherit config from |
| `clusterScanTemplate` | ClusterScanSpec | — | Template for each created ClusterScan |
| `successfulJobsHistoryLimit` | int32 | `3` | How many completed ClusterScans to keep |
| `failedJobsHistoryLimit` | int32 | `1` | How many failed ClusterScans to keep |

## Status Fields

| Field | Description |
|-------|-------------|
| `lastScheduleTime` | Time of the last triggered ClusterScan |
| `nextScheduleTime` | Computed next cron slot |
| `active` | Name of the currently running ClusterScan (if any) |

## Suspending a Schedule

```bash
kubectl patch scanschedule nightly-scan -n clamav-system \
  --type merge -p '{"spec":{"suspend":true}}'
```

## Triggering a Manual Run

Create a ClusterScan manually — it is independent from the schedule:

```bash
kubectl apply -f - <<EOF
apiVersion: clamav.io/v1alpha1
kind: ClusterScan
metadata:
  name: manual-run-$(date +%Y%m%d)
  namespace: clamav-system
spec:
  concurrent: 5
  priority: high
EOF
```
