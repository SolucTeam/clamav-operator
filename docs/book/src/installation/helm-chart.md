# Using Helm Charts

> **Warning:** The `--wait` flag is required when installing the operator with providers. Without it the webhook TLS setup may not be complete before the first reconcile runs.

## Basic Install

```bash
helm install clamav-operator oci://ghcr.io/solucteam/charts/clamav-operator \
  --namespace clamav-system \
  --create-namespace \
  --wait
```

## Install with Custom Values

Create a `values-override.yaml`:

```yaml
replicaCount: 2

webhook:
  certManager:
    enabled: true   # default — uses cert-manager for TLS

scanner:
  image:
    repository: ghcr.io/solucteam/clamav-node-scanner
    tag: latest

monitoring:
  serviceMonitor:
    enabled: true
  prometheusRule:
    enabled: true
```

Then install:

```bash
helm install clamav-operator oci://ghcr.io/solucteam/charts/clamav-operator \
  --namespace clamav-system \
  --create-namespace \
  -f values-override.yaml \
  --wait
```

## Upgrade

```bash
# 1. Wait for existing NodeScans to complete (optional but recommended)
kubectl -n clamav-system wait nodescan --all --for=condition=Complete --timeout=5m

# 2. Upgrade CRDs first (Helm does not upgrade CRDs automatically)
kubectl apply --server-side -f \
  https://github.com/SolucTeam/clamav-operator/releases/latest/download/crds.yaml

# 3. Upgrade the release
helm upgrade clamav-operator oci://ghcr.io/solucteam/charts/clamav-operator \
  --namespace clamav-system \
  --reuse-values \
  --wait

# 4. Run the Helm test suite to verify
helm test clamav-operator -n clamav-system
```

> **CRD Warning:** Helm never upgrades CRDs that already exist in the cluster (to prevent accidental data loss). Always apply CRDs manually before `helm upgrade` when upgrading across minor versions.

## Uninstall

```bash
helm uninstall clamav-operator -n clamav-system
# CRDs are NOT deleted (helm.sh/resource-policy: keep)
# Delete manually if desired:
kubectl delete crd nodescans.clamav.io clusterscans.clamav.io scanschedules.clamav.io scanpolicies.clamav.io
```

## Available Values

Refer to the [Helm chart README](https://github.com/SolucTeam/clamav-operator/blob/main/helm/clamav-operator/README.md) for the full list of configurable values.
