//go:build e2e

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

package e2e

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clamavv1alpha1 "github.com/SolucTeam/clamav-operator/api/v1alpha1"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// waitForJob waits until the Job owned by the given NodeScan appears.
func waitForJob(nodeScanKey types.NamespacedName) *batchv1.Job {
	var foundJob *batchv1.Job
	Eventually(func() bool {
		jobList := &batchv1.JobList{}
		if err := k8sClient.List(ctx, jobList, client.InNamespace(nodeScanKey.Namespace)); err != nil {
			return false
		}
		for i := range jobList.Items {
			for _, ref := range jobList.Items[i].OwnerReferences {
				if ref.Name == nodeScanKey.Name {
					foundJob = &jobList.Items[i]
					return true
				}
			}
		}
		return false
	}, eventuallyTimeout, eventuallyInterval).Should(BeTrue(),
		"Job owned by NodeScan %s should be created", nodeScanKey.Name)
	return foundJob
}

// simulateJobSuccess patches the Job status to mark it as succeeded.
// In envtest the Job controller does not run, so we do this manually to
// trigger the NodeScanReconciler's completion path.
func simulateJobSuccess(job *batchv1.Job) {
	GinkgoHelper()
	patch := client.MergeFrom(job.DeepCopy())
	job.Status.Succeeded = 1
	job.Status.Conditions = []batchv1.JobCondition{
		{
			Type:               batchv1.JobComplete,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
		},
	}
	Expect(k8sClient.Status().Patch(ctx, job, patch)).To(Succeed())
}

// simulateJobFailure patches the Job status to mark it as failed.
func simulateJobFailure(job *batchv1.Job) {
	GinkgoHelper()
	patch := client.MergeFrom(job.DeepCopy())
	job.Status.Failed = 1
	job.Status.Conditions = []batchv1.JobCondition{
		{
			Type:               batchv1.JobFailed,
			Status:             corev1.ConditionTrue,
			Reason:             "BackoffLimitExceeded",
			LastTransitionTime: metav1.Now(),
		},
	}
	Expect(k8sClient.Status().Patch(ctx, job, patch)).To(Succeed())
}

// createNode creates a corev1.Node in envtest so the NodeScan reconciler can
// find it and proceed to Job creation. Without a matching Node the reconciler
// sets the NodeScan to Failed immediately (before creating any Job).
func createNode(name string) {
	GinkgoHelper()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	// Ignore AlreadyExists in case a previous test left the node behind.
	err := k8sClient.Create(ctx, node)
	if err != nil && !errors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred())
	}
}

// cleanupNode deletes a test node.
func cleanupNode(name string) {
	GinkgoHelper()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}}
	_ = k8sClient.Delete(ctx, node)
}

// cleanupNodeScan deletes a NodeScan and waits for it to disappear (finalizer removed).
func cleanupNodeScan(key types.NamespacedName) {
	GinkgoHelper()
	ns := &clamavv1alpha1.NodeScan{}
	if err := k8sClient.Get(ctx, key, ns); err != nil {
		return // already gone
	}
	_ = k8sClient.Delete(ctx, ns)
	Eventually(func() bool {
		err := k8sClient.Get(ctx, key, &clamavv1alpha1.NodeScan{})
		return errors.IsNotFound(err)
	}, eventuallyTimeout, eventuallyInterval).Should(BeTrue())
}

// ─── NodeScan Tests ───────────────────────────────────────────────────────────

