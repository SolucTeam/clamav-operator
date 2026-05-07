# Monitoring & Alerting

## Enabling Prometheus Integration

```yaml
# values-override.yaml
monitoring:
  serviceMonitor:
    enabled: true
    interval: 30s
  prometheusRule:
    enabled: true
```

## Grafana Dashboard

The repository ships a pre-built Grafana dashboard at `grafana-dashboard-clamav-operator.json`.

Import it via **Dashboards → Import → Upload JSON file** or paste the JSON directly. The dashboard
is self-contained: a `datasource` variable lets you select the Prometheus instance at import time,
so it works on any cluster without modification.

### Multi-cluster setup

The dashboard supports multiple clusters in a single view when all clusters report to the same
Prometheus (Thanos, VictoriaMetrics, or Prometheus remote_write federation).

Each cluster's Prometheus must set an `external_labels.cluster` label:

```yaml
# kube-prometheus-stack values
prometheus:
  prometheusSpec:
    externalLabels:
      cluster: "prod-eu-west"   # unique per cluster
```

Once metrics are federated, the dashboard exposes four cascading variables:

```
datasource  →  cluster  →  namespace  →  node
```

Select **All** on the `cluster` variable to see all clusters simultaneously, or pick one to
drill down. The `namespace` and `node` dropdowns automatically scope to the selected cluster(s).

### Dashboard rows

| Row | Content |
|-----|---------|
| **Security** | Infected files, partial results, last scan age, running/completed/failed counts |
| **Scan Activity** | Hourly detections, files scanned, completed vs failed, schedule executions |
| **Node Detail** | Per-node last scan table, scan duration p50/p95, files scanned per node |
| **Incremental & Cache** | Cache hit rate, skipped files, time saved, cache size, strategy distribution |
| **ClusterScans** | ClusterScan status over time, node progression |
| **Notifications** | Sent vs failed by channel, delivery success rate |
| **Storage & Maintenance** | Results dir size, cache file size, report rotation, cache pruning |
| **Performance & Resources** | Avg duration, throughput (files/s), cAdvisor CPU/RAM, Node.js process metrics |
| **Security & Freshness** | Signature age, OOMKills, parse retries, cache invalidations, node coverage ratio |

---

## Metrics Reference

All metrics are exposed on `:8080/metrics` by the operator pod.

### Scan lifecycle

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `clamav_nodescans_total` | Counter | `namespace`, `node`, `status` | NodeScans created, by final phase |
| `clamav_nodescans_running` | Gauge | `namespace` | Currently running NodeScans |
| `clamav_files_scanned_total` | Counter | `namespace`, `node` | Total files scanned (cumulative) |
| `clamav_files_infected_total` | Counter | `namespace`, `node` | Total infected files detected |
| `clamav_scan_duration_seconds` | Histogram | `namespace`, `node` | Scan duration — buckets: 30s 1m 2m 5m 10m 20m 30m 60m |
| `clamav_nodescan_last_completion_timestamp` | Gauge | `namespace`, `node` | Unix timestamp of last successful scan completion |
| `clamav_nodescan_partial_results` | Gauge | `namespace`, `node` | 1 if last scan has partial/unreliable results |

### Performance

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `clamav_scan_files_per_second` | Gauge | `namespace`, `node` | Scan throughput of the last completed scan (files/s) |
| `clamav_scanner_memory_rss_bytes` | Gauge | `namespace`, `node` | RSS memory of the scanner Node.js process at end of scan. Use cAdvisor `container_memory_working_set_bytes` for full container memory. |
| `clamav_scanner_cpu_user_seconds` | Gauge | `namespace`, `node` | User-space CPU consumed by the scanner Node.js process. Use cAdvisor `container_cpu_usage_seconds_total` for full container CPU. |

### Incremental scanning & cache

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `clamav_incremental_scans_total` | Counter | `namespace`, `node`, `strategy` | Scans by effective strategy (`full` or `incremental`) |
| `clamav_files_skipped_incremental_total` | Counter | `namespace`, `node` | Files skipped because unchanged since last scan |
| `clamav_cache_hit_rate_percent` | Gauge | `namespace`, `node` | Cache hit rate as a percentage (0–100) |
| `clamav_time_saved_incremental_seconds` | Counter | `namespace`, `node` | Estimated time saved by skipping unchanged files |
| `clamav_scan_cache_size_bytes` | Gauge | `namespace`, `node` | Logical size of files tracked in the incremental cache |
| `clamav_scan_cache_files_total` | Gauge | `namespace`, `node` | Number of file entries tracked in the incremental cache |
| `clamav_cache_age_seconds` | Gauge | `namespace`, `node` | Age of the cache file at scan time (seconds). `-1` = no valid cache (first scan or invalidated). |
| `clamav_cache_invalidations_total` | Counter | `namespace`, `node`, `reason` | Cache invalidations by reason: `first_scan`, `signature_change`, `corrupted` |

