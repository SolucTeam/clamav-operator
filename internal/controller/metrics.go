/*
Copyright 2025 The ClamAV Operator Authors.
*/

package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	clamavv1alpha1 "github.com/SolucTeam/clamav-operator/api/v1alpha1"
)

var (
	// NodeScan metrics
	nodeScansTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "clamav_nodescans_total",
			Help: "Total number of NodeScans created",
		},
		[]string{"namespace", "node", "status"},
	)

	nodeScansRunning = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "clamav_nodescans_running",
			Help: "Number of currently running NodeScans",
		},
		[]string{"namespace"},
	)

	filesScannedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "clamav_files_scanned_total",
			Help: "Total number of files scanned",
		},
		[]string{"namespace", "node"},
	)

	filesInfectedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "clamav_files_infected_total",
			Help: "Total number of infected files found",
		},
		[]string{"namespace", "node"},
	)

	scanDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "clamav_scan_duration_seconds",
			Help:    "Duration of ClamAV scans in seconds",
			Buckets: []float64{30, 60, 120, 300, 600, 1200, 1800, 3600},
		},
		[]string{"namespace", "node"},
	)

	// ClusterScan metrics
	clusterScanNodesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "clamav_clusterscan_nodes_total",
			Help: "Total number of nodes in a ClusterScan",
		},
		[]string{"namespace", "clusterscan"},
	)

	clusterScanNodesCompleted = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "clamav_clusterscan_nodes_completed",
			Help: "Number of completed nodes in a ClusterScan",
		},
		[]string{"namespace", "clusterscan"},
	)

	clusterScanNodesFailed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "clamav_clusterscan_nodes_failed",
			Help: "Number of failed nodes in a ClusterScan",
		},
		[]string{"namespace", "clusterscan"},
	)

	clusterScansTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "clamav_clusterscans_total",
			Help: "Total number of ClusterScans",
		},
		[]string{"namespace", "status"},
	)

	// ScanPolicy metrics
	scanPolicyUsageTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "clamav_scanpolicy_usage_total",
			Help: "Number of times a ScanPolicy has been used",
		},
		[]string{"namespace", "policy"},
	)

	// ScanSchedule metrics
	scanScheduleExecutionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "clamav_scanschedule_executions_total",
			Help: "Total number of ScanSchedule executions",
		},
		[]string{"namespace", "schedule", "status"},
	)

	// ✅ NOUVEAU : Incremental scan metrics
	incrementalScansTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "clamav_incremental_scans_total",
			Help: "Total number of incremental scans",
		},
		[]string{"namespace", "node", "strategy"},
	)

	filesSkippedIncremental = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "clamav_files_skipped_incremental_total",
			Help: "Total number of files skipped in incremental scans",
		},
		[]string{"namespace", "node"},
	)

	cacheHitRate = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "clamav_cache_hit_rate_percent",
			Help: "Cache hit rate percentage for incremental scans",
		},
		[]string{"namespace", "node"},
	)

	timeSavedSeconds = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "clamav_time_saved_incremental_seconds",
			Help: "Time saved by incremental scanning in seconds",
		},
		[]string{"namespace", "node"},
	)

	scanCacheSizeBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "clamav_scan_cache_size_bytes",
			Help: "Size of scan cache in bytes",
		},
		[]string{"namespace", "node"},
	)

	scanCacheFiles = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "clamav_scan_cache_files_total",
			Help: "Number of files tracked in scan cache",
		},
		[]string{"namespace", "node"},
	)

	// nodeScanLastCompletionTimestamp is the Unix timestamp of the last successful
	// NodeScan completion per node.  Used by the ClamAVNoRecentScans alert:
	//   time() - clamav_nodescan_last_completion_timestamp > 86400
	nodeScanLastCompletionTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "clamav_nodescan_last_completion_timestamp",
			Help: "Unix timestamp of the last successful NodeScan completion per node",
		},
		[]string{"namespace", "node"},
	)

	// Notification metrics — used by the ClamAVNotificationFailed alert.
	notificationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "clamav_notifications_sent_total",
			Help: "Total number of notification attempts (all channels combined)",
		},
		[]string{"namespace", "channel"}, // channel: slack | email | webhook
	)

	notificationsFailed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "clamav_notifications_failed_total",
			Help: "Total number of notification delivery failures after all retries",
		},
		[]string{"namespace", "channel"},
	)

	// nodeScanPartialResults tracks scans where result parsing was incomplete.
	// Used by the ClamAVPartialScanResults alert.
	nodeScanPartialResults = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "clamav_nodescan_partial_results",
			Help: "1 if the NodeScan has partial/unreliable results, 0 otherwise",
		},
		[]string{"namespace", "node"},
	)

	// ── Maintenance metrics ───────────────────────────────────────────────────

	// reportsRotatedTotal counts JSON scan reports deleted by the rotation logic.
	reportsRotatedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "clamav_reports_rotated_total",
			Help: "Total number of old scan report files deleted by the rotation logic (per node)",
		},
		[]string{"namespace", "node"},
	)

	// cacheEntriesPrunedTotal counts stale cache entries removed after each scan.
	cacheEntriesPrunedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "clamav_cache_entries_pruned_total",
			Help: "Total number of stale incremental cache entries removed (files deleted from node filesystem)",
		},
		[]string{"namespace", "node"},
	)

	// ── Storage metrics ───────────────────────────────────────────────────────

	// resultsDirBytes tracks the total size of the hostPath results directory per node.
	// This is /var/log/clamav-scans on the node — contains JSON reports, txt summaries,
	// and the incremental cache file.
	resultsDirBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "clamav_results_dir_bytes",
			Help: "Total size in bytes of the scan results directory on the node hostPath (/var/log/clamav-scans)",
		},
		[]string{"namespace", "node"},
	)

	// cacheFileBytes tracks the size of the incremental cache JSON file per node.
	cacheFileBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "clamav_cache_file_bytes",
			Help: "Size in bytes of the incremental scan cache file on the node hostPath",
		},
		[]string{"namespace", "node"},
	)

	// ── Performance metrics ───────────────────────────────────────────────────

	// scanFilesPerSecond is the scan throughput (files/s) of the last completed
	// scan per node.  Computed from NodeScan.Status.FilesScanned / Duration.
	// Useful for capacity planning and detecting performance regressions between
	// scanner image versions or node hardware changes.
	scanFilesPerSecond = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "clamav_scan_files_per_second",
			Help: "Scan throughput in files per second for the last completed scan (files_scanned / duration_seconds)",
		},
		[]string{"namespace", "node"},
	)

	// scannerMemoryRSSBytes is the resident set size of the scanner process
	// (Node.js orchestrator) at the end of the scan, as reported by
	// process.memoryUsage().rss.  In standalone mode this reflects the Node.js
	// wrapper only (clamscan runs as a separate child process tracked by cAdvisor).
	// In remote mode it reflects the full scanner memory usage.
	scannerMemoryRSSBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "clamav_scanner_memory_rss_bytes",
			Help: "RSS memory of the scanner Node.js process at end of scan (bytes). Use cAdvisor container_memory_working_set_bytes for total container memory.",
		},
		[]string{"namespace", "node"},
	)

	// scannerCPUUserSeconds is the user-space CPU time consumed by the scanner
	// Node.js process during the scan, in seconds.  Child processes (clamscan)
	// are not included; use cAdvisor container_cpu_usage_seconds_total for the
	// full container CPU budget.
	scannerCPUUserSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "clamav_scanner_cpu_user_seconds",
			Help: "User-space CPU seconds consumed by the scanner Node.js process during the scan. Use cAdvisor for full container CPU.",
		},
		[]string{"namespace", "node"},
	)

	// ── Security / freshness metrics ─────────────────────────────────────────

	// signatureDBAgeSec is the age of the ClamAV signature database in seconds,
	// computed as time() − mtime(newest .cvd/.cld file).
	// A value > 86400 (24 h) means signatures have not been updated today —
	// drives the ClamAVSignaturesStale alert.
	signatureDBAgeSec = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "clamav_signature_db_age_seconds",
			Help: "Age of the ClamAV signature database in seconds (time since newest .cvd/.cld file was written). >86400 = stale.",
		},
		[]string{"namespace", "node"},
	)

	// jobOOMKillsTotal counts scanner Job pods that were OOMKilled (exit 137).
	// Each OOMKill represents a scan that failed due to insufficient memory —
	// the resource limits should be increased.
	jobOOMKillsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "clamav_job_oom_kills_total",
			Help: "Total number of scanner Job pods terminated by OOMKill (exit code 137). Indicates memory limits are too low.",
		},
		[]string{"namespace", "node"},
	)

	// parseRetriesTotal counts how many times the operator had to retry parsing
	// the scanner job output logs.  High values indicate flaky log streaming or
	// scanner output truncation.
	parseRetriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "clamav_parse_retries_total",
			Help: "Total number of scan log parse retries by the operator. High values indicate log streaming issues.",
		},
		[]string{"namespace", "node"},
	)

	// cacheInvalidationsTotal counts how many times the incremental cache was
	// discarded before a scan, labeled by the reason.
	// reason: 'first_scan' | 'signature_change' | 'corrupted'
	cacheInvalidationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "clamav_cache_invalidations_total",
			Help: "Total number of incremental cache invalidations by reason (first_scan, signature_change, corrupted).",
		},
		[]string{"namespace", "node", "reason"},
	)

	// cacheAgeSec is the age of the incremental cache file at the time of the
	// scan (seconds since last write).  A large value means the node has not had
	// a successful scan in a long time and the cache baseline may be very old.
	// -1 = no valid cache (first scan or invalidated).
	cacheAgeSec = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "clamav_cache_age_seconds",
			Help: "Age of the incremental cache file at scan time (seconds). -1 = no valid cache (first scan or invalidated).",
		},
		[]string{"namespace", "node"},
	)
)

