# Architecture

## Repository Layout

```
clamav-operator/
├── api/
│   ├── v1alpha1/          # CRD types + validation + conversion hub
│   └── v1beta1/           # Storage version types
├── build/
│   └── Dockerfile         # Operator image
├── cmd/
│   └── manager/
│       └── main.go        # Entry point: registers controllers + webhooks
├── helm/
│   └── clamav-operator/   # Helm chart
├── internal/
│   ├── controller/
│   │   ├── nodescan_controller.go      # NodeScan → Job lifecycle
│   │   ├── clusterscan_controller.go   # ClusterScan → NodeScan fan-out
│   │   ├── scanschedule_controller.go  # Cron → ClusterScan
│   │   ├── common.go                   # Shared helpers (sanitizeLabelValue, jitter)
│   │   ├── defaults.go                 # Resource profiles by priority
│   │   └── metrics.go                  # Prometheus metric registration
│   └── notification/
│       └── notifier.go    # Slack · Email · Webhook · Teams
├── scanner/               # Node.js scanner container
│   ├── src/
│   │   ├── index.js       # Main entry point
│   │   ├── scanner.js     # clamscan / clamd wrapper
│   │   ├── incremental.js # Cache load/save, strategy resolution
│   │   ├── report.js      # JSON report generation
│   │   └── config.js      # Environment variable parsing
│   └── Dockerfile
└── test/
    └── e2e/               # End-to-end tests (envtest)
```

## Controllers

### NodeScanReconciler

Owns the `NodeScan → Job` lifecycle:

1. Reads `ScanPolicy` (if referenced)
2. Checks node existence
3. Creates the scanner `Job` with the correct env vars, volumes, and security context
4. Watches the Job; on completion, reads pod logs and parses the JSON report
5. Updates `NodeScan.Status` (phase, filesScanned, filesInfected, strategyUsed)
6. Dispatches notifications asynchronously
7. Patches the Job TTL to 1 h after success (was 24 h while running)

### ClusterScanReconciler

Fan-out controller:

1. Lists nodes matching `spec.nodeSelector`
2. Creates one `NodeScan` per node (respecting `spec.concurrent`)
3. Watches NodeScan statuses to aggregate phase and counters
4. Transitions to `Completed` / `Failed` when all NodeScans finish

### ScanScheduleReconciler

Cron controller:

1. Computes the next cron slot from `spec.schedule` and `status.lastScheduleTime`
2. At the slot, creates a `ClusterScan` owned by the ScanSchedule
3. Records `LastScheduleTime` anchored to the cron grid (not `time.Now()`)
4. Respects `spec.suspend`

## Key Design Decisions

**Label sanitization** — node names can exceed 63 characters (Kubernetes label value limit). All label values derived from node names or scan names pass through `sanitizeLabelValue()` which uses a SHA-256 hash suffix when truncation is needed, preserving uniqueness.

**Configurable retry settings** — `NodeScanReconciler` exposes `ParseMaxRetries` and `ParseRetryBaseSeconds` fields so tests can use minimal values (1 retry, 1 s) without touching constants.

**Fire-and-forget notifications** — notifications run in a goroutine with a separate context so a slow or unreachable endpoint never blocks the reconcile loop or causes requeues.
