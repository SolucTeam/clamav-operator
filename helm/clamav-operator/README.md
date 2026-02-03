# ClamAV Operator Helm Chart

Helm chart for deploying the ClamAV Operator in Kubernetes.

## Prerequisites

- Kubernetes 1.24+
- Helm 3.0+
- ClamAV deployed and accessible in the cluster

## Installation

### Install from local sources

```bash
# Create namespace and install
helm install clamav-operator ./helm/clamav-operator \
  --namespace clamav-system \
  --create-namespace

# Install with custom values
helm install clamav-operator ./helm/clamav-operator \
  --namespace clamav-system \
  --create-namespace \
  --values my-values.yaml

# Install with inline parameters
helm install clamav-operator ./helm/clamav-operator \
  --namespace clamav-system \
  --create-namespace \
  --set operator.image.tag=v1.0.0 \
  --set scanner.clamav.host=clamav.clamav.svc.cluster.local
```

## Configuration

### Main Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `operator.replicaCount` | Number of operator replicas | `1` |
| `operator.image.repository` | Operator image repository | `ghcr.io/solucteam/clamav-operator` |
| `operator.image.tag` | Operator image tag | `""` (chart version) |
| `operator.resources.limits.cpu` | Operator CPU limit | `500m` |
| `operator.resources.limits.memory` | Operator memory limit | `256Mi` |
| `scanner.image.repository` | Scanner image repository | `ghcr.io/solucteam/clamav-node-scanner` |
| `scanner.image.tag` | Scanner image tag | `1.0.3` |
| `scanner.clamav.host` | ClamAV service host | `clamav.clamav.svc.cluster.local` |
| `scanner.clamav.port` | ClamAV service port | `3310` |
| `crds.install` | Install CRDs | `true` |
| `crds.keep` | Keep CRDs on uninstall | `true` |
| `rbac.create` | Create RBAC resources | `true` |
| `monitoring.serviceMonitor.enabled` | Enable Prometheus ServiceMonitor | `false` |
| `monitoring.prometheusRule.enabled` | Enable PrometheusRule | `false` |
| `defaultScanPolicy.enabled` | Create default ScanPolicy | `true` |

### Example Custom Values File

```yaml
# my-values.yaml

operator:
  replicaCount: 2
  image:
    tag: v1.0.0
  resources:
    limits:
      cpu: 1000m
      memory: 512Mi
    requests:
      cpu: 200m
      memory: 256Mi

scanner:
  clamav:
    host: clamav.clamav.svc.cluster.local
    port: 3310
  resources:
    limits:
      cpu: 4000m
      memory: 2Gi

monitoring:
  serviceMonitor:
    enabled: true
    interval: 60s
  prometheusRule:
    enabled: true

defaultScanPolicy:
  enabled: true
  spec:
    paths:
      - /var/lib
      - /opt
      - /usr/local
    excludePatterns:
      - "*.tmp"
      - "/var/lib/docker/*"
    maxConcurrent: 10
```

## Post-Installation Usage

### Verify Deployment

```bash
# Check operator is running
kubectl get pods -n clamav-system

# Check CRDs
kubectl get crd | grep clamav

# Check logs
kubectl logs -n clamav-system deployment/clamav-operator-controller-manager -f
```

### Scan a Node

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

# Watch the scan
kubectl get nodescan -n clamav-system -w
```

### Scan the Entire Cluster

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
```

## Upgrade

```bash
# Upgrade with new version
helm upgrade clamav-operator ./helm/clamav-operator \
  --namespace clamav-system \
  --values my-values.yaml

# Upgrade with new image tag
helm upgrade clamav-operator ./helm/clamav-operator \
  --namespace clamav-system \
  --reuse-values \
  --set operator.image.tag=v1.1.0
```

## Uninstallation

```bash
# Uninstall the chart (keeps CRDs by default)
helm uninstall clamav-operator --namespace clamav-system

# Manually delete CRDs if needed
kubectl delete crd nodescans.clamav.io
kubectl delete crd clusterscans.clamav.io
kubectl delete crd scanpolicies.clamav.io
kubectl delete crd scanschedules.clamav.io
kubectl delete crd scancacheresources.clamav.io
```

## Monitoring

The chart can automatically create:
- A **ServiceMonitor** for Prometheus Operator
- **PrometheusRules** with pre-configured alerts

### Available Metrics

```promql
# Running scans
clamav_nodescan_running

# Infected files
sum(clamav_files_infected_total)

# Scan duration
avg(clamav_scan_duration_seconds)
```

### Pre-configured Alerts

- **ClamAVMalwareDetected** - Malware detected
- **ClamAVScanFailed** - Scan failed
- **ClamAVNoRecentScans** - No recent scans

## Troubleshooting

### Operator Not Starting

```bash
# Check events
kubectl get events -n clamav-system --sort-by='.lastTimestamp'

# Check logs
kubectl logs -n clamav-system deployment/clamav-operator-controller-manager

# Check RBAC
kubectl auth can-i --list --as=system:serviceaccount:clamav-system:clamav-operator
```

### Scans Not Creating

```bash
# Check scanner ServiceAccount exists
kubectl get sa -n clamav-system clamav-scanner

# Check permissions
kubectl auth can-i create jobs --as=system:serviceaccount:clamav-system:clamav-scanner
```

### Webhooks Not Working

```bash
# Check certificates
kubectl get secret -n clamav-system clamav-operator-webhook-server-cert

# Temporarily disable webhooks
helm upgrade clamav-operator ./helm/clamav-operator \
  --namespace clamav-system \
  --reuse-values \
  --set webhook.enabled=false
```

## Support

- Documentation: [GitHub Wiki](https://github.com/SolucTeam/clamav-operator/wiki)
- Issues: [GitHub Issues](https://github.com/SolucTeam/clamav-operator/issues)
- Discussions: [GitHub Discussions](https://github.com/SolucTeam/clamav-operator/discussions)

## License

Apache License 2.0
