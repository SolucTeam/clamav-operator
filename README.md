# ClamAV Operator

Kubernetes Operator for managing ClamAV antivirus scans on Kubernetes clusters.

## Description

The ClamAV Operator enables automated antivirus scanning across your Kubernetes cluster nodes:

- **Scan individual nodes** via the `NodeScan` resource
- **Scan the entire cluster** via the `ClusterScan` resource
- **Define reusable scan policies** via `ScanPolicy`
- **Schedule automatic scans** via `ScanSchedule`
- **Cache scan results** via `ScanCacheResource` for incremental scanning

## Features

- Kubernetes-native API with Custom Resource Definitions (CRDs)
- Parallel scans with concurrency control
- Reusable scan policies with resource management
- Automatic scheduling (cron-based)
- Incremental scanning with caching support
- Notifications (Slack, Email, Webhook)
- Prometheus metrics
- Kubernetes events
- Webhook validation
- Priority-based resource allocation
- Startup validation checks

## Requirements

- Kubernetes 1.24+
- ClamAV deployed in the cluster (service available)
- kubectl configured
- Helm 3.x (for Helm installation)

## Installation

### Using Helm (Recommended)

```bash
# Add the repository (if published)
helm repo add solucteam https://solucteam.github.io/charts

# Install the operator
helm install clamav-operator solucteam/clamav-operator -n clamav-system --create-namespace

# Or install from local chart
helm install clamav-operator ./helm/clamav-operator -n clamav-system --create-namespace
```

### Custom Configuration

Create a custom values file to override defaults:

```yaml
# custom-values.yaml
operator:
  replicaCount: 2
  resources:
    limits:
      cpu: 1000m
      memory: 512Mi

scanner:
  clamav:
    host: my-clamav.namespace.svc.cluster.local

monitoring:
  serviceMonitor:
    enabled: true
  prometheusRule:
    enabled: true
```

```bash
helm install clamav-operator ./helm/clamav-operator -n clamav-system -f custom-values.yaml
```

### Using kubectl

```bash
# Install the CRDs
kubectl apply -f config/crd/bases/

# Deploy the operator
kubectl apply -f dist/install.yaml
```

### Build from Source

```bash
# Clone the repository
git clone https://github.com/SolucTeam/clamav-operator.git
cd clamav-operator

# Generate manifests
make manifests

# Build the Docker image
make docker-build IMG=ghcr.io/solucteam/clamav-operator:latest

# Push the image
make docker-push IMG=ghcr.io/solucteam/clamav-operator:latest

# Deploy
make deploy IMG=ghcr.io/solucteam/clamav-operator:latest
```

## Usage

### Scan a Specific Node

```yaml
apiVersion: clamav.io/v1alpha1
kind: NodeScan
metadata:
  name: scan-worker-01
  namespace: clamav-system
spec:
  nodeName: worker-01
  priority: high        # high, medium, low
  maxConcurrent: 10
  paths:
    - /var/lib
    - /opt
```

```bash
kubectl apply -f nodescan.yaml
kubectl get nodescan -n clamav-system
kubectl describe nodescan scan-worker-01 -n clamav-system
```

### Scan the Entire Cluster

```yaml
apiVersion: clamav.io/v1alpha1
kind: ClusterScan
metadata:
  name: nightly-scan
  namespace: clamav-system
spec:
  nodeSelector:
    matchLabels:
      node-role.kubernetes.io/worker: ""
  scanPolicy: production-policy
  concurrent: 3
```

### Create a Scan Policy

```yaml
apiVersion: clamav.io/v1alpha1
kind: ScanPolicy
metadata:
  name: production-policy
  namespace: clamav-system
spec:
  paths:
    - /var/lib
    - /opt
    - /usr/local

  excludePatterns:
    - "*.tmp"
    - "/var/lib/docker/overlay2/*"

  maxConcurrent: 5
  fileTimeout: 300000
  maxFileSize: 524288000

  resources:
    requests:
      cpu: 500m
      memory: 512Mi
    limits:
      cpu: 2000m
      memory: 1Gi

  notifications:
    slack:
      enabled: true
      webhookSecretRef:
        name: slack-webhook
        key: url
      channel: "#security-alerts"
```

### Schedule Automatic Scans

```yaml
apiVersion: clamav.io/v1alpha1
kind: ScanSchedule
metadata:
  name: daily-full-scan
  namespace: clamav-system
spec:
  schedule: "0 2 * * *"  # Every day at 2 AM

  clusterScan:
    nodeSelector:
      matchLabels:
        node-role.kubernetes.io/worker: ""
    scanPolicy: production-policy
    concurrent: 2

  successfulScansHistoryLimit: 10
  failedScansHistoryLimit: 3
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `POD_NAMESPACE` | Namespace where the operator runs | Auto-detected |
| `SCANNER_IMAGE` | Image for scanner jobs | From Helm values |
| `CLAMAV_HOST` | ClamAV service hostname | `clamav.clamav.svc.cluster.local` |
| `CLAMAV_PORT` | ClamAV service port | `3310` |
| `SCANNER_SERVICE_ACCOUNT` | ServiceAccount for scanner jobs | `clamav-scanner` |
| `ENABLE_LEADER_ELECTION` | Enable leader election for HA | `true` |

See [docs/ENVIRONMENT.md](docs/ENVIRONMENT.md) for complete documentation.

### Priority-Based Resources

The operator automatically allocates resources based on scan priority:

| Priority | CPU Request | Memory Request | CPU Limit | Memory Limit |
|----------|-------------|----------------|-----------|--------------|
| high     | 500m        | 512Mi          | 2000m     | 1Gi          |
| medium   | 100m        | 256Mi          | 1000m     | 512Mi        |
| low      | 50m         | 128Mi          | 500m      | 256Mi        |

## Monitoring

### Prometheus Metrics

The operator exposes the following metrics:

```promql
# Active scans
clamav_nodescan_running

