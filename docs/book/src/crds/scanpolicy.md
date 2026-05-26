# ScanPolicy

A `ScanPolicy` is a reusable configuration object. `NodeScan`, `ClusterScan`, and `ScanSchedule` can reference a policy by name via `spec.scanPolicy`. Fields set directly on the scan resource take precedence over the policy.

## Example

```yaml
apiVersion: clamav.io/v1alpha1
kind: ScanPolicy
metadata:
  name: production-policy
  namespace: clamav-system
spec:
  paths:
    - /var/lib
    - /opt/app
  excludePatterns:
    - ".*\\.log$"
    - ".*/tmp/.*"
  maxConcurrent: 5
  fileTimeout: 30000
  maxFileSize: 104857600
  resources:
    requests: { cpu: 200m, memory: 256Mi }
    limits:   { cpu: 1000m, memory: 1Gi }
  notifications:
    slack:
      enabled: true
      channel: "#security-alerts"
      webhookSecretRef: { name: slack-webhook, key: url }
      onlyOnInfection: true
    teams:
      enabled: true
      webhookSecretRef: { name: teams-webhook-secret, key: url }
      onlyOnInfection: true
```

## Spec Fields

| Field | Type | Description |
|-------|------|-------------|
| `paths` | []string | Paths to scan |
| `excludePatterns` | []string | Regex exclusion patterns |
| `maxConcurrent` | int32 | **Remote mode only** — concurrent clamd TCP connections. Ignored in standalone mode. |
| `fileTimeout` | int64 | Per-file timeout in milliseconds |
| `maxFileSize` | int64 | Max file size in bytes |
| `resources` | ResourceRequirements | CPU/memory for the scanner Job |
| `notifications` | NotificationConfig | Slack, Email, Webhook, Teams settings |

## Referencing a Policy

```yaml
apiVersion: clamav.io/v1alpha1
kind: ClusterScan
metadata:
  name: guided-scan
  namespace: clamav-system
spec:
  concurrent: 3
  scanPolicy: production-policy   # ← reference
```

Fields defined on the `ClusterScan` (or its `nodeScanTemplate`) override the policy.
