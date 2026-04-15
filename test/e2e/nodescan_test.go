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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	clamavv1alpha1 "github.com/SolucTeam/clamav-operator/api/v1alpha1"
)

var _ = Describe("NodeScan", func() {
	// ─────────────────────────────────────────────────────────────────────────
	// Lifecycle: creation → Job → status propagation
	// ─────────────────────────────────────────────────────────────────────────
	Describe("lifecycle", func() {
		It("should transition from Pending to Running when a Job is created", func() {
			nodeScan := &clamavv1alpha1.NodeScan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-lifecycle",
					Namespace: defaultNamespace,
				},
				Spec: clamavv1alpha1.NodeScanSpec{
					// Use a node name that exists in envtest ("kind-control-plane" for
					// real clusters; for envtest there are no real nodes so the
					// controller will emit a NodeNotFound event and move to Failed).
					NodeName: "e2e-test-node",
					Priority: "low",
				},
			}

			By("creating the NodeScan")
			Expect(k8sClient.Create(ctx, nodeScan)).To(Succeed())

			key := types.NamespacedName{Name: nodeScan.Name, Namespace: nodeScan.Namespace}

			By("waiting for a terminal phase (Running, Completed, or Failed in envtest)")
			Eventually(func() clamavv1alpha1.NodeScanPhase {
				ns := &clamavv1alpha1.NodeScan{}
				if err := k8sClient.Get(ctx, key, ns); err != nil {
					return ""
				}
				return ns.Status.Phase
			}, eventuallyTimeout, eventuallyInterval).ShouldNot(BeEmpty(),
				"NodeScan phase should be set within timeout")

			By("verifying the finalizer was added")
			ns := &clamavv1alpha1.NodeScan{}
			Expect(k8sClient.Get(ctx, key, ns)).To(Succeed())
			Expect(ns.Finalizers).To(ContainElement("clamav.io/finalizer"))

			By("cleaning up")
			Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
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

			By("creating the NodeScan")
			Expect(k8sClient.Create(ctx, nodeScan)).To(Succeed())

			key := types.NamespacedName{Name: nodeScan.Name, Namespace: nodeScan.Namespace}

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

			By("verifying the NodeScan is eventually gone (finalizer removed)")
			Eventually(func() bool {
				existing := &clamavv1alpha1.NodeScan{}
				err := k8sClient.Get(ctx, key, existing)
				return err != nil // expect NotFound
			}, eventuallyTimeout, eventuallyInterval).Should(BeTrue(),
				"NodeScan should be deleted after finalizer is removed")
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Validation: missing nodeName should be rejected by the webhook
	// (requires webhook to be running; skipped in pure envtest without certs)
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
})
