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
		Client:                fakeClient,
		Scheme:                scheme,
		Recorder:              record.NewFakeRecorder(100),
		Clientset:             fake.NewSimpleClientset(),
		ScannerImage:          "test-scanner:latest",
		ClamavHost:            "clamav.test.svc",
		ClamavPort:            3310,
		ScannerServiceAccount: "clamav-scanner",
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
			// HostPID was removed (audit 2026-10): file scanning does not need
			// host process visibility. Assert it stays OFF — re-enabling it
			// should be a deliberate, reviewed decision, not a regression.
			assert.False(t, job.Spec.Template.Spec.HostPID)
			assert.False(t, job.Spec.Template.Spec.HostIPC)

			// Resources are intentionally not set by the operator — the user
			// controls them via ScanPolicy.spec.resources or NodeScan.spec.resources.
			// Verify the container exists; resource assertions belong in integration tests.
			assert.NotEmpty(t, job.Spec.Template.Spec.Containers)
		})
	}
}

// envMap flattens the scanner container's env vars for lookups.
func envMap(t *testing.T, job *batchv1.Job) map[string]corev1.EnvVar {
	t.Helper()
	require.NotEmpty(t, job.Spec.Template.Spec.Containers)
	env := make(map[string]corev1.EnvVar, len(job.Spec.Template.Spec.Containers[0].Env))
	for _, e := range job.Spec.Template.Spec.Containers[0].Env {
		env[e.Name] = e
	}
	return env
}

func TestSignatureRegistryEnvVars(t *testing.T) {
	t.Run("disabled when SignatureRegistry is nil (baked-in signatures)", func(t *testing.T) {
		r := newTestNodeScanReconciler()
		assert.Nil(t, r.signatureRegistryEnvVars())
	})

	t.Run("disabled when source is image", func(t *testing.T) {
		r := newTestNodeScanReconciler()
		r.SignatureRegistry = &SignatureRegistryConfig{Source: "image"}
		assert.Nil(t, r.signatureRegistryEnvVars())
	})

	t.Run("registry source without secret injects refs and url", func(t *testing.T) {
		r := newTestNodeScanReconciler()
		r.SignatureRegistry = &SignatureRegistryConfig{
			Source:      "registry",
			RegistryURL: "https://registry.tooling.example.internal",
			Refs:        []string{"platform-iac/clamav-main:latest", "platform-iac/clamav-daily:latest"},
		}
		vars := r.signatureRegistryEnvVars()

		names := make([]string, 0, len(vars))
		for _, v := range vars {
			names = append(names, v.Name)
		}
		assert.ElementsMatch(t,
			[]string{"SIGNATURE_SOURCE", "SIGNATURE_REGISTRY_URL", "SIGNATURE_REFS"}, names)
		assert.Equal(t, "registry", vars[0].Value)
		assert.Equal(t, "https://registry.tooling.example.internal", vars[1].Value)
		assert.Equal(t,
			"platform-iac/clamav-main:latest,platform-iac/clamav-daily:latest", vars[2].Value)
	})

	t.Run("existingSecret injects username/password via secretKeyRef", func(t *testing.T) {
		r := newTestNodeScanReconciler()
		r.SignatureRegistry = &SignatureRegistryConfig{
			Source:      "registry",
			RegistryURL: "https://registry.tooling.example.internal",
			Refs:        []string{"platform-iac/clamav-daily:latest"},
			SecretName:  "clamav-registry-auth",
		}
		vars := r.signatureRegistryEnvVars()
		require.Len(t, vars, 5)

		byName := make(map[string]corev1.EnvVar, len(vars))
		for _, v := range vars {
			byName[v.Name] = v
		}
		for _, key := range []string{"SIGNATURE_REGISTRY_USER", "SIGNATURE_REGISTRY_PASSWORD"} {
			e, ok := byName[key]
			require.True(t, ok, "%s must be present", key)
			require.NotNil(t, e.ValueFrom, "%s must use ValueFrom", key)
			require.NotNil(t, e.ValueFrom.SecretKeyRef)
			assert.Equal(t, "clamav-registry-auth", e.ValueFrom.SecretKeyRef.Name)
			// Values must never be inlined into the Job spec.
			assert.Empty(t, e.Value)
		}
		assert.Equal(t, "username", byName["SIGNATURE_REGISTRY_USER"].ValueFrom.SecretKeyRef.Key)
		assert.Equal(t, "password", byName["SIGNATURE_REGISTRY_PASSWORD"].ValueFrom.SecretKeyRef.Key)
	})
}

