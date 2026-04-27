# Release Process

Releases are fully automated via GitHub Actions. Pushing a `v*` tag triggers `release.yml`, which calls the `docker-build.yml` workflow to build, sign, and push all images, then creates a GitHub Release with assets.

## Steps

```bash
# 1. Update CHANGELOG.md
# 2. Bump version references if needed
# 3. Tag and push
git tag v0.5.0
git push origin v0.5.0
```

The pipeline automatically:

1. Builds `clamav-operator` and `clamav-node-scanner` images (amd64 + arm64)
2. Builds the `clamav-node-scanner:latest-airgap` variant (DOWNLOAD_SIGS=false, weekly signature cache)
3. Signs images with cosign
4. Generates SBOM (SPDX)
5. Runs Trivy vulnerability scan
6. Publishes the Helm chart to `oci://ghcr.io/solucteam/charts/clamav-operator`
7. Creates a GitHub Release with `install.yaml` and `crds.yaml` assets

## Image Tags

| Event | Tags produced |
|-------|--------------|
| Push to `main` | `latest`, `sha-<commit>`, `main` |
| Tag `v1.2.3` | `v1.2.3`, `v1.2`, `v1`, `latest`, `sha-<commit>` |
| Airgap variant | same tags + `-airgap` suffix |

## Signing

All release images are signed with cosign (keyless, OIDC-based). Verify with:

```bash
cosign verify \
  --certificate-identity-regexp="https://github.com/SolucTeam/clamav-operator" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  ghcr.io/solucteam/clamav-operator:latest
```
