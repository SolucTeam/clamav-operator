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
	clamavv1beta1 "github.com/SolucTeam/clamav-operator/api/v1beta1"
)

// conversionTests exercises the v1alpha1 ↔ v1beta1 conversion logic.
// These tests create an object via one version and read it back via the other,
// verifying that all fields round-trip correctly.
var _ = Describe("API Conversion v1alpha1 ↔ v1beta1", func() {
	Describe("NodeScan", func() {
		It("should round-trip from v1alpha1 to v1beta1 without data loss", func() {
			alpha := &clamavv1alpha1.NodeScan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-conversion-nodescan",
					Namespace: defaultNamespace,
				},
				Spec: clamavv1alpha1.NodeScanSpec{
					NodeName:      "conversion-test-node",
					ScanPolicy:    "default-policy",
					Priority:      "high",
					Paths:         []string{"/var/lib", "/opt"},
					MaxConcurrent: 4,
					FileTimeout:   120000,
					MaxFileSize:   52428800,
					Strategy:      clamavv1alpha1.ScanStrategySmart,
				},
			}

			By("creating the NodeScan via v1alpha1")
			Expect(k8sClient.Create(ctx, alpha)).To(Succeed())
			key := types.NamespacedName{Name: alpha.Name, Namespace: alpha.Namespace}

			By("reading it back via v1beta1 and checking field parity")
			beta := &clamavv1beta1.NodeScan{}
			Expect(k8sClient.Get(ctx, key, beta)).To(Succeed())

			Expect(beta.Spec.NodeName).To(Equal(alpha.Spec.NodeName))
			Expect(beta.Spec.ScanPolicy).To(Equal(alpha.Spec.ScanPolicy))
			Expect(beta.Spec.Priority).To(Equal(alpha.Spec.Priority))
			Expect(beta.Spec.Paths).To(Equal(alpha.Spec.Paths))
			Expect(beta.Spec.MaxConcurrent).To(Equal(alpha.Spec.MaxConcurrent))
			Expect(beta.Spec.FileTimeout).To(Equal(alpha.Spec.FileTimeout))
			Expect(beta.Spec.MaxFileSize).To(Equal(alpha.Spec.MaxFileSize))
			Expect(string(beta.Spec.Strategy)).To(Equal(string(alpha.Spec.Strategy)))

			By("cleaning up")
			Expect(k8sClient.Delete(ctx, alpha)).To(Succeed())
		})

		It("should round-trip from v1beta1 to v1alpha1 without data loss", func() {
			beta := &clamavv1beta1.NodeScan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-conversion-nodescan-reverse",
					Namespace: defaultNamespace,
				},
				Spec: clamavv1beta1.NodeScanSpec{
					NodeName:      "conversion-test-node-b",
					ScanPolicy:    "custom-policy",
					Priority:      "low",
					Paths:         []string{"/usr/local"},
					MaxConcurrent: 2,
					Strategy:      clamavv1beta1.ScanStrategyIncremental,
				},
			}

			By("creating the NodeScan via v1beta1")
			Expect(k8sClient.Create(ctx, beta)).To(Succeed())
			key := types.NamespacedName{Name: beta.Name, Namespace: beta.Namespace}

			By("reading it back via v1alpha1 and checking field parity")
			alpha := &clamavv1alpha1.NodeScan{}
			Expect(k8sClient.Get(ctx, key, alpha)).To(Succeed())

			Expect(alpha.Spec.NodeName).To(Equal(beta.Spec.NodeName))
			Expect(alpha.Spec.ScanPolicy).To(Equal(beta.Spec.ScanPolicy))
			Expect(alpha.Spec.Priority).To(Equal(beta.Spec.Priority))
			Expect(alpha.Spec.Paths).To(Equal(beta.Spec.Paths))
			Expect(alpha.Spec.MaxConcurrent).To(Equal(beta.Spec.MaxConcurrent))
			Expect(string(alpha.Spec.Strategy)).To(Equal(string(beta.Spec.Strategy)))

			By("cleaning up")
			Expect(k8sClient.Delete(ctx, beta)).To(Succeed())
		})
	})
})
