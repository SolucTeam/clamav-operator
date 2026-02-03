/*
Copyright 2025 The ClamAV Operator Authors.
*/

package controllers

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
	)
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

		// ✅ NOUVEAU : Enregistrer les métriques incrémentales si présentes
		if nodeScan.Status.StrategyUsed != "" && nodeScan.Status.StrategyUsed != clamavv1alpha1.ScanStrategyFull {
			strategy := string(nodeScan.Status.StrategyUsed)
			incrementalScansTotal.WithLabelValues(namespace, node, strategy).Inc()

			if nodeScan.Status.FilesSkippedIncremental > 0 {
				filesSkippedIncremental.WithLabelValues(namespace, node).Add(float64(nodeScan.Status.FilesSkippedIncremental))
				cacheHitRate.WithLabelValues(namespace, node).Set(nodeScan.Status.CacheHitRate)
			}

			if nodeScan.Status.TimeSaved > 0 {
				timeSavedSeconds.WithLabelValues(namespace, node).Add(float64(nodeScan.Status.TimeSaved))
			}
		}
	}
}

// updateNodeScanRunningMetrics updates the gauge for running NodeScans
func updateNodeScanRunningMetrics(namespace string, count int) {
	nodeScansRunning.WithLabelValues(namespace).Set(float64(count))
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

// ✅ NOUVEAU : recordScanCacheMetrics enregistre les métriques du cache
func recordScanCacheMetrics(namespace, nodeName string, sizeBytes int64, filesCount int64) {
	scanCacheSizeBytes.WithLabelValues(namespace, nodeName).Set(float64(sizeBytes))
	scanCacheFiles.WithLabelValues(namespace, nodeName).Set(float64(filesCount))
}

// ✅ NOUVEAU : recordIncrementalMetrics enregistre les métriques du scan incrémental
// (Cette fonction est appelée depuis nodescan_controller.go)
func recordIncrementalMetrics(nodeScan *clamavv1alpha1.NodeScan) {
	namespace := nodeScan.Namespace
	node := nodeScan.Spec.NodeName
	strategy := string(nodeScan.Status.StrategyUsed)

	if strategy != "" && strategy != string(clamavv1alpha1.ScanStrategyFull) {
		incrementalScansTotal.WithLabelValues(namespace, node, strategy).Inc()

		if nodeScan.Status.FilesSkippedIncremental > 0 {
			filesSkippedIncremental.WithLabelValues(namespace, node).Add(float64(nodeScan.Status.FilesSkippedIncremental))
			cacheHitRate.WithLabelValues(namespace, node).Set(nodeScan.Status.CacheHitRate)
			timeSavedSeconds.WithLabelValues(namespace, node).Add(float64(nodeScan.Status.TimeSaved))
		}
	}
}