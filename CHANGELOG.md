# Changelog

All notable changes to this project will be documented in this file.


## [v0.5.0] - 2026-04-27

## 🚀 Features

* Merge pull request #39 from SolucTeam/feat/v1beta1-conversion-webhook (59952f1)
* feat(crds): activate v1beta1 storage version and fix CRD schemas (98e6bfd)

## 🐛 Bug Fixes


## 🔧 Other Changes

## [v0.4.3] - 2026-04-27

## 🚀 Features

## 🐛 Bug Fixes

* Merge pull request #35 from SolucTeam/feat/fix-label (55194e3)
* fix: label truncation for long node names + RBAC watch pods (99198c1)

## 🔧 Other Changes


## [v0.4.2] - 2026-04-22

## 🚀 Features

## 🐛 Bug Fixes

* Merge pull request #34 from SolucTeam/fix/config-ci (9e80f2a)
* fix: fix freshclam --datadir libfreshclam init failure in airgap job v2 (25076fe)

## 🔧 Other Changes


## [v0.4.1] - 2026-04-21

## 🚀 Features

## 🐛 Bug Fixes

* Merge pull request #32 from SolucTeam/fix/config-error (081e1ee)
* fix: networkpolicy freshclam egress policyTypes always rendered (5de0b50)

## 🔧 Other Changes


## [v0.4.0] - 2026-04-19

## 🚀 Features

* Merge pull request #29 from SolucTeam/feat/clamav-update (f5a53a3)
* feat: rendre l'image du crd-patcher configurable (repository, tag, imagePullSecrets) (fcd7338)

## 🐛 Bug Fixes

## 🔧 Other Changes


## [v0.3.1] - 2026-04-17

## 🚀 Features

## 🐛 Bug Fixes

* Merge pull request #28 from SolucTeam/fix/clamav (ceaa70a)
* fix: webhook scope Namespaced, WebhookConfig.Enabled, StartTime nil dereference (0abef35)

## 🔧 Other Changes


## [v0.3.0] - 2026-04-17

## 🚀 Features

* Merge pull request #27 from SolucTeam/feat/webhook-tls-and-operator-fixes (90faea9)
* feat: add cert-manager webhook TLS, fix race conditions and operator bugs (c5a5b92)

## 🐛 Bug Fixes

## 🔧 Other Changes


## [v0.2.1] - 2026-02-28

## 🚀 Features

## 🐛 Bug Fixes

* fix: replace float64 CacheHitRate with int32 for CRD compatibility (0fea2f6)
* fix: gofmt formatting on controller files (16b75bd)
* fix(helm): move CRDs to chart-root crds/ per Helm 3 conventions (61cf394)
* fix(lint+ci): fix gofmt, misspell, and helm lint path (9f025d9)
* fix(lint): resolve all golangci-lint errors (e897ad5)
* fix(helm): move test hook into templates/tests/ for Helm convention (6711437)
* fix: replace float64 CacheHitRate with int32 for CRD compatibility (2b15b07)

## 🔧 Other Changes
## [v0.2.0] - 2026-04-15

## 🚀 Features

## 🐛 Bug Fixes

## 🔧 Other Changes


## [v0.1.2] - 2026-03-01

## 🚀 Features

## 🐛 Bug Fixes

* fix: add id-token: write permission to release.yml for workflow_call (a6dae1a)
* fix: workflow error and bug (276360c)
* fix: workflow error and bug (6d9f4bb)
* fix: workflow error and bug (8af681b)
* fix: workflow error and bug (5a34742)
* fix: workflow error bug fetch branch main (4176689)
* fix: workflow error bug fetch main (11ef638)
* fix: workflow docker build bug (e3aec32)
* fix: workflow docker build bug (28ffebd)

## 🔧 Other Changes


## [v0.1.1] - 2026-03-01

## 🚀 Features

## 🐛 Bug Fixes

## 🔧 Other Changes


## [v0.1.0] - 2026-03-01

## 🚀 Features

* feat: initial reorganize and harden operator for prod (cd0a400)
* feat: initial reorganize and harden operator for prod (d3271a5)

## 🐛 Bug Fixes

## 🔧 Other Changes