func init() {
	// Register all metrics with controller-runtime's registry
	metrics.Registry.MustRegister(
		nodeScansTotal,
		nodeScansRunning,
		filesScannedTotal,
		filesInfectedTotal,
		scanDuration,
		clusterScanNodesTotal,
		clusterScanNodesCompleted,
		clusterScanNodesFailed,
		clusterScansTotal,
		scanPolicyUsageTotal,
		scanScheduleExecutionsTotal,
		// Incremental metrics
		incrementalScansTotal,
		filesSkippedIncremental,
		cacheHitRate,
		timeSavedSeconds,
		scanCacheSizeBytes,
		scanCacheFiles,
		// Staleness alert support
		nodeScanLastCompletionTimestamp,
		// Notification delivery
		notificationsTotal,
		notificationsFailed,
		// Partial results
		nodeScanPartialResults,
		// Maintenance metrics
		reportsRotatedTotal,
		cacheEntriesPrunedTotal,
		// Storage metrics
		resultsDirBytes,
		cacheFileBytes,
		// Performance metrics
		scanFilesPerSecond,
		scannerMemoryRSSBytes,
		scannerCPUUserSeconds,
		// Security / freshness
		signatureDBAgeSec,
		jobOOMKillsTotal,
		parseRetriesTotal,
		cacheInvalidationsTotal,
		cacheAgeSec,
	)
}

