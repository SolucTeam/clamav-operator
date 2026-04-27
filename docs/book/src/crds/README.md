# CRD Reference

The operator introduces four custom resources under the `clamav.io` API group.

| Resource | Version | Scope | Description |
|----------|---------|-------|-------------|
| [`NodeScan`](./nodescan.md) | `v1alpha1`, `v1beta1` | Namespaced | Scan a single node |
| [`ClusterScan`](./clusterscan.md) | `v1alpha1` | Namespaced | Scan all (or selected) nodes |
| [`ScanSchedule`](./scanschedule.md) | `v1beta1` | Namespaced | Recurring cluster scans on a cron schedule |
| [`ScanPolicy`](./scanpolicy.md) | `v1alpha1` | Namespaced | Reusable scan configuration |

## API Versioning

The storage version is **`v1beta1`**. `v1alpha1` objects are automatically converted via the conversion webhook.

```bash
# List all CRDs installed by the operator
kubectl get crd | grep clamav.io
```
