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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clamavv1alpha1 "github.com/SolucTeam/clamav-operator/api/v1alpha1"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// cleanupClusterScan deletes a ClusterScan and waits for it to disappear.
func cleanupClusterScan(key types.NamespacedName) {
	GinkgoHelper()
	cs := &clamavv1alpha1.ClusterScan{}
	if err := k8sClient.Get(ctx, key, cs); err != nil {
		return
	}
	_ = k8sClient.Delete(ctx, cs)
	Eventually(func() bool {
		err := k8sClient.Get(ctx, key, &clamavv1alpha1.ClusterScan{})
		return errors.IsNotFound(err)
	}, eventuallyTimeout, eventuallyInterval).Should(BeTrue())
}

// cleanupScanPolicy deletes a ScanPolicy.
func cleanupScanPolicy(key types.NamespacedName) {
	GinkgoHelper()
	sp := &clamavv1alpha1.ScanPolicy{}
	if err := k8sClient.Get(ctx, key, sp); err != nil {
		return
	}
	_ = k8sClient.Delete(ctx, sp)
}

// ─── ClusterScan Tests ────────────────────────────────────────────────────────

var _ = Describe("ClusterScan", func() {

	// ─────────────────────────────────────────────────────────────────────────
	// Basic lifecycle: ClusterScan should be processed by the reconciler
	// ─────────────────────────────────────────────────────────────────────────
	Describe("lifecycle", func() {
		It("should set a phase on the ClusterScan after creation", func() {
			clusterScan := &clamavv1alpha1.ClusterScan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-clusterscan-basic",
					Namespace: defaultNamespace,
				},
				Spec: clamavv1alpha1.ClusterScanSpec{
					Priority:   "low",
					Concurrent: 2,
				},
			}
			key := types.NamespacedName{Name: clusterScan.Name, Namespace: clusterScan.Namespace}
			defer cleanupClusterScan(key)

			By("creating the ClusterScan")
			Expect(k8sClient.Create(ctx, clusterScan)).To(Succeed())

			By("verifying the controller processes it (phase or TotalNodes set)")
			Eventually(func() bool {
				cs := &clamavv1alpha1.ClusterScan{}
				if err := k8sClient.Get(ctx, key, cs); err != nil {
					return false
				}
				// In envtest there are no real nodes — the ClusterScan will
				// complete immediately (0 nodes) or move to a terminal phase.
				return cs.Status.Phase != ""
			}, eventuallyTimeout, eventuallyInterval).Should(BeTrue(),
				"ClusterScan should have a phase set after reconciliation")
		})

		It("should complete immediately when there are no matching nodes (envtest)", func() {
			clusterScan := &clamavv1alpha1.ClusterScan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-clusterscan-nonodes",
					Namespace: defaultNamespace,
				},
				Spec: clamavv1alpha1.ClusterScanSpec{
					Priority:   "low",
					Concurrent: 1,
					// NodeSelector that matches nothing
					NodeSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"e2e-no-such-label": "true",
						},
					},
				},
			}
			key := types.NamespacedName{Name: clusterScan.Name, Namespace: clusterScan.Namespace}
			defer cleanupClusterScan(key)

			By("creating the ClusterScan with a non-matching node selector")
			Expect(k8sClient.Create(ctx, clusterScan)).To(Succeed())

			By("verifying ClusterScan reaches Completed with 0 nodes")
			Eventually(func() clamavv1alpha1.ClusterScanPhase {
				cs := &clamavv1alpha1.ClusterScan{}
				if err := k8sClient.Get(ctx, key, cs); err != nil {
					return ""
				}
				return cs.Status.Phase
			}, eventuallyTimeout, eventuallyInterval).Should(Equal(clamavv1alpha1.ClusterScanPhaseCompleted),
				"ClusterScan with no matching nodes should complete with 0 NodeScans")

			By("verifying TotalNodes is 0")
			cs := &clamavv1alpha1.ClusterScan{}
			Expect(k8sClient.Get(ctx, key, cs)).To(Succeed())
			Expect(cs.Status.TotalNodes).To(Equal(int32(0)))
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// NodeScan ownership: ClusterScan should own the NodeScans it creates
	// ─────────────────────────────────────────────────────────────────────────
	Describe("NodeScan ownership", func() {
		It("should garbage-collect NodeScans when the ClusterScan is deleted", func() {
			clusterScan := &clamavv1alpha1.ClusterScan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-clusterscan-gc",
					Namespace: defaultNamespace,
				},
				Spec: clamavv1alpha1.ClusterScanSpec{
					Priority:   "low",
					Concurrent: 1,
				},
			}
			key := types.NamespacedName{Name: clusterScan.Name, Namespace: clusterScan.Namespace}

			By("creating the ClusterScan")
			Expect(k8sClient.Create(ctx, clusterScan)).To(Succeed())

			By("waiting for the ClusterScan to be processed")
			Eventually(func() bool {
				cs := &clamavv1alpha1.ClusterScan{}
				return k8sClient.Get(ctx, key, cs) == nil && cs.Status.Phase != ""
			}, eventuallyTimeout, eventuallyInterval).Should(BeTrue())

			By("deleting the ClusterScan")
			cs := &clamavv1alpha1.ClusterScan{}
			Expect(k8sClient.Get(ctx, key, cs)).To(Succeed())
			Expect(k8sClient.Delete(ctx, cs)).To(Succeed())

			By("verifying ClusterScan is eventually deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, key, &clamavv1alpha1.ClusterScan{})
				return errors.IsNotFound(err)
			}, eventuallyTimeout, eventuallyInterval).Should(BeTrue())

			By("verifying owned NodeScans are also cleaned up (cascade GC)")
			Eventually(func() int {
				nsList := &clamavv1alpha1.NodeScanList{}
				if err := k8sClient.List(ctx, nsList,
					client.InNamespace(defaultNamespace),
					client.MatchingLabels{"clamav.io/cluster-scan": clusterScan.Name},
				); err != nil {
					return -1
				}
				return len(nsList.Items)
			}, eventuallyTimeout, eventuallyInterval).Should(Equal(0),
				"NodeScans owned by the deleted ClusterScan should be garbage-collected")
		})
	})
})

