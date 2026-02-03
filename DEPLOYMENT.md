# ClamAV Operator Deployment Guide

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Installation](#installation)
3. [Configuration](#configuration)
4. [Usage Examples](#usage-examples)
5. [Troubleshooting](#troubleshooting)

## Prerequisites

### Kubernetes Cluster

- Kubernetes 1.24+
- kubectl configured
- Admin access to the cluster

### ClamAV Service

ClamAV must be deployed and accessible:

```bash
# Verify ClamAV is available
kubectl get svc -n clamav clamav

# Test connectivity
kubectl run -it --rm debug --image=busybox --restart=Never -- \
  nc -zv clamav.clamav.svc.cluster.local 3310
```

## Installation

### Option 1: Using Helm (Recommended)

```bash
# Create namespace
kubectl create namespace clamav-system

# Install using Helm
helm install clamav-operator ./helm/clamav-operator -n clamav-system

# Or with custom values
helm install clamav-operator ./helm/clamav-operator -n clamav-system -f custom-values.yaml
```

### Option 2: Using Kustomize

```bash
# Create namespace
kubectl create namespace clamav-system

# Install CRDs
kubectl apply -k config/crd

# Deploy the operator
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
# Verify operator is running
kubectl get pods -n clamav-system

# Check logs
kubectl logs -n clamav-system deployment/clamav-operator-controller-manager -f

# Verify CRDs
kubectl get crd | grep clamav
```

## Configuration

### Scanner ServiceAccount

Scanner jobs require a ServiceAccount with appropriate permissions:

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

Then update your Helm values or deployment to reference the secret.

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

# Monitor the scan
kubectl get nodescan scan-worker-01 -n clamav-system -w

# View details
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

# Monitor progress
kubectl get clusterscan full-cluster-scan -n clamav-system -w

# View node scans
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

# Verify schedule
kubectl get scanschedule daily-full-scan -n clamav-system

# View scan history
kubectl get clusterscan -n clamav-system -l clamav.io/schedule=daily-full-scan
```

## Troubleshooting

### Operator Not Starting

```bash
# Check events
kubectl get events -n clamav-system --sort-by='.lastTimestamp'

# Check logs
kubectl logs -n clamav-system deployment/clamav-operator-controller-manager

# Check permissions
kubectl auth can-i --list --as=system:serviceaccount:clamav-system:clamav-operator-controller-manager
```

### Scans Not Creating

```bash
# Verify node exists
kubectl get node <node-name>

# Check NodeScan events
kubectl describe nodescan <scan-name> -n clamav-system

# Check ServiceAccount permissions
kubectl auth can-i create jobs --as=system:serviceaccount:clamav-system:clamav-scanner -n clamav-system
```

### Jobs Failing

```bash
# View job logs
kubectl logs -n clamav-system -l clamav.io/nodescan=<scan-name>

# Check ClamAV connectivity
kubectl run -it --rm debug --image=busybox --restart=Never -- \
  nc -zv clamav.clamav.svc.cluster.local 3310

# Check scanner image
kubectl describe job -n clamav-system <job-name>
```

### Webhooks Not Working

```bash
# Check webhook configuration
kubectl get validatingwebhookconfigurations

# Check certificates
kubectl get secret -n clamav-system webhook-server-cert

# Temporarily disable webhooks (not recommended for production)
kubectl delete validatingwebhookconfigurations clamav-operator-validating-webhook-configuration
```

## Monitoring

### Prometheus Metrics

The operator exposes metrics on port 8080:

```bash
# Port-forward to operator
kubectl port-forward -n clamav-system deployment/clamav-operator-controller-manager 8080:8080

# Access metrics
curl http://localhost:8080/metrics | grep clamav
```

### Grafana Dashboard

Import the dashboard from `config/grafana/dashboard.json`

## Upgrade

### Upgrading the Operator

```bash
# Build new version
make docker-build IMG=ghcr.io/solucteam/clamav-operator:v1.1.0
make docker-push IMG=ghcr.io/solucteam/clamav-operator:v1.1.0

# Update deployment
make deploy IMG=ghcr.io/solucteam/clamav-operator:v1.1.0
```

### Upgrading CRDs

```bash
# Generate new manifests
make manifests

# Apply CRDs
kubectl apply -k config/crd
```

## Uninstallation

```bash
# Remove operator
make undeploy

# Remove CRDs (caution: deletes all resources!)
kubectl delete crd nodescans.clamav.io
kubectl delete crd clusterscans.clamav.io
kubectl delete crd scanpolicies.clamav.io
kubectl delete crd scanschedules.clamav.io
kubectl delete crd scancacheresources.clamav.io

# Remove namespace
kubectl delete namespace clamav-system
```

## Support

- Documentation: [GitHub Wiki](https://github.com/SolucTeam/clamav-operator/wiki)
- Issues: [GitHub Issues](https://github.com/SolucTeam/clamav-operator/issues)
- Discussions: [GitHub Discussions](https://github.com/SolucTeam/clamav-operator/discussions)