# Infected files detected
clamav_files_infected_total

# Scan duration
clamav_scan_duration_seconds

# Files scanned
clamav_files_scanned_total
```

### Grafana Dashboards

Pre-configured dashboards are available in `config/grafana/`.

### Logs

Operator logs are structured in JSON:

```bash
kubectl logs -n clamav-system deployment/clamav-operator-controller-manager -f
```

## Development

### Setup

```bash
# Install dependencies
go mod download

# Generate code
make generate

# Run tests
make test

# Run the operator locally
make run
```

### Running Tests

```bash
# Unit tests
make test

# Integration tests (requires cluster)
make test-e2e

# Coverage report
make test-coverage
```

## Architecture

```
                    ┌─────────────────────────────────────────┐
                    │           ClamAV Operator               │
                    │  ┌───────────────────────────────────┐  │
                    │  │          Controllers              │  │
                    │  │  - NodeScan Controller            │  │
                    │  │  - ClusterScan Controller         │  │
                    │  │  - ScanPolicy Controller          │  │
                    │  │  - ScanSchedule Controller        │  │
                    │  │  - ScanCache Controller           │  │
                    │  └───────────────────────────────────┘  │
                    └──────────────────┬──────────────────────┘
                                       │
                                       ▼
                          ┌───────────────────────┐
                          │    Kubernetes API     │
                          │    - CRDs             │
                          │    - Jobs             │
                          │    - Nodes            │
                          └───────────┬───────────┘
                                      │
                    ┌─────────────────┼─────────────────┐
                    │                 │                 │
                    ▼                 ▼                 ▼
           ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
           │ Scanner Job  │  │ Scanner Job  │  │ Scanner Job  │
           │   (Node 1)   │  │   (Node 2)   │  │   (Node N)   │
           └──────┬───────┘  └──────┬───────┘  └──────┬───────┘
                  │                 │                 │
                  └─────────────────┴─────────────────┘
                                    │
                                    ▼
                          ┌───────────────────────┐
                          │    ClamAV Service     │
                          │    (clamd daemon)     │
                          └───────────────────────┘
```

## API Reference

### NodeScan

| Field | Type | Description |
|-------|------|-------------|
| `spec.nodeName` | string | Target node name |
| `spec.priority` | string | Priority level (high/medium/low) |
| `spec.paths` | []string | Paths to scan |
| `spec.excludePatterns` | []string | Patterns to exclude |
| `spec.scanPolicy` | string | Reference to ScanPolicy |
| `spec.maxConcurrent` | int | Max concurrent file scans |

### ClusterScan

| Field | Type | Description |
|-------|------|-------------|
| `spec.nodeSelector` | LabelSelector | Node selection criteria |
| `spec.scanPolicy` | string | Reference to ScanPolicy |
| `spec.concurrent` | int | Max concurrent NodeScans |

### ScanPolicy

| Field | Type | Description |
|-------|------|-------------|
| `spec.paths` | []string | Default paths to scan |
| `spec.excludePatterns` | []string | Patterns to exclude |
| `spec.maxConcurrent` | int | Max concurrent file scans |
| `spec.fileTimeout` | int64 | File scan timeout (ms) |
| `spec.maxFileSize` | int64 | Max file size to scan |
| `spec.resources` | ResourceRequirements | Pod resources |
| `spec.notifications` | NotificationConfig | Notification settings |

### ScanSchedule

| Field | Type | Description |
|-------|------|-------------|
| `spec.schedule` | string | Cron expression |
| `spec.nodeScan` | NodeScanSpec | NodeScan template |
| `spec.clusterScan` | ClusterScanSpec | ClusterScan template |
| `spec.successfulScansHistoryLimit` | int | History limit |

## Troubleshooting

### Common Issues

**Scan job not starting:**
```bash
# Check operator logs
kubectl logs -n clamav-system deployment/clamav-operator-controller-manager

# Check events
kubectl get events -n clamav-system --sort-by='.lastTimestamp'
```

**ClamAV service not reachable:**
```bash
# Verify ClamAV service
kubectl get svc -n clamav

# Test connectivity
kubectl run test --rm -it --image=busybox -- nc -zv clamav.clamav.svc.cluster.local 3310
```

**Permission denied errors:**
```bash
# Verify RBAC
kubectl auth can-i create jobs --as=system:serviceaccount:clamav-system:clamav-operator -n clamav-system
```

## License

Copyright 2025 The ClamAV Operator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Support

- Issues: [GitHub Issues](https://github.com/SolucTeam/clamav-operator/issues)
- Discussions: [GitHub Discussions](https://github.com/SolucTeam/clamav-operator/discussions)
