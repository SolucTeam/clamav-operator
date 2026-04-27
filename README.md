<div align="center">
  <img src="docs/assets/logo.svg" alt="clamav-operator" width="520"/>
  <br/><br/>

  [![Build](https://github.com/SolucTeam/clamav-operator/actions/workflows/docker-build.yml/badge.svg)](https://github.com/SolucTeam/clamav-operator/actions/workflows/docker-build.yml)
  [![Release](https://img.shields.io/github/v/release/SolucTeam/clamav-operator?color=blue&logo=github)](https://github.com/SolucTeam/clamav-operator/releases)
  [![License](https://img.shields.io/badge/license-Apache%202.0-green?logo=apache)](https://github.com/SolucTeam/clamav-operator/blob/main/LICENSE)
  [![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev)
  [![Kubernetes](https://img.shields.io/badge/Kubernetes-1.24+-326CE5?logo=kubernetes&logoColor=white)](https://kubernetes.io)
  [![Helm](https://img.shields.io/badge/Helm-OCI-0F1689?logo=helm&logoColor=white)](https://helm.sh)
  [![Arch](https://img.shields.io/badge/arch-amd64%20%7C%20arm64-lightgrey?logo=linux)](https://github.com/SolucTeam/clamav-operator/releases)
  [![SBOM](https://img.shields.io/badge/SBOM-SPDX-blueviolet?logo=dependabot)](https://github.com/SolucTeam/clamav-operator/releases)
</div>

# ClamAV Operator

The **ClamAV Operator** is a Kubernetes-native antivirus operator that automates ClamAV scanning across every node in your cluster with zero external dependencies.

It reconciles a set of custom resources (`NodeScan`, `ClusterScan`, `ScanSchedule`, `ScanPolicy`) into Kubernetes Jobs that run `clamscan` directly on each node, report results back to the CRD status, and fire notifications on infection.

## 📖 Documentation

Please see our [documentation site](https://solucteam.github.io/clamav-operator) for in-depth documentation.

## ✨ Features

- **Standalone mode** — `clamscan` + signatures embedded in the scanner image; no central ClamAV service, no single point of failure
- **Incremental scanning** — scan only new/modified files, alternate with periodic full scans (up to 10× faster)
- **Air-gap support** — signatures baked into the image at build time; no outbound internet at runtime
- **Cron scheduling** — drift-free cluster-wide scans via `ScanSchedule`; anchored to the cron grid, not to `time.Now()`
- **Prometheus metrics** — `clamav_files_infected_total`, `clamav_scan_duration_seconds`, running scan gauge, and more
- **Notifications** — Slack, Email (SMTP/TLS), generic Webhook, Microsoft Teams; fire-and-forget, never block a scan
- **Multi-arch images** — `linux/amd64` and `linux/arm64`; built, scanned (Trivy) and cosign-signed on every release
- **Supply-chain security** — SBOM (SPDX), cosign image signing, Trivy CVE scanning in CI
- **GitOps-ready** — `observedGeneration` in Status; ArgoCD and Flux know exactly when reconciliation is done
- **Admission webhooks** — invalid CRs are rejected at create/update time
- **cert-manager TLS** — webhook certificates fully managed by the Helm chart

## 🚀 Quick Start

```bash
helm install clamav-operator oci://ghcr.io/solucteam/charts/clamav-operator \
  --namespace clamav-system \
  --create-namespace \
  --wait
```

Then trigger a cluster-wide scan:

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

See the [Quickstart guide](https://solucteam.github.io/clamav-operator/getting-started/quickstart.html) for full details.

## 🤝 Community, Discussion and Support

Pull requests and feedback on issues are very welcome!

See our [contributor guide](docs/book/src/developer/contributing.md) for how to build, test, and submit changes.

Commits follow the [Conventional Commits](https://www.conventionalcommits.org/) convention: `feat:`, `fix:`, `docs:`, `chore:`, etc.

## 📄 License

Copyright 2025 The ClamAV Operator Authors.
Licensed under the [Apache License, Version 2.0](LICENSE).
