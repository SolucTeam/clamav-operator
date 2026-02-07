# ClamAV Operator Helm Chart

Helm chart for deploying the ClamAV Operator in Kubernetes.

## Prerequisites

- Kubernetes 1.24+
- Helm 3.0+
- **No external ClamAV service required** (standalone mode is the default)

## Installation

### Install from local sources

```bash
# Create namespace and install (standalone mode by default)
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
  --set scanner.mode=standalone \
  --set scanner.freshclam.enabled=true
```

## Configuration

### Operator Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `operator.replicaCount` | Number of operator replicas | `1` |
| `operator.image.repository` | Operator image repository | `ghcr.io/solucteam/clamav-operator` |
| `operator.image.tag` | Operator image tag | `""` (chart version) |
| `operator.resources.limits.cpu` | Operator CPU limit | `500m` |
| `operator.resources.limits.memory` | Operator memory limit | `256Mi` |

### Scanner Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `scanner.image.repository` | Scanner image repository | `ghcr.io/solucteam/clamav-node-scanner` |
| `scanner.image.tag` | Scanner image tag | `1.0.3` |
| `scanner.mode` | Scan mode: `standalone` or `remote` | `standalone` |
| `scanner.resources.requests.cpu` | Scanner CPU request | `500m` |
| `scanner.resources.requests.memory` | Scanner memory request | `512Mi` |
| `scanner.resources.limits.cpu` | Scanner CPU limit | `2000m` |
| `scanner.resources.limits.memory` | Scanner memory limit | `1Gi` |

### Scanner Standalone Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `scanner.standalone.clamscanPath` | Path to clamscan binary | `/usr/bin/clamscan` |
| `scanner.standalone.clamavDbPath` | Path to signature database | `/var/lib/clamav` |

### Scanner Remote Parameters (Legacy)

| Parameter | Description | Default |
|-----------|-------------|---------|
| `scanner.clamav.host` | ClamAV service host | `clamav.clamav.svc.cluster.local` |
| `scanner.clamav.port` | ClamAV service port | `3310` |

### Freshclam Parameters (Standalone Only)

| Parameter | Description | Default |
|-----------|-------------|---------|
| `scanner.freshclam.enabled` | Enable signature auto-update CronJob | `true` |
| `scanner.freshclam.schedule` | Cron schedule for updates | `0 */6 * * *` |
| `scanner.freshclam.image.repository` | Freshclam image | `clamav/clamav` |
| `scanner.freshclam.image.tag` | Freshclam image tag | `1.3` |
| `scanner.freshclam.resources.limits.cpu` | Freshclam CPU limit | `500m` |
| `scanner.freshclam.resources.limits.memory` | Freshclam memory limit | `256Mi` |

### Signature Persistence Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `scanner.signatures.persistent` | Use a PVC for signatures | `false` |
| `scanner.signatures.pvcName` | PVC name | `clamav-signatures` |
| `scanner.signatures.storageClass` | StorageClass (empty = default) | `""` |
| `scanner.signatures.accessMode` | PVC access mode | `ReadWriteOnce` |
| `scanner.signatures.size` | PVC size | `1Gi` |

### Incremental Scan Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `scanner.incremental.enabled` | Enable incremental scanning | `true` |
| `scanner.incremental.strategy` | Strategy: `full`, `incremental`, `smart` | `smart` |
| `scanner.incremental.fullScanInterval` | Full scan every N incremental runs (smart) | `10` |
| `scanner.incremental.maxFileAgeHours` | Max file age for incremental scans | `24` |
| `scanner.incremental.skipUnchangedFiles` | Skip files with same mtime+size | `true` |

### Other Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `crds.install` | Install CRDs | `true` |
| `crds.keep` | Keep CRDs on uninstall | `true` |
| `rbac.create` | Create RBAC resources | `true` |
| `monitoring.serviceMonitor.enabled` | Enable Prometheus ServiceMonitor | `false` |
| `monitoring.prometheusRule.enabled` | Enable PrometheusRule | `false` |
| `defaultScanPolicy.enabled` | Create default ScanPolicy | `true` |
| `networkPolicy.enabled` | Enable network policies | `false` |

## Example Values Files

### Standalone with Incremental Scanning (Recommended)

```yaml
# standalone-values.yaml
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

monitoring:
  serviceMonitor:
    enabled: true
  prometheusRule:
    enabled: true
```

### Air-Gap (No Internet)

```yaml
# airgap-values.yaml
scanner:
  mode: standalone
  freshclam:
    enabled: false
  image:
    repository: my-registry.internal/clamav-node-scanner
    tag: "1.1.0-airgap"
```

### Remote Mode (Legacy)

```yaml
# remote-values.yaml
scanner:
  mode: remote
  clamav:
    host: clamav.clamav.svc.cluster.local
    port: 3310
```

### Production HA

```yaml
# production-values.yaml
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
  mode: standalone
  freshclam:
    enabled: true
  incremental:
    enabled: true
    strategy: smart
  signatures:
    persistent: true
    storageClass: gp3
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

networkPolicy:
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
kubectl get pods -n clamav-system
kubectl get crd | grep clamav
kubectl logs -n clamav-system deployment/clamav-operator-controller-manager -f

# Verify freshclam CronJob (standalone mode)
kubectl get cronjob -n clamav-system
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
  --set operator.image.tag=v1.1.0 \
  --set scanner.image.tag=1.1.0

# Migrate from remote to standalone
helm upgrade clamav-operator ./helm/clamav-operator \
  --namespace clamav-system \
  --reuse-values \
  --set scanner.mode=standalone \
  --set scanner.freshclam.enabled=true
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

# Remove PVC (if persistent signatures)
kubectl delete pvc -n clamav-system clamav-signatures
```

## Monitoring

The chart can automatically create a **ServiceMonitor** for Prometheus Operator and **PrometheusRules** with pre-configured alerts.

### Available Metrics

```promql
clamav_nodescan_running
sum(clamav_files_infected_total)
avg(clamav_scan_duration_seconds)
```

### Pre-configured Alerts

- **ClamAVMalwareDetected** — Malware detected
- **ClamAVScanFailed** — Scan failed
- **ClamAVNoRecentScans** — No recent scans

## Troubleshooting

### Operator Not Starting

```bash
kubectl get events -n clamav-system --sort-by='.lastTimestamp'
kubectl logs -n clamav-system deployment/clamav-operator-controller-manager
kubectl auth can-i --list --as=system:serviceaccount:clamav-system:clamav-operator
```

### Scanner Failing (Standalone)

```bash
kubectl logs -n clamav-system -l clamav.io/nodescan=<scan-name>
# "No ClamAV signatures found" → build with DOWNLOAD_SIGS=true or set freshclam.enabled=true
```

### Scanner Failing (Remote)

```bash
kubectl logs -n clamav-system -l clamav.io/nodescan=<scan-name>
kubectl run -it --rm debug --image=busybox --restart=Never -- \
  nc -zv clamav.clamav.svc.cluster.local 3310
```

### Freshclam CronJob Failing

```bash
kubectl get cronjob -n clamav-system
kubectl logs -n clamav-system -l app.kubernetes.io/component=freshclam --tail=50
# Air-gap? Set scanner.freshclam.enabled=false
```

### Webhooks Not Working

```bash
kubectl get secret -n clamav-system clamav-operator-webhook-server-cert
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
