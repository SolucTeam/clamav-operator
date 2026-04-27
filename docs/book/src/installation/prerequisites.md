# Prerequisites

## Kubernetes

- Version **1.24 or later**
- Cluster-admin access (the operator installs ClusterRoles and ValidatingWebhookConfigurations)

## cert-manager

The operator's admission and conversion webhooks require TLS certificates. cert-manager manages the full CA chain automatically.

Install cert-manager if not already present:

```bash
helm repo add jetstack https://charts.jetstack.io
helm repo update
helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true \
  --wait
```

Minimum version: **cert-manager 1.13**.

> **Note:** You can disable cert-manager integration with `webhook.certManager.enabled=false` and supply your own certificates, but this is not recommended for production.

## Helm

Version **3.x** or later. The operator is distributed as an OCI Helm chart.

```bash
helm version
# version.BuildInfo{Version:"v3.x.x", ...}
```

## Container Registry Access

The operator and scanner images are hosted on GitHub Container Registry (`ghcr.io`). In air-gapped environments, mirror them to your internal registry first — see the [Air-Gap guide](../configuration/airgap.md).

| Image | Default tag |
|-------|-------------|
| `ghcr.io/solucteam/clamav-operator` | `latest` |
| `ghcr.io/solucteam/clamav-node-scanner` | `latest` |
| `ghcr.io/solucteam/clamav-node-scanner` | `latest-airgap` |
