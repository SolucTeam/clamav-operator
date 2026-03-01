<div align="center">
  <img src="docs/assets/logo.svg" alt="clamav-operator" width="480"/>
  <br/><br/>
  <a href="https://github.com/SolucTeam/clamav-operator/actions/workflows/docker-build.yml">
    <img src="https://github.com/SolucTeam/clamav-operator/actions/workflows/docker-build.yml/badge.svg" alt="Build"/>
  </a>
  <a href="https://github.com/SolucTeam/clamav-operator/releases">
    <img src="https://img.shields.io/github/v/release/SolucTeam/clamav-operator?color=blue" alt="Release"/>
  </a>
  <a href="https://github.com/SolucTeam/clamav-operator/blob/main/LICENSE">
    <img src="https://img.shields.io/badge/license-Apache%202.0-green" alt="License"/>
  </a>
  <img src="https://img.shields.io/badge/Go-1.24-00ADD8?logo=go" alt="Go"/>
  <img src="https://img.shields.io/badge/Kubernetes-1.24+-326CE5?logo=kubernetes" alt="Kubernetes"/>
  <img src="https://img.shields.io/badge/arch-amd64%20%7C%20arm64-lightgrey" alt="Arch"/>
</div>

<br/>

> **Kubernetes-native antivirus operator** — automated ClamAV scanning across every node in your cluster, with zero external dependencies.

---

## Features

| | |
|---|---|
| 🛡️ **Standalone mode** | `clamscan` + signatures embedded in the scanner image — no central ClamAV service needed |
| 🔄 **Incremental scanning** | Smart strategy: only scan new/modified files, alternating with periodic full scans |
| ✈️ **Air-gap support** | Signatures baked into the image at build time — no internet at runtime |
| 📅 **Cron scheduling** | Automatic cluster-wide scans via `ScanSchedule` |
| 📊 **Prometheus metrics** | `clamav_files_infected_total`, `clamav_scan_duration_seconds`, and more |
| 🔔 **Notifications** | Slack, Email (SMTP/TLS), generic Webhook — fire-and-forget, non-blocking |
| 🏗️ **Multi-arch images** | `linux/amd64` and `linux/arm64` — built and signed on every release |
| 🔒 **Supply-chain security** | SBOM (SPDX), cosign image signing, Trivy vulnerability scanning |
| 📦 **GitOps-ready** | `observedGeneration` in Status — ArgoCD and Flux know when reconciliation is done |

---

## Architecture

```
┌────────────────────────────────────────────────────────────────────┐
│                        clamav-operator                             │
│                                                                    │
│  cmd/manager/main.go                                               │
│       │                                                            │
│       ├── internal/controller/                                     │
│       │       ├── nodescan_controller.go    ← Reconcile routing    │
│       │       ├── clusterscan_controller.go                        │
│       │       ├── scanschedule_controller.go                       │
│       │       ├── incremental_scan_controller.go                   │
│       │       ├── defaults.go   metrics.go   common.go             │
│       │       └── startup_checks.go                                │
│       │                                                            │
│       ├── internal/notification/                                   │
│       │       └── notifier.go  ← Slack · Email · Webhook           │
│       │                          (fire-and-forget goroutine)        │
│       │                                                            │
│       └── api/v1alpha1/                                            │
│               ├── nodescan_types.go                                │
│               ├── clusterscan_types.go                             │
│               ├── scanpolicy_types.go                              │
│               ├── scanschedule_types.go                            │
│               └── incremental_scan_types.go                        │
└────────────────────────────┬───────────────────────────────────────┘
                             │  creates/watches
                             ▼
               ┌─────────────────────────┐
               │    Kubernetes API       │
               │  CRDs · Jobs · Nodes    │
               └────────────┬────────────┘
                            │  Job per node (maxConcurrent: 3)
           ┌────────────────┼────────────────┐
           ▼                ▼                ▼
    ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
    │ Scanner Job │  │ Scanner Job │  │ Scanner Job │
    │   Node 1    │  │   Node 2    │  │   Node N    │
    │  clamscan   │  │  clamscan   │  │  clamscan   │
    │  (local)    │  │  (local)    │  │  (local)    │
    └─────────────┘  └─────────────┘  └─────────────┘
         Standalone: scan runs locally, zero network dependency
```

