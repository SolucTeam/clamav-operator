# ClamAV Operator

Kubernetes Operator pour gérer les scans antivirus ClamAV sur les clusters Kubernetes.

## Description

Le ClamAV Operator permet de :
- **Scanner des nodes individuels** via la ressource `NodeScan`
- **Scanner tout le cluster** via la ressource `ClusterScan`
- **Définir des politiques de scan** réutilisables via `ScanPolicy`
- **Planifier des scans automatiques** via `ScanSchedule`

## Fonctionnalités

✅ API Kubernetes native avec Custom Resource Definitions (CRDs)
✅ Scans parallèles avec contrôle de concurrence
✅ Politiques de scan réutilisables
✅ Planification automatique (cron)
✅ Notifications (Slack, Email, Webhook)
✅ Métriques Prometheus
✅ Events Kubernetes
✅ Validation via webhooks

## Installation

### Prérequis

- Kubernetes 1.24+
- ClamAV déployé dans le cluster (service disponible)
- kubectl configuré

### Déploiement rapide

```bash
# Installer les CRDs
kubectl apply -f https://raw.githubusercontent.com/.../clamav-operator/config/crd/bases/clamav.platform.numspot.com_nodescans.yaml
kubectl apply -f https://raw.githubusercontent.com/.../clamav-operator/config/crd/bases/clamav.platform.numspot.com_clusterscans.yaml
kubectl apply -f https://raw.githubusercontent.com/.../clamav-operator/config/crd/bases/clamav.platform.numspot.com_scanpolicies.yaml
kubectl apply -f https://raw.githubusercontent.com/.../clamav-operator/config/crd/bases/clamav.platform.numspot.com_scanschedules.yaml

# Déployer l'operator
kubectl apply -f dist/install.yaml
```

### Build depuis les sources

```bash
# Cloner le repository
git clone https://gitlab.../platform-iac/clamav-operator.git
cd clamav-operator

# Générer les manifests
make manifests

# Build l'image Docker
make docker-build IMG=registry.example.com/clamav-operator:latest

# Push l'image
make docker-push IMG=registry.example.com/clamav-operator:latest

# Déployer
make deploy IMG=registry.example.com/clamav-operator:latest
```

## Usage

### Scanner un node spécifique

```yaml
apiVersion: clamav.platform.numspot.com/v1alpha1
kind: NodeScan
metadata:
  name: scan-worker-01
  namespace: clamav
spec:
  nodeName: worker-01
  priority: high
  maxConcurrent: 10
  paths:
    - /var/lib
    - /opt
```

```bash
kubectl apply -f nodescan.yaml
kubectl get nodescan -n clamav
kubectl describe nodescan scan-worker-01 -n clamav
```

### Scanner tout le cluster

```yaml
apiVersion: clamav.platform.numspot.com/v1alpha1
kind: ClusterScan
metadata:
  name: nightly-scan
  namespace: clamav
spec:
  nodeSelector:
    matchLabels:
      node-role.kubernetes.io/worker: ""
  scanPolicy: production-policy
  concurrent: 3
```

### Créer une politique de scan

```yaml
apiVersion: clamav.platform.numspot.com/v1alpha1
kind: ScanPolicy
metadata:
  name: production-policy
  namespace: clamav
spec:
  paths:
    - /var/lib
    - /opt
    - /usr/local
  
  excludePatterns:
    - "*.tmp"
    - "/var/lib/docker/overlay2/*"
  
  maxConcurrent: 5
  fileTimeout: 300000
  maxFileSize: 524288000
  
  resources:
    requests:
      cpu: 500m
      memory: 512Mi
    limits:
      cpu: 2000m
      memory: 1Gi
  
  notifications:
    slack:
      enabled: true
      webhookSecretRef:
        name: slack-webhook
        key: url
      channel: "#security-alerts"
```

### Planifier des scans automatiques

```yaml
apiVersion: clamav.platform.numspot.com/v1alpha1
kind: ScanSchedule
metadata:
  name: daily-full-scan
  namespace: clamav
spec:
  schedule: "0 2 * * *"  # Tous les jours à 2h
  
  clusterScan:
    nodeSelector:
      matchLabels:
        node-role.kubernetes.io/worker: ""
    scanPolicy: production-policy
    concurrent: 2
  
  successfulScansHistoryLimit: 10
  failedScansHistoryLimit: 3
```

## Monitoring

### Métriques Prometheus

L'operator expose automatiquement des métriques :

```promql
# Nombre de scans en cours
clamav_nodescan_running

# Fichiers infectés détectés
clamav_files_infected_total

# Durée des scans
clamav_scan_duration_seconds
```

### Dashboards Grafana

Des dashboards pré-configurés sont disponibles dans le répertoire `config/grafana/`.

### Logs

Les logs de l'operator sont structurés en JSON :

```bash
kubectl logs -n clamav-system deployment/clamav-operator-controller-manager -f
```

## Développement

### Setup

```bash
# Installer les dépendances
go mod download

# Générer le code
make generate

# Lancer les tests
make test

# Lancer l'operator localement
make run
```

### Contribuer

Voir [CONTRIBUTING.md](CONTRIBUTING.md) pour les détails.

## Architecture

```
┌─────────────────────────────────────────┐
│         ClamAV Operator                  │
│  ┌───────────────────────────────────┐   │
│  │  Controllers                       │   │
│  │  - NodeScan Controller             │   │
│  │  - ClusterScan Controller          │   │
│  │  - ScanPolicy Controller           │   │
│  │  - ScanSchedule Controller         │   │
│  └───────────────────────────────────┘   │
└─────────────────────────────────────────┘
                    │
                    ▼
        ┌───────────────────────┐
        │   Kubernetes API       │
        │   - CRDs               │
        │   - Jobs               │
        │   - Nodes              │
        └───────────────────────┘
                    │
                    ▼
        ┌───────────────────────┐
        │   Scanner Jobs         │
        │   (per node)           │
        └───────────────────────┘
                    │
                    ▼
        ┌───────────────────────┐
        │   ClamAV Service       │
        └───────────────────────┘
```

## License

Copyright 2025 Platform Team - Numspot.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

## Support

Pour toute question ou problème :
- 🐛 [Issues](https://github.com/SolucTeam/clamav-operator/issues)
- 💬 Slack : `#platform-security`
- 📧 Email : platform-team@numspot.com
