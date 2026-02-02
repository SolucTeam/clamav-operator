# Guide de Déploiement ClamAV Operator

## 📋 Table des Matières

1. [Prérequis](#prérequis)
2. [Installation](#installation)
3. [Configuration](#configuration)
4. [Exemples d'utilisation](#exemples-dutilisation)
5. [Troubleshooting](#troubleshooting)

## Prérequis

### Cluster Kubernetes

- Kubernetes 1.24+
- kubectl configuré
- Accès admin au cluster

### ClamAV Service

ClamAV doit être déployé et accessible :

```bash
# Vérifier que ClamAV est disponible
kubectl get svc -n clamav clamav

# Tester la connectivité
kubectl run -it --rm debug --image=busybox --restart=Never -- \
  nc -zv clamav.clamav.svc.cluster.local 3310
```

## Installation

### Étape 1 : Créer le namespace

```bash
kubectl create namespace clamav-system
```

### Étape 2 : Installer les CRDs

```bash
# Depuis le repository local
kubectl apply -k config/crd

# Ou depuis les fichiers individuels
kubectl apply -f config/crd/bases/clamav.platform.numspot.com_nodescans.yaml
kubectl apply -f config/crd/bases/clamav.platform.numspot.com_clusterscans.yaml
kubectl apply -f config/crd/bases/clamav.platform.numspot.com_scanpolicies.yaml
kubectl apply -f config/crd/bases/clamav.platform.numspot.com_scanschedules.yaml
```

### Étape 3 : Créer le ServiceAccount et RBAC

```bash
kubectl apply -f config/rbac/service_account.yaml
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/rbac/role_binding.yaml
kubectl apply -f config/rbac/leader_election_role.yaml
kubectl apply -f config/rbac/leader_election_role_binding.yaml
```

### Étape 4 : Déployer l'Operator

```bash
# Méthode 1 : Via Kustomize (recommandé)
make deploy IMG=registry.tooling.cloudgouv-eu-west-1.numspot.cloud/platform-iac/clamav-operator:latest

# Méthode 2 : Fichier all-in-one
kubectl apply -f dist/install.yaml

# Méthode 3 : Manifests individuels
kubectl apply -f config/manager/manager.yaml
```

### Étape 5 : Vérifier le déploiement

```bash
# Vérifier que l'operator est running
kubectl get pods -n clamav-system

# Vérifier les logs
kubectl logs -n clamav-system deployment/clamav-operator-controller-manager -f

# Vérifier les CRDs
kubectl get crd | grep clamav
```

## Configuration

### ServiceAccount pour les Scans

Les jobs de scan nécessitent un ServiceAccount avec permissions :

```bash
kubectl apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: clamav-scanner
  namespace: clamav
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: clamav-scanner-role
rules:
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["batch"]
    resources: ["jobs"]
    verbs: ["create", "get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: clamav-scanner-binding
subjects:
  - kind: ServiceAccount
    name: clamav-scanner
    namespace: clamav
roleRef:
  kind: ClusterRole
  name: clamav-scanner-role
  apiGroup: rbac.authorization.k8s.io
EOF
```

### ImagePullSecret

Si votre registry est privé :

```bash
kubectl create secret docker-registry numspot-registry \
  --docker-server=registry.tooling.cloudgouv-eu-west-1.numspot.cloud \
  --docker-username=YOUR_USERNAME \
  --docker-password=YOUR_PASSWORD \
  --namespace=clamav
```

## Exemples d'utilisation

### 1. Créer une ScanPolicy

```bash
kubectl apply -f - <<EOF
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
    - "*.log"
    - "/var/lib/docker/overlay2/*"
    - "/var/lib/containerd/*"
  
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
EOF
```

### 2. Scanner un node

```bash
kubectl apply -f - <<EOF
apiVersion: clamav.platform.numspot.com/v1alpha1
kind: NodeScan
metadata:
  name: scan-worker-01
  namespace: clamav
spec:
  nodeName: worker-01
  scanPolicy: production-policy
  priority: high
EOF

# Surveiller le scan
kubectl get nodescan scan-worker-01 -n clamav -w

# Voir les détails
kubectl describe nodescan scan-worker-01 -n clamav
```

### 3. Scanner tout le cluster

```bash
kubectl apply -f - <<EOF
apiVersion: clamav.platform.numspot.com/v1alpha1
kind: ClusterScan
metadata:
  name: full-cluster-scan
  namespace: clamav
spec:
  nodeSelector:
    matchLabels:
      node-role.kubernetes.io/worker: ""
  scanPolicy: production-policy
  concurrent: 3
EOF

# Surveiller la progression
kubectl get clusterscan full-cluster-scan -n clamav -w

# Voir les scans par node
kubectl get nodescan -n clamav -l clamav.platform.numspot.com/clusterscan=full-cluster-scan
```

### 4. Planifier des scans automatiques

```bash
kubectl apply -f - <<EOF
apiVersion: clamav.platform.numspot.com/v1alpha1
kind: ScanSchedule
metadata:
  name: daily-full-scan
  namespace: clamav
spec:
  schedule: "0 2 * * *"
  
  clusterScan:
    nodeSelector:
      matchLabels:
        node-role.kubernetes.io/worker: ""
    scanPolicy: production-policy
    concurrent: 2
  
  successfulScansHistoryLimit: 10
  failedScansHistoryLimit: 3
  concurrencyPolicy: Forbid
EOF

# Vérifier le schedule
kubectl get scanschedule daily-full-scan -n clamav

# Voir l'historique des scans créés
kubectl get clusterscan -n clamav -l clamav.platform.numspot.com/schedule=daily-full-scan
```

## Troubleshooting

### L'operator ne démarre pas

```bash
# Vérifier les events
kubectl get events -n clamav-system --sort-by='.lastTimestamp'

# Vérifier les logs
kubectl logs -n clamav-system deployment/clamav-operator-controller-manager

# Vérifier les permissions
kubectl auth can-i --list --as=system:serviceaccount:clamav-system:clamav-operator-controller-manager
```

### Les scans ne se créent pas

```bash
# Vérifier que le node existe
kubectl get node <node-name>

# Vérifier les events du NodeScan
kubectl describe nodescan <scan-name> -n clamav

# Vérifier les permissions du ServiceAccount
kubectl auth can-i create jobs --as=system:serviceaccount:clamav:clamav-scanner -n clamav
```

### Les jobs échouent

```bash
# Voir les logs du job
kubectl logs -n clamav -l clamav.platform.numspot.com/nodescan=<scan-name>

# Vérifier la connexion à ClamAV
kubectl run -it --rm debug --image=busybox --restart=Never -- \
  nc -zv clamav.clamav.svc.cluster.local 3310

# Vérifier l'image du scanner
kubectl describe job -n clamav <job-name>
```

### Webhooks ne fonctionnent pas

```bash
# Vérifier que les webhooks sont configurés
kubectl get validatingwebhookconfigurations

# Vérifier les certificats
kubectl get secret -n clamav-system webhook-server-cert

# Désactiver temporairement les webhooks
kubectl delete validatingwebhookconfigurations clamav-operator-validating-webhook-configuration
```

## Monitoring

### Métriques Prometheus

L'operator expose des métriques sur le port 8080 :

```bash
# Port-forward vers l'operator
kubectl port-forward -n clamav-system deployment/clamav-operator-controller-manager 8080:8080

# Accéder aux métriques
curl http://localhost:8080/metrics | grep clamav
```

### Grafana Dashboard

Importer le dashboard depuis `config/grafana/dashboard.json`

## Mise à jour

### Mise à jour de l'operator

```bash
# Build nouvelle version
make docker-build IMG=registry.../clamav-operator:v1.1.0
make docker-push IMG=registry.../clamav-operator:v1.1.0

# Mettre à jour le déploiement
make deploy IMG=registry.../clamav-operator:v1.1.0
```

### Mise à jour des CRDs

```bash
# Générer les nouveaux manifests
make manifests

# Appliquer les CRDs
kubectl apply -k config/crd
```

## Désinstallation

```bash
# Supprimer l'operator
make undeploy

# Supprimer les CRDs (attention: supprime toutes les ressources !)
kubectl delete crd nodescans.clamav.platform.numspot.com
kubectl delete crd clusterscans.clamav.platform.numspot.com
kubectl delete crd scanpolicies.clamav.platform.numspot.com
kubectl delete crd scanschedules.clamav.platform.numspot.com

# Supprimer le namespace
kubectl delete namespace clamav-system
```

## Support

- 📖 Documentation : https://docs.clamav-operator.io
- 🐛 Issues : https://gitlab.../platform-iac/clamav-operator/-/issues
- 💬 Slack : #clamav-operator