### Custom Resources

| CRD | Purpose |
|-----|---------|
| `NodeScan` | Trigger a scan on a specific node |
| `ClusterScan` | Trigger scans on all matching nodes simultaneously |
| `ScanPolicy` | Reusable scan configuration (paths, resources, notifications) |
| `ScanSchedule` | Cron-based recurring cluster scans |
| `ScanCacheResource` | Incremental scan cache stored in chunked ConfigMaps (≤ 900 KB/chunk) |

---

## Quick Start

### Install with Helm

```bash
helm install clamav-operator ./helm/clamav-operator \
  -n clamav-operator-system --create-namespace
```

### Install with kubectl (single file)

```bash
kubectl apply -f https://github.com/SolucTeam/clamav-operator/releases/latest/download/install.yaml
```

### Scan a Node

```yaml
apiVersion: clamav.io/v1alpha1
kind: NodeScan
metadata:
  name: scan-worker-01
  namespace: clamav-operator-system
spec:
  nodeName: worker-01
  scanPolicyRef:
    name: default-scan-policy
    namespace: clamav-operator-system
```

```bash
kubectl apply -f nodescan.yaml
kubectl get nodescan scan-worker-01 -w
# NAME            NODE        PHASE       INFECTED   SCANNED   AGE
# scan-worker-01  worker-01   Completed   0          142847    4m12s
```

### Schedule Nightly Scans

```yaml
apiVersion: clamav.io/v1alpha1
kind: ScanSchedule
metadata:
  name: nightly
  namespace: clamav-operator-system
spec:
  schedule: "0 2 * * *"      # Every day at 02:00 UTC
  scanPolicyRef:
    name: default-scan-policy
    namespace: clamav-operator-system
  nodeSelector:
    matchLabels:
      kubernetes.io/os: linux
```

---

## Scanner Modes

### Standalone (Default)

Each scanner Job carries its own `clamscan` binary and virus signatures. Scans run locally on each node with **zero network dependency**. This eliminates the central ClamAV service as a single point of failure.

```yaml
# helm/values.yaml (default)
scanner:
  mode: standalone
  freshclam:
    enabled: true
    schedule: "0 */6 * * *"   # Update signatures every 6 hours
```

### Air-Gap (No Internet)

Signatures are baked into the scanner image at build time. Nothing is downloaded at runtime.

```bash
# Build the air-gap image (DOWNLOAD_SIGS=false)
make docker-build-scanner-airgap
```

```yaml
scanner:
  mode: standalone
  freshclam:
    enabled: false             # Signatures pre-loaded — no internet needed
  image:
    repository: my-registry.internal/clamav-node-scanner
    tag: "latest-airgap"
```

### Remote / Legacy

Connects to a central `clamd` service. Use this if you already manage a ClamAV deployment separately.

```yaml
scanner:
  mode: remote
  clamav:
    host: clamav.clamav.svc.cluster.local
    port: 3310
```

---

## Incremental Scanning

| Strategy | Behavior |
|----------|----------|
| `full` | Scan every file on every run |
| `incremental` | Only scan new or modified files since the last run |
| `smart` | Alternate: N incremental runs, then one forced full scan |

