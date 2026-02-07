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
- **Standalone scanner mode** — clamscan + signatures embedded in the scanner image, zero network dependency
- **Remote scanner mode** — connects to a central clamd service (legacy)
- **Air-gap support** — signatures pre-loaded in the image, no internet required
- **Incremental scanning** — only scan new/modified files, with smart strategy alternating full/incremental
- Parallel scans with concurrency control
- Reusable scan policies with resource management
- Automatic scheduling (cron-based)
- Freshclam CronJob for automatic signature updates
- Notifications (Slack, Email, Webhook)
- Prometheus metrics
- Kubernetes events
- Webhook validation
- Priority-based resource allocation
- Startup validation checks
- Multi-architecture Docker images (amd64/arm64)

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
           │  ┌────────┐  │  │  ┌────────┐  │  │  ┌────────┐  │
           │  │clamscan│  │  │  │clamscan│  │  │  │clamscan│  │
           │  │ local  │  │  │  │ local  │  │  │  │ local  │  │
           │  └────────┘  │  │  └────────┘  │  │  └────────┘  │
           └──────────────┘  └──────────────┘  └──────────────┘
                  ↑ Standalone mode: scan locally, zero network
```

In **remote mode** (legacy), each scanner Job connects to a central ClamAV service instead of using a local binary.

## Requirements

- Kubernetes 1.24+
- kubectl configured
- Helm 3.x (for Helm installation)
- **No external ClamAV service required** (standalone mode is the default)

## Quick Start

### Install with Helm

```bash
# Install the operator (standalone mode by default — no ClamAV service needed)
helm install clamav-operator ./helm/clamav-operator \
  -n clamav-system --create-namespace
```

### Scan a Node

```yaml
apiVersion: clamav.io/v1alpha1
kind: NodeScan
metadata:
  name: scan-worker-01
  namespace: clamav-system
spec:
  nodeName: worker-01
  priority: high
  scanPolicy: default-policy
```

```bash
kubectl apply -f nodescan.yaml
kubectl get nodescan -n clamav-system -w
```

## Scanner Modes

### Standalone (Default)

Each scanner Job carries its own `clamscan` binary and virus signatures. Scans execute locally on each node with zero network dependency. This eliminates the central ClamAV service as a single point of failure.

```yaml
scanner:
  mode: standalone
  freshclam:
    enabled: true               # auto-update signatures every 6h
    schedule: "0 */6 * * *"
```

### Air-Gap (Standalone without internet)

For disconnected environments, signatures are baked into the scanner image at build time. No downloads at runtime.

```yaml
scanner:
  mode: standalone
  freshclam:
    enabled: false              # no internet access needed
```

Build the air-gap image:

```bash
make docker-build-scanner-airgap
```

### Remote (Legacy)

Connects to a central clamd service. Use this if you already have ClamAV deployed separately.

```yaml
scanner:
  mode: remote
  clamav:
    host: clamav.clamav.svc.cluster.local
    port: 3310
```

## Incremental Scanning

The operator supports three scanning strategies to optimize performance:

| Strategy | Description |
|----------|-------------|
| `full` | Scan every file on every run |
| `incremental` | Only scan new or modified files since the last run |
| `smart` | Alternate between incremental and full scans automatically |

```yaml
scanner:
  incremental:
    enabled: true
    strategy: smart
    fullScanInterval: 10        # full scan every 10 incremental runs
    maxFileAgeHours: 24
    skipUnchangedFiles: true
```

## Installation

### Using Helm (Recommended)

```bash
# Install from local chart
helm install clamav-operator ./helm/clamav-operator -n clamav-system --create-namespace

# Or with custom values
helm install clamav-operator ./helm/clamav-operator -n clamav-system -f custom-values.yaml
```

### Custom Configuration

```yaml
# custom-values.yaml
operator:
  replicaCount: 2
  resources:
    limits:
      cpu: 1000m
      memory: 512Mi

scanner:
  mode: standalone
  freshclam:
    enabled: true
  incremental:
    enabled: true
    strategy: smart
  signatures:
    persistent: true            # store signatures on a PVC

monitoring:
  serviceMonitor:
    enabled: true
  prometheusRule:
    enabled: true
```

### Using kubectl

```bash
kubectl apply -f config/crd/bases/
kubectl apply -f dist/install.yaml
```

### Build from Source

```bash
git clone https://github.com/SolucTeam/clamav-operator.git
cd clamav-operator

# Build all images (operator + scanner)
make docker-build-all