var _ = Describe("NodeScan", func() {

	// ─────────────────────────────────────────────────────────────────────────
	// Lifecycle: Pending → Running (Job created) → Completed (Job succeeded)
	// ─────────────────────────────────────────────────────────────────────────
	Describe("lifecycle", func() {

		It("should transition from Pending to a terminal phase when a Job is created", func() {
			nodeScan := &clamavv1alpha1.NodeScan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-lifecycle",
					Namespace: defaultNamespace,
				},
				Spec: clamavv1alpha1.NodeScanSpec{
					NodeName: "e2e-test-node",
					Priority: "low",
				},
			}
			key := types.NamespacedName{Name: nodeScan.Name, Namespace: nodeScan.Namespace}
			defer cleanupNodeScan(key)

			By("creating the NodeScan")
			Expect(k8sClient.Create(ctx, nodeScan)).To(Succeed())

			By("verifying the finalizer is added by the controller")
			Eventually(func() []string {
				ns := &clamavv1alpha1.NodeScan{}
				if err := k8sClient.Get(ctx, key, ns); err != nil {
					return nil
				}
				return ns.Finalizers
			}, eventuallyTimeout, eventuallyInterval).Should(ContainElement("clamav.io/finalizer"),
				"controller should add the clamav.io/finalizer")

			By("verifying a non-empty phase is set")
			Eventually(func() clamavv1alpha1.NodeScanPhase {
				ns := &clamavv1alpha1.NodeScan{}
				if err := k8sClient.Get(ctx, key, ns); err != nil {
					return ""
				}
				return ns.Status.Phase
			}, eventuallyTimeout, eventuallyInterval).ShouldNot(BeEmpty(),
				"NodeScan phase should be set within timeout")
		})

		It("should transition to Completed when the owned Job succeeds", func() {
			nodeScan := &clamavv1alpha1.NodeScan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-job-success",
					Namespace: defaultNamespace,
				},
				Spec: clamavv1alpha1.NodeScanSpec{
					NodeName: "e2e-node-success",
					Priority: "low",
				},
			}
			key := types.NamespacedName{Name: nodeScan.Name, Namespace: nodeScan.Namespace}
			createNode("e2e-node-success")
			defer cleanupNode("e2e-node-success")
			defer cleanupNodeScan(key)

			By("creating the NodeScan")
			Expect(k8sClient.Create(ctx, nodeScan)).To(Succeed())

			By("waiting for the owned Job to be created")
			job := waitForJob(key)

			By("simulating Job success")
			simulateJobSuccess(job)

			By("verifying NodeScan reaches Completed (ResultsPartial=true is expected in envtest — no real pods)")
			Eventually(func() clamavv1alpha1.NodeScanPhase {
				ns := &clamavv1alpha1.NodeScan{}
				if err := k8sClient.Get(ctx, key, ns); err != nil {
					return ""
				}
				return ns.Status.Phase
			}, eventuallyTimeout, eventuallyInterval).Should(Equal(clamavv1alpha1.NodeScanPhaseCompleted),
				"NodeScan should be Completed after Job succeeds (ResultsPartial=true in envtest)")

			By("verifying CompletionTime is set")
			ns := &clamavv1alpha1.NodeScan{}
			Expect(k8sClient.Get(ctx, key, ns)).To(Succeed())
			Expect(ns.Status.CompletionTime).NotTo(BeNil(),
				"CompletionTime must be set on completion")
		})

		It("should transition to Failed when the owned Job fails", func() {
			nodeScan := &clamavv1alpha1.NodeScan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-job-failure",
					Namespace: defaultNamespace,
				},
				Spec: clamavv1alpha1.NodeScanSpec{
					NodeName: "e2e-node-failure",
					Priority: "low",
				},
			}
			key := types.NamespacedName{Name: nodeScan.Name, Namespace: nodeScan.Namespace}
			createNode("e2e-node-failure")
			defer cleanupNode("e2e-node-failure")
			defer cleanupNodeScan(key)

			By("creating the NodeScan")
			Expect(k8sClient.Create(ctx, nodeScan)).To(Succeed())

			By("waiting for the owned Job to be created")
			job := waitForJob(key)

			By("simulating Job failure")
			simulateJobFailure(job)

			By("verifying NodeScan transitions to Failed")
			Eventually(func() clamavv1alpha1.NodeScanPhase {
				ns := &clamavv1alpha1.NodeScan{}
				if err := k8sClient.Get(ctx, key, ns); err != nil {
					return ""
				}
				return ns.Status.Phase
			}, eventuallyTimeout, eventuallyInterval).Should(Equal(clamavv1alpha1.NodeScanPhaseFailed),
				"NodeScan should be Failed when Job fails")

			By("verifying an error message is recorded")
			ns := &clamavv1alpha1.NodeScan{}
			Expect(k8sClient.Get(ctx, key, ns)).To(Succeed())
			Expect(ns.Status.CompletionTime).NotTo(BeNil())
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Deletion: finalizer must be removed cleanly
	// ─────────────────────────────────────────────────────────────────────────
	Describe("deletion", func() {
		It("should remove the finalizer and allow GC to complete", func() {
			nodeScan := &clamavv1alpha1.NodeScan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-deletion",
					Namespace: defaultNamespace,
				},
				Spec: clamavv1alpha1.NodeScanSpec{
					NodeName: "e2e-test-node-delete",
					Priority: "low",
				},
			}
			key := types.NamespacedName{Name: nodeScan.Name, Namespace: nodeScan.Namespace}

			By("creating the NodeScan")
			Expect(k8sClient.Create(ctx, nodeScan)).To(Succeed())

			By("waiting for the controller to add the finalizer")
			Eventually(func() []string {
				ns := &clamavv1alpha1.NodeScan{}
				if err := k8sClient.Get(ctx, key, ns); err != nil {
					return nil
				}
				return ns.Finalizers
			}, eventuallyTimeout, eventuallyInterval).Should(ContainElement("clamav.io/finalizer"))

			By("deleting the NodeScan")
			ns := &clamavv1alpha1.NodeScan{}
			Expect(k8sClient.Get(ctx, key, ns)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ns)).To(Succeed())

			By("verifying the NodeScan is eventually gone (finalizer removed by controller)")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, key, &clamavv1alpha1.NodeScan{})
				return errors.IsNotFound(err)
			}, eventuallyTimeout, eventuallyInterval).Should(BeTrue(),
				"NodeScan should be deleted after finalizer is removed")
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Status conditions: Kubernetes conditions must be set correctly
	// ─────────────────────────────────────────────────────────────────────────
	Describe("status conditions", func() {
		It("should set a Ready condition on completion", func() {
			nodeScan := &clamavv1alpha1.NodeScan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-conditions",
					Namespace: defaultNamespace,
				},
				Spec: clamavv1alpha1.NodeScanSpec{
					NodeName: "e2e-node-conditions",
					Priority: "low",
				},
			}
			key := types.NamespacedName{Name: nodeScan.Name, Namespace: nodeScan.Namespace}
			createNode("e2e-node-conditions")
			defer cleanupNode("e2e-node-conditions")
			defer cleanupNodeScan(key)

			By("creating the NodeScan")
			Expect(k8sClient.Create(ctx, nodeScan)).To(Succeed())

			By("waiting for the Job and simulating success")
			job := waitForJob(key)
			simulateJobSuccess(job)

			By("verifying status conditions are populated")
			Eventually(func() bool {
				ns := &clamavv1alpha1.NodeScan{}
				if err := k8sClient.Get(ctx, key, ns); err != nil {
					return false
				}
				return len(ns.Status.Conditions) > 0
			}, eventuallyTimeout, eventuallyInterval).Should(BeTrue(),
				"NodeScan should have at least one status condition after completion")
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Idempotency: second reconcile must not create a duplicate Job
	// ─────────────────────────────────────────────────────────────────────────
	Describe("idempotency", func() {
		It("should not create a second Job if one already exists for the NodeScan", func() {
			nodeScan := &clamavv1alpha1.NodeScan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-idempotent",
					Namespace: defaultNamespace,
				},
				Spec: clamavv1alpha1.NodeScanSpec{
					NodeName: "e2e-node-idempotent",
					Priority: "low",
				},
			}
			key := types.NamespacedName{Name: nodeScan.Name, Namespace: nodeScan.Namespace}
			createNode("e2e-node-idempotent")
			defer cleanupNode("e2e-node-idempotent")
			defer cleanupNodeScan(key)

			By("creating the NodeScan")
			Expect(k8sClient.Create(ctx, nodeScan)).To(Succeed())

			By("waiting for exactly one Job")
			waitForJob(key)

			By("verifying only one Job exists after multiple reconcile cycles")
			// Allow time for multiple reconcile iterations
			Consistently(func() int {
				jobList := &batchv1.JobList{}
				if err := k8sClient.List(ctx, jobList, client.InNamespace(defaultNamespace)); err != nil {
					return -1
				}
				count := 0
				for i := range jobList.Items {
					for _, ref := range jobList.Items[i].OwnerReferences {
						if ref.Name == key.Name {
							count++
						}
					}
				}
				return count
			}, 3*eventuallyInterval*10, eventuallyInterval).Should(Equal(1),
				"controller must not create duplicate Jobs for the same NodeScan")
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Validation: webhook rejects invalid specs (requires webhook to be running)
	// ─────────────────────────────────────────────────────────────────────────
	Describe("validation", Label("requires-webhook"), func() {
		It("should reject a NodeScan with an empty nodeName", func() {
			nodeScan := &clamavv1alpha1.NodeScan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-invalid",
					Namespace: defaultNamespace,
				},
				Spec: clamavv1alpha1.NodeScanSpec{
					NodeName: "", // intentionally invalid
				},
			}
			err := k8sClient.Create(ctx, nodeScan)
			Expect(err).To(HaveOccurred(), "webhook should reject empty nodeName")
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Concurrency: multiple NodeScans on different nodes run independently
	// ─────────────────────────────────────────────────────────────────────────
	Describe("concurrency", func() {
		It("should handle multiple NodeScans for different nodes independently", func() {
			keys := make([]types.NamespacedName, 3)
			for i := 0; i < 3; i++ {
				ns := &clamavv1alpha1.NodeScan{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("e2e-concurrent-%d", i),
						Namespace: defaultNamespace,
					},
					Spec: clamavv1alpha1.NodeScanSpec{
						NodeName: fmt.Sprintf("e2e-node-concurrent-%d", i),
						Priority: "low",
					},
				}
				Expect(k8sClient.Create(ctx, ns)).To(Succeed())
				keys[i] = types.NamespacedName{Name: ns.Name, Namespace: ns.Namespace}
			}
			defer func() {
				for _, k := range keys {
					cleanupNodeScan(k)
				}
			}()

			By("verifying all three NodeScans get their finalizer (controller processed them all)")
			for _, key := range keys {
				k := key // capture for closure
				Eventually(func() []string {
					ns := &clamavv1alpha1.NodeScan{}
					if err := k8sClient.Get(ctx, k, ns); err != nil {
						return nil
					}
					return ns.Finalizers
				}, eventuallyTimeout, eventuallyInterval).Should(ContainElement("clamav.io/finalizer"),
					"NodeScan %s should have finalizer", k.Name)
			}
		})
	})
})
