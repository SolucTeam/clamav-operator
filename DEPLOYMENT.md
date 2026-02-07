# ClamAV Operator Deployment Guide

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Installation](#installation)
3. [Scanner Modes](#scanner-modes)
4. [Configuration](#configuration)
5. [Usage Examples](#usage-examples)
6. [Upgrade](#upgrade)
7. [Troubleshooting](#troubleshooting)
8. [Uninstallation](#uninstallation)

## Prerequisites

### Kubernetes Cluster

- Kubernetes 1.24+
- kubectl configured
- Admin access to the cluster

### ClamAV Service (Remote Mode Only)

If using `scanner.mode: remote`, ClamAV must be deployed and accessible:

```bash
kubectl get svc -n clamav clamav
kubectl run -it --rm debug --image=busybox --restart=Never -- \
  nc -zv clamav.clamav.svc.cluster.local 3310
```

In **standalone mode** (default), no external ClamAV service is required — the scanner image embeds its own `clamscan` binary and signatures.

## Installation

### Option 1: Using Helm (Recommended)

```bash
kubectl create namespace clamav-system

# Standalone mode (default — no ClamAV service needed)
helm install clamav-operator ./helm/clamav-operator -n clamav-system

# Or with custom values
helm install clamav-operator ./helm/clamav-operator -n clamav-system -f custom-values.yaml
```

#### Standalone Mode (Default)

```yaml
# custom-values.yaml — standalone with incremental scanning
scanner:
  mode: standalone
  freshclam:
    enabled: true
    schedule: "0 */6 * * *"
  incremental:
    enabled: true
    strategy: smart
  signatures:
    persistent: true
```

#### Air-Gap Mode (No Internet)

```yaml
# airgap-values.yaml
scanner:
  mode: standalone
  freshclam:
    enabled: false       # signatures are pre-loaded in the image
  image:
    repository: my-registry.internal/clamav-node-scanner
    tag: "1.1.0-airgap"
```

Build the air-gap image:

```bash
make docker-build-scanner-airgap
# or
docker build --build-arg DOWNLOAD_SIGS=false -t my-registry.internal/clamav-node-scanner:1.1.0-airgap scanner/
```

#### Remote Mode (Legacy)

```yaml
# remote-values.yaml — connects to existing ClamAV service
scanner:
  mode: remote
  clamav:
    host: clamav.clamav.svc.cluster.local
    port: 3310
```

### Option 2: Using Kustomize

```bash
kubectl create namespace clamav-system
kubectl apply -k config/crd
make deploy IMG=ghcr.io/solucteam/clamav-operator:latest
```

### Option 3: Manual Installation

#### Step 1: Create namespace

```bash
kubectl create namespace clamav-system
```

#### Step 2: Install CRDs

```bash
kubectl apply -f config/crd/bases/clamav.io_nodescans.yaml
kubectl apply -f config/crd/bases/clamav.io_clusterscans.yaml
kubectl apply -f config/crd/bases/clamav.io_scanpolicies.yaml
kubectl apply -f config/crd/bases/clamav.io_scanschedules.yaml
kubectl apply -f config/crd/bases/clamav.io_scancacheresources.yaml
```

#### Step 3: Create RBAC

```bash
kubectl apply -f config/rbac/service_account.yaml
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/rbac/role_binding.yaml
kubectl apply -f config/rbac/leader_election_role.yaml
kubectl apply -f config/rbac/leader_election_role_binding.yaml
```

#### Step 4: Deploy the Operator

```bash
kubectl apply -f config/manager/manager.yaml
```

### Verify Installation

```bash
kubectl get pods -n clamav-system
kubectl logs -n clamav-system deployment/clamav-operator-controller-manager -f
kubectl get crd | grep clamav
```

## Scanner Modes

### Standalone Mode

The standalone scanner embeds `clamscan` and ClamAV virus signatures directly in the scanner container image. Each scanner Job runs locally on the target node with zero network dependency.

**Advantages:** no SPOF, no network latency, works in air-gapped environments.

**Signature updates** are handled by an optional `freshclam` CronJob that runs on a configurable schedule (default: every 6 hours). When `scanner.signatures.persistent: true`, signatures are stored on a PVC shared across runs.

### Remote Mode

The remote scanner connects to a central `clamd` service. This is the legacy behavior, useful if you already manage a ClamAV deployment separately.

**Requirement:** a reachable clamd service (configured via `scanner.clamav.host` / `scanner.clamav.port`).

### Mode Comparison

| Feature | Standalone | Remote |
|---------|-----------|--------|
| Network dependency | None | Requires clamd service |
| SPOF | None | Central clamd |
| Air-gap support | Yes | No |
| Signature management | In-image or freshclam CronJob | Managed by clamd |
| Latency | Local I/O only | Network round-trip per file |
| Resource per node | Higher (clamscan in each pod) | Lower (thin client) |

## Configuration

### Scanner ServiceAccount

Scanner jobs require a ServiceAccount with appropriate permissions (created automatically by the Helm chart):

```bash
kubectl apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: clamav-scanner
  namespace: clamav-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: clamav-scanner-role
rules:
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["batch"]
    resources: ["jobs"]
    verbs: ["create", "get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: clamav-scanner-binding
subjects:
  - kind: ServiceAccount
    name: clamav-scanner
    namespace: clamav-system
roleRef:
  kind: ClusterRole
  name: clamav-scanner-role
  apiGroup: rbac.authorization.k8s.io
EOF
```

### Private Registry (Optional)

If your container images are in a private registry:

```bash
kubectl create secret docker-registry regcred \
  --docker-server=your-registry.example.com \
  --docker-username=YOUR_USERNAME \
  --docker-password=YOUR_PASSWORD \
  --namespace=clamav-system
```

Then set in your Helm values:

```yaml
scanner:
  imagePullSecrets:
    - name: regcred
```

## Usage Examples

### 1. Create a ScanPolicy

```bash
kubectl apply -f - <<EOF
apiVersion: clamav.io/v1alpha1
kind: ScanPolicy
metadata:
  name: default-policy
  namespace: clamav-system
spec:
  paths:
    - /var/lib
    - /opt
    - /usr/local

  excludePatterns:
    - "*.tmp"
    - "*.log"
    - "/var/lib/docker/overlay2/*"
    - "/var/lib/containerd/*"

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
EOF
```

### 2. Scan a Node

```bash
kubectl apply -f - <<EOF
apiVersion: clamav.io/v1alpha1
kind: NodeScan
metadata:
  name: scan-worker-01
  namespace: clamav-system
spec:
  nodeName: worker-01
  scanPolicy: default-policy
  priority: high
EOF

kubectl get nodescan scan-worker-01 -n clamav-system -w
kubectl describe nodescan scan-worker-01 -n clamav-system
```

### 3. Scan the Entire Cluster

```bash
kubectl apply -f - <<EOF
apiVersion: clamav.io/v1alpha1
kind: ClusterScan
metadata:
  name: full-cluster-scan
  namespace: clamav-system
spec:
  nodeSelector:
    matchLabels:
      node-role.kubernetes.io/worker: ""
  scanPolicy: default-policy
  concurrent: 3
EOF

kubectl get clusterscan full-cluster-scan -n clamav-system -w
kubectl get nodescan -n clamav-system -l clamav.io/clusterscan=full-cluster-scan
```

### 4. Schedule Automatic Scans

```bash
kubectl apply -f - <<EOF
apiVersion: clamav.io/v1alpha1
kind: ScanSchedule
metadata:
  name: daily-full-scan
  namespace: clamav-system
spec:
  schedule: "0 2 * * *"

  clusterScan:
    nodeSelector:
      matchLabels:
        node-role.kubernetes.io/worker: ""
    scanPolicy: default-policy
    concurrent: 2

  successfulScansHistoryLimit: 10
  failedScansHistoryLimit: 3
  concurrencyPolicy: Forbid
EOF

kubectl get scanschedule daily-full-scan -n clamav-system
```

### 5. Check Freshclam Signature Updates (Standalone Mode)

```bash
# View freshclam CronJob
kubectl get cronjob -n clamav-system -l app.kubernetes.io/component=freshclam

# View last signature update
kubectl get jobs -n clamav-system -l app.kubernetes.io/component=freshclam --sort-by=.status.startTime

# Check signature PVC (if persistent)
kubectl get pvc -n clamav-system clamav-signatures
```

## Upgrade

### Upgrading the Operator

```bash
# Using Helm
helm upgrade clamav-operator ./helm/clamav-operator \
  --namespace clamav-system \
  --values my-values.yaml

# Or just update image tags
helm upgrade clamav-operator ./helm/clamav-operator \
  --namespace clamav-system \
  --reuse-values \
  --set operator.image.tag=v1.1.0 \
  --set scanner.image.tag=1.1.0
```

### Migrating from Remote to Standalone

1. Update your values:

```yaml
scanner:
  mode: standalone       # was: remote
  freshclam:
    enabled: true
```

2. Upgrade the release:

```bash
helm upgrade clamav-operator ./helm/clamav-operator -n clamav-system -f values.yaml
```

3. The operator will now create scanner Jobs with `SCAN_MODE=standalone` and the local clamscan binary. The ClamAV service is no longer needed.

### Upgrading CRDs

```bash
make manifests
kubectl apply -k config/crd
```

## Troubleshooting

### Operator Not Starting

```bash
kubectl get events -n clamav-system --sort-by='.lastTimestamp'
kubectl logs -n clamav-system deployment/clamav-operator-controller-manager
kubectl auth can-i --list --as=system:serviceaccount:clamav-system:clamav-operator-controller-manager
```

### Scans Not Creating

```bash
kubectl get node <node-name>
kubectl describe nodescan <scan-name> -n clamav-system
kubectl auth can-i create jobs --as=system:serviceaccount:clamav-system:clamav-scanner -n clamav-system
```

### Scanner Job Failing (Standalone)

```bash
# Check scanner logs
kubectl logs -n clamav-system -l clamav.io/nodescan=<scan-name>

# Common errors:
#   "clamscan not found" → scanner image missing ClamAV binaries
#   "No ClamAV signatures found" → build image with DOWNLOAD_SIGS=true or inject signatures
#   "Error: Permission denied" → check securityContext and hostPID settings
```

### Scanner Job Failing (Remote)

```bash
kubectl logs -n clamav-system -l clamav.io/nodescan=<scan-name>

# Check ClamAV connectivity
kubectl run -it --rm debug --image=busybox --restart=Never -- \
  nc -zv clamav.clamav.svc.cluster.local 3310

kubectl describe job -n clamav-system <job-name>
```

### Freshclam CronJob Failing

```bash
# Check CronJob status
kubectl get cronjob -n clamav-system

# Check latest Job logs
kubectl logs -n clamav-system -l app.kubernetes.io/component=freshclam --tail=50

# Common issue: network not reachable (expected in air-gap → set freshclam.enabled=false)
```

### Webhooks Not Working

```bash
kubectl get validatingwebhookconfigurations
kubectl get secret -n clamav-system webhook-server-cert

# Temporarily disable
kubectl delete validatingwebhookconfigurations clamav-operator-validating-webhook-configuration
```

## Monitoring

### Prometheus Metrics

```bash
kubectl port-forward -n clamav-system deployment/clamav-operator-controller-manager 8080:8080
curl http://localhost:8080/metrics | grep clamav
```

### Grafana Dashboard

Import the dashboard from `config/grafana/dashboard.json`.

## Uninstallation

```bash
# Remove operator
helm uninstall clamav-operator --namespace clamav-system

# Or using make
make undeploy

# Remove CRDs (caution: deletes all scan resources!)
kubectl delete crd nodescans.clamav.io
kubectl delete crd clusterscans.clamav.io
kubectl delete crd scanpolicies.clamav.io
kubectl delete crd scanschedules.clamav.io
kubectl delete crd scancacheresources.clamav.io

# Remove PVC (if persistent signatures were used)
kubectl delete pvc -n clamav-system clamav-signatures

# Remove namespace
kubectl delete namespace clamav-system
```

## Support

- Documentation: [GitHub Wiki](https://github.com/SolucTeam/clamav-operator/wiki)
- Issues: [GitHub Issues](https://github.com/SolucTeam/clamav-operator/issues)
- Discussions: [GitHub Discussions](https://github.com/SolucTeam/clamav-operator/discussions)
