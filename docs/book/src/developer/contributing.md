# Contributing

## Prerequisites

- Go 1.24+
- Node.js 20+
- Docker (for image builds)
- `make`
- `kubectl` + a running cluster or `envtest`

## Build

```bash
# Operator binary
make build

# Docker images
make docker-build                   # operator image
make docker-build-scanner           # scanner image (with signature download)
make docker-build-scanner-airgap    # scanner image (DOWNLOAD_SIGS=false)
```

## Test

```bash
# Unit tests (fast, no cluster needed)
make test

# E2E tests (requires envtest binaries)
make test-e2e

# E2E against a real cluster
USE_EXISTING_CLUSTER=true make test-e2e
```

## Lint

```bash
make lint          # golangci-lint
make lint-scanner  # ESLint (Node.js scanner)
```

## Commit Convention

This project follows [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add Teams notification channel
fix: label truncation for long node names
docs: update air-gap guide
chore: bump controller-runtime to v0.18
```

Types: `feat`, `fix`, `docs`, `chore`, `ci`, `test`, `refactor`, `perf`

## Pull Request Checklist

- [ ] Unit tests added or updated
- [ ] `make test` passes
- [ ] `make lint` passes (no new warnings)
- [ ] CRD changes regenerated with `make generate manifests`
- [ ] CHANGELOG updated if user-facing
- [ ] Docs updated if behaviour changes