### Security & freshness

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `clamav_signature_db_age_seconds` | Gauge | `namespace`, `node` | Age of the ClamAV signature database (seconds since newest `.cvd`/`.cld` file). `>86400` = stale. |
| `clamav_job_oom_kills_total` | Counter | `namespace`, `node` | Scanner Job pods terminated by OOMKill (exit code 137) |
| `clamav_parse_retries_total` | Counter | `namespace`, `node` | Log parse retries by the operator. High values indicate log streaming issues. |

### Storage & maintenance

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `clamav_results_dir_bytes` | Gauge | `namespace`, `node` | Total size of the scan results directory on the node hostPath (`/var/log/clamav-scans`) |
| `clamav_cache_file_bytes` | Gauge | `namespace`, `node` | Size of the incremental cache JSON file on the node hostPath |
| `clamav_reports_rotated_total` | Counter | `namespace`, `node` | Old scan report files deleted by the rotation logic |
| `clamav_cache_entries_pruned_total` | Counter | `namespace`, `node` | Stale cache entries removed (files deleted from the node since last scan) |

### ClusterScan & scheduling

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `clamav_clusterscans_total` | Counter | `namespace`, `status` | ClusterScans by final phase |
| `clamav_clusterscan_nodes_total` | Gauge | `namespace`, `clusterscan` | Total nodes in a ClusterScan |
| `clamav_clusterscan_nodes_completed` | Gauge | `namespace`, `clusterscan` | Completed nodes in a ClusterScan |
| `clamav_clusterscan_nodes_failed` | Gauge | `namespace`, `clusterscan` | Failed nodes in a ClusterScan |
| `clamav_scanschedule_executions_total` | Counter | `namespace`, `schedule`, `status` | ScanSchedule executions by status |
| `clamav_scanpolicy_usage_total` | Counter | `namespace`, `policy` | Times a ScanPolicy has been applied |

### Notifications

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `clamav_notifications_sent_total` | Counter | `namespace`, `channel` | Notification attempts (all channels) |
| `clamav_notifications_failed_total` | Counter | `namespace`, `channel` | Notification delivery failures after all retries |

---

## PrometheusRules

The Helm chart ships 14 pre-built alert rules when `monitoring.prometheusRule.enabled=true`.

### Security alerts

| Alert | Severity | Condition | Description |
|-------|----------|-----------|-------------|
| `ClamAVMalwareDetected` | critical | `clamav_files_infected_total > 0` | Malware detected on a node |
| `ClamAVNoRecentScans` | warning | No completed scan in last 24 h | Nodes not being scanned |
| `ClamAVScanFailed` | warning | Scan failures in the last 10 min | Scanner Job exiting non-zero |
| `ClamAVPartialScanResults` | warning | `clamav_nodescan_partial_results == 1` | Result parsing incomplete — data unreliable |
| `ClamAVSignaturesStale` | warning | Signature age > 24 h for 1 h | ClamAV signatures not updated — missed detections possible |

### Reliability alerts

| Alert | Severity | Condition | Description |
|-------|----------|-----------|-------------|
| `ClamAVScanStuck` | warning | Running scan with no completion > 10 h | Job zombie or node connectivity loss |
| `ClamAVHighScanDuration` | warning | p95 scan duration > 2 h | Filesystem too large or I/O saturation |
| `ClamAVLowScanThroughput` | warning | Throughput < 5 files/s for 30 min | I/O degradation or resource pressure |
| `ClamAVOOMKillsElevated` | warning | > 2 OOMKills in 1 h | Memory limits too low for the workload |
| `ClamAVOperatorReconcileErrors` | warning | Reconcile error rate > 0.1/s for 10 min | Operator struggling to reconcile resources |

### Infrastructure alerts

| Alert | Severity | Condition | Description |
|-------|----------|-----------|-------------|
| `ClamAVNotificationFailed` | warning | Notification failures > 0 | Slack/email/webhook not reachable |
| `ClamAVWebhookCertExpiringSoon` | warning | Cert expiry < 7 days | TLS certificate about to expire |
| `ClamAVResultsDirTooLarge` | warning | Results dir > 500 MB | Disk pressure on node hostPath |
| `ClamAVCacheFileTooLarge` | warning | Cache file > 100 MB | Incremental cache unusually large |

---

## Quick Health Check

```bash
# Operator metrics
kubectl -n clamav-system port-forward svc/clamav-operator 8080:8080 &
curl -s localhost:8080/metrics | grep clamav_

# Recent scan activity
kubectl -n clamav-system get nodescans --sort-by=.metadata.creationTimestamp | tail -10

# Any infected files
kubectl -n clamav-system get nodescans \
  -o jsonpath='{range .items[?(@.status.filesInfected>0)]}{.metadata.name}{"\t"}{.status.filesInfected}{"\n"}{end}'

# Signature freshness
curl -s localhost:8080/metrics | grep clamav_signature_db_age_seconds

# Check for OOMKills
curl -s localhost:8080/metrics | grep clamav_job_oom_kills_total

# Force a full scan on a specific node (bypasses incremental cache for one run)
kubectl annotate nodescan <nodescan-name> clamav.io/force-full-scan=true
```
