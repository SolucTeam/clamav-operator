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
4. [Maintenance](#maintenance)
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

| Alert | Meaning | Runbook section |
|-------|---------|----------------|
| `ClamAVNoRecentScans` | No node completed a scan in the last 24 h | [#1](#1-operator-not-reconciling), [#2](#2-scanner-job-stuck--never-completes) |
| `ClamAVInfectedFilesDetected` | `clamav_files_infected_total > 0` | [#3](#3-infected-file-detected) |
| `ClamAVScanJobFailed` | A scanner Job exited non-zero | [#2](#2-scanner-job-stuck--never-completes) |
| `ClamAVOperatorDown` | Operator deployment has 0 ready replicas | [#1](#1-operator-not-reconciling) |

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
| Job timeout | Adjust `spec.activeDeadlineSeconds` on the NodeScan or ClusterScan |

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

## Maintenance

### Upgrade the operator

```bash
helm upgrade clamav-operator oci://ghcr.io/solucteam/charts/clamav-operator \
  --version <new-version> \
  -n $NS \
  --reuse-values

# Verify
helm test clamav-operator -n $NS
```

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