// recordNotificationAttempt increments the sent counter for a given channel.
func recordNotificationAttempt(namespace, channel string) {
	notificationsTotal.WithLabelValues(namespace, channel).Inc()
}

// recordNotificationFailed increments the failure counter for a given channel.
func recordNotificationFailed(namespace, channel string) {
	notificationsFailed.WithLabelValues(namespace, channel).Inc()
}

// recordPartialResults sets the partial results gauge for a node.
func recordPartialResults(namespace, node string, partial bool) {
	val := 0.0
	if partial {
		val = 1.0
	}
	nodeScanPartialResults.WithLabelValues(namespace, node).Set(val)
}

// recordNodeScanMetrics records metrics for a NodeScan
func recordNodeScanMetrics(nodeScan *clamavv1alpha1.NodeScan, phase clamavv1alpha1.NodeScanPhase) {
	namespace := nodeScan.Namespace
	node := nodeScan.Spec.NodeName
	status := string(phase)

	nodeScansTotal.WithLabelValues(namespace, node, status).Inc()

	if phase == clamavv1alpha1.NodeScanPhaseCompleted {
		if nodeScan.Status.FilesScanned > 0 {
			filesScannedTotal.WithLabelValues(namespace, node).Add(float64(nodeScan.Status.FilesScanned))
		}
		if nodeScan.Status.FilesInfected > 0 {
			filesInfectedTotal.WithLabelValues(namespace, node).Add(float64(nodeScan.Status.FilesInfected))
		}
		if nodeScan.Status.Duration > 0 {
			scanDuration.WithLabelValues(namespace, node).Observe(float64(nodeScan.Status.Duration))
		}

		// Staleness tracking — drives the ClamAVNoRecentScans alert.
		nodeScanLastCompletionTimestamp.WithLabelValues(namespace, node).SetToCurrentTime()

		// Throughput — files per second.  Guards against zero-duration edge case
		// (e.g. an empty scan with 0 files that finishes instantly).
		if nodeScan.Status.Duration > 0 && nodeScan.Status.FilesScanned > 0 {
			fps := float64(nodeScan.Status.FilesScanned) / float64(nodeScan.Status.Duration)
			scanFilesPerSecond.WithLabelValues(namespace, node).Set(fps)
		}

		// Scanner process memory / CPU — populated from the scan_complete log
		// fields memory_rss_bytes and cpu_user_seconds (Node.js process stats).
		if nodeScan.Status.ScannerMemoryRSSBytes > 0 {
			scannerMemoryRSSBytes.WithLabelValues(namespace, node).Set(float64(nodeScan.Status.ScannerMemoryRSSBytes))
		}
		if nodeScan.Status.ScannerCPUUserSeconds > 0 {
			scannerCPUUserSeconds.WithLabelValues(namespace, node).Set(nodeScan.Status.ScannerCPUUserSeconds)
		}

		// Record incremental metrics if the scan used an incremental strategy.
		if nodeScan.Status.StrategyUsed != "" && nodeScan.Status.StrategyUsed != clamavv1alpha1.ScanStrategyFull {
			strategy := string(nodeScan.Status.StrategyUsed)
			incrementalScansTotal.WithLabelValues(namespace, node, strategy).Inc()

			if nodeScan.Status.FilesSkippedIncremental > 0 {
				filesSkippedIncremental.WithLabelValues(namespace, node).Add(float64(nodeScan.Status.FilesSkippedIncremental))
				cacheHitRate.WithLabelValues(namespace, node).Set(float64(nodeScan.Status.CacheHitRate))
			}

			if nodeScan.Status.TimeSaved > 0 {
				timeSavedSeconds.WithLabelValues(namespace, node).Add(float64(nodeScan.Status.TimeSaved))
			}
		}
	}
}

