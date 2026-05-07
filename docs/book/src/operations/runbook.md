# ClamAV Operator — Operational Runbook

> **Audience:** SRE / Platform engineers  
> **Scope:** Day-2 operations for the clamav-operator running in a Kubernetes cluster

---

## Table of Contents

1. [Key Commands](#key-commands)
2. [Alerts](#alerts)
3. [Scenarios](#scenarios)
   - [Operator not reconciling](#1-operator-not-reconciling)
   - [Scanner Job stuck / never completes](#2-scanner-job-stuck--never-completes)
   - [Infected file detected](#3-infected-file-detected)
   - [Freshclam failing — signatures outdated](#4-freshclam-failing--signatures-outdated)
   - [Incremental cache corrupted](#5-incremental-cache-corrupted)
   - [OOMKilled scanner pod](#6-oomkilled-scanner-pod)
   - [NodeScan permanently Failed](#7-nodescan-permanently-failed)
   - [NodeScan stuck in Pending — Job name invalid (long FQDN nodes)](#11-nodescan-stuck-in-pending--job-name-invalid-long-fqdn-nodes)
   - [Notifications not delivered (ClamAVNotificationFailed)](#8-notifications-not-delivered)
   - [Partial scan results (resultsPartial: true)](#9-partial-scan-results)
   - [EMERGENCY — Webhook certificate expired](#10-emergency--webhook-certificate-expired)
   - [Low scan throughput](#13-low-scan-throughput)
   - [Results directory too large](#12-results-directory-too-large)
4. [Maintenance](#maintenance)
   - [Upgrade the operator](#upgrade-the-operator)
   - [CRD upgrade procedure](#crd-upgrade-procedure)
   - [Rollback procedure](#rollback-procedure)
5. [Escalation](#escalation)

---

## Key Commands

```bash
NS=clamav-system          # adjust to your namespace
RELEASE=clamav-operator   # adjust to your Helm release name

# Operator logs (live)
kubectl -n $NS logs -l control-plane=controller-manager -f

# List all NodeScans and their phase
kubectl -n $NS get nodescans -o wide

# List all ClusterScans
kubectl -n $NS get clusterscans -o wide

# Describe a failed NodeScan
kubectl -n $NS describe nodescan <name>

# List scanner Jobs (running + recent)
kubectl -n $NS get jobs -l app.kubernetes.io/managed-by=clamav-operator

# Tail logs of a running scanner Job
kubectl -n $NS logs -l job-name=<job-name> -f

# Check Prometheus metrics
kubectl -n $NS port-forward svc/$RELEASE 8080:8080 &
curl -s http://localhost:8080/metrics | grep clamav_

# Run Helm test suite
helm test $RELEASE -n $NS --logs
```

---

## Alerts

| Alert | Severity | Meaning | Runbook section |
|-------|----------|---------|----------------|
| `ClamAVMalwareDetected` | critical | Infected files detected on a node | [#3](#3-infected-file-detected) |
| `ClamAVScanFailed` | warning | A scanner Job exited non-zero | [#2](#2-scanner-job-stuck--never-completes) |
| `ClamAVNoRecentScans` | warning | No node completed a scan in the last 24 h | [#1](#1-operator-not-reconciling), [#2](#2-scanner-job-stuck--never-completes) |
| `ClamAVNotificationFailed` | warning | Notification not delivered after retries | [#8](#8-notifications-not-delivered) |
| `ClamAVPartialScanResults` | warning | NodeScan has `resultsPartial: true` — data unreliable | [#9](#9-partial-scan-results) |
| `ClamAVWebhookCertExpiringSoon` | warning | Webhook TLS cert expires in < 7 days | [#10](#10-emergency--webhook-certificate-expired) |
| `ClamAVResultsDirTooLarge` | warning | Results directory > 500 MB on a node hostPath | [#12](#12-results-directory-too-large) |
| `ClamAVCacheFileTooLarge` | warning | Incremental cache file > 100 MB | [#12](#12-results-directory-too-large) |
| `ClamAVOperatorReconcileErrors` | warning | Operator reconcile error rate > 0.1/s for 10 min | [#1](#1-operator-not-reconciling) |
| `ClamAVScanStuck` | warning | Running scan with no completion > 10 h | [#2](#2-scanner-job-stuck--never-completes) |
| `ClamAVHighScanDuration` | warning | p95 scan duration > 2 h | [#2](#2-scanner-job-stuck--never-completes) |
| `ClamAVLowScanThroughput` | warning | Throughput < 5 files/s for 30 min | [#13](#13-low-scan-throughput) |
| `ClamAVSignaturesStale` | warning | Signature database age > 24 h | [#4](#4-freshclam-failing--signatures-outdated) |
| `ClamAVOOMKillsElevated` | warning | More than 2 OOMKills in 1 h | [#6](#6-oomkilled-scanner-pod) |

---

## Scenarios

### 1. Operator not reconciling

**Symptoms:** NodeScans stay in `Pending`, no Jobs are created, `ClamAVNoRecentScans` alert fires.

**Diagnosis:**
```bash
# Check operator pod status
kubectl -n $NS get pod -l control-plane=controller-manager

# Look for errors in operator logs
kubectl -n $NS logs -l control-plane=controller-manager --tail=100 | grep -i error

# Check leader election (multi-replica deployments)
kubectl -n $NS get lease clamav-operator-leader -o yaml
```

**Common causes and fixes:**

| Cause | Fix |
|-------|-----|
| OOMKilled | Increase `operator.resources.limits.memory` and `helm upgrade` |
| CrashLoopBackOff | Check logs for panic; verify RBAC with `helm test` |
| ImagePullBackOff | Verify image tag and pull secret |
| Leader election lost | Restart operator pods: `kubectl -n $NS rollout restart deploy/<manager>` |

---

### 2. Scanner Job stuck / never completes

**Symptoms:** NodeScan stays in `Running` phase for > 1 h, Job pod not progressing.

**Diagnosis:**
```bash
# Find the stuck Job
kubectl -n $NS get jobs

# Get the scanner pod
kubectl -n $NS get pods -l job-name=<job-name>

# Check pod events
kubectl -n $NS describe pod <pod-name>

# Tail scanner logs
kubectl -n $NS logs <pod-name> -f
```

**Common causes and fixes:**

| Cause | Fix |
|-------|-----|
| Node is not schedulable | Check node taints/tolerations; add tolerations in `values.yaml` |
| Image pull failure | Verify `scanner.image` and `scanner.imagePullSecrets` |
| `clamscan` not found in image | Verify `scanner.standalone.clamscanPath` matches the image |
| Signatures missing (standalone) | Enable freshclam or rebuild image with signatures |
| Job timeout | The scan Job deadline defaults to **7200 s (2 h)**. If your nodes take longer to scan, increase it via `scanner.jobActiveDeadlineSeconds` in `values.yaml` (e.g. `14400` for 4 h) and run `helm upgrade`. |

**Force-delete a stuck NodeScan:**
```bash
kubectl -n $NS delete nodescan <name> --grace-period=0
# The operator finalizer will clean up the Job automatically
```

---

### 3. Infected file detected

**Symptoms:** `clamav_files_infected_total > 0`, `ClamAVInfectedFilesDetected` alert fires.

**Immediate response:**
```bash
# Find the infected NodeScan
kubectl -n $NS get nodescans -o json | \
  jq '.items[] | select(.status.filesInfected > 0) | {name: .metadata.name, node: .spec.nodeName, files: .status.infectedFiles}'

# Get full details
kubectl -n $NS describe nodescan <name>
```

**Decision tree:**
1. **Known false positive** (e.g. EICAR test file) → whitelist via `ScanPolicy.spec.excludePaths`
2. **Real infection** → isolate the node immediately:
   ```bash
   kubectl cordon <node-name>
   # Escalate to security team — do NOT uncordon until cleared
   ```
3. **Unsure** → pull the scanner pod logs and extract the exact file path + virus name, then escalate.

**Add a path exclusion (false positive):**
```yaml
apiVersion: clamav.io/v1alpha1
kind: ScanPolicy
metadata:
  name: default-policy
  namespace: clamav-system
spec:
  excludePaths:
    - /path/to/known-false-positive
```

---

### 4. Freshclam failing — signatures outdated

**Symptoms:** Freshclam CronJob fails repeatedly; `clamav_nodescan_last_completion_timestamp` is recent but infected files may be missed.

**Diagnosis:**
```bash
# Check freshclam CronJob history
kubectl -n $NS get cronjob -l app.kubernetes.io/component=freshclam
kubectl -n $NS get jobs -l app.kubernetes.io/component=freshclam

# Tail the last failed freshclam job
kubectl -n $NS logs job/<freshclam-job-name>
```

**Common causes and fixes:**

| Cause | Fix |
|-------|-----|
| No internet access (air-gap) | Disable freshclam; use image-baked signatures. See [airgap-signatures.md](./airgap-signatures.md) |
| DNS resolution failure | Check cluster DNS; verify `database.clamav.net` is resolvable |
| Rate-limited by ClamAV mirrors | Reduce `freshclam.schedule` frequency (no more than every 4 h) |
| Disk full | Increase PVC size or clean up old signature files |

**Manual trigger (force immediate update):**
```bash
kubectl -n $NS create job freshclam-manual \
  --from=cronjob/<release>-freshclam
kubectl -n $NS logs -f job/freshclam-manual
```

---

### 5. Incremental cache corrupted

**Symptoms:** Scanner pod exits with a JSON parse error on cache load; scan never completes.

**Diagnosis:**
```bash
kubectl -n $NS logs <scanner-pod-name> | grep -i "cache\|error"
```

Look for: `Failed to parse cache file` or `Incremental cache corrupted`.

**Fix — delete the ScanCacheResource for the affected node:**
```bash
# List all caches
kubectl -n $NS get scancacheresources

# Delete the corrupted one (the operator will recreate it on the next full scan)
kubectl -n $NS delete scancacheresource <node-name>
```

The next scan for that node will automatically run as a **full scan** and rebuild the cache.

---

### 6. OOMKilled scanner pod

**Symptoms:** Scanner pod status is `OOMKilled`; NodeScan phase is `Failed`.

**Diagnosis:**
```bash
kubectl -n $NS describe pod <scanner-pod> | grep -A5 "OOMKilled\|Limits"
```

**Fix:**
```bash
# Increase scanner memory limit
helm upgrade $RELEASE oci://ghcr.io/solucteam/charts/clamav-operator \
  -n $NS \
  --set scanner.resources.limits.memory=3Gi \
  --set scanner.resources.requests.memory=1Gi \
  --reuse-values
```

For production, use `values-production.yaml` which sets `limits.memory=2Gi` by default — sufficient for nodes with up to ~500k files.

---

### 7. NodeScan permanently Failed

**Symptoms:** A NodeScan is stuck in `Failed` phase and will not be retried automatically.

**Fix — delete and let the controller recreate:**
```bash
kubectl -n $NS delete nodescan <name>
```

If driven by a ClusterScan, the ClusterScan controller will reschedule it automatically. If it was manually created, re-apply the manifest.

---

### 8. Notifications not delivered

**Symptoms:** `ClamAVNotificationFailed` alert fires. Malware was detected but team received no Slack/email/webhook/Teams alert.

**Diagnosis:**
```bash
# Check for notification events on the affected NodeScan
kubectl -n $NS describe nodescan <name> | grep -A5 "NotificationFailed"

# Check metric
kubectl -n $NS port-forward svc/clamav-operator 8080:8080 &
curl -s localhost:8080/metrics | grep notifications_failed
```

**Common causes:**

| Cause | Fix |
|-------|-----|
| Slack webhook URL expired/rotated | Update the Secret referenced by `notifications.slack.webhookSecretRef` |
| SMTP server unreachable | Verify port 465/587 is accessible from operator pod. Check `networkPolicy.egress` |
| Webhook endpoint returns non-2xx | Check endpoint health; review `notifications.webhook.url` |
| Teams webhook URL expired/rotated | Update the Secret referenced by `notifications.teams.webhookSecretRef` |
| Network policy blocking egress | Add egress rule for the notification endpoint IP/port |

**Test Slack connectivity from the operator pod:**
```bash
kubectl -n $NS exec deploy/clamav-operator -- \
  wget -qO- https://hooks.slack.com/ --spider
```

---

### 9. Partial scan results

**Symptoms:** NodeScan has `status.resultsPartial: true`. The `ClamAVPartialScanResults` alert fires.

> ⚠️ **Do NOT treat a scan with `resultsPartial: true` as clean.** Result parsing failed — the data is incomplete and unreliable.

**Diagnosis:**
```bash
kubectl -n $NS describe nodescan <name> | grep -i "partial\|error\|condition"
kubectl -n $NS logs job/<associated-job-name> | tail -30
```

**Fix — force a new scan:**
```bash
# Delete the NodeScan — if driven by ClusterScan/ScanSchedule it will be recreated
kubectl -n $NS delete nodescan <name>

# Or manually trigger a new ClusterScan
kubectl -n $NS create -f - <<EOF
apiVersion: clamav.io/v1alpha1
kind: ClusterScan
metadata:
  name: rescan-$(date +%s)
spec:
  priority: high
  concurrent: 3
EOF
```

---

### 10. EMERGENCY — Webhook certificate expired

**Symptoms:** All ClamAV CRD operations fail with:
```
Error: failed calling webhook "vnodescan.kb.io": x509: certificate has expired
```

#### With cert-manager (default from v0.5.0)

cert-manager handles rotation automatically. If it missed a renewal:

```bash
# Force cert-manager to renew immediately
kubectl -n $NS delete secret clamav-operator-webhook-server-cert

# Watch cert-manager recreate it (~30 seconds)
kubectl -n $NS get certificate -w

# Restart operator to load the new cert
kubectl -n $NS rollout restart deploy/clamav-operator
```

#### Without cert-manager (legacy / self-signed)

```bash
NS=clamav-system
RELEASE=clamav-operator

# 1. Generate new self-signed cert
openssl req -x509 -newkey rsa:4096 -keyout /tmp/tls.key -out /tmp/tls.crt \
  -days 365 -nodes \
  -subj "/CN=${RELEASE}-webhook-service" \
  -addext "subjectAltName=DNS:${RELEASE}-webhook-service,DNS:${RELEASE}-webhook-service.${NS}.svc,DNS:${RELEASE}-webhook-service.${NS}.svc.cluster.local"

# 2. Replace the webhook TLS secret
kubectl -n $NS create secret tls ${RELEASE}-webhook-server-cert \
  --cert=/tmp/tls.crt --key=/tmp/tls.key \
  --dry-run=client -o yaml | kubectl apply -f -

# 3. Update caBundle in the ValidatingWebhookConfiguration
CA_BUNDLE=$(base64 -w0 /tmp/tls.crt)
kubectl patch validatingwebhookconfiguration ${RELEASE}-validating-webhook-configuration \
  --type='json' \
  -p="[{\"op\":\"replace\",\"path\":\"/webhooks/0/clientConfig/caBundle\",\"value\":\"${CA_BUNDLE}\"}]"

# 4. Also update conversion webhook if CRD conversion is configured
kubectl patch crd nodescans.clamav.io \
  --type='json' \
  -p="[{\"op\":\"replace\",\"path\":\"/spec/conversion/webhook/clientConfig/caBundle\",\"value\":\"${CA_BUNDLE}\"}]"

# 5. Restart operator to load new cert
kubectl -n $NS rollout restart deploy/${RELEASE}

# 6. Verify
kubectl -n $NS get nodescans  # should work without TLS error

# 7. Cleanup
rm /tmp/tls.key /tmp/tls.crt
```

### 11. NodeScan stuck in Pending — Job name invalid (long FQDN nodes)

**Symptoms:** NodeScans for specific nodes stay permanently in `Pending`. No Job is ever created.
Operator logs show:

```
Job.batch "nodescan-...-ip-10-3-78-130.-1ca3be324f" is invalid:
metadata.name: Invalid value: "...-ip-10-3-78-130.-1ca3be324f":
a lowercase RFC 1123 subdomain must consist of lower case alphanumeric
characters, '-' or '.', and must start and end with an alphanumeric character
```

**Root cause:** The node has a long FQDN hostname. The operator truncates the
NodeScan name to 63 characters before appending a hash suffix. If the truncation
point lands on a `.` or `-`, the resulting Job name contains a sequence like
`130.-1ca3be324f` (dot followed by a hyphen-prefixed label), which violates RFC 1123.

Fixed in **v0.5.3** — upgrade resolves this permanently.

**Diagnosis:**
```bash
# Identify affected nodes (long hostnames, typically > 45 chars after "nodescan-")
kubectl get nodes -o custom-columns=NAME:.metadata.name | awk 'length() > 45'

# Confirm the error in operator logs
kubectl -n  logs -l control-plane=controller-manager --tail=200 | grep "RFC 1123\|Invalid value"

# Check stuck NodeScans
kubectl -n  get nodescan -o wide | grep Pending
```

**Workaround (pre-v0.5.3):** Force a fresh reconcile by deleting and recreating the stuck NodeScans
after upgrading to a fixed version. The NodeScans are recreated automatically by the ClusterScan
or ScanSchedule controller.

```bash
# Delete all stuck Pending NodeScans (they will be recreated by the operator)
kubectl -n  delete nodescan 
```

**Permanent fix:** Upgrade to v0.5.3+.

---

---

### 12. Results directory too large

**Symptoms:** `ClamAVResultsDirTooLarge` (> 500 MB) or `ClamAVCacheFileTooLarge` (> 100 MB) fires.

**Diagnosis:**
```bash
# Check metric values
curl -s localhost:8080/metrics | grep -E "clamav_results_dir_bytes|clamav_cache_file_bytes"

# On the node directly
du -sh /var/log/clamav-scans/
ls -lh /var/log/clamav-scans/
```

**Causes and fixes:**

| Cause | Fix |
|-------|-----|
| Too many reports retained | Lower `scanner.incremental.maxScanReports` (default 30) |
| Cache tracking millions of files | Normal for large nodes — check `clamav_scan_cache_files_total`. If excessive, verify pruning is working. |
| Old reports not being rotated | Verify `MAX_SCAN_REPORTS` env var is set on the scanner Job |

**Manual cleanup:**
```bash
# SSH onto the node and remove old reports (keep last 10)
ls -t /var/log/clamav-scans/*.json | tail -n +11 | xargs rm -f

# Delete the cache file to force a full scan rebuild
rm /var/log/clamav-scans/<node-name>_scan_cache.json
```

---

### 13. Low scan throughput

**Symptoms:** `ClamAVLowScanThroughput` fires — `clamav_scan_files_per_second < 5` for 30 min.

**Diagnosis:**
```bash
# Check current throughput per node
curl -s localhost:8080/metrics | grep clamav_scan_files_per_second

# Check OOMKill pressure (memory thrashing slows I/O)
curl -s localhost:8080/metrics | grep clamav_job_oom_kills_total

# Check scanner pod resource usage on the affected node
kubectl -n $NS get pod -l job-name=<scanner-job> -o wide
kubectl -n $NS top pod <scanner-pod>
```

**Common causes and fixes:**

| Cause | Fix |
|-------|-----|
| Node I/O saturation | Check node disk utilization; consider scheduling scans during off-peak hours |
| Memory pressure / OOMKill risk | Increase `scanner.resources.limits.memory` |
| CPU throttling | Increase `scanner.resources.limits.cpu` or lower CFS quota |
| Scanning very large files | Set `maxFileSize` to skip files above a threshold (e.g. `104857600` = 100 MB) |
| Network filesystem (NFS/Ceph) | Latency expected — adjust or exclude these paths in `ScanPolicy.spec.excludePaths` |

**Force a full scan to establish a new baseline:**
```bash
kubectl annotate nodescan <nodescan-name> clamav.io/force-full-scan=true
```

---

## Maintenance

### Upgrade the operator

```bash
# 1. ALWAYS update CRDs FIRST (helm upgrade does not update CRDs)
kubectl apply -f https://raw.githubusercontent.com/SolucTeam/clamav-operator/refs/heads/main/helm/clamav-operator/crds/clamav.io_nodescans.yaml
kubectl apply -f https://raw.githubusercontent.com/SolucTeam/clamav-operator/refs/heads/main/helm/clamav-operator/crds/clamav.io_clusterscans.yaml
kubectl apply -f https://raw.githubusercontent.com/SolucTeam/clamav-operator/refs/heads/main/helm/clamav-operator/crds/clamav.io_scanpolicies.yaml
kubectl apply -f https://raw.githubusercontent.com/SolucTeam/clamav-operator/refs/heads/main/helm/clamav-operator/crds/clamav.io_scanschedules.yaml

# Or from a local clone:
kubectl apply -f helm/clamav-operator/crds/

# 2. Upgrade the Helm release
helm upgrade clamav-operator oci://ghcr.io/solucteam/charts/clamav-operator \
  --version <new-version> \
  -n $NS \
  --reuse-values \
  --wait

# 3. Verify
kubectl rollout status deploy/clamav-operator -n $NS
helm test clamav-operator -n $NS
```

### CRD upgrade procedure

> ⚠️ `helm upgrade` does **not** update CRDs. This is a Helm design decision.  
> Always apply CRDs manually before upgrading the Helm release.

```bash
# Check currently installed CRD versions
kubectl get crd -o custom-columns='NAME:.metadata.name,VERSIONS:.spec.versions[*].name' | grep clamav

# Apply CRDs from local chart (or from git tag)
kubectl apply -f helm/clamav-operator/crds/

# Verify all CRDs are Established
kubectl get crd | grep clamav.io
# All should show: ESTABLISHED = True

# Verify existing resources still parse correctly
kubectl get nodescans,clusterscans,scanpolicies,scanschedules -n $NS
```

### Rollback procedure

```bash
# View Helm release history
helm history clamav-operator -n $NS

# Rollback to previous release
helm rollback clamav-operator -n $NS

# Rollback to a specific revision
helm rollback clamav-operator <revision> -n $NS

# Verify
kubectl rollout status deploy/clamav-operator -n $NS
```

> ⚠️ If rollback requires **downgrading CRDs** (rare):
> ```bash
> # Get CRDs from the old git tag
> git checkout v<old-version> -- helm/clamav-operator/crds/
> kubectl apply -f helm/clamav-operator/crds/
> git checkout HEAD -- helm/clamav-operator/crds/  # restore
> ```

### Rotate ClamAV signatures (air-gap)

See [airgap-signatures.md](./airgap-signatures.md).

### Scale operator for maintenance

```bash
# Scale down (stops all reconciliation — use only for emergency maintenance)
kubectl -n $NS scale deploy/<manager-name> --replicas=0

# Scale back up
kubectl -n $NS scale deploy/<manager-name> --replicas=2
```

---

## Escalation

| Situation | Contact |
|-----------|---------|
| Confirmed malware on a node | Security team immediately — cordon the node first |
| Operator unavailable > 15 min | Platform on-call |
| Freshclam unreachable > 48 h | Platform on-call — risk of missed detections |
