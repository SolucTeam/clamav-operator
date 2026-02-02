# Environment Variables Reference

This document describes all environment variables used by the ClamAV Operator and its components.

## Operator Configuration

These environment variables configure the ClamAV Operator itself.

### Command-Line Arguments (as env vars via Helm)

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `--metrics-bind-address` | Address for metrics endpoint | `:8080` | No |
| `--health-probe-bind-address` | Address for health probes | `:8081` | No |
| `--leader-elect` | Enable leader election | `false` | No |
| `--scanner-image` | Container image for the scanner | `registry.tooling.../clamav-node-scanner:1.0.3` | Yes |
| `--clamav-host` | ClamAV service hostname | `clamav.clamav.svc.cluster.local` | Yes |
| `--clamav-port` | ClamAV service port | `3310` | Yes |

### Helm Values

Configure these via Helm `values.yaml`:

```yaml
operator:
  replicaCount: 1
  leaderElection:
    enabled: true

scanner:
  clamav:
    host: clamav.clamav.svc.cluster.local
    port: 3310
```

## Scanner Job Environment Variables

These environment variables are passed to scanner jobs (pods).

| Variable | Description | Default | Source |
|----------|-------------|---------|--------|
| `NODE_NAME` | Name of the node being scanned | - | NodeScan.spec.nodeName |
| `HOST_ROOT` | Mount point for host filesystem | `/host` | Fixed |
| `RESULTS_DIR` | Directory for scan results | `/results` | Fixed |
| `CLAMAV_HOST` | ClamAV service hostname | From operator config | Operator config |
| `CLAMAV_PORT` | ClamAV service port | From operator config | Operator config |
| `PATHS_TO_SCAN` | Comma-separated list of paths to scan | `/host/var/lib,/host/opt` | NodeScan.spec.paths or ScanPolicy.spec.paths |
| `MAX_CONCURRENT` | Maximum concurrent file scans | `5` | NodeScan.spec.maxConcurrent or ScanPolicy |
| `FILE_TIMEOUT` | Timeout for scanning a single file (ms) | `300000` | NodeScan.spec.fileTimeout or ScanPolicy |
| `CONNECT_TIMEOUT` | Timeout for ClamAV connection (ms) | `60000` | ScanPolicy.spec.connectTimeout |
| `MAX_FILE_SIZE` | Maximum file size to scan (bytes) | `104857600` | NodeScan.spec.maxFileSize or ScanPolicy |

## Default Resource Requirements

### Default Scanner Resources (Priority: medium)

```yaml
resources:
  requests:
    cpu: 100m
    memory: 256Mi
  limits:
    cpu: 1000m
    memory: 512Mi
```

### High Priority Scanner Resources

```yaml
resources:
  requests:
    cpu: 500m
    memory: 512Mi
  limits:
    cpu: 2000m
    memory: 1Gi
```

### Low Priority Scanner Resources

```yaml
resources:
  requests:
    cpu: 50m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 256Mi
```

## Configuration Priority

Values are resolved in the following order (highest priority first):

1. **NodeScan.spec** - Explicit configuration on the NodeScan resource
2. **ScanPolicy.spec** - Configuration from the referenced ScanPolicy
3. **Priority-based defaults** - Based on `priority: high|medium|low`
4. **Global defaults** - Hardcoded defaults in the operator

## Example Configurations

### Minimal Configuration

```yaml
apiVersion: clamav.platform.numspot.com/v1alpha1
kind: NodeScan
metadata:
  name: quick-scan
spec:
  nodeName: worker-1
  # Uses all defaults
```

### Full Configuration

```yaml
apiVersion: clamav.platform.numspot.com/v1alpha1
kind: NodeScan
metadata:
  name: detailed-scan
spec:
  nodeName: worker-1
  priority: high
  scanPolicy: security-policy
  paths:
    - /host/var/lib
    - /host/opt
    - /host/home
  excludePatterns:
    - "*.log"
    - "*.tmp"
    - "/host/var/lib/docker/overlay2/*"
  maxConcurrent: 10
  fileTimeout: 600000      # 10 minutes
  maxFileSize: 524288000   # 500MB
  ttlSecondsAfterFinished: 3600  # 1 hour
  resources:
    requests:
      cpu: 500m
      memory: 512Mi
    limits:
      cpu: 2
      memory: 2Gi
```

## Validation Constraints

| Parameter | Min | Max | Notes |
|-----------|-----|-----|-------|
| `maxConcurrent` (NodeScan) | 1 | 20 | Files scanned in parallel |
| `concurrent` (ClusterScan) | 1 | 50 | Nodes scanned in parallel |
| `fileTimeout` | 1000 ms | 3600000 ms | Per-file timeout |
| `maxFileSize` | 1024 bytes | 10737418240 bytes | Skip larger files |
| `paths` count | 1 | 100 | Number of paths to scan |
| `excludePatterns` count | 0 | 200 | Number of exclude patterns |

## Monitoring & Metrics

The operator exposes Prometheus metrics at the configured metrics address:

| Metric | Type | Description |
|--------|------|-------------|
| `clamav_nodescan_total` | Counter | Total number of NodeScans |
| `clamav_nodescan_duration_seconds` | Histogram | Scan duration |
| `clamav_files_scanned_total` | Counter | Total files scanned |
| `clamav_files_infected_total` | Counter | Total infected files found |
| `clamav_nodescan_failed` | Gauge | Number of failed scans |
| `clamav_nodescan_last_completion_timestamp` | Gauge | Timestamp of last completed scan |

## Troubleshooting

### Common Issues

1. **Scanner jobs stuck in Pending**: Check if the scanner ServiceAccount exists and has the required RBAC permissions.

2. **Connection to ClamAV fails**: Verify `CLAMAV_HOST` and `CLAMAV_PORT` are correct and the ClamAV service is reachable.

3. **OOM killed scanner pods**: Increase memory limits in resources configuration.

4. **Slow scans**: Reduce `maxConcurrent` or increase resource limits.

### Debug Mode

Enable debug logging by setting:

```yaml
env:
  - name: LOG_LEVEL
    value: debug
```
