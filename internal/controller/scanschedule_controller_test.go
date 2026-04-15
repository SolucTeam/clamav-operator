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
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	clamavv1alpha1 "github.com/SolucTeam/clamav-operator/api/v1alpha1"
)

// newTestScanScheduleReconciler builds a ScanScheduleReconciler backed by a
// fake client containing the given objects.
func newTestScanScheduleReconciler(objs ...client.Object) *ScanScheduleReconciler {
	scheme := newTestScheme()
	fakeClient := fakeclient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&clamavv1alpha1.ScanSchedule{}).
		Build()

	return &ScanScheduleReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(100),
	}
}

// TestScanScheduleReconciler_NotFound verifies that a missing object is handled
// gracefully (no-op, no error).
func TestScanScheduleReconciler_NotFound(t *testing.T) {
	r := newTestScanScheduleReconciler()

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "nonexistent",
			Namespace: "default",
		},
	})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// TestScanScheduleReconciler_InvalidCron verifies that an invalid cron
// expression causes the reconciler to return an error.
func TestScanScheduleReconciler_InvalidCron(t *testing.T) {
	schedule := &clamavv1alpha1.ScanSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bad-schedule",
			Namespace: "default",
		},
		Spec: clamavv1alpha1.ScanScheduleSpec{
			Schedule: "not-a-valid-cron",
		},
	}

	r := newTestScanScheduleReconciler(schedule)

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "bad-schedule",
			Namespace: "default",
		},
	})

	assert.Error(t, err, "invalid cron expression should return an error")
}

// TestScanScheduleReconciler_Suspended verifies that a suspended schedule does
// not create any ClusterScan objects.
func TestScanScheduleReconciler_Suspended(t *testing.T) {
	suspended := true
	schedule := &clamavv1alpha1.ScanSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "suspended-schedule",
			Namespace: "default",
		},
		Spec: clamavv1alpha1.ScanScheduleSpec{
			Schedule: "0 * * * *", // every hour
			Suspend:  suspended,
		},
	}

	r := newTestScanScheduleReconciler(schedule)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "suspended-schedule",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Greater(t, result.RequeueAfter.Seconds(), 0.0, "should requeue for next run time")

	// Verify no ClusterScan was created
	var clusterScans clamavv1alpha1.ClusterScanList
	err = r.List(context.Background(), &clusterScans, client.InNamespace("default"))
	require.NoError(t, err)
	assert.Empty(t, clusterScans.Items, "suspended schedule should not create ClusterScan")
}

// TestScanScheduleReconciler_FirstRun verifies that the very first reconciliation
// (LastScheduleTime == nil) does NOT trigger a ClusterScan.
// This mirrors the Kubernetes CronJob contract: a schedule that has never run
// before waits for the next natural occurrence rather than firing immediately.
func TestScanScheduleReconciler_FirstRun(t *testing.T) {
	schedule := &clamavv1alpha1.ScanSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "first-run-schedule",
			Namespace: "default",
		},
		Spec: clamavv1alpha1.ScanScheduleSpec{
			Schedule: "* * * * *", // every minute
		},
		// Status.LastScheduleTime intentionally left nil → first reconcile
	}

	r := newTestScanScheduleReconciler(schedule)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "first-run-schedule",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Greater(t, result.RequeueAfter.Seconds(), 0.0, "should requeue for next scheduled time")

	// No ClusterScan must be created on the very first reconcile
	var clusterScans clamavv1alpha1.ClusterScanList
	err = r.List(context.Background(), &clusterScans, client.InNamespace("default"))
	require.NoError(t, err)
	assert.Empty(t, clusterScans.Items, "first reconcile should not create a ClusterScan")
}

