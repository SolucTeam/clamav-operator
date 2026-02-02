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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	clamavv1alpha1 "github.com/SolucTeam/clamav-operator/api/v1alpha1"
)

func newTestClusterScanReconciler(objs ...client.Object) *ClusterScanReconciler {
	scheme := newTestScheme()
	fakeClient := fakeclient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&clamavv1alpha1.ClusterScan{}, &clamavv1alpha1.NodeScan{}).
		Build()

	return &ClusterScanReconciler{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(100),
	}
}

func TestClusterScanReconciler_Reconcile_NotFound(t *testing.T) {
	r := newTestClusterScanReconciler()

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "nonexistent",
			Namespace: "default",
		},
	})

	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestClusterScanReconciler_Reconcile_CreateNodeScans(t *testing.T) {
	nodes := []client.Object{
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-1",
				Labels: map[string]string{
					"kubernetes.io/os": "linux",
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
				Labels: map[string]string{
					"kubernetes.io/os": "linux",
				},
			},
		},
	}

	clusterScan := &clamavv1alpha1.ClusterScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-scan",
			Namespace: "default",
		},
		Spec: clamavv1alpha1.ClusterScanSpec{
			Concurrent: 2,
		},
	}

	objs := append(nodes, clusterScan)
	r := newTestClusterScanReconciler(objs...)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-cluster-scan",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, result.RequeueAfter)

	// Verify NodeScans were created
	var nodeScans clamavv1alpha1.NodeScanList
	err = r.List(context.Background(), &nodeScans, client.InNamespace("default"))
	require.NoError(t, err)
	assert.Len(t, nodeScans.Items, 2)
}

func TestClusterScanReconciler_Reconcile_WithNodeSelector(t *testing.T) {
	nodes := []client.Object{
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-1",
				Labels: map[string]string{
					"node-role": "worker",
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "control-plane-1",
				Labels: map[string]string{
					"node-role": "control-plane",
				},
			},
		},
	}

	clusterScan := &clamavv1alpha1.ClusterScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-scan",
			Namespace: "default",
		},
		Spec: clamavv1alpha1.ClusterScanSpec{
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"node-role": "worker",
				},
			},
			Concurrent: 1,
		},
	}

	objs := append(nodes, clusterScan)
	r := newTestClusterScanReconciler(objs...)

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-cluster-scan",
			Namespace: "default",
		},
	})

	require.NoError(t, err)

	// Verify only 1 NodeScan was created (for worker node only)
	var nodeScans clamavv1alpha1.NodeScanList
	err = r.List(context.Background(), &nodeScans, client.InNamespace("default"))
	require.NoError(t, err)
	assert.Len(t, nodeScans.Items, 1)
	assert.Equal(t, "worker-1", nodeScans.Items[0].Spec.NodeName)
}

func TestClusterScanReconciler_Reconcile_Completion(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
		},
	}

	clusterScan := &clamavv1alpha1.ClusterScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-scan",
			Namespace: "default",
		},
		Spec: clamavv1alpha1.ClusterScanSpec{
			Concurrent: 1,
		},
		Status: clamavv1alpha1.ClusterScanStatus{
			Phase:      clamavv1alpha1.ClusterScanPhaseRunning,
			TotalNodes: 1,
		},
	}

	// Create a completed NodeScan
	nodeScan := &clamavv1alpha1.NodeScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-scan-node-1",
			Namespace: "default",
			Labels: map[string]string{
				"clamav.platform.numspot.com/clusterscan": "test-cluster-scan",
			},
		},
		Spec: clamavv1alpha1.NodeScanSpec{
			NodeName: "node-1",
		},
		Status: clamavv1alpha1.NodeScanStatus{
			Phase:        clamavv1alpha1.NodeScanPhaseCompleted,
			FilesScanned: 1000,
		},
	}

	r := newTestClusterScanReconciler(node, clusterScan, nodeScan)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-cluster-scan",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	// Should not requeue when completed
	assert.Equal(t, ctrl.Result{}, result)

	// Verify ClusterScan is marked as completed
	var updatedScan clamavv1alpha1.ClusterScan
	err = r.Get(context.Background(), types.NamespacedName{
		Name:      "test-cluster-scan",
		Namespace: "default",
	}, &updatedScan)
	require.NoError(t, err)
	assert.Equal(t, clamavv1alpha1.ClusterScanPhaseCompleted, updatedScan.Status.Phase)
}

func TestClusterScanReconciler_Reconcile_ConcurrencyLimit(t *testing.T) {
	nodes := []client.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-2"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-3"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-4"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-5"}},
	}

	clusterScan := &clamavv1alpha1.ClusterScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-scan",
			Namespace: "default",
		},
		Spec: clamavv1alpha1.ClusterScanSpec{
			Concurrent: 2, // Only 2 at a time
		},
	}

	objs := append(nodes, clusterScan)
	r := newTestClusterScanReconciler(objs...)

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-cluster-scan",
			Namespace: "default",
		},
	})

	require.NoError(t, err)

	// Verify only 2 NodeScans were created (respecting concurrency limit)
	var nodeScans clamavv1alpha1.NodeScanList
	err = r.List(context.Background(), &nodeScans, client.InNamespace("default"))
	require.NoError(t, err)
	assert.LessOrEqual(t, len(nodeScans.Items), 2)
}

func TestClusterScanReconciler_Deletion(t *testing.T) {
	now := metav1.Now()
	clusterScan := &clamavv1alpha1.ClusterScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-cluster-scan",
			Namespace:         "default",
			DeletionTimestamp: &now,
			Finalizers:        []string{clusterScanFinalizer},
		},
		Spec: clamavv1alpha1.ClusterScanSpec{
			Concurrent: 1,
		},
	}

	// Create associated NodeScans
	nodeScan := &clamavv1alpha1.NodeScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-scan-node-1",
			Namespace: "default",
			Labels: map[string]string{
				"clamav.platform.numspot.com/clusterscan": "test-cluster-scan",
			},
		},
		Spec: clamavv1alpha1.NodeScanSpec{
			NodeName: "node-1",
		},
	}

	r := newTestClusterScanReconciler(clusterScan, nodeScan)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-cluster-scan",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify NodeScan was deleted
	var nodeScans clamavv1alpha1.NodeScanList
	err = r.List(context.Background(), &nodeScans, client.InNamespace("default"))
	require.NoError(t, err)
	assert.Empty(t, nodeScans.Items)
}