File metadata is stored in chunked ConfigMaps (never in the CRD itself — etcd's 1.5 MB limit is respected).

```yaml
scanner:
  incremental:
    enabled: true
    strategy: smart
    fullScanInterval: 10       # Full scan every 10 incremental runs
    maxFileAgeHours: 24
    skipUnchangedFiles: true
```

---

## Notifications

Configured in `ScanPolicy.spec.notifications`. Each channel runs in a **fire-and-forget goroutine** — notification failures never block or fail a scan.

```yaml
spec:
  notifications:
    slack:
      enabled: true
      channel: "#security-alerts"
      webhookSecretRef:
        name: slack-webhook
        key: url
      onlyOnInfection: true    # Only alert when malware is found

    email:
      enabled: true
      smtpServer: "smtp.example.com:465"
      from: "clamav@example.com"
      recipients: ["security@example.com"]
      smtpAuthSecretRef:
        name: smtp-credentials

    webhook:
      url: "https://hooks.example.com/clamav"
      headers:
        Authorization: "Bearer my-token"
      onlyOnInfection: false
```

---

## Job Lifecycle & Reliability

- **`ActiveDeadlineSeconds: 7200`** — Jobs are automatically killed after 2 hours. No zombie pods.
- **Differentiated TTL** — Succeeded jobs are cleaned up after **1 hour** (results already persisted in Status). Failed jobs are kept **24 hours** for post-mortem.
- **`FailureReason` + `ExitCode` in Status** — `containerStatus.state.terminated` is captured before the pod is GC'd. Failure context survives pod deletion.
- **`observedGeneration` in Status** — GitOps tools (ArgoCD, Flux) know exactly when reconciliation is complete.
- **Patch instead of Update** — All finalizer and annotation mutations use `client.MergeFrom` patch to avoid 409 Conflict races.

---

## Project Structure

```
clamav-operator/
├── api/v1alpha1/                    # CRD type definitions + webhooks
│   ├── nodescan_types.go
│   ├── clusterscan_types.go
│   ├── scanpolicy_types.go
│   ├── scanschedule_types.go
│   ├── incremental_scan_types.go   # snake_case (Go convention)
│   └── zz_generated.deepcopy.go
│
├── internal/                        # Private packages (not importable externally)
│   ├── controller/                  # Reconciliation loops
│   │   ├── nodescan_controller.go
│   │   ├── clusterscan_controller.go
│   │   ├── scanschedule_controller.go
│   │   ├── incremental_scan_controller.go
│   │   ├── common.go               # requeueWithJitter, shared helpers
│   │   ├── defaults.go             # Resource profiles, constants
│   │   ├── metrics.go
│   │   └── startup_checks.go
│   └── notification/               # Decoupled alerting (testable without cluster)
│       └── notifier.go             # Slack · Email · Webhook
│
├── cmd/manager/main.go              # Operator entry point
│
├── config/
│   ├── crd/bases/                   # Generated CRD manifests
│   ├── rbac/                        # manager-role + editor/viewer roles per CRD
│   ├── manager/                     # Deployment, ServiceAccount, Service
│   ├── default/                     # Kustomize root (CRDs + RBAC + manager + webhooks)
│   ├── samples/                     # Example CRs (one per CRD)
│   └── webhook/
│
├── dist/
│   └── install.yaml                 # Generated by CI: kustomize build config/default
│
├── scanner/                         # Standalone scanner (Node.js + ClamAV)
│   ├── Dockerfile
│   └── src/
│       ├── index.js                 # Entry point
│       ├── scanner.js               # Recursive directory scan
│       ├── incremental.js           # Smart incremental cache
│       ├── report.js                # JSON + text report
│       └── __tests__/
│
├── helm/clamav-operator/            # Helm chart
├── build/Dockerfile                 # Operator image (Go)
├── .github/workflows/
│   ├── docker-build.yml             # Multi-arch build + Trivy scan
│   └── release.yml                  # Semantic versioning + SBOM + cosign + install.yaml
├── .golangci.yml                    # Linting configuration
└── Makefile
```

---

## Configuration

### Operator Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--scanner-image` | `ghcr.io/solucteam/clamav-node-scanner:...` | Scanner container image |
| `--clamav-host` | `clamav.clamav.svc.cluster.local` | ClamAV host (remote mode) |
| `--clamav-port` | `3310` | ClamAV port (remote mode) |
| `--skip-startup-checks` | `false` | Disable startup validation |
| `--leader-elect` | `false` | Enable leader election (HA) |
| `--metrics-bind-address` | `:8080` | Prometheus metrics endpoint |

### Priority-Based Resources

| Priority | CPU Req | Mem Req | Ephemeral Storage | CPU Lim | Mem Lim |
|----------|---------|---------|-------------------|---------|---------|
| `high`   | 500m    | 512Mi   | 512Mi             | 2000m   | 4Gi     |
| `default`| 100m    | 256Mi   | 256Mi             | 1000m   | 2Gi     |
| `low`    | 50m     | 128Mi   | 128Mi             | 500m    | 1Gi     |

### RBAC Roles

Beyond the `manager-role` (operator), the following `ClusterRole` objects are provided for end users:

| Role | Verbs | Use case |
|------|-------|----------|
| `nodescan-editor-role` | get, list, watch, create, update, patch, delete | DevOps managing scans |
| `nodescan-viewer-role` | get, list, watch | Read-only monitoring / audit |
| `clusterscan-editor-role` | … | |
| `clusterscan-viewer-role` | … | |
| `scanpolicy-editor-role` | … | |
| `scanpolicy-viewer-role` | … | |
| `scanschedule-editor-role` | … | |
| `scanschedule-viewer-role` | … | |

---

## Monitoring

```bash
# Live metrics
kubectl port-forward -n clamav-operator-system deployment/clamav-operator 8080:8080
curl -s http://localhost:8080/metrics | grep clamav_
```

Key metrics:

```promql
clamav_nodescan_running          # Active scans
clamav_files_infected_total      # Total infected files found (counter)
clamav_files_scanned_total       # Total files scanned (counter)
clamav_scan_duration_seconds     # Scan duration histogram
```

---

## Development

```bash
# Clone & setup
git clone https://github.com/SolucTeam/clamav-operator.git
cd clamav-operator
go mod download

# Generate CRD manifests + DeepCopy
make manifests generate

# Lint
make lint                        # uses .golangci.yml

# Unit tests (no cluster needed)
make test

# Scanner tests (Node.js)
cd scanner && node --test src/__tests__/

# e2e tests (requires Kind)
make test-e2e

# Build all images locally
make docker-build-all

# Build air-gap scanner
make docker-build-scanner-airgap

# Generate dist/install.yaml locally
make build-installer
```

### CI Workflows

| Workflow | Trigger | What it does |
|----------|---------|--------------|
| `docker-build.yml` | Push to `main`, PRs, version tags | go vet + test, multi-arch build (amd64/arm64), Trivy CVE scan |
| `release.yml` | Push to `main` with `feat:`/`fix:` commits | Semantic versioning, generate `dist/install.yaml`, SBOM, cosign sign, GitHub Release |
| `commit-validation.yml` | Every PR | Conventional Commits format check |
| `dependabot-automerge.yml` | Dependabot PRs | Auto-merge patch/minor dependency updates |

---

## Troubleshooting

**Scan job not starting:**
```bash
kubectl logs -n clamav-operator-system deployment/clamav-operator -f
kubectl get events -n clamav-operator-system --sort-by='.lastTimestamp'
```

**Check failure reason without a running pod:**
```bash
# Failure context is stored in NodeScan Status — survives pod GC
kubectl get nodescan scan-worker-01 -o jsonpath='{.status.failureReason}'
kubectl get nodescan scan-worker-01 -o jsonpath='{.status.exitCode}'
```

**No ClamAV signatures found (standalone):**
```bash
# Ensure the scanner image was built with DOWNLOAD_SIGS=true
kubectl logs -n clamav-operator-system -l clamav.io/nodescan=<scan-name>
```

**Remote mode — clamd unreachable:**
```bash
kubectl run test --rm -it --image=busybox -- nc -zv clamav.clamav.svc.cluster.local 3310
```

**Permission denied:**
```bash
kubectl auth can-i create jobs \
  --as=system:serviceaccount:clamav-operator-system:clamav-operator \
  -n clamav-operator-system
```

---

## License

Copyright 2025 The ClamAV Operator Authors.
Licensed under the [Apache License, Version 2.0](LICENSE).

---

## Contributing

1. Fork the repository
2. Create your feature branch: `git checkout -b feat/my-feature`
3. Commit using [Conventional Commits](https://www.conventionalcommits.org): `feat: add X`, `fix: resolve Y`
4. Push and open a Pull Request

Issues and discussions: [github.com/SolucTeam/clamav-operator](https://github.com/SolucTeam/clamav-operator/issues)