// TestScanScheduleReconciler_DueRun verifies that when a schedule has already
// run before (LastScheduleTime is set) and a new slot has passed, the
// reconciler creates exactly one ClusterScan.
func TestScanScheduleReconciler_DueRun(t *testing.T) {
	pastTime := metav1.Time{Time: time.Now().Add(-2 * time.Minute)}
	schedule := &clamavv1alpha1.ScanSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "due-run-schedule",
			Namespace: "default",
		},
		Spec: clamavv1alpha1.ScanScheduleSpec{
			Schedule: "* * * * *", // every minute — guaranteed to be due
		},
		Status: clamavv1alpha1.ScanScheduleStatus{
			LastScheduleTime: &pastTime, // set 2 minutes ago → one slot has passed
		},
	}

	r := newTestScanScheduleReconciler(schedule)

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "due-run-schedule",
			Namespace: "default",
		},
	})

	require.NoError(t, err)

	var clusterScans clamavv1alpha1.ClusterScanList
	err = r.List(context.Background(), &clusterScans, client.InNamespace("default"))
	require.NoError(t, err)
	assert.Len(t, clusterScans.Items, 1, "should create exactly one ClusterScan when schedule is due")
}

// TestScanScheduleReconciler_ConcurrencyForbid verifies that when
// ConcurrencyPolicy=Forbid and a scan is already active, no new ClusterScan is
// created.
func TestScanScheduleReconciler_ConcurrencyForbid(t *testing.T) {
	// Pre-create an active ClusterScan
	existing := &clamavv1alpha1.ClusterScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "existing-scan",
			Namespace: "default",
			Labels:    map[string]string{"clamav.io/schedule": "forbid-schedule"},
		},
	}

	schedule := &clamavv1alpha1.ScanSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "forbid-schedule",
			Namespace: "default",
		},
		Spec: clamavv1alpha1.ScanScheduleSpec{
			Schedule:          "* * * * *",
			ConcurrencyPolicy: "Forbid",
		},
		Status: clamavv1alpha1.ScanScheduleStatus{
			Active: []corev1.ObjectReference{
				{Name: "existing-scan", Namespace: "default"},
			},
		},
	}

	r := newTestScanScheduleReconciler(existing, schedule)

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "forbid-schedule",
			Namespace: "default",
		},
	})

	require.NoError(t, err)

	// Should not have created a second ClusterScan
	var clusterScans clamavv1alpha1.ClusterScanList
	err = r.List(context.Background(), &clusterScans, client.InNamespace("default"))
	require.NoError(t, err)
	assert.Len(t, clusterScans.Items, 1, "Forbid policy should not create a new ClusterScan when one is active")
}

// TestScanScheduleReconciler_HistoryCleanup verifies that excess completed
// ClusterScan history is pruned according to SuccessfulScansHistoryLimit.
func TestScanScheduleReconciler_HistoryCleanup(t *testing.T) {
	// Create more completed scans than the history limit allows
	objs := []client.Object{}
	for i := 0; i < 5; i++ {
		cs := &clamavv1alpha1.ClusterScan{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "scan-" + string(rune('a'+i)),
				Namespace: "default",
				Labels:    map[string]string{"clamav.io/schedule": "cleanup-schedule"},
			},
			Status: clamavv1alpha1.ClusterScanStatus{
				Phase: clamavv1alpha1.ClusterScanPhaseCompleted,
			},
		}
		objs = append(objs, cs)
	}

	limit := int32(2)
	schedule := &clamavv1alpha1.ScanSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cleanup-schedule",
			Namespace: "default",
		},
		Spec: clamavv1alpha1.ScanScheduleSpec{
			Schedule:                    "0 3 * * *", // daily at 3am — won't trigger now
			SuccessfulScansHistoryLimit: &limit,
		},
		Status: clamavv1alpha1.ScanScheduleStatus{
			// Set last schedule to now so it won't trigger
			LastScheduleTime: &metav1.Time{Time: metav1.Now().Time},
		},
	}

	objs = append(objs, schedule)
	r := newTestScanScheduleReconciler(objs...)

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "cleanup-schedule",
			Namespace: "default",
		},
	})

	require.NoError(t, err)

	// Verify excess completed scans were deleted
	var remaining clamavv1alpha1.ClusterScanList
	err = r.List(context.Background(), &remaining,
		client.InNamespace("default"),
		client.MatchingLabels{"clamav.io/schedule": "cleanup-schedule"},
	)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(remaining.Items), int(limit),
		"history cleanup should prune to SuccessfulScansHistoryLimit")
}
