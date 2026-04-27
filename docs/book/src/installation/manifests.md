# Using Manifests

For environments without Helm or when using GitOps tools (ArgoCD, Flux), the operator can be installed via plain Kubernetes manifests published with each release.

## Install Latest Release

```bash
kubectl apply -f \
  https://github.com/SolucTeam/clamav-operator/releases/latest/download/install.yaml
```

## Install a Specific Version

```bash
VERSION=v0.4.1
kubectl apply -f \
  https://github.com/SolucTeam/clamav-operator/releases/download/${VERSION}/install.yaml
```

## What the Manifest Contains

The `install.yaml` bundle includes, in order:

1. CRD definitions (`NodeScan`, `ClusterScan`, `ScanSchedule`, `ScanPolicy`)
2. Namespace (`clamav-system`)
3. ServiceAccount, ClusterRole, ClusterRoleBinding
4. Deployment (operator)
5. Service (webhook + metrics)
6. ValidatingWebhookConfiguration
7. cert-manager `Certificate` and `Issuer` resources

## GitOps with ArgoCD

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: clamav-operator
  namespace: argocd
spec:
  source:
    repoURL: https://github.com/SolucTeam/clamav-operator
    targetRevision: v0.4.1
    path: helm/clamav-operator
    helm:
      releaseName: clamav-operator
      valueFiles:
        - values-production.yaml
  destination:
    server: https://kubernetes.default.svc
    namespace: clamav-system
  syncPolicy:
    automated:
      prune: false      # Never auto-delete CRDs
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
      - ServerSideApply=true
```
