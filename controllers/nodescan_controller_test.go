/*
Copyright 2025 Platform Team - Numspot.

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

package controllers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	clamavv1alpha1 "github.com/SolucTeam/clamav-operator/api/v1alpha1"
)

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	_ = clamavv1alpha1.AddToScheme(scheme)
	return scheme
}

func newTestNodeScanReconciler(objs ...client.Object) *NodeScanReconciler {
	scheme := newTestScheme()
	fakeClient := fakeclient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&clamavv1alpha1.NodeScan{}).
		Build()

	return &NodeScanReconciler{
		Client:       fakeClient,
		Scheme:       scheme,
		Recorder:     record.NewFakeRecorder(100),
		Clientset:    fake.NewSimpleClientset(),
		ScannerImage: "test-scanner:latest",
		ClamavHost:   "clamav.test.svc",
		ClamavPort:   3310,
	}
}

func TestNodeScanReconciler_Reconcile_NotFound(t *testing.T) {
	r := newTestNodeScanReconciler()

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "nonexistent",
			Namespace: "default",
		},
	})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestNodeScanReconciler_Reconcile_NodeNotFound(t *testing.T) {
	nodeScan := &clamavv1alpha1.NodeScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scan",
			Namespace: "default",
		},
		Spec: clamavv1alpha1.NodeScanSpec{
			NodeName: "nonexistent-node",
		},
	}

	r := newTestNodeScanReconciler(nodeScan)

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-scan",
			Namespace: "default",
		},
	})

	// Should update status to failed due to node not found
	assert.NoError(t, err)

	// Verify the NodeScan status was updated
	var updatedScan clamavv1alpha1.NodeScan
	err = r.Get(context.Background(), types.NamespacedName{Name: "test-scan", Namespace: "default"}, &updatedScan)
	require.NoError(t, err)
	assert.Equal(t, clamavv1alpha1.NodeScanPhaseFailed, updatedScan.Status.Phase)
}

func TestNodeScanReconciler_Reconcile_CreateJob(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}

	nodeScan := &clamavv1alpha1.NodeScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scan",
			Namespace: "default",
		},
		Spec: clamavv1alpha1.NodeScanSpec{
			NodeName:      "test-node",
			Priority:      "medium",
			MaxConcurrent: 5,
		},
	}

	r := newTestNodeScanReconciler(node, nodeScan)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-scan",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	// Should requeue after job creation
	assert.Equal(t, 30*time.Second, result.RequeueAfter)

	// Verify Job was created
	var job batchv1.Job
	err = r.Get(context.Background(), types.NamespacedName{
		Name:      "nodescan-test-scan",
		Namespace: "default",
	}, &job)
	require.NoError(t, err)
	assert.Equal(t, "test-node", job.Spec.Template.Spec.NodeName)
}

func TestNodeScanReconciler_Reconcile_WithScanPolicy(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}

	scanPolicy := &clamavv1alpha1.ScanPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "default",
		},
		Spec: clamavv1alpha1.ScanPolicySpec{
			Paths:         []string{"/custom/path"},
			MaxConcurrent: 10,
		},
	}

	nodeScan := &clamavv1alpha1.NodeScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scan",
			Namespace: "default",
		},
		Spec: clamavv1alpha1.NodeScanSpec{
			NodeName:   "test-node",
			ScanPolicy: "test-policy",
		},
	}

	r := newTestNodeScanReconciler(node, scanPolicy, nodeScan)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-scan",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, result.RequeueAfter)
}

func TestNodeScanReconciler_Reconcile_Deletion(t *testing.T) {
	now := metav1.Now()
	nodeScan := &clamavv1alpha1.NodeScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-scan",
			Namespace:         "default",
			DeletionTimestamp: &now,
			Finalizers:        []string{nodeScanFinalizer},
		},
		Spec: clamavv1alpha1.NodeScanSpec{
			NodeName: "test-node",
		},
	}

	r := newTestNodeScanReconciler(nodeScan)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-scan",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestConstructJobForNodeScan(t *testing.T) {
	tests := []struct {
		name       string
		nodeScan   *clamavv1alpha1.NodeScan
		scanPolicy *clamavv1alpha1.ScanPolicy
		wantErr    bool
	}{
		{
			name: "basic node scan",
			nodeScan: &clamavv1alpha1.NodeScan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-scan",
					Namespace: "default",
					UID:       "test-uid",
				},
				Spec: clamavv1alpha1.NodeScanSpec{
					NodeName: "test-node",
					Priority: "medium",
				},
			},
			scanPolicy: nil,
			wantErr:    false,
		},
		{
			name: "with scan policy",
			nodeScan: &clamavv1alpha1.NodeScan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-scan",
					Namespace: "default",
					UID:       "test-uid",
				},
				Spec: clamavv1alpha1.NodeScanSpec{
					NodeName: "test-node",
				},
			},
			scanPolicy: &clamavv1alpha1.ScanPolicy{
				Spec: clamavv1alpha1.ScanPolicySpec{
					Paths:         []string{"/custom"},
					MaxConcurrent: 10,
				},
			},
			wantErr: false,
		},
		{
			name: "high priority scan",
			nodeScan: &clamavv1alpha1.NodeScan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-scan",
					Namespace: "default",
					UID:       "test-uid",
				},
				Spec: clamavv1alpha1.NodeScanSpec{
					NodeName: "test-node",
					Priority: "high",
				},
			},
			scanPolicy: nil,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestNodeScanReconciler()
			job, err := r.constructJobForNodeScan(tt.nodeScan, tt.scanPolicy)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, job)

			// Verify job properties
			assert.Equal(t, tt.nodeScan.Spec.NodeName, job.Spec.Template.Spec.NodeName)
			assert.Equal(t, "clamav-scanner", job.Spec.Template.Spec.ServiceAccountName)
			assert.True(t, job.Spec.Template.Spec.HostPID)

			// Verify resources are set
			container := job.Spec.Template.Spec.Containers[0]
			assert.NotEmpty(t, container.Resources.Requests)
			assert.NotEmpty(t, container.Resources.Limits)
		})
	}
}

func TestGetResourcesForPriority(t *testing.T) {
	tests := []struct {
		priority        string
		expectedCPUReq  string
		expectedMemReq  string
		expectedCPULim  string
		expectedMemLim  string
	}{
		{
			priority:       "high",
			expectedCPUReq: "500m",
			expectedMemReq: "512Mi",
			expectedCPULim: "2",
			expectedMemLim: "1Gi",
		},
		{
			priority:       "medium",
			expectedCPUReq: "100m",
			expectedMemReq: "256Mi",
			expectedCPULim: "1",
			expectedMemLim: "512Mi",
		},
		{
			priority:       "low",
			expectedCPUReq: "50m",
			expectedMemReq: "128Mi",
			expectedCPULim: "500m",
			expectedMemLim: "256Mi",
		},
		{
			priority:       "",
			expectedCPUReq: "100m",
			expectedMemReq: "256Mi",
			expectedCPULim: "1",
			expectedMemLim: "512Mi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.priority, func(t *testing.T) {
			resources := GetResourcesForPriority(tt.priority)

			assert.NotNil(t, resources.Requests)
			assert.NotNil(t, resources.Limits)
		})
	}
}