# Or build individually
make docker-build IMG=ghcr.io/solucteam/clamav-operator:latest
make docker-build-scanner SCANNER_IMG=ghcr.io/solucteam/clamav-node-scanner:latest
make docker-build-scanner-airgap   # air-gap variant

# Push
make docker-push-all

# Deploy
make deploy IMG=ghcr.io/solucteam/clamav-operator:latest
```

## Project Structure

```
clamav-operator/
├── api/v1alpha1/           # CRD type definitions (Go)
├── build/Dockerfile        # Operator image (Go)
├── cmd/manager/            # Operator entry point
├── controllers/            # Reconcilers (NodeScan, ClusterScan, …)
├── scanner/                # Standalone scanner (Node.js)
│   ├── Dockerfile          # Scanner image (Node.js + ClamAV)
│   ├── package.json
│   └── src/
│       ├── index.js        # Entry point
│       ├── config.js       # Environment-driven configuration
│       ├── logger.js       # Structured JSON logging
│       ├── init-scanner.js # Standalone / remote init
│       ├── scanner.js      # Recursive directory scan
│       ├── incremental.js  # Incremental cache & smart strategy
│       ├── report.js       # JSON + text report generation
│       └── __tests__/      # Unit tests
├── helm/clamav-operator/   # Helm chart
├── config/                 # Kustomize bases (CRDs, RBAC, webhooks)
├── .github/workflows/      # CI/CD (Docker build + Trivy scan)
├── Makefile
└── docs/
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
| `SCAN_MODE` | Scanner mode (`standalone` or `remote`) | `standalone` |
| `SCANNER_IMAGE` | Image for scanner jobs | From Helm values |
| `CLAMAV_HOST` | ClamAV service hostname (remote mode) | `clamav.clamav.svc.cluster.local` |
| `CLAMAV_PORT` | ClamAV service port (remote mode) | `3310` |
| `CLAMSCAN_PATH` | Path to clamscan binary (standalone mode) | `/usr/bin/clamscan` |
| `CLAMAV_DB_PATH` | Path to signature databases (standalone mode) | `/var/lib/clamav` |
| `UPDATE_SIGNATURES` | Run freshclam before scanning | `false` |
| `INCREMENTAL_ENABLED` | Enable incremental scanning | `false` |
| `SCAN_STRATEGY` | Scan strategy (`full`, `incremental`, `smart`) | `full` |
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

## CI/CD

The project includes a GitHub Actions workflow (`.github/workflows/docker-build.yml`) that builds and pushes both images:

| Image | Registry | Description |
|-------|----------|-------------|
| `clamav-operator` | `ghcr.io/<owner>/clamav-operator` | Go operator |
| `clamav-node-scanner` | `ghcr.io/<owner>/clamav-node-scanner` | Node.js standalone scanner |
| `clamav-node-scanner:*-airgap` | `ghcr.io/<owner>/clamav-node-scanner` | Scanner without pre-downloaded signatures |

The workflow runs on push to `main`, release branches, version tags (`v*`), and pull requests. It includes multi-arch builds (amd64/arm64), GHA caching, and Trivy security scanning.

## Monitoring

### Prometheus Metrics

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

Operator and scanner logs are structured in JSON:

```bash
# Operator logs
kubectl logs -n clamav-system deployment/clamav-operator-controller-manager -f

# Scanner job logs
kubectl logs -n clamav-system -l clamav.io/nodescan=scan-worker-01
```

## Development

### Setup

```bash
go mod download
make generate
make test

# Scanner tests
cd scanner && node --test src/__tests__/
```

### Running Tests

```bash
# Go unit tests
make test

# Node.js scanner tests
cd scanner && node --test src/__tests__/

# Integration tests (requires cluster)
make test-e2e
```

### Building Images Locally

```bash
make docker-build-all           # Build operator + scanner
make docker-build-scanner-airgap # Build air-gap scanner variant
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
kubectl logs -n clamav-system deployment/clamav-operator-controller-manager
kubectl get events -n clamav-system --sort-by='.lastTimestamp'
```

**Standalone scanner — no signatures found:**
```bash
# Check the scanner image contains signatures
kubectl logs -n clamav-system -l clamav.io/nodescan=<scan-name>
# Look for: "No ClamAV signatures found"
# Fix: ensure the image was built with DOWNLOAD_SIGS=true, or inject signatures
```

**Remote mode — ClamAV service not reachable:**
```bash
kubectl get svc -n clamav
kubectl run test --rm -it --image=busybox -- nc -zv clamav.clamav.svc.cluster.local 3310
```

**Permission denied errors:**
```bash
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
