<div align="center">
  <img src="docs/assets/logo.svg" alt="clamav-operator" width="380"/>
</div>

# Deployment Guide

> Comprehensive guide for installing, configuring, and operating the ClamAV Operator in production Kubernetes environments.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Installation](#installation)
3. [Scanner Modes](#scanner-modes)
4. [Configuration Reference](#configuration-reference)
5. [Usage Examples](#usage-examples)
6. [RBAC & Access Control](#rbac--access-control)
7. [Monitoring](#monitoring)
8. [Upgrade](#upgrade)
9. [Troubleshooting](#troubleshooting)
10. [Uninstallation](#uninstallation)

---

## Prerequisites

| Requirement | Version | Notes |
|-------------|---------|-------|
| Kubernetes | 1.24+ | Cluster admin access required |
| kubectl | Any recent | Configured against target cluster |
| Helm | 3.x | For Helm installation only |
| kustomize | 5.x | For raw Kustomize installation |

**ClamAV service is NOT required** in the default standalone mode — the scanner image embeds its own `clamscan` binary and virus signatures.

---

## Installation

### Option 1 — Single File (Recommended for quick start)

Every release publishes a pre-built `install.yaml` generated from `kustomize build config/default`:

```bash
# Apply the latest release
kubectl apply -f https://github.com/SolucTeam/clamav-operator/releases/latest/download/install.yaml

# Or a specific version
kubectl apply -f https://github.com/SolucTeam/clamav-operator/releases/download/v1.2.0/install.yaml

# Verify
kubectl get pods -n clamav-operator-system
kubectl get crd | grep clamav
```

### Option 2 — Helm (Recommended for production)

```bash
kubectl create namespace clamav-operator-system

# Standalone mode (default — no ClamAV service needed)
helm install clamav-operator ./helm/clamav-operator \
  -n clamav-operator-system

# With custom values
helm install clamav-operator ./helm/clamav-operator \
  -n clamav-operator-system \
  -f my-values.yaml
```

<details>
<summary><strong>Standalone with incremental scanning (recommended)</strong></summary>

```yaml
# my-values.yaml
scanner:
  mode: standalone
  freshclam:
    enabled: true
    schedule: "0 */6 * * *"   # Update signatures every 6 hours
  incremental:
    enabled: true
    strategy: smart
    fullScanInterval: 10
  signatures:
    persistent: true           # Store signatures on a PVC across restarts

monitoring:
  serviceMonitor:
    enabled: true              # Requires Prometheus Operator
  prometheusRule:
    enabled: true
```

</details>

<details>
<summary><strong>Air-gap mode (no internet)</strong></summary>

```yaml
# airgap-values.yaml
scanner:
  mode: standalone
  freshclam:
    enabled: false             # Signatures pre-baked in image
  image:
    repository: my-registry.internal/clamav-node-scanner
    tag: "latest-airgap"
```

Build the air-gap image:

```bash
make docker-build-scanner-airgap
# or directly:
docker build --build-arg DOWNLOAD_SIGS=false \
  -t my-registry.internal/clamav-node-scanner:latest-airgap scanner/
docker push my-registry.internal/clamav-node-scanner:latest-airgap
```

</details>

<details>
<summary><strong>Remote / legacy mode</strong></summary>

```yaml
# remote-values.yaml
scanner:
  mode: remote
  clamav:
    host: clamav.clamav.svc.cluster.local
    port: 3310
```

Verify connectivity before installing:

```bash
kubectl run -it --rm debug --image=busybox --restart=Never -- \
  nc -zv clamav.clamav.svc.cluster.local 3310
```

</details>

### Option 3 — Kustomize

```bash
# Install CRDs only
kubectl apply -k config/crd

# Full deployment (CRDs + RBAC + manager + webhooks)
kustomize build config/default | kubectl apply -f -

# Or via make
make deploy IMG=ghcr.io/solucteam/clamav-operator:latest
```

### Option 4 — Build from Source

```bash
git clone https://github.com/SolucTeam/clamav-operator.git
cd clamav-operator

# Build all images (operator + scanner)
make docker-build-all IMG=my-registry/clamav-operator:dev

# Build air-gap scanner variant
make docker-build-scanner-airgap

# Generate install.yaml locally
make build-installer

# Deploy
make deploy IMG=my-registry/clamav-operator:dev
```

### Verify Installation

```bash
# All pods running
kubectl get pods -n clamav-operator-system

# CRDs registered
kubectl get crd | grep clamav.io

# Operator healthy
kubectl logs -n clamav-operator-system deployment/clamav-operator -f
```

---

## Scanner Modes

### Standalone (Default)

The standalone scanner embeds `clamscan` and ClamAV virus signatures directly in the scanner container image. Each scanner Job runs **locally on the target node** with zero network dependency.

- No single point of failure
- Works in air-gapped environments
- Signature updates via optional `freshclam` CronJob (default: every 6 hours)
- When `scanner.signatures.persistent: true`, signatures are stored on a PVC shared across runs

### Remote / Legacy

The remote scanner connects to a central `clamd` service. Use this only if you already manage a ClamAV deployment separately.

**Requirement:** a reachable `clamd` service at the configured host:port.

### Mode Comparison

| Feature | Standalone | Remote |
|---------|-----------|--------|
| Network dependency | None | Requires `clamd` service |
| Single point of failure | None | Central `clamd` |
| Air-gap support | ✅ Yes | ❌ No |
| Signature management | In-image or freshclam CronJob | Managed by `clamd` |
| Latency | Local I/O only | Network round-trip |
| Resource per node | Higher (clamscan in each pod) | Lower (thin client) |

---

## Configuration Reference

### Operator Flags

Set via `operator.args` in `values.yaml` or `config/manager/manager.yaml`:

| Flag | Default | Description |
|------|---------|-------------|
| `--scanner-image` | `ghcr.io/solucteam/clamav-node-scanner:...` | Scanner container image for Jobs |
| `--clamav-host` | `clamav.clamav.svc.cluster.local` | ClamAV service hostname (remote mode) |
| `--clamav-port` | `3310` | ClamAV service port (remote mode) |
| `--skip-startup-checks` | `false` | Skip connectivity validation at startup |
| `--leader-elect` | `false` | Enable leader election (required for HA / multiple replicas) |
| `--metrics-bind-address` | `:8080` | Prometheus metrics endpoint |
| `--health-probe-bind-address` | `:8081` | Liveness and readiness probe endpoint |

### Job Lifecycle Constants

| Constant | Value | Description |
|----------|-------|-------------|
| `ActiveDeadlineSeconds` | 7200 (2h) | Jobs killed after this wall-clock time — no zombie pods |
| `TTLSecondsAfterFinished` (success) | 3600 (1h) | Succeeded jobs cleaned up after results are in Status |
| `TTLSecondsAfterFinished` (failure) | 86400 (24h) | Failed jobs kept for post-mortem analysis |
| `DefaultMaxConcurrent` | 3 | Max simultaneous NodeScan jobs per ClusterScan |

### Resource Profiles

Configured via `ScanPolicy.spec.resources.profile`:

| Profile | CPU Req | Mem Req | Ephemeral Storage Req | CPU Lim | Mem Lim | Ephemeral Lim |
|---------|---------|---------|----------------------|---------|---------|---------------|
| `high`  | 500m    | 512Mi   | 512Mi                | 2000m   | 4Gi     | 4Gi           |
| `default` | 100m  | 256Mi   | 256Mi                | 1000m   | 2Gi     | 2Gi           |
| `low`   | 50m     | 128Mi   | 128Mi                | 500m    | 1Gi     | 1Gi           |

### ScanPolicy Reference

```yaml
apiVersion: clamav.io/v1alpha1
kind: ScanPolicy
metadata:
  name: production-policy
  namespace: clamav-operator-system
spec:
  # Paths to scan on each node
  scanPaths:
    - /etc
    - /usr
    - /var
    - /opt

  # Paths to skip
  excludePaths:
    - /proc
    - /sys
    - /dev
    - /var/lib/docker/overlay2
    - /var/lib/containerd

  # Performance
  maxConcurrent: 3           # Max concurrent file scans within a single Job
  fileTimeout: 300000        # Per-file timeout in ms (5 minutes)
  maxFileSize: 524288000     # Skip files larger than 500 MB

  # Resource profile
  resources:
    profile: default         # high | default | low

  # Notifications (all channels independent — fire-and-forget)
  notifications:
    slack:
      enabled: true
      channel: "#security-alerts"
      webhookSecretRef:
        name: slack-webhook
        key: url
      onlyOnInfection: true

    email:
      enabled: true
      smtpServer: "smtp.example.com:465"
      from: "clamav-operator@example.com"
      recipients:
        - "security@example.com"
        - "ops@example.com"
      smtpAuthSecretRef:
        name: smtp-credentials
      onlyOnInfection: true

    webhook:
      url: "https://hooks.example.com/security"
      headers:
        X-Source: "clamav-operator"
      secretRef:
        name: webhook-auth        # Secret keys become request headers
      onlyOnInfection: false      # Always notify (scanned + infected)
```

### Notification Secrets

```bash
# Slack webhook URL
kubectl create secret generic slack-webhook \
  --from-literal=url='https://hooks.slack.com/services/T.../B.../...' \
  -n clamav-operator-system

# SMTP credentials
kubectl create secret generic smtp-credentials \
  --from-literal=username='clamav@example.com' \
  --from-literal=password='my-smtp-password' \
  -n clamav-operator-system
```

### Private Registry

```bash
kubectl create secret docker-registry regcred \
  --docker-server=my-registry.internal \
  --docker-username=MY_USER \
  --docker-password=MY_PASS \
  -n clamav-operator-system
```

```yaml
# values.yaml
scanner:
  imagePullSecrets:
    - name: regcred
```

---

## Usage Examples

### 1. Create a ScanPolicy

```bash
kubectl apply -f config/samples/scanpolicy_v1alpha1.yaml
# or:
kubectl apply -f - <<'EOF'
apiVersion: clamav.io/v1alpha1
kind: ScanPolicy
metadata:
  name: default-scan-policy
  namespace: clamav-operator-system
spec:
  scanPaths: [/etc, /usr, /var]
  excludePaths: [/proc, /sys, /dev]
  maxFileSize: 524288000
  maxConcurrent: 3
  resources:
    profile: default
EOF
```

### 2. Scan a Single Node

```bash
kubectl apply -f - <<'EOF'
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
EOF

# Watch progress
kubectl get nodescan scan-worker-01 -n clamav-operator-system -w

# View results (even after pod is deleted)
kubectl get nodescan scan-worker-01 -n clamav-operator-system \
  -o jsonpath='{.status}' | jq .
```

### 3. Scan the Entire Cluster

```bash
kubectl apply -f - <<'EOF'
apiVersion: clamav.io/v1alpha1
kind: ClusterScan
metadata:
  name: full-cluster-scan
  namespace: clamav-operator-system
spec:
  nodeSelector:
    matchLabels:
      kubernetes.io/os: linux
  scanPolicyRef:
    name: default-scan-policy
    namespace: clamav-operator-system
EOF

# Watch individual NodeScan jobs
kubectl get nodescan -n clamav-operator-system \
  -l clamav.io/clusterscan=full-cluster-scan -w
```

### 4. Schedule Automatic Scans

```bash
kubectl apply -f - <<'EOF'
apiVersion: clamav.io/v1alpha1
kind: ScanSchedule
metadata:
  name: daily-scan
  namespace: clamav-operator-system
spec:
  schedule: "0 2 * * *"           # 02:00 UTC daily
  scanPolicyRef:
    name: default-scan-policy
    namespace: clamav-operator-system
  nodeSelector:
    matchLabels:
      kubernetes.io/os: linux
  historyLimit:
    succeeded: 3
    failed: 5
EOF

kubectl get scanschedule daily-scan -n clamav-operator-system
```

### 5. Enable Incremental Scanning

```yaml
# values.yaml
scanner:
  incremental:
    enabled: true
    strategy: smart           # incremental by default, full every 10 runs
    fullScanInterval: 10
    maxFileAgeHours: 24
    skipUnchangedFiles: true
```

Scan state is stored in `ScanCacheResource` objects backed by chunked ConfigMaps (≤ 900 KB per chunk — safely within etcd's 1.5 MB object limit).

### 6. Check Failure Details

```bash
# Failure reason and exit code are stored in Status — no running pod needed
kubectl get nodescan scan-worker-01 -n clamav-operator-system \
  -o jsonpath='{.status.failureReason}'

kubectl get nodescan scan-worker-01 -n clamav-operator-system \
  -o jsonpath='{.status.exitCode}'

kubectl get nodescan scan-worker-01 -n clamav-operator-system \
  -o jsonpath='{.status.duration}'
```

---

## RBAC & Access Control

### Operator Permissions

The `manager-role` (ClusterRole) grants the operator all permissions it needs. This is managed automatically by Helm or `kustomize build config/rbac`.

### End-User Roles

Pre-built `ClusterRole` objects are provided for end users in `config/rbac/`:

```bash
# Grant read-only access to NodeScan objects
kubectl create clusterrolebinding security-viewers \
  --clusterrole=nodescan-viewer-role \
  --group=security-team

# Grant full access to ScanPolicy objects to the SRE team
kubectl create clusterrolebinding sre-policy-editors \
  --clusterrole=scanpolicy-editor-role \
  --group=sre-team
```

Available roles: `nodescan-{editor,viewer}-role`, `clusterscan-{editor,viewer}-role`, `scanpolicy-{editor,viewer}-role`, `scanschedule-{editor,viewer}-role`.

### Scanner ServiceAccount

Scanner Jobs run as the `clamav-scanner` ServiceAccount (created by Helm). It requires host filesystem access via `hostPath` mounts and `hostPID: true`. Review the scanner pod security context in your security baseline before deploying.

---

## Monitoring

### Prometheus Metrics

```bash
# Port-forward metrics endpoint
kubectl port-forward -n clamav-operator-system deployment/clamav-operator 8080:8080

# Query metrics
curl -s http://localhost:8080/metrics | grep clamav_
```

Key metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `clamav_nodescan_running` | Gauge | Currently running NodeScan jobs |
| `clamav_files_scanned_total` | Counter | Total files scanned (all time) |
| `clamav_files_infected_total` | Counter | Total infected files detected |
| `clamav_scan_duration_seconds` | Histogram | Scan duration per NodeScan |
| `clamav_scan_errors_total` | Counter | Scanner errors (timeout, permission…) |

### Grafana Dashboard

Import `config/grafana/dashboard.json` into your Grafana instance. The dashboard shows infection rate, scan throughput, job duration heatmap, and per-node status.

### Alerts (PrometheusRule)

```yaml
# Enable via values.yaml
monitoring:
  prometheusRule:
    enabled: true
# Pre-configured alerts: InfectionDetected (critical), ScanJobStuck (warning)
```

### Logs

Operator and scanner logs are structured JSON:

```bash
# Operator logs
kubectl logs -n clamav-operator-system deployment/clamav-operator -f

# Scanner Job logs (still running)
kubectl logs -n clamav-operator-system -l clamav.io/nodescan=scan-worker-01 -f

# Scanner Job logs (completed — check TTL)
kubectl logs -n clamav-operator-system job/<job-name>
```

---

## Upgrade

### Upgrading with Helm

```bash
helm upgrade clamav-operator ./helm/clamav-operator \
  --namespace clamav-operator-system \
  --values my-values.yaml

# Update only image tags
helm upgrade clamav-operator ./helm/clamav-operator \
  --namespace clamav-operator-system \
  --reuse-values \
  --set operator.image.tag=v1.2.0 \
  --set scanner.image.tag=1.2.0
```

### Upgrading CRDs

CRDs must be upgraded separately (Helm does not update CRDs on `helm upgrade` by design):

```bash
# Re-apply CRDs
kubectl apply -k config/crd
# or
make manifests && kubectl apply -k config/crd
```

### Upgrading with install.yaml

```bash
kubectl apply -f https://github.com/SolucTeam/clamav-operator/releases/download/v1.2.0/install.yaml
```

### Migrating from Remote to Standalone

```yaml
# values.yaml — change only this block
scanner:
  mode: standalone        # was: remote
  freshclam:
    enabled: true
```

```bash
helm upgrade clamav-operator ./helm/clamav-operator \
  -n clamav-operator-system -f values.yaml
```

The operator will now create scanner Jobs with `SCAN_MODE=standalone`. The ClamAV service is no longer needed.

---

## Troubleshooting

### Operator not starting

```bash
kubectl get events -n clamav-operator-system --sort-by='.lastTimestamp'
kubectl logs -n clamav-operator-system deployment/clamav-operator --previous
kubectl auth can-i --list \
  --as=system:serviceaccount:clamav-operator-system:clamav-operator
```

### NodeScan stuck in `Pending`

```bash
# Check if the target node exists
kubectl get node <node-name>

# Check operator events
kubectl describe nodescan <scan-name> -n clamav-operator-system

# Check if Job was created
kubectl get jobs -n clamav-operator-system \
  -l clamav.io/nodescan=<scan-name>
```

### Scanner Job failing (standalone)

```bash
kubectl logs -n clamav-operator-system -l clamav.io/nodescan=<scan-name>

# Common errors:
# "clamscan not found"        → Scanner image missing ClamAV binary
# "No ClamAV signatures"      → Build with DOWNLOAD_SIGS=true
# "Permission denied"         → Check hostPID / hostPath / securityContext
# "Error: connection refused" → Remote mode but no clamd service
```

### Read failure reason after pod is deleted

```bash
# FailureReason and ExitCode are captured in Status before pod GC
kubectl get nodescan <scan-name> -n clamav-operator-system \
  -o jsonpath='{.status.failureReason}  exit={.status.exitCode}'
```

### Webhooks not working

```bash
kubectl get validatingwebhookconfigurations | grep clamav
kubectl get secret -n clamav-operator-system webhook-server-cert

# Temporarily disable (do not leave disabled in production)
kubectl delete validatingwebhookconfigurations \
  clamav-operator-validating-webhook-configuration
```

### Freshclam CronJob failing

```bash
kubectl get cronjob -n clamav-operator-system
kubectl logs -n clamav-operator-system \
  -l app.kubernetes.io/component=freshclam --tail=50
# If air-gap environment → set scanner.freshclam.enabled: false
```

### etcd object too large

If you see `Request entity too large` or `etcdserver: request is too large`, a `ScanCacheResource` may pre-date the chunked ConfigMap migration. Delete the offending object:

```bash
kubectl delete scancacheresource -n clamav-operator-system <node-name>
# The operator will recreate it using chunked ConfigMaps (≤ 900 KB/chunk)
```

---

## Uninstallation

```bash
# Option A — Helm
helm uninstall clamav-operator --namespace clamav-operator-system

# Option B — make
make undeploy

# Option C — install.yaml
kubectl delete -f https://github.com/SolucTeam/clamav-operator/releases/latest/download/install.yaml

# Remove CRDs (⚠️ deletes ALL scan resources in the cluster!)
kubectl delete -k config/crd
# or individually:
kubectl delete crd \
  nodescans.clamav.io \
  clusterscans.clamav.io \
  scanpolicies.clamav.io \
  scanschedules.clamav.io \
  scancacheresources.clamav.io

# Remove signatures PVC (if persistent mode was used)
kubectl delete pvc -n clamav-operator-system clamav-signatures

# Remove namespace
kubectl delete namespace clamav-operator-system
```

---

## Support

- Documentation: [GitHub Wiki](https://github.com/SolucTeam/clamav-operator/wiki)
- Issues: [GitHub Issues](https://github.com/SolucTeam/clamav-operator/issues)
- Discussions: [GitHub Discussions](https://github.com/SolucTeam/clamav-operator/discussions)
