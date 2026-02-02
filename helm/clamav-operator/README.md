# ClamAV Operator Helm Chart

Helm chart pour déployer le ClamAV Operator dans Kubernetes.

## Prérequis

- Kubernetes 1.24+
- Helm 3.0+
- ClamAV déployé et accessible dans le cluster

## Installation

### Ajouter le repository Helm (optionnel si vous avez un registry Helm)

```bash
helm repo add clamav-operator https://charts.numspot.com
helm repo update
```

### Installer le chart

```bash
# Installation simple avec valeurs par défaut
helm install clamav-operator clamav-operator/clamav-operator \
  --namespace clamav-system \
  --create-namespace

# Installation avec fichier de valeurs personnalisé
helm install clamav-operator clamav-operator/clamav-operator \
  --namespace clamav-system \
  --create-namespace \
  --values my-values.yaml

# Installation avec paramètres en ligne
helm install clamav-operator clamav-operator/clamav-operator \
  --namespace clamav-system \
  --create-namespace \
  --set operator.image.tag=v1.0.0 \
  --set scanner.clamav.host=clamav.clamav.svc.cluster.local
```

### Installation depuis les sources locales

```bash
cd helm/clamav-operator
helm install clamav-operator . \
  --namespace clamav-system \
  --create-namespace
```

## Configuration

### Paramètres principaux

| Paramètre | Description | Valeur par défaut |
|-----------|-------------|-------------------|
| `operator.replicaCount` | Nombre de replicas de l'operator | `1` |
| `operator.image.repository` | Repository de l'image operator | `registry.../clamav-operator` |
| `operator.image.tag` | Tag de l'image operator | `""` (chart version) |
| `operator.resources.limits.cpu` | CPU limit operator | `500m` |
| `operator.resources.limits.memory` | Memory limit operator | `256Mi` |
| `scanner.image.repository` | Repository de l'image scanner | `registry.../clamav-node-scanner` |
| `scanner.image.tag` | Tag de l'image scanner | `1.0.3` |
| `scanner.clamav.host` | Host du service ClamAV | `clamav.clamav.svc.cluster.local` |
| `scanner.clamav.port` | Port du service ClamAV | `3310` |
| `crds.install` | Installer les CRDs | `true` |
| `crds.keep` | Garder les CRDs lors du uninstall | `true` |
| `rbac.create` | Créer les ressources RBAC | `true` |
| `monitoring.serviceMonitor.enabled` | Activer ServiceMonitor Prometheus | `true` |
| `monitoring.prometheusRule.enabled` | Activer PrometheusRule | `true` |
| `defaultScanPolicy.enabled` | Créer une ScanPolicy par défaut | `true` |

### Exemple de fichier values personnalisé

```yaml
# my-values.yaml

operator:
  replicaCount: 2
  image:
    tag: v1.0.0
  resources:
    limits:
      cpu: 1000m
      memory: 512Mi
    requests:
      cpu: 200m
      memory: 256Mi

scanner:
  clamav:
    host: clamav.prod.svc.cluster.local
    port: 3310
  resources:
    limits:
      cpu: 4000m
      memory: 2Gi

monitoring:
  serviceMonitor:
    enabled: true
    interval: 60s
  prometheusRule:
    enabled: true

defaultScanPolicy:
  enabled: true
  spec:
    paths:
      - /var/lib
      - /opt
      - /usr/local
    excludePatterns:
      - "*.tmp"
      - "/var/lib/docker/*"
    maxConcurrent: 10
```

## Utilisation après installation

### Vérifier le déploiement

```bash
# Vérifier que l'operator est running
kubectl get pods -n clamav-system

# Vérifier les CRDs
kubectl get crd | grep clamav

# Vérifier les logs
kubectl logs -n clamav-system deployment/clamav-operator-controller-manager -f
```

### Scanner un node

```bash
kubectl apply -f - <<EOF
apiVersion: clamav.platform.numspot.com/v1alpha1
kind: NodeScan
metadata:
  name: scan-worker-01
  namespace: clamav-system
spec:
  nodeName: worker-01
  scanPolicy: default-policy
  priority: high
EOF

# Surveiller le scan
kubectl get nodescan -n clamav-system -w
```

### Scanner tout le cluster

```bash
kubectl apply -f - <<EOF
apiVersion: clamav.platform.numspot.com/v1alpha1
kind: ClusterScan
metadata:
  name: full-cluster-scan
  namespace: clamav-system
spec:
  nodeSelector:
    matchLabels:
      node-role.kubernetes.io/worker: ""
  scanPolicy: default-policy
  concurrent: 3
EOF
```

## Mise à jour

```bash
# Mise à jour avec nouvelle version
helm upgrade clamav-operator clamav-operator/clamav-operator \
  --namespace clamav-system \
  --values my-values.yaml

# Mise à jour avec nouveau tag d'image
helm upgrade clamav-operator clamav-operator/clamav-operator \
  --namespace clamav-system \
  --reuse-values \
  --set operator.image.tag=v1.1.0
```

## Désinstallation

```bash
# Désinstaller le chart (garde les CRDs par défaut)
helm uninstall clamav-operator --namespace clamav-system

# Supprimer les CRDs manuellement si nécessaire
kubectl delete crd nodescans.clamav.platform.numspot.com
kubectl delete crd clusterscans.clamav.platform.numspot.com
kubectl delete crd scanpolicies.clamav.platform.numspot.com
kubectl delete crd scanschedules.clamav.platform.numspot.com
```

## Monitoring

Le chart crée automatiquement :
- Un **ServiceMonitor** pour Prometheus Operator
- Des **PrometheusRules** avec alertes pré-configurées

### Métriques disponibles

```promql
# Scans en cours
clamav_nodescan_running

# Fichiers infectés
sum(clamav_files_infected_total)

# Durée des scans
avg(clamav_scan_duration_seconds)
```

### Alertes pré-configurées

- **ClamAVMalwareDetected** - Malware détecté
- **ClamAVScanFailed** - Scan échoué
- **ClamAVNoRecentScans** - Pas de scan récent

## Troubleshooting

### L'operator ne démarre pas

```bash
# Vérifier les events
kubectl get events -n clamav-system --sort-by='.lastTimestamp'

# Vérifier les logs
kubectl logs -n clamav-system deployment/clamav-operator-controller-manager

# Vérifier RBAC
kubectl auth can-i --list --as=system:serviceaccount:clamav-system:clamav-operator
```

### Les scans ne se créent pas

```bash
# Vérifier que le ServiceAccount scanner existe
kubectl get sa -n clamav-system clamav-scanner

# Vérifier les permissions
kubectl auth can-i create jobs --as=system:serviceaccount:clamav-system:clamav-scanner
```

### Webhooks ne fonctionnent pas

```bash
# Vérifier les certificats
kubectl get secret -n clamav-system clamav-operator-webhook-server-cert

# Désactiver temporairement
helm upgrade clamav-operator clamav-operator/clamav-operator \
  --namespace clamav-system \
  --reuse-values \
  --set webhook.enabled=false
```

## Support

- Documentation : https://docs.clamav-operator.io
- Issues : https://gitlab.../platform-iac/clamav-operator/-/issues
- Slack : #clamav-operator

## License

Apache License 2.0
