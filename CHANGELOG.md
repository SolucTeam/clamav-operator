# Changelog

All notable changes to this project will be documented in this file.


## [v0.7.1] - 2026-05-07

## 🚀 Features

_No new features_

## 🐛 Bug Fixes

* Merge pull request #55 from SolucTeam/fix/crd-float64-cpu-field (205a8b9)
* fix(operator): rename ScannerCPUUserSeconds float64 to ScannerCPUUserMilliseconds int64 (35152d1)

## 🔧 Other Changes

_No other changes_


## [v0.6.1] - 2026-05-06

## 🚀 Features

_No new features_

## 🐛 Bug Fixes

* Merge pull request #53 from SolucTeam/fix/configurable-job-deadline (d3af41f)
* fix(operator): make scanner Job activeDeadlineSeconds configurable (9b83ffc)

## 🔧 Other Changes

* chore: update CHANGELOG for v0.6.0 (3e174b5)


## [v0.6.0] - 2026-05-06

## 🚀 Features

* Merge pull request #52 from SolucTeam/feat/scan-report-rotation (d8fe97b)
* feat(scanner): add automatic report rotation to prevent unbounded disk growth on hostPath (af9bbd8)

## 🐛 Bug Fixes

_No bug fixes_

## 🔧 Other Changes

* chore: update CHANGELOG for v0.5.8 (06d408c)


## [Unreleased]

## 🚀 Features

* feat(scanner): automatic rotation of scan reports on the node hostPath — keeps the N most recent JSON reports per node (default 30) and deletes older ones after each scan, preventing unbounded disk growth. Configurable via `scanner.incremental.maxScanReports` (Helm) or `MAX_SCAN_REPORTS` env var.
* feat(scanner): automatic pruning of stale incremental cache entries — files deleted from the node filesystem are removed from the cache at the end of each scan, keeping cache size proportional to the actual number of files on the node.
* feat(operator): `activeDeadlineSeconds` is now configurable via `--job-active-deadline-seconds` flag and `scanner.jobActiveDeadlineSeconds` Helm value (default 0 = keep 7200 s built-in). Allows operators with large filesystems to increase the Job timeout beyond 2 h.
* feat(observability): add 8 new Prometheus metrics — `clamav_scan_files_per_second`, `clamav_scanner_memory_rss_bytes`, `clamav_scanner_cpu_user_seconds`, `clamav_signature_db_age_seconds`, `clamav_job_oom_kills_total`, `clamav_parse_retries_total`, `clamav_cache_invalidations_total`, `clamav_cache_age_seconds`. Total metric count: 33.
* feat(observability): add 6 new PrometheusRule alerts — `ClamAVSignaturesStale`, `ClamAVOOMKillsElevated`, `ClamAVScanStuck`, `ClamAVHighScanDuration`, `ClamAVLowScanThroughput`, `ClamAVOperatorReconcileErrors`. Total alert count: 14.
* feat(observability): fix 3 existing alert bugs — `ClamAVScanFailed` false negatives (window/for mismatch), `ClamAVNoRecentScans` permanent trigger on nodes that never scanned (timestamp=0 guard), `ClamAVWebhookCertExpiringSoon` silent failure when cert-manager is not installed (`absent()` guard).
* feat(observability): raise `ClamAVScanStuck` threshold from 3 h to 10 h to avoid false positives on large filesystems.
* feat(scanner): emit `memory_rss_bytes` and `cpu_user_seconds` in the `scan_complete` log line — Node.js process resource usage at end of scan.
* feat(scanner): emit `signature_db_mtime_seconds` in the `scan_complete` log line — mtime of the newest `.cvd`/`.cld` file, used by the operator to compute `clamav_signature_db_age_seconds`.
* feat(scanner): emit `cache_age_seconds`, `cache_invalidation_reason`, `cache_tracked_files`, `cache_tracked_bytes` in the `scan_complete` log line — enables cache health metrics and invalidation tracking.
* feat(operator): surface `scannerMemoryRSSBytes` and `scannerCPUUserSeconds` in `NodeScan.Status` — persisted to the CRD and converted between v1alpha1/v1beta1.
* feat(operator): detect OOMKilled scanner pods (exit code 137) and increment `clamav_job_oom_kills_total`.
* feat(operator): call `removeForceFullScanAnnotation` in both the Completed and Failed handlers — annotation is now reliably cleaned up after every forced scan.
* feat(operator): `recordScanPolicyUsage` is now called on every reconcile that applies a ScanPolicy (was dead code).
* feat(operator): `recordScanCacheMetrics` is now called after each scan with logical cache size (was dead code — scanner never emitted the required fields).
* feat(dashboard): Grafana dashboard updated to 56 panels across 9 rows — add Performance & Resources and Security & Freshness rows with cAdvisor CPU/RAM, Node.js process metrics, signature age, OOMKills, cache invalidations, and node coverage ratio.
* feat(dashboard): dashboard is now multi-cluster ready — `cluster` variable with cascading `cluster → namespace → node` chain, `cluster=~"$cluster"` filter on all 62 PromQL expressions.
* feat(dashboard): all stat panels have colour thresholds (green/yellow/red), timeseries panels use smooth lines with fill opacity, overrides apply consistent per-series colours.

