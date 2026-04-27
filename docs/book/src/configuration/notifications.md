# Notifications

Notifications are configured on the `ScanPolicy` resource under `spec.notifications`. All channels fire asynchronously — a slow or unavailable endpoint never blocks a scan.

Each channel supports `onlyOnInfection: true` (default) to suppress alerts for clean scans.

Failed notifications are retried up to 3 times with exponential backoff. Persistent failures increment the `clamav_notification_failures_total` metric and emit a Kubernetes Event on the NodeScan.

## Slack

```yaml
spec:
  notifications:
    slack:
      enabled: true
      channel: "#security-alerts"
      webhookSecretRef:
        name: slack-webhook
        key: url
      onlyOnInfection: true
```

```bash
kubectl create secret generic slack-webhook \
  --from-literal=url='https://hooks.slack.com/services/T.../B.../...' \
  -n clamav-system
```

## Email (SMTP/TLS)

```yaml
spec:
  notifications:
    email:
      enabled: true
      smtpServer: "smtp.example.com:465"
      from: "clamav@example.com"
      recipients:
        - "security@example.com"
      smtpAuthSecretRef:
        name: smtp-credentials
      onlyOnInfection: true
```

```bash
kubectl create secret generic smtp-credentials \
  --from-literal=username='clamav@example.com' \
  --from-literal=password='my-smtp-password' \
  -n clamav-system
```

## Generic Webhook

```yaml
spec:
  notifications:
    webhook:
      enabled: true
      url: "https://hooks.example.com/clamav"
      headers:
        Authorization: "Bearer my-token"
        X-Source: "clamav-operator"
      onlyOnInfection: false
```

## Microsoft Teams

The Teams notifier sends an **Adaptive Card (schema v1.2)** via an Incoming Webhook. Compatible with both legacy Office 365 Connector URLs and new Power Automate / Workflows webhook URLs (`https://prod-*.logic.azure.com/...`).

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

```bash
kubectl create secret generic teams-webhook-secret \
  --from-literal=url='https://prod-xx.westeurope.logic.azure.com/...' \
  -n clamav-system
```

## Testing Notifications

Trigger a test notification without waiting for a real infection:

```bash
# Create a NodeScan that targets a non-existent node — it will fail and
# (if onlyOnInfection: false) trigger the notification channel.
kubectl apply -f - <<EOF
apiVersion: clamav.io/v1alpha1
kind: NodeScan
metadata:
  name: notification-test
  namespace: clamav-system
spec:
  nodeName: test-node
  priority: low
EOF
```
