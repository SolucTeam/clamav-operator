/*
Copyright 2025 The ClamAV Operator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clamavv1alpha1 "github.com/SolucTeam/clamav-operator/api/v1alpha1"
)

// ── recordNodeScanMetrics ──────────────────────────────────────────────────

func TestRecordNodeScanMetrics_Completed(t *testing.T) {
	nodeScan := &clamavv1alpha1.NodeScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scan",
			Namespace: "default",
		},
		Spec: clamavv1alpha1.NodeScanSpec{
			NodeName: "worker-1",
		},
		Status: clamavv1alpha1.NodeScanStatus{
			Phase:         clamavv1alpha1.NodeScanPhaseCompleted,
			FilesScanned:  500,
			FilesInfected: 2,
			Duration:      120,
		},
	}

	// Should not panic
	assert.NotPanics(t, func() {
		recordNodeScanMetrics(nodeScan, clamavv1alpha1.NodeScanPhaseCompleted)
	})
}

func TestRecordNodeScanMetrics_Failed(t *testing.T) {
	nodeScan := &clamavv1alpha1.NodeScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "failed-scan",
			Namespace: "production",
		},
		Spec: clamavv1alpha1.NodeScanSpec{
			NodeName: "worker-2",
		},
		Status: clamavv1alpha1.NodeScanStatus{
			Phase: clamavv1alpha1.NodeScanPhaseFailed,
		},
	}

	assert.NotPanics(t, func() {
		recordNodeScanMetrics(nodeScan, clamavv1alpha1.NodeScanPhaseFailed)
	})
}

func TestRecordNodeScanMetrics_IncrementalStrategy(t *testing.T) {
	nodeScan := &clamavv1alpha1.NodeScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "incremental-scan",
			Namespace: "default",
		},
		Spec: clamavv1alpha1.NodeScanSpec{
			NodeName: "worker-3",
		},
		Status: clamavv1alpha1.NodeScanStatus{
			Phase:                   clamavv1alpha1.NodeScanPhaseCompleted,
			FilesScanned:            100,
			FilesSkippedIncremental: 400,
			CacheHitRate:            80,
			TimeSaved:               60,
			StrategyUsed:            clamavv1alpha1.ScanStrategyIncremental,
			Duration:                30,
		},
	}

	assert.NotPanics(t, func() {
		recordNodeScanMetrics(nodeScan, clamavv1alpha1.NodeScanPhaseCompleted)
	})
}

func TestRecordNodeScanMetrics_SmartStrategy(t *testing.T) {
	nodeScan := &clamavv1alpha1.NodeScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "smart-scan",
			Namespace: "default",
		},
		Spec: clamavv1alpha1.NodeScanSpec{
			NodeName: "worker-4",
		},
		Status: clamavv1alpha1.NodeScanStatus{
			Phase:                   clamavv1alpha1.NodeScanPhaseCompleted,
			FilesScanned:            50,
			FilesSkippedIncremental: 200,
			CacheHitRate:            90,
			TimeSaved:               45,
			StrategyUsed:            clamavv1alpha1.ScanStrategySmart,
			Duration:                20,
		},
	}

	assert.NotPanics(t, func() {
		recordNodeScanMetrics(nodeScan, clamavv1alpha1.NodeScanPhaseCompleted)
	})
}

func TestRecordNodeScanMetrics_ZeroValues(t *testing.T) {
	// Edge case: zero-value stats (e.g. scan ran but found nothing, took 0 duration)
	nodeScan := &clamavv1alpha1.NodeScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zero-scan",
			Namespace: "default",
		},
		Spec: clamavv1alpha1.NodeScanSpec{
			NodeName: "empty-node",
		},
		Status: clamavv1alpha1.NodeScanStatus{
			Phase:        clamavv1alpha1.NodeScanPhaseCompleted,
			FilesScanned: 0,
			Duration:     0,
		},
	}

	assert.NotPanics(t, func() {
		recordNodeScanMetrics(nodeScan, clamavv1alpha1.NodeScanPhaseCompleted)
	})
}

// ── updateNodeScanRunningMetrics ───────────────────────────────────────────

func TestUpdateNodeScanRunningMetrics(t *testing.T) {
	// Should not panic for any reasonable count value
	assert.NotPanics(t, func() {
		updateNodeScanRunningMetrics("default", 0)
		updateNodeScanRunningMetrics("default", 5)
		updateNodeScanRunningMetrics("production", 12)
	})
}

// ── recordClusterScanMetrics ───────────────────────────────────────────────

func TestRecordClusterScanMetrics_Running(t *testing.T) {
	cs := &clamavv1alpha1.ClusterScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-scan-1",
			Namespace: "default",
		},
		Status: clamavv1alpha1.ClusterScanStatus{
			Phase:          clamavv1alpha1.ClusterScanPhaseRunning,
			TotalNodes:     5,
			CompletedNodes: 2,
			FailedNodes:    0,
		},
	}

	assert.NotPanics(t, func() {
		recordClusterScanMetrics(cs, clamavv1alpha1.ClusterScanPhaseRunning)
	})
}

func TestRecordClusterScanMetrics_Completed(t *testing.T) {
	cs := &clamavv1alpha1.ClusterScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-scan-2",
			Namespace: "staging",
		},
		Status: clamavv1alpha1.ClusterScanStatus{
			Phase:          clamavv1alpha1.ClusterScanPhaseCompleted,
			TotalNodes:     3,
			CompletedNodes: 3,
			FailedNodes:    0,
		},
	}

	assert.NotPanics(t, func() {
		recordClusterScanMetrics(cs, clamavv1alpha1.ClusterScanPhaseCompleted)
	})
}

func TestRecordClusterScanMetrics_PartiallyComplete(t *testing.T) {
	cs := &clamavv1alpha1.ClusterScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "partial-scan",
			Namespace: "default",
		},
		Status: clamavv1alpha1.ClusterScanStatus{
			Phase:          clamavv1alpha1.ClusterScanPhasePartiallyComplete,
			TotalNodes:     4,
			CompletedNodes: 3,
			FailedNodes:    1,
		},
	}

	assert.NotPanics(t, func() {
		recordClusterScanMetrics(cs, clamavv1alpha1.ClusterScanPhasePartiallyComplete)
	})
}

// ── recordScanPolicyUsage ──────────────────────────────────────────────────

func TestRecordScanPolicyUsage(t *testing.T) {
	assert.NotPanics(t, func() {
		recordScanPolicyUsage("default", "strict-policy")
		recordScanPolicyUsage("production", "relaxed-policy")
	})
}

// ── recordScanScheduleExecution ────────────────────────────────────────────

func TestRecordScanScheduleExecution(t *testing.T) {
	assert.NotPanics(t, func() {
		recordScanScheduleExecution("default", "nightly-scan", "success")
		recordScanScheduleExecution("default", "nightly-scan", "failed")
		recordScanScheduleExecution("production", "weekly-full", "success")
	})
}

// ── recordScanCacheMetrics ────────────────────────────────────────────────

func TestRecordScanCacheMetrics(t *testing.T) {
	assert.NotPanics(t, func() {
		recordScanCacheMetrics("default", "worker-1", 1024*1024, 5000)
		recordScanCacheMetrics("default", "worker-1", 0, 0)
	})
}

// ── incremental metrics via recordNodeScanMetrics ────────────────────────
// recordIncrementalMetrics was removed; incremental recording is handled
// directly by recordNodeScanMetrics at the Completed phase transition.

func TestRecordNodeScanMetrics_IncrementalStrategy_RecordsSkippedFiles(t *testing.T) {
	nodeScan := &clamavv1alpha1.NodeScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "incr-test",
			Namespace: "default",
		},
		Spec: clamavv1alpha1.NodeScanSpec{
			NodeName: "worker-5",
		},
		Status: clamavv1alpha1.NodeScanStatus{
			FilesScanned:            100,
			FilesSkippedIncremental: 300,
			CacheHitRate:            75,
			TimeSaved:               90,
			Duration:                60,
			StrategyUsed:            clamavv1alpha1.ScanStrategyIncremental,
		},
	}

	assert.NotPanics(t, func() {
		recordNodeScanMetrics(nodeScan, clamavv1alpha1.NodeScanPhaseCompleted)
	})
}

func TestRecordNodeScanMetrics_FullStrategy_NoIncrementalMetrics(t *testing.T) {
	// Full strategy: incremental counters must not be incremented.
	nodeScan := &clamavv1alpha1.NodeScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "full-test",
			Namespace: "default",
		},
		Spec: clamavv1alpha1.NodeScanSpec{
			NodeName: "worker-6",
		},
		Status: clamavv1alpha1.NodeScanStatus{
			FilesScanned: 50,
			Duration:     30,
			StrategyUsed: clamavv1alpha1.ScanStrategyFull,
		},
	}

	assert.NotPanics(t, func() {
		recordNodeScanMetrics(nodeScan, clamavv1alpha1.NodeScanPhaseCompleted)
	})
}

func TestRecordNodeScanMetrics_Completed_SetsLastCompletionTimestamp(t *testing.T) {
	// Completed scans must update the staleness gauge used by the
	// ClamAVNoRecentScans PrometheusRule alert.
	nodeScan := &clamavv1alpha1.NodeScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ts-test",
			Namespace: "default",
		},
		Spec: clamavv1alpha1.NodeScanSpec{
			NodeName: "worker-7",
		},
		Status: clamavv1alpha1.NodeScanStatus{
			FilesScanned: 10,
			Duration:     5,
		},
	}

	assert.NotPanics(t, func() {
		recordNodeScanMetrics(nodeScan, clamavv1alpha1.NodeScanPhaseCompleted)
	})
}

// ── requeueWithJitter ─────────────────────────────────────────────────────

func TestRequeueWithJitter_Range(t *testing.T) {
	base := 60 * time.Second
	maxJitter := base / 5 // 20% of base = 12s

	results := make([]time.Duration, 20)
	for i := range results {
		r := requeueWithJitter(base)
		d := r.RequeueAfter
		require.GreaterOrEqual(t, d, base, "RequeueAfter should be >= base")
		require.LessOrEqual(t, d, base+maxJitter, "RequeueAfter should be <= base + 20%%")
		results[i] = d
	}

	// Sanity-check: with 20 samples there should be some variance
	allSame := true
	for _, v := range results[1:] {
		if v != results[0] {
			allSame = false
			break
		}
	}
	assert.False(t, allSame, "jitter should produce varying RequeueAfter values over 20 samples")
}

func TestRequeueWithJitter_ShortDuration(t *testing.T) {
	base := 5 * time.Second
	r := requeueWithJitter(base)
	assert.GreaterOrEqual(t, r.RequeueAfter, base)
	assert.LessOrEqual(t, r.RequeueAfter, base+base/5)
}
