# Contributing to clamav-operator

Thank you for your interest in improving the ClamAV Operator! Contributions of
all kinds are welcome: bug reports, documentation, code, dashboards, examples,
and ideas.

The full contributor guide (build, test, release process) lives in the
documentation site: [docs/book/src/developer/contributing.md](docs/book/src/developer/contributing.md)
([rendered](https://solucteam.github.io/clamav-operator/developer/contributing.html)).

## TL;DR

1. **Discuss first** for significant changes — open a
   [GitHub Discussion](https://github.com/SolucTeam/clamav-operator/discussions)
   or an issue before investing time in a large PR.
2. **Fork & branch** — `feat/my-feature` or `fix/my-fix`.
3. **Conventional Commits** — `feat:`, `fix:`, `docs:`, `chore:`, `test:`, `refactor:`.
4. **Tests pass** — `make test` (unit) and `make test-e2e` (envtest) must be green.
5. **Keep PRs focused** — one logical change per PR.

## Ways to contribute

- 🐞 [Report a bug](https://github.com/SolucTeam/clamav-operator/issues/new?template=bug_report.yml)
- 💡 [Propose a feature](https://github.com/SolucTeam/clamav-operator/issues/new?template=feature_request.yml)
- ❓ [Ask a question](https://github.com/SolucTeam/clamav-operator/discussions/new?category=q-a)
- 📖 Improve the documentation (`docs/book/src/`)
- 📊 Improve Grafana dashboards or Prometheus alert rules (`grafana/`)
- 🚀 Add examples (`examples/`) — GitOps recipes are especially welcome

## Report a security vulnerability

**Please do not open a public issue for security vulnerabilities.**
See [SECURITY.md](SECURITY.md) for the private disclosure process.

## Code of conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md).
By participating, you are expected to uphold it.

## Licensing

Contributions are licensed under the Apache License 2.0, identical to the
project. By submitting a PR you agree your work is released under that license.
