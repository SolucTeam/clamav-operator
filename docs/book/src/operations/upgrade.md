# Upgrade Guide

## Standard Upgrade

```bash
# 1. Check the release notes for breaking changes
# https://github.com/SolucTeam/clamav-operator/releases

# 2. Wait for in-flight scans to complete (optional but recommended)
kubectl -n clamav-system wait nodescan --all \
  --for=jsonpath='{.status.phase}'=Completed \
  --timeout=10m 2>/dev/null || true

# 3. Apply CRDs manually (Helm never upgrades existing CRDs)
kubectl apply --server-side -f \
  https://github.com/SolucTeam/clamav-operator/releases/latest/download/crds.yaml

# 4. Upgrade the Helm release
helm upgrade clamav-operator oci://ghcr.io/solucteam/charts/clamav-operator \
  --namespace clamav-system \
  --reuse-values \
  --wait

# 5. Verify
helm test clamav-operator -n clamav-system
kubectl -n clamav-system get pods
```

## CRD Migrations

When upgrading across minor versions, new fields may be added to the CRDs. The conversion webhook handles `v1alpha1 ↔ v1beta1` automatically.

> **⚠️ Warning:** Never run `helm upgrade` without first applying CRDs manually. Helm silently skips CRD upgrades to avoid accidental data loss, so your cluster will have stale schema definitions.

## Rollback

```bash
helm rollback clamav-operator -n clamav-system
```

To roll back CRDs, apply the previous version's CRD manifest:

```bash
VERSION=v0.4.0
kubectl apply --server-side -f \
  https://github.com/SolucTeam/clamav-operator/releases/download/${VERSION}/crds.yaml
```

## Version Matrix

| Operator | Kubernetes | cert-manager | Go |
|----------|-----------|-------------|-----|
| v0.4.x | 1.24 – 1.32 | ≥ 1.13 | 1.24 |
| v0.3.x | 1.24 – 1.30 | ≥ 1.12 | 1.23 |