// updateNodeScanRunningMetrics sets the absolute count of running NodeScans in a
// namespace. Prefer incNodeScanRunning / decNodeScanRunning at phase-transition
// sites so that we don't need an extra List call on every reconcile.
func updateNodeScanRunningMetrics(namespace string, count int) {
	nodeScansRunning.WithLabelValues(namespace).Set(float64(count))
}

// incNodeScanRunning increments the running gauge when a NodeScan enters the
// Running phase.  Call this exactly once per NodeScan at the Running transition.
func incNodeScanRunning(namespace string) {
	nodeScansRunning.WithLabelValues(namespace).Inc()
}

// decNodeScanRunning decrements the running gauge when a NodeScan leaves the
// Running phase (Completed or Failed).
func decNodeScanRunning(namespace string) {
	nodeScansRunning.WithLabelValues(namespace).Dec()
}

// recordClusterScanMetrics records metrics for a ClusterScan
func recordClusterScanMetrics(clusterScan *clamavv1alpha1.ClusterScan, phase clamavv1alpha1.ClusterScanPhase) {
	namespace := clusterScan.Namespace
	name := clusterScan.Name
	status := string(phase)

	// Record total ClusterScans by status
	clusterScansTotal.WithLabelValues(namespace, status).Inc()

	// Update node counts
	clusterScanNodesTotal.WithLabelValues(namespace, name).Set(float64(clusterScan.Status.TotalNodes))
	clusterScanNodesCompleted.WithLabelValues(namespace, name).Set(float64(clusterScan.Status.CompletedNodes))
	clusterScanNodesFailed.WithLabelValues(namespace, name).Set(float64(clusterScan.Status.FailedNodes))
}

// recordScanPolicyUsage records when a ScanPolicy is used
func recordScanPolicyUsage(namespace, policyName string) {
	scanPolicyUsageTotal.WithLabelValues(namespace, policyName).Inc()
}

// recordScanScheduleExecution records when a ScanSchedule executes
func recordScanScheduleExecution(namespace, scheduleName, status string) {
	scanScheduleExecutionsTotal.WithLabelValues(namespace, scheduleName, status).Inc()
}

// recordScanCacheMetrics records cache size and file count metrics for a node.
func recordScanCacheMetrics(namespace, nodeName string, sizeBytes int64, filesCount int64) {
	scanCacheSizeBytes.WithLabelValues(namespace, nodeName).Set(float64(sizeBytes))
	scanCacheFiles.WithLabelValues(namespace, nodeName).Set(float64(filesCount))
}
