# ClamAV Operator — Air-Gap Signature Management

> **Audience:** SRE / Platform engineers operating in restricted or air-gapped environments  
> **Scope:** Building, deploying, and rotating ClamAV signatures when `database.clamav.net` is unreachable

---

## Table of Contents

1. [Overview](#overview)
2. [Disable Freshclam](#disable-freshclam)
3. [Build a Scanner Image with Baked-In Signatures](#build-a-scanner-image-with-baked-in-signatures)
4. [Deploy the Air-Gap Image](#deploy-the-air-gap-image)
5. [Rotate Signatures (Upgrade Cycle)](#rotate-signatures-upgrade-cycle)
6. [Validate Signature Currency](#validate-signature-currency)
7. [Automation Tips](#automation-tips)

---

## Overview

By default the operator uses a freshclam CronJob to pull updated signatures from `database.clamav.net`.
In air-gapped or restricted clusters this connection is unavailable, so signatures must be **baked into the scanner image** at build time and rotated by pushing a new image.

```
Air-gap model
─────────────────────────────────────────────────────────────
  Internet-connected build host
    ↓ docker build (downloads signatures)
    ↓ docker push → internal registry
  Air-gapped cluster
    ↓ scanner Job pulls from internal registry
    ↓ clamscan uses /var/lib/clamav/* from the image layer
─────────────────────────────────────────────────────────────
```

---

## Disable Freshclam

Set `scanner.freshclam.enabled=false` so the operator does not create the freshclam CronJob (it would fail in an air-gapped environment and pollute your logs with errors).

```yaml
# In your values override file
scanner:
  freshclam:
    enabled: false
  signatures:
    persistent: false   # No PVC needed — signatures live inside the image
```

Apply the change:

```bash
helm upgrade clamav-operator oci://ghcr.io/solucteam/charts/clamav-operator \
  -n clamav-system \
  -f my-airgap-values.yaml \
  --reuse-values
```

Verify the CronJob is gone:

```bash
kubectl -n clamav-system get cronjob
# Expected: No resources found.
```

---

## Build a Scanner Image with Baked-In Signatures

The scanner `Dockerfile` supports a `DOWNLOAD_SIGS` build argument. When set to `true` the build stage runs `freshclam` to download the latest signature database into the image layer.

**Requirements on the build host:**
- Docker (or Buildah/Podman)
- Internet access to `database.clamav.net`

```bash
# Clone the repository (or use your internal mirror)
git clone https://github.com/SolucTeam/clamav-operator.git
cd clamav-operator/scanner

# Build with signatures baked in
# Tag with the signature date for traceability (YYYY-MM-DD)
SIG_DATE=$(date +%Y-%m-%d)
IMAGE=registry.internal.example.com/clamav-node-scanner:${SIG_DATE}

docker build \
  --build-arg DOWNLOAD_SIGS=true \
  -t "${IMAGE}" \
  -f Dockerfile .

# Verify signatures were downloaded
docker run --rm "${IMAGE}" clamscan --version
# Expected output includes: ClamAV 1.x.x / <database date>

# Push to your internal registry
docker push "${IMAGE}"
```

> **Tip:** Also build and push the scanner image without `DOWNLOAD_SIGS=true` as a base image for faster subsequent builds (use `--cache-from`).

---

## Deploy the Air-Gap Image

Point the operator at your internal image by setting `scanner.image` in your values:

```yaml
# airgap-values.yaml
scanner:
  image:
    repository: registry.internal.example.com/clamav-node-scanner
    tag: "2024-11-15"          # pin to the dated tag you just built
    pullPolicy: IfNotPresent

  freshclam:
    enabled: false

  signatures:
    persistent: false
```

```bash
helm upgrade clamav-operator oci://ghcr.io/solucteam/charts/clamav-operator \
  -n clamav-system \
  -f airgap-values.yaml \
  --reuse-values
```

Trigger a test scan to confirm the image works:

```bash
kubectl apply -f - <<EOF
apiVersion: clamav.io/v1alpha1
kind: ClusterScan
metadata:
  name: airgap-validation
  namespace: clamav-system
spec:
  concurrent: 1
EOF

kubectl -n clamav-system get nodescans -w
```

---

## Rotate Signatures (Upgrade Cycle)

ClamAV releases signature database updates multiple times per day. In an air-gapped environment you decide the rotation cadence — weekly is a reasonable baseline; daily is better for high-security environments.

### Step 1 — Build a new dated image (on your internet-connected build host)

```bash
SIG_DATE=$(date +%Y-%m-%d)
IMAGE=registry.internal.example.com/clamav-node-scanner:${SIG_DATE}

docker build \
  --build-arg DOWNLOAD_SIGS=true \
  -t "${IMAGE}" \
  scanner/

docker push "${IMAGE}"
```

### Step 2 — Update the tag in your values override

```bash
# In-place edit (or update your GitOps manifest)
sed -i "s/tag: \".*\"/tag: \"${SIG_DATE}\"/" airgap-values.yaml
```

### Step 3 — Roll out the new image

```bash
helm upgrade clamav-operator oci://ghcr.io/solucteam/charts/clamav-operator \
  -n clamav-system \
  -f airgap-values.yaml \
  --reuse-values
```

New scanner Jobs will use the updated image immediately. In-progress scans are unaffected — they continue with the old image until they complete naturally.

### Step 4 — Verify

```bash
# Confirm scanner pods are using the new image
kubectl -n clamav-system get pods -l app.kubernetes.io/managed-by=clamav-operator \
  -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.containers[0].image}{"\n"}{end}'

# Check that signatures are current inside a running scanner pod
kubectl -n clamav-system exec -it <scanner-pod> -- clamscan --version
```

---

## Validate Signature Currency

Use this one-liner to check the signature database date embedded in any running scanner pod:

```bash
kubectl -n clamav-system exec \
  $(kubectl -n clamav-system get pod -l app.kubernetes.io/managed-by=clamav-operator \
    -o jsonpath='{.items[0].metadata.name}') \
  -- clamscan --version
```

Expected output:

```
ClamAV 1.4.1/27489/Fri Nov 15 10:22:04 2024
               ↑        ↑
         signature #  build date
```

If the build date is older than your policy threshold (e.g. > 7 days), trigger a rotation.

### Prometheus alert for stale signatures

If you have Prometheus, add this alert to catch rotations that were missed:

```yaml
# In your PrometheusRule or Alertmanager rules
groups:
  - name: clamav-airgap
    rules:
      - alert: ClamAVSignaturesStale
        expr: |
          (time() - clamav_nodescan_last_completion_timestamp) > 86400
          and on()
          absent(kube_cronjob_info{cronjob=~".*freshclam.*"})
        for: 1h
        labels:
          severity: warning
        annotations:
          summary: "ClamAV signatures may be stale (air-gap mode, no freshclam)"
          description: "No completed scan in the last 24 h and freshclam CronJob is absent. Verify signature rotation."
```

---

## Automation Tips

### GitOps / CI pipeline

Integrate signature rotation into your internal CI system:

```yaml
# Example: weekly GitOps rotation job
schedule: "0 3 * * 1"   # Every Monday at 03:00
steps:
  - Build dated image with DOWNLOAD_SIGS=true
  - Push to internal registry
  - Open a PR updating scanner.image.tag in airgap-values.yaml
  - Auto-merge if all tests pass
  - ArgoCD / Flux syncs the Helm release automatically
```

### Keeping multiple versions

Tag images with both a date tag and `latest` to simplify rollback:

```bash
docker tag "${IMAGE}" registry.internal.example.com/clamav-node-scanner:latest
docker push registry.internal.example.com/clamav-node-scanner:latest
```

Use the dated tag in production (`values.yaml`) so rollback is a single tag change; use `latest` only in staging.

### Emergency rollback

If a new signature database causes false positives or scanner crashes:

```bash
# Roll back to the previous dated image
helm upgrade clamav-operator oci://ghcr.io/solucteam/charts/clamav-operator \
  -n clamav-system \
  --set scanner.image.tag=2024-11-08 \   # previous known-good date
  --reuse-values
```