// ─── ScanPolicy Tests ─────────────────────────────────────────────────────────

var _ = Describe("ScanPolicy", func() {

	// ─────────────────────────────────────────────────────────────────────────
	// Basic CRUD: ScanPolicy should be persisted and readable
	// ─────────────────────────────────────────────────────────────────────────
	Describe("CRUD", func() {
		It("should create and retrieve a ScanPolicy", func() {
			policy := &clamavv1alpha1.ScanPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-scanpolicy-basic",
					Namespace: defaultNamespace,
				},
				Spec: clamavv1alpha1.ScanPolicySpec{
					Paths: []string{"/var/lib", "/opt"},
					ExcludePatterns: []string{
						"*.tmp",
						"*.log",
					},
					MaxConcurrent: 3,
				},
			}
			key := types.NamespacedName{Name: policy.Name, Namespace: policy.Namespace}
			defer cleanupScanPolicy(key)

			By("creating the ScanPolicy")
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			By("reading it back and verifying fields are preserved")
			retrieved := &clamavv1alpha1.ScanPolicy{}
			Expect(k8sClient.Get(ctx, key, retrieved)).To(Succeed())
			Expect(retrieved.Spec.Paths).To(ConsistOf("/var/lib", "/opt"))
			Expect(retrieved.Spec.MaxConcurrent).To(Equal(int32(3)))
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Reference from NodeScan: a NodeScan that references a ScanPolicy should
	// pick up the policy's configuration during reconciliation.
	// ─────────────────────────────────────────────────────────────────────────
	Describe("reference from NodeScan", func() {
		It("should process a NodeScan that references a ScanPolicy", func() {
			policy := &clamavv1alpha1.ScanPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-policy-ref",
					Namespace: defaultNamespace,
				},
				Spec: clamavv1alpha1.ScanPolicySpec{
					Paths:              []string{"/tmp"},
					MaxConcurrent: 2,
				},
			}
			policyKey := types.NamespacedName{Name: policy.Name, Namespace: policy.Namespace}
			defer cleanupScanPolicy(policyKey)
			Expect(k8sClient.Create(ctx, policy)).To(Succeed())

			nodeScan := &clamavv1alpha1.NodeScan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-ns-with-policy",
					Namespace: defaultNamespace,
				},
				Spec: clamavv1alpha1.NodeScanSpec{
					NodeName:   "e2e-node-policy",
					ScanPolicy: policy.Name,
					Priority:   "low",
				},
			}
			key := types.NamespacedName{Name: nodeScan.Name, Namespace: nodeScan.Namespace}
			defer cleanupNodeScan(key)

			By("creating the NodeScan with policy reference")
			Expect(k8sClient.Create(ctx, nodeScan)).To(Succeed())

			By("verifying the controller processes it (finalizer added)")
			Eventually(func() []string {
				ns := &clamavv1alpha1.NodeScan{}
				if err := k8sClient.Get(ctx, key, ns); err != nil {
					return nil
				}
				return ns.Finalizers
			}, eventuallyTimeout, eventuallyInterval).Should(ContainElement("clamav.io/finalizer"))
		})
	})
})
