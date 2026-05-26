# ClamAV Operator Helm Chart

[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/clamav-operator)](https://artifacthub.io/packages/helm/clamav-operator/clamav-operator)
![Chart Version](https://img.shields.io/badge/chart-v1.1.0-blue)
![App Version](https://img.shields.io/badge/app-v0.1.0-green)

Helm chart for deploying the **ClamAV Operator** — a Kubernetes operator for cluster-wide antivirus scanning using ClamAV. Supports standalone (embedded signatures) and remote clamd modes, incremental scanning, Prometheus monitoring, and scheduled scans.

## CRDs Installed

| CRD | Description |
|-----|-------------|
| `nodescans.clamav.io` | Scan a single node |
| `clusterscans.clamav.io` | Scan all (or selected) nodes |
| `scanpolicies.clamav.io` | Reusable scan configuration |
| `scanschedules.clamav.io` | Cron-based scheduled scanning |
| `scancacheresources.clamav.io` | Incremental scan state cache |

## Prerequisites

- Kubernetes 1.24+
- Helm 3.0+
- **No external ClamAV service required** (standalone mode is the default)
- **cert-manager** (optional, but strongly recommended for production webhook TLS — see [Webhook TLS](#webhook-tls))

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
| `scanner.image.tag` | Scanner image tag | `""` (chart appVersion) |
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

### Freshclam Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `scanner.freshclam.enabled` | Enable the freshclam signature-update CronJob | `true` |
| `scanner.freshclam.schedule` | Cron schedule for updates | `0 */6 * * *` |
| `scanner.freshclam.image.repository` | Freshclam image | `clamav/clamav` |
| `scanner.freshclam.image.tag` | Freshclam image tag | `1.3` |
| `scanner.freshclam.resources.limits.cpu` | Freshclam CPU limit | `500m` |
| `scanner.freshclam.resources.limits.memory` | Freshclam memory limit | `256Mi` |

> **Standalone mode limitation:** In standalone mode, the freshclam CronJob writes signatures to its own volume (emptyDir or PVC). Scanner job pods do not mount that volume — they read signatures from `CLAMAV_DB_PATH` inside the scanner container image. Enabling freshclam therefore has **no effect** on what scanner jobs scan with in standalone mode. Signatures must be baked into the scanner image at build time. The freshclam CronJob is only effective in **remote mode**, where it shares a PVC with the running clamd daemon.

### Signature Persistence Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `scanner.signatures.persistent` | Use a PVC for signatures | `false` |
| `scanner.signatures.pvcName` | PVC name | `clamav-signatures` |
| `scanner.signatures.storageClass` | StorageClass (empty = default) | `""` |
| `scanner.signatures.accessMode` | PVC access mode. Use `ReadWriteMany` for multi-node signature sharing (requires NFS/CephFS/EFS/…). `ReadWriteOnce` only works if all scanner pods land on the same node as freshclam. | `ReadWriteOnce` |
| `scanner.signatures.size` | PVC size | `1Gi` |

> **`ReadWriteMany` is required** when `signatures.persistent=true`. Scanner jobs run on different nodes; a `ReadWriteOnce` PVC can only be mounted by pods on the single node where it is bound — all other scanner pods fail with *"Multi-Attach error"*. RWX requires a storage backend that supports it: NFS, CephFS, Azure Files, AWS EFS, GCP Filestore. Standard block storage (EBS, GCP Persistent Disk) does **not** support RWX — use the image-embedded approach instead (`persistent: false`).

### Incremental Scan Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `scanner.incremental.enabled` | Enable incremental scanning | `true` |
| `scanner.incremental.strategy` | Strategy: `full`, `incremental`, `smart` | `smart` |
| `scanner.incremental.fullScanInterval` | Full scan every N incremental runs (smart) | `10` |
| `scanner.incremental.maxFileAgeHours` | Max file age for incremental scans | `24` |
| `scanner.incremental.skipUnchangedFiles` | Skip files with same mtime+size | `true` |

> **Automatic cache invalidation on signature update (v3 cache format):** The scanner computes a fingerprint of all `.cvd`/`.cld` files in `CLAMAV_DB_PATH` (mtime + size). This fingerprint is stored with the cache. If signatures have been updated since the last scan, the fingerprint differs and the incremental cache is automatically discarded — forcing a full scan on the next run. This guarantees that every file is re-evaluated against the new signatures, even files whose content and metadata are unchanged. No manual cache deletion is required after a freshclam update.

### Webhook & TLS Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `webhook.enabled` | Enable admission + conversion webhooks | `true` |
| `webhook.port` | Webhook server port (synced to `--webhook-port` flag and Service) | `9443` |
| `webhook.certManager.enabled` | Use cert-manager to manage webhook TLS | `true` |
| `webhook.certificates.autoGenerate` | Auto-generate self-signed certs (dev only, not rotated) | `true` |
| `webhook.certificates.caBundle` | Base64-encoded CA bundle for manual TLS | `""` |

> **cert-manager is enabled by default.** From v0.5.0, `webhook.certManager.enabled` defaults to `true`. Install cert-manager before deploying the operator, or explicitly set `webhook.certManager.enabled: false` to fall back to auto-generated self-signed certs (not automatically rotated — not recommended for production).

When `webhook.certManager.enabled: true` the chart creates a full self-signed CA chain (SelfSigned Issuer → CA Certificate → CA Issuer → leaf Certificate). The `ValidatingWebhookConfiguration` is annotated with `cert-manager.io/inject-ca-from` so cert-manager's cainjector populates `caBundle` automatically.

A Helm post-install/post-upgrade hook Job patches the three CRDs (`nodescans`, `clusterscans`, `scanschedules`) with `conversion.strategy: Webhook` so the `/convert` endpoint is correctly wired.

### CRD Patcher Hook Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `crdPatcher.image.repository` | kubectl image used by the post-install hook Job | `registry.k8s.io/kubectl` |
| `crdPatcher.image.tag` | Image tag — pin to your cluster minor version | `"1.29"` |
| `crdPatcher.image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `crdPatcher.imagePullSecrets` | Pull secrets for the hook Job (air-gap / private registry) | `[]` |

```yaml
# Air-gap example
crdPatcher:
  image:
    repository: my-registry.internal/kubectl
    tag: "1.30"
  imagePullSecrets:
    - name: regcred
```

### Scanner Image Pull Secrets

| Parameter | Description | Default |
|-----------|-------------|---------|
| `scanner.imagePullSecrets` | Pull secrets injected into every scanner Job pod | `[]` |

```yaml
# Private registry example
scanner:
  imagePullSecrets:
    - name: regcred
```

These secrets are forwarded from the operator pod environment (`SCANNER_IMAGE_PULL_SECRETS`) into the `imagePullSecrets` field of each Job's pod spec.

### Network Policy Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `networkPolicy.enabled` | Enable NetworkPolicy resources | `true` |
| `networkPolicy.controlPlaneCIDR` | CIDR of the Kubernetes API server / control-plane nodes. Used for egress to the API server and ingress from the API server to the webhook port. Set to your actual control-plane CIDR in production. | `"0.0.0.0/0"` |
| `networkPolicy.apiServerPort` | API server port. Standard clusters use `6443`; some managed clusters (AKS, GKE) expose it on `443`. | `6443` |
| `networkPolicy.nodesCIDR` | CIDR of cluster nodes. Kubelet liveness/readiness probes originate from node IPs — a `namespaceSelector` cannot match them. Set to your node subnet in production. | `"0.0.0.0/0"` |
| `networkPolicy.ingress` | Additional ingress rules (e.g. Prometheus scrape). Kubelet probe and webhook rules are generated automatically. | `[]` |
| `networkPolicy.egress` | Additional egress rules for the operator pod (ClamAV, DNS, …). API server egress is generated automatically. | `[]` |
| `networkPolicy.freshclamEgress` | Egress rules for the freshclam CronJob pod. Only rendered when `scanner.mode=standalone` and `scanner.freshclam.enabled=true`. | `[]` |

> **Why `ipBlock` and not `namespaceSelector`?**
> The Kubernetes API server and the kubelet both run as host processes on nodes, not as pods. `namespaceSelector` only matches pod-to-pod traffic; it has no effect on traffic that originates outside the pod network. Without `ipBlock` rules, the operator pod cannot reach the API server (startup checks fail with `i/o timeout`) and kubelet probes are silently dropped (liveness/readiness never pass).

```yaml
# Production example — restrict to actual CIDRs
networkPolicy:
  enabled: true
  controlPlaneCIDR: "10.0.0.10/32"   # exact API server IP
  apiServerPort: 6443
  nodesCIDR: "10.0.1.0/24"           # worker node subnet
```

### Default Scan Schedule Parameters

Disabled by default. Enable to automatically run cluster-wide scans on a cron schedule without creating resources manually.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `defaultScanSchedule.enabled` | Create a default ScanSchedule resource | `false` |
| `defaultScanSchedule.name` | ScanSchedule resource name | `default-schedule` |
| `defaultScanSchedule.schedule` | Cron expression | `0 2 * * *` (daily at 02:00) |
| `defaultScanSchedule.suspend` | Pause scheduled runs without deleting the resource | `false` |
| `defaultScanSchedule.concurrencyPolicy` | `Forbid` / `Allow` / `Replace` | `Forbid` |
| `defaultScanSchedule.scanPolicy` | ScanPolicy to use | `default-policy` |
| `defaultScanSchedule.concurrent` | Max nodes scanned in parallel | `3` |
| `defaultScanSchedule.priority` | NodeScan priority: `high` / `medium` / `low` | `low` |
| `defaultScanSchedule.nodeSelector` | Restrict to specific nodes (empty = all nodes) | `{}` |
| `defaultScanSchedule.successfulScansHistoryLimit` | Successful ClusterScans to retain | `3` |
| `defaultScanSchedule.failedScansHistoryLimit` | Failed ClusterScans to retain | `1` |
| `defaultScanSchedule.startingDeadlineSeconds` | Max delay (s) before skipping a missed run | `""` |

```yaml
# Enable automatic nightly scans on worker nodes only
defaultScanSchedule:
  enabled: true
  schedule: "0 2 * * *"
  concurrent: 3
  priority: low
  nodeSelector:
    matchLabels:
      node-role.kubernetes.io/worker: ""
```

### Other Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `crds.install` | Install CRDs | `true` |
| `crds.keep` | Keep CRDs on uninstall | `true` |
| `rbac.create` | Create RBAC resources | `true` |
| `monitoring.serviceMonitor.enabled` | Enable Prometheus ServiceMonitor | `false` |
| `monitoring.prometheusRule.enabled` | Enable PrometheusRule | `false` |
| `defaultScanPolicy.enabled` | Create default ScanPolicy | `true` |

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
    # persistent=true shares freshclam-updated signatures with scanner jobs via PVC.
    # Requires a ReadWriteMany-capable storage class (NFS, CephFS, Azure Files, EFS…).
    # If your cluster only has block storage (EBS, GCP PD), leave persistent: false
    # and bake signatures into the scanner image instead.
    persistent: true
    storageClass: nfs-client   # replace with your RWX-capable StorageClass

monitoring:
  serviceMonitor:
    enabled: true
  prometheusRule:
    enabled: true
```

### Air-Gap (No Internet)

In standalone mode the scanner runs `clamscan` from **inside the scanner container image**. The signatures database (`CLAMAV_DB_PATH`, default `/var/lib/clamav`) is read from the container image filesystem — it is **not** supplied at runtime by the freshclam CronJob. This means two things:

1. **Signatures must be baked into the scanner image at build time.** Use a Dockerfile that runs `freshclam` during the build (internet access at build time is enough) or COPYs a pre-downloaded database directory.
2. **`freshclam.enabled` has no effect on scanner jobs in standalone mode.** The CronJob writes to its own isolated volume that scanner pods never mount. Disable it for air-gap environments.

To refresh signatures in an air-gapped cluster, rebuild the scanner image (or update the pre-baked database files) and roll out a new image tag.

```yaml
# airgap-values.yaml
scanner:
  mode: standalone
  # Disable freshclam — no internet access and it cannot feed standalone scanner jobs anyway.
  freshclam:
    enabled: false
  # Use a custom image that has ClamAV signatures baked in at build time.
  # The image must contain a valid database at /var/lib/clamav (or wherever
  # scanner.standalone.clamavDbPath points).
  image:
    repository: my-registry.internal/clamav-node-scanner
    tag: "1.1.0-with-sigs"   # image built with freshclam run during docker build

# All operator/scanner images must be mirrored to your internal registry.
operator:
  image:
    repository: my-registry.internal/clamav-operator
  imagePullSecrets:
    - name: regcred

crdPatcher:
  image:
    repository: my-registry.internal/kubectl
    tag: "1.30"
  imagePullSecrets:
    - name: regcred
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

### Production with cert-manager TLS

```yaml
# cert-manager-values.yaml
webhook:
  enabled: true
  certManager:
    enabled: true    # Requires cert-manager installed in the cluster
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
    storageClass: efs-sc   # must be a ReadWriteMany-capable StorageClass (NFS, EFS, CephFS…)
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
  controlPlaneCIDR: "10.0.0.10/32"   # restrict to your actual API server IP/CIDR
  apiServerPort: 6443
  nodesCIDR: "10.0.1.0/24"           # restrict to your node subnet

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
    maxConcurrent: 1  # standalone: ignored (single clamscan process); remote: clamd TCP connections
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

### Run Helm Tests

```bash
# Run connectivity checks (healthz, readyz, metrics)
helm test clamav-operator --namespace clamav-system

# Show test logs
helm test clamav-operator --namespace clamav-system --logs
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

## Notifications

Notifications are configured on the `ScanPolicy` resource under `spec.notifications`. The operator supports Slack, Email, Generic Webhook, and Microsoft Teams. All channels support `onlyOnInfection: true` (default) to suppress alerts for clean scans.

### Retry Behaviour

All notification channels use exponential backoff with **3 attempts** (delays: 5 s → 10 s → 20 s). Each channel is retried independently — a Slack failure does not block the email attempt. Failures are recorded in the `clamav_notifications_failed_total` metric (labelled by `channel`) and surfaced as a Kubernetes event on the NodeScan.

### Slack

```yaml
spec:
  notifications:
    slack:
      enabled: true
      webhookSecretRef:
        name: slack-webhook-secret
        key: url
      channel: "#security-alerts"
      onlyOnInfection: true
```

Store the webhook URL in a Secret:
```bash
kubectl create secret generic slack-webhook-secret \
  --from-literal=url=https://hooks.slack.com/services/... \
  -n clamav-system
```

### Email

```yaml
spec:
  notifications:
    email:
      enabled: true
      smtpServer: "smtp.example.com:587"
      smtpAuthSecretRef:
        name: smtp-credentials
        namespace: clamav-system
      from: "clamav@example.com"
      recipients:
        - "security@example.com"
        - "ops@example.com"
      onlyOnInfection: true
```

### Generic Webhook

```yaml
spec:
  notifications:
    webhook:
      enabled: true
      url: "https://my-siem.example.com/api/clamav-events"
      headers:
        Content-Type: application/json
      secretRef:
        name: webhook-auth-secret
        namespace: clamav-system
      onlyOnInfection: false   # notify on every scan completion
```

### Microsoft Teams

The Teams notifier sends an **Adaptive Card (schema v1.2)** via an Incoming Webhook. It is compatible with both the legacy Office 365 Connector format and the new Power Automate / Workflows webhook URLs (`https://prod-*.logic.azure.com/...`).

The card includes: node name, scan phase, files scanned / infected, duration, list of infected files (up to 10), and a warning banner when results are partial.

```yaml
spec:
  notifications:
    teams:
      enabled: true
      webhookSecretRef:
        name: teams-webhook-secret
        key: url
      onlyOnInfection: true
```

Store the webhook URL in a Secret:
```bash
kubectl create secret generic teams-webhook-secret \
  --from-literal=url=https://prod-XX.westeurope.logic.azure.com/workflows/... \
  -n clamav-system
```

You can also set `webhookURL` inline (not recommended for production — the URL is visible in the ScanPolicy spec):

```yaml
spec:
  notifications:
    teams:
      enabled: true
      webhookURL: "https://prod-XX.westeurope.logic.azure.com/workflows/..."
```

### Full ScanPolicy with All Notification Channels

```yaml
apiVersion: clamav.io/v1alpha1
kind: ScanPolicy
metadata:
  name: default-policy
  namespace: clamav-system
spec:
  paths:
    - /var/lib
    - /opt
  maxConcurrent: 1  # standalone: ignored (single clamscan process); remote: clamd TCP connections
  notifications:
    slack:
      enabled: true
      webhookSecretRef:
        name: slack-webhook-secret
        key: url
      channel: "#security-alerts"
    email:
      enabled: true
      smtpServer: "smtp.example.com:587"
      smtpAuthSecretRef:
        name: smtp-credentials
        namespace: clamav-system
      from: "clamav@example.com"
      recipients:
        - "security@example.com"
    teams:
      enabled: true
      webhookSecretRef:
        name: teams-webhook-secret
        key: url
  quarantine:
    enabled: true
    action: alert-only
```

---

## Upgrade

> ⚠️ **`helm upgrade` does NOT update CRDs.** This is a Helm design decision. You must apply CRDs manually **before** upgrading the Helm release, otherwise the operator may start with outdated schemas and silently reject new fields.

```bash
NS=clamav-system
NEW_VERSION=<new-version>   # e.g. 0.5.0

# Step 1 — Update CRDs (required every upgrade)
kubectl apply -f helm/clamav-operator/crds/
# Wait for all CRDs to reach Established=True
kubectl wait --for=condition=Established crd \
  nodescans.clamav.io clusterscans.clamav.io \
  scanpolicies.clamav.io scanschedules.clamav.io \
  --timeout=60s

# Step 2 — Upgrade the Helm release
helm upgrade clamav-operator oci://ghcr.io/solucteam/charts/clamav-operator \
  --version $NEW_VERSION \
  -n $NS \
  --reuse-values \
  --wait

# Step 3 — Verify
kubectl rollout status deploy/clamav-operator -n $NS
helm test clamav-operator -n $NS
```

**Other common upgrade scenarios:**

```bash
# Pin a specific image tag
helm upgrade clamav-operator oci://ghcr.io/solucteam/charts/clamav-operator \
  -n $NS --reuse-values \
  --set operator.image.tag=v1.1.0

# Migrate from remote to standalone
helm upgrade clamav-operator oci://ghcr.io/solucteam/charts/clamav-operator \
  -n $NS --reuse-values \
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

# Notification delivery tracking (labelled by namespace and channel)
clamav_notifications_total
clamav_notifications_failed_total

# Partial result tracking (labelled by namespace and node)
clamav_nodescan_partial_results
```

### Pre-configured Alerts

| Alert | Condition | Severity |
|-------|-----------|----------|
| `ClamAVMalwareDetected` | `clamav_files_infected_total > 0` | critical |
| `ClamAVScanFailed` | Scanner Job exited non-zero | warning |
| `ClamAVNoRecentScans` | No node scanned in the last 24 h | warning |
| `ClamAVOperatorDown` | Operator deployment has 0 ready replicas | critical |
| `ClamAVNotificationFailed` | Notification not delivered after 3 retries | warning |
| `ClamAVPartialScanResults` | NodeScan has `resultsPartial: true` — data unreliable | warning |
| `ClamAVWebhookCertExpiringSoon` | Webhook TLS cert expires in < 7 days | warning |

> **`ClamAVPartialScanResults`**: Do NOT treat a scan with `resultsPartial: true` as clean. Result parsing failed — the data is incomplete. Delete the NodeScan to trigger a re-scan.

> **`ClamAVWebhookCertExpiringSoon`**: Requires `monitoring.prometheusRule.enabled: true` and cert-manager. If this alert fires, see the [runbook](../../docs/runbook.md#10-emergency--webhook-certificate-expired) for renewal steps.

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
```

**"No ClamAV signatures found in /var/lib/clamav"**

In standalone mode the scanner reads signatures from `CLAMAV_DB_PATH` **inside the container image**. The freshclam CronJob writes to its own isolated volume and cannot update what scanner jobs read. The signatures must be present in the scanner image itself.

Fix options:
- **Rebuild the scanner image** with signatures baked in: run `freshclam` during `docker build` or `COPY` a pre-downloaded ClamAV database directory into `/var/lib/clamav`.
- **Set `scanner.standalone.clamavDbPath`** to a path that exists and is populated in your image.

Setting `freshclam.enabled=true` does **not** fix this error in standalone mode — it creates a CronJob that downloads signatures to a separate volume that scanner pods never mount.

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
# Check cert-manager Certificate status
kubectl get certificate -n clamav-system
# READY=True → secret provisioned, cainjector will populate caBundle shortly

# Verify caBundle is non-empty in the ValidatingWebhookConfiguration
kubectl get validatingwebhookconfiguration \
  clamav-operator-validating-webhook-configuration \
  -o jsonpath='{.webhooks[0].clientConfig.caBundle}' | wc -c
# Must be > 0. If 0, ensure cert-manager cainjector is running:
kubectl get pods -n cert-manager -l app=cainjector

# Check that the CRD conversion hook Job completed
kubectl get job -n clamav-system | grep crd-patcher
kubectl logs -n clamav-system job/clamav-operator-crd-patcher

# Emergency: disable webhooks entirely
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