func TestConstructJobForNodeScan_SignatureRegistry(t *testing.T) {
	nodeScan := &clamavv1alpha1.NodeScan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scan",
			Namespace: "default",
			UID:       "test-uid",
		},
		Spec: clamavv1alpha1.NodeScanSpec{
			NodeName: "test-node",
			Priority: "medium",
		},
	}

	t.Run("no SIGNATURE_* env vars when disabled", func(t *testing.T) {
		r := newTestNodeScanReconciler(nodeScan)
		job, err := r.constructJobForNodeScan(nodeScan, nil)
		require.NoError(t, err)

		env := envMap(t, job)
		for _, name := range []string{
			"SIGNATURE_SOURCE", "SIGNATURE_REGISTRY_URL", "SIGNATURE_REFS",
			"SIGNATURE_REGISTRY_USER", "SIGNATURE_REGISTRY_PASSWORD",
		} {
			assert.NotContains(t, env, name)
		}
	})

	t.Run("SIGNATURE_* env vars injected when registry source enabled", func(t *testing.T) {
		r := newTestNodeScanReconciler(nodeScan)
		r.SignatureRegistry = &SignatureRegistryConfig{
			Source:      "registry",
			RegistryURL: "https://registry.tooling.example.internal",
			Refs: []string{
				"platform-iac/clamav-main:latest",
				"platform-iac/clamav-daily:latest",
				"platform-iac/clamav-bytecode:latest",
			},
			SecretName: "clamav-registry-auth",
		}
		job, err := r.constructJobForNodeScan(nodeScan, nil)
		require.NoError(t, err)

		env := envMap(t, job)
		assert.Equal(t, "registry", env["SIGNATURE_SOURCE"].Value)
		assert.Equal(t, "https://registry.tooling.example.internal", env["SIGNATURE_REGISTRY_URL"].Value)
		assert.Equal(t,
			"platform-iac/clamav-main:latest,platform-iac/clamav-daily:latest,platform-iac/clamav-bytecode:latest",
			env["SIGNATURE_REFS"].Value)
		assert.Equal(t, "clamav-registry-auth",
			env["SIGNATURE_REGISTRY_USER"].ValueFrom.SecretKeyRef.Name)
		assert.Equal(t, "password",
			env["SIGNATURE_REGISTRY_PASSWORD"].ValueFrom.SecretKeyRef.Key)
	})
}

func TestGetResourcesForPriority(t *testing.T) {
	// GetResourcesForPriority intentionally returns empty ResourceRequirements
	// regardless of priority. The operator imposes no resource constraints;
	// sizing is the user's responsibility via ScanPolicy.spec.resources or
	// NodeScan.spec.resources (or the cluster's LimitRange / namespace defaults).
	priorities := []string{"high", "medium", "low", ""}
	for _, priority := range priorities {
		t.Run(priority, func(t *testing.T) {
			resources := GetResourcesForPriority(priority)
			assert.Nil(t, resources.Requests, "expected no resource requests for priority %q", priority)
			assert.Nil(t, resources.Limits, "expected no resource limits for priority %q", priority)
		})
	}
}

