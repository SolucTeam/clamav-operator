# Network Policies

When `networkPolicy.enabled=true` (default), the Helm chart creates NetworkPolicy objects that restrict ingress and egress for both the operator and the scanner Jobs.

## Operator Pod

| Direction | Allowed |
|-----------|---------|
| Ingress | Kubernetes API server (webhook calls on port 9443) |
| Ingress | Prometheus scrape on port 8080 |
| Egress | Kubernetes API server |
| Egress | Notification endpoints (Slack, SMTP, Teams, Webhook) |
| Egress | freshclam to `database.clamav.net:443` (if freshclam enabled) |

## Scanner Job Pods

| Direction | Allowed |
|-----------|---------|
| Egress | Kubernetes API server (to write NodeScan status) |
| Egress | `clamd` service on port 3310 (remote mode only) |
| All other | Denied |

## Configuring CIDRs

The default values use open CIDRs (`0.0.0.0/0`). **Tighten these for production:**

```yaml
networkPolicy:
  enabled: true
  controlPlaneCIDR: "10.0.0.0/16"   # your API server CIDR
  nodesCIDR: "10.1.0.0/16"          # your node CIDR
  egressRules:
    - ports:
        - port: 443
          protocol: TCP
      # freshclam + notification SaaS endpoints
```

## Air-Gap / No Egress

In air-gapped environments, disable freshclam and restrict all external egress:

```yaml
scanner:
  freshclam:
    enabled: false

networkPolicy:
  enabled: true
  egressRules: []   # deny all external egress from scanner pods
```
