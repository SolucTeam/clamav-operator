# ClamAV Operator

The **ClamAV Operator** is a Kubernetes-native antivirus operator that automates ClamAV scanning across every node in your cluster with zero external dependencies.

It reconciles a set of custom resources (`NodeScan`, `ClusterScan`, `ScanSchedule`, `ScanPolicy`) into Kubernetes Jobs that run `clamscan` directly on each node, report results back to the CRD status, and fire notifications on infection.

## Features

- **Standalone mode** — `clamscan` + signatures embedded in the scanner image; no central ClamAV service, no single point of failure
- **Incremental scanning** — scan only new/modified files, alternate with periodic full scans (up to 10× faster)
- **Air-gap support** — signatures baked into the image at build time; no outbound internet at runtime
- **Cron scheduling** — drift-free cluster-wide scans via `ScanSchedule`; anchored to the cron grid, not to `time.Now()`
- **Prometheus metrics** — `clamav_files_infected_total`, `clamav_scan_duration_seconds`, running scan gauge, and more
- **Notifications** — Slack, Email (SMTP/TLS), generic Webhook, Microsoft Teams; fire-and-forget, never block a scan
- **Multi-arch images** — `linux/amd64` and `linux/arm64`; built, scanned (Trivy) and cosign-signed on every release
- **Supply-chain security** — SBOM (SPDX), cosign image signing, Trivy CVE scanning in CI
- **GitOps-ready** — `observedGeneration` in Status; ArgoCD and Flux know exactly when reconciliation is done
- **Admission webhooks** — invalid CRs (bad cron, negative limits, empty nodeName) are rejected at create/update time
- **cert-manager TLS** — webhook certificates fully managed by the Helm chart
- **Private registry support** — `scanner.imagePullSecrets` forwarded into every scanner Job pod

## Getting Started

- [Quickstart](./getting-started/quickstart.md) — install the operator and run your first cluster scan in minutes
- [Concepts](./getting-started/concepts.md) — understand the CRD model and reconciliation flow
- [Installation](./installation/README.md) — Helm, manifests, prerequisites
- [Developer Guide](./developer/README.md) — build, test, contribute