// TestGetJobFailureInfo verifies failure diagnosis for both classic
// (container terminated) and never-started (OOM admission rejection,
// unschedulable, image pull) Job failures. Pods that never ran a container
// must NOT be reported as "UnknownError (exit 0)".
func TestGetJobFailureInfo(t *testing.T) {
	jobLabels := map[string]string{"job-name": "scan-job"}
	newJob := func() *batchv1.Job {
		return &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "scan-job", Namespace: "default"},
			Spec: batchv1.JobSpec{
				Selector: &metav1.LabelSelector{MatchLabels: jobLabels},
			},
		}
	}
	newPod := func(mutate func(*corev1.Pod)) *corev1.Pod {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "scan-job-pod",
				Namespace: "default",
				Labels:    jobLabels,
			},
		}
		if mutate != nil {
			mutate(pod)
		}
		return pod
	}

	tests := []struct {
		name         string
		pods         []*corev1.Pod
		wantReason   string
		wantExitCode int32
	}{
		{
			name: "terminated container returns real reason and exit code",
			pods: []*corev1.Pod{newPod(func(p *corev1.Pod) {
				p.Status.ContainerStatuses = []corev1.ContainerStatus{{
					Name: "scanner",
					State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
						Reason:   "Error",
						ExitCode: 2,
					}},
				}}
			})},
			wantReason:   "Error",
			wantExitCode: 2,
		},
		{
			name: "terminated container without reason defaults to Error",
			pods: []*corev1.Pod{newPod(func(p *corev1.Pod) {
				p.Status.ContainerStatuses = []corev1.ContainerStatus{{
					Name: "scanner",
					State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 1,
					}},
				}}
			})},
			wantReason:   "Error",
			wantExitCode: 1,
		},
		{
			name: "kubelet admission rejection (OutOfmemory) is reported with exit -1",
			pods: []*corev1.Pod{newPod(func(p *corev1.Pod) {
				p.Status.Reason = "OutOfmemory"
			})},
			wantReason:   "OutOfmemory",
			wantExitCode: -1,
		},
		{
			name: "evicted pod is reported with exit -1",
			pods: []*corev1.Pod{newPod(func(p *corev1.Pod) {
				p.Status.Reason = "Evicted"
			})},
			wantReason:   "Evicted",
			wantExitCode: -1,
		},
		{
			name: "unschedulable pod is reported with exit -1",
			pods: []*corev1.Pod{newPod(func(p *corev1.Pod) {
				p.Status.Conditions = []corev1.PodCondition{{
					Type:   corev1.PodScheduled,
					Status: corev1.ConditionFalse,
					Reason: "Unschedulable",
				}}
			})},
			wantReason:   "Unschedulable",
			wantExitCode: -1,
		},
		{
			name: "stuck image pull is reported with exit -1",
			pods: []*corev1.Pod{newPod(func(p *corev1.Pod) {
				p.Status.ContainerStatuses = []corev1.ContainerStatus{{
					Name:  "scanner",
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}},
				}}
			})},
			wantReason:   "ImagePullBackOff",
			wantExitCode: -1,
		},
		{
			name: "last termination state wins over restart wrapper",
			pods: []*corev1.Pod{newPod(func(p *corev1.Pod) {
				p.Status.ContainerStatuses = []corev1.ContainerStatus{{
					Name: "scanner",
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
						Reason: "CrashLoopBackOff",
					}},
					LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
						Reason:   "OOMKilled",
						ExitCode: 137,
					}},
				}}
			})},
			wantReason:   "OOMKilled",
			wantExitCode: 137,
		},
		{
			name:         "no pods falls back to UnknownError exit 0",
			pods:         nil,
			wantReason:   "UnknownError",
			wantExitCode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := make([]client.Object, 0, len(tt.pods))
			for _, p := range tt.pods {
				objs = append(objs, p)
			}
			r := newTestNodeScanReconciler(objs...)

			reason, exitCode := r.getJobFailureInfo(context.Background(), newJob())
			assert.Equal(t, tt.wantReason, reason)
			assert.Equal(t, tt.wantExitCode, exitCode)
		})
	}
}