## 🐛 Bug Fixes

* fix(alerts): `ClamAVScanFailed` — changed `increase([5m]) for: 5m` to `increase([10m]) for: 0m` to prevent the counter window expiring before the `for` clause fires.
* fix(alerts): `ClamAVNoRecentScans` — added `clamav_nodescan_last_completion_timestamp > 0` guard to prevent permanent firing on nodes that have never completed a scan.
* fix(alerts): `ClamAVWebhookCertExpiringSoon` — wrapped with `unless absent(...)` to silence the alert when cert-manager is not installed and the metric does not exist.
* fix(dashboard): removed hardcoded `strategy="smart"` target from the strategy panel (scanner only emits `full` or `incremental`).
* fix(dashboard): datasource template variable — added `"query": "prometheus"` so Grafana correctly lists Prometheus datasources only.

## ⚠️ Known Limitations

_None_

## 🔧 Other Changes

* docs: add `MAX_SCAN_REPORTS` and incremental env vars to ENVIRONMENT.md
* docs: document report retention, cache pruning, and `maxFileAgeHours` pitfall in scanning-modes.md
* docs: correct `activeDeadlineSeconds` documentation in runbook — field is now configurable via `--job-active-deadline-seconds` / `scanner.jobActiveDeadlineSeconds`
* docs(monitoring): rewrite metrics reference — 33 metrics across 7 categories, 14 alerts across 3 categories, Grafana multi-cluster setup guide.
* docs(runbook): add alerts table with all 14 rules, add scenarios #12 (results dir too large) and #13 (low scan throughput), update OOMKill scenario with new metric.
* docs(crds): add `scannerMemoryRSSBytes`, `scannerCPUUserSeconds`, `duration`, `exitCode` to NodeScan status fields; document `clamav.io/force-full-scan` annotation.


## [v0.5.8] - 2026-05-06

## 🚀 Features

_No new features_

## 🐛 Bug Fixes

* Merge pull request #51 from SolucTeam/fix/nodescan-ttl-recreation-standalone-warning (b6896a6)
* fix: prevent job recreation for terminal NodeScans and gate ClamAV connectivity check on remote mode (0e8f32d)

## 🔧 Other Changes

* chore: update CHANGELOG for v0.5.7 (c3a0536)


## [v0.5.7] - 2026-04-30

## 🚀 Features

_No new features_

## 🐛 Bug Fixes

* Merge pull request #46 from SolucTeam/fix/standalone-oom-ha-hooks (86272a4)
* fix: standalone OOM, HA operator, chicken-and-egg CRD hooks (c5112aa)
* fix: standalone OOM, HA operator, chicken-and-egg CRD hooks (35f3d77)

## 🔧 Other Changes

* chore: update CHANGELOG for v0.5.6 (d900426)


## [v0.5.6] - 2026-04-29

## 🚀 Features

_No new features_

## 🐛 Bug Fixes

* Merge pull request #45 from SolucTeam/fix/crd-spec-to-scanner-wiring (6f49e2b)
* fix(operator): wire CRD spec fields to scanner Job and use Status Patch (1ef54c4)

## 🔧 Other Changes

* chore: update CHANGELOG for v0.5.5 (dfa7559)


## [v0.5.5] - 2026-04-28

## 🚀 Features

_No new features_

## 🐛 Bug Fixes

* Merge pull request #44 from SolucTeam/fix/controller-stability (ee41f43)
* fix: stabilize controllers and reduce CPU pressure (98d7fb6)

## 🔧 Other Changes

* chore: update CHANGELOG for v0.5.4 (edc7f44)


## [v0.5.4] - 2026-04-27

## 🚀 Features

_No new features_

## 🐛 Bug Fixes

* Merge pull request #43 from SolucTeam/fix/job-name-rfc1123-fqdn (7bd3f4b)
* fix(controller): trim trailing separators from job names, add observedGeneration to CRD schemas (6e29334)

## 🔧 Other Changes

* chore: update CHANGELOG for v0.5.3 (38bc5e0)


## [v0.5.3] - 2026-04-27

## 🚀 Features

_No new features_

## 🐛 Bug Fixes

* Merge pull request #42 from SolucTeam/fix/scanschedule-never (d5f1085)
* fix(release): skip self-triggered tag re-run and fix changelog range (634fe82)

## 🔧 Other Changes

* chore: update CHANGELOG for v0.5.2 (abb7236)


## [v0.5.2] - 2026-04-27

## 🚀 Features

_No new features_

## 🐛 Bug Fixes

* Merge pull request #41 from SolucTeam/fix/scanschedule-never-fires (b7303ec)
* fix(controller): seed cron loop from CreationTimestamp, remove GenerationChangedPredicate (f32e384)

## 🔧 Other Changes

* chore: update CHANGELOG for v0.5.1 (f0e5d19)


## [v0.5.1] - 2026-04-27

## 🚀 Features

_No new features_

## 🐛 Bug Fixes

* Merge pull request #40 from SolucTeam/fix/workflow-log (0416c5e)
* fix(release): fix changelog truncation, awk duplication, and rebuild CHANGELOG.md (7fd2824)

## 🔧 Other Changes

* chore: update CHANGELOG for v0.5.0 (e32e60f)


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


