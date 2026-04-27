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

## Metrics Reference

All metrics are exposed on `:8080/metrics`.

| Metric | Type | Description |
|--------|------|-------------|
| `clamav_files_scanned_total` | Counter | Total files scanned |
| `clamav_files_infected_total` | Counter | Total infected files found |
| `clamav_files_skipped_total` | Counter | Files skipped (errors / exclusions) |
| `clamav_scan_duration_seconds` | Histogram | Scan duration per NodeScan |
| `clamav_nodescans_total` | Counter | NodeScans by status (`completed`, `failed`) |
| `clamav_nodescans_running` | Gauge | Currently running NodeScans |
| `clamav_notification_failures_total` | Counter | Notification delivery failures |
| `clamav_incremental_scans_total` | Counter | Incremental scans by strategy |
| `clamav_files_skipped_incremental_total` | Counter | Files skipped by incremental logic |
| `clamav_incremental_cache_hit_rate` | Gauge | Cache hit rate percentage |
| `clamav_time_saved_incremental_seconds` | Counter | Estimated time saved by incremental scans |

## PrometheusRules

The Helm chart ships with pre-built alert rules when `monitoring.prometheusRule.enabled=true`:

| Alert | Severity | Description |
|-------|----------|-------------|
| `ClamAVInfectionDetected` | critical | Malware found on a node |
| `ClamAVScanFailed` | warning | NodeScan reached Failed phase |
| `ClamAVScanStale` | warning | No completed scan in last 25 hours |
| `ClamAVNotificationFailed` | warning | Notification not delivered after retries |
| `ClamAVPartialScanResults` | warning | Result parsing incomplete |
| `ClamAVWebhookCertExpiringSoon` | warning | Webhook TLS cert expires in < 7 days |
| `ClamAVHighInfectionRate` | critical | Infection rate > 1% of scanned files |

## Quick Health Check

```bash
# Operator metrics
kubectl -n clamav-system port-forward svc/clamav-operator 8080:8080 &
curl -s localhost:8080/metrics | grep clamav_

# Recent scan activity
kubectl -n clamav-system get nodescans --sort-by=.metadata.creationTimestamp | tail -10

# Any infected files in the last 24 h
kubectl -n clamav-system get nodescans \
  -o jsonpath='{range .items[?(@.status.filesInfected>0)]}{.metadata.name}{"\t"}{.status.filesInfected}{"\n"}{end}'
```
