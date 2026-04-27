# Quickstart

This guide gets the operator running and executes a cluster-wide scan in under 5 minutes.

## Prerequisites

- Kubernetes 1.24+
- Helm 3.x
- cert-manager ≥ 1.13 installed in the cluster ([install guide](https://cert-manager.io/docs/installation/helm/))

## 1 — Install the Operator

```bash
helm install clamav-operator oci://ghcr.io/solucteam/charts/clamav-operator \
  --namespace clamav-system \
  --create-namespace \
  --wait
```

Verify the operator is running:

```bash
kubectl -n clamav-system get pods
# NAME                              READY   STATUS    RESTARTS   AGE
# clamav-operator-xxxxxxxxx-xxxxx   1/1     Running   0          30s
```

## 2 — Run a Cluster Scan

```bash
kubectl apply -f - <<EOF
apiVersion: clamav.io/v1alpha1
kind: ClusterScan
metadata:
  name: my-first-scan
  namespace: clamav-system
spec:
  concurrent: 2
  priority: low
EOF
```

Watch progress:

```bash
kubectl -n clamav-system get nodescans -w
```

## 3 — Check Results

```bash
kubectl -n clamav-system get clusterscan my-first-scan -o yaml
```

Key fields in the status:

```yaml
status:
  phase: Completed
  totalNodes: 3
  completedNodes: 3
  filesScanned: 142857
  filesInfected: 0
```

## Next Steps

- Schedule recurring scans with [ScanSchedule](../crds/scanschedule.md)
- Tune scan paths and exclusions with [ScanPolicy](../crds/scanpolicy.md)
- Configure [Notifications](../configuration/notifications.md) for Slack / Teams / Email
- Set up [Monitoring](../operations/monitoring.md) with Prometheus alerts
