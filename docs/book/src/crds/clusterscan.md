# ClusterScan

A `ClusterScan` scans all (or a selected subset of) nodes in the cluster. It creates one `NodeScan` per matching node, respecting the `concurrent` concurrency limit.

## Example

```yaml
apiVersion: clamav.io/v1alpha1
kind: ClusterScan
metadata:
  name: full-cluster-scan
  namespace: clamav-system
spec:
  concurrent: 3
  priority: low
  nodeSelector:
    matchLabels:
      node-role.kubernetes.io/worker: ""
  nodeScanTemplate:
    strategy: smart
    paths:
      - /var/lib
      - /opt
    incrementalConfig:
      fullScanInterval: 7
```

## Spec Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `concurrent` | int32 | `3` | Max NodeScans running simultaneously |
| `priority` | string | `default` | Passed to each NodeScan |
| `scanPolicy` | string | — | Name of a `ScanPolicy` to inherit config from |
| `nodeSelector` | LabelSelector | — | Only scan matching nodes; omit to scan all nodes |
| `nodeScanTemplate` | NodeScanSpec | — | Template applied to each created NodeScan |

## Status Fields

| Field | Description |
|-------|-------------|
| `phase` | `Pending`, `Running`, `Completed`, or `Failed` |
| `totalNodes` | Total nodes selected |
| `completedNodes` | Nodes that reached a terminal phase |
| `failedNodes` | Nodes whose scan failed |
| `filesScanned` | Aggregate files scanned across all nodes |
| `filesInfected` | Aggregate infections found |

## NodeScan Ownership

Each NodeScan created by a ClusterScan is owned by it (via `ownerReference`). Deleting the ClusterScan cascades to all its NodeScans and their Jobs.

```bash
# List NodeScans belonging to a ClusterScan
kubectl -n clamav-system get nodescans \
  -l clamav.io/clusterscan=full-cluster-scan
```
