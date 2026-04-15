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

var _ = Describe("ScanSchedule", func() {
	// ─────────────────────────────────────────────────────────────────────────
	// First-run behaviour: should NOT trigger immediately on creation
	// ─────────────────────────────────────────────────────────────────────────
	Describe("first-run behaviour", func() {
		It("should NOT create a ClusterScan immediately after creation (waits for cron time)", func() {
			schedule := &clamavv1alpha1.ScanSchedule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-schedule-no-immediate",
					Namespace: defaultNamespace,
				},
				Spec: clamavv1alpha1.ScanScheduleSpec{
					// Far-future cron so it never fires during the test window
					Schedule: "0 0 1 1 *", // January 1st midnight
					ClusterScan: clamavv1alpha1.ClusterScanSpec{
						Priority:   "low",
						Concurrent: 1,
					},
					ConcurrencyPolicy: "Forbid",
				},
			}

			By("creating the ScanSchedule")
			Expect(k8sClient.Create(ctx, schedule)).To(Succeed())
			key := types.NamespacedName{Name: schedule.Name, Namespace: schedule.Namespace}

			By("waiting for NextScheduleTime to be populated")
			Eventually(func() bool {
				ss := &clamavv1alpha1.ScanSchedule{}
				if err := k8sClient.Get(ctx, key, ss); err != nil {
					return false
				}
				return ss.Status.NextScheduleTime != nil
			}, eventuallyTimeout, eventuallyInterval).Should(BeTrue(),
				"controller should set NextScheduleTime")

			By("verifying no ClusterScan was created immediately")
			ss := &clamavv1alpha1.ScanSchedule{}
			Expect(k8sClient.Get(ctx, key, ss)).To(Succeed())
			Expect(ss.Status.LastScheduleTime).To(BeNil(),
				"no scan should have run yet — first run waits for cron time")
			Expect(ss.Status.Active).To(BeEmpty(),
				"Active list should be empty on first reconcile")

			By("cleaning up")
			Expect(k8sClient.Delete(ctx, ss)).To(Succeed())
		})
	})

	// ─────────────────────────────────────────────────────────────────────────
	// Suspend: suspended schedules must not create new ClusterScans
	// ─────────────────────────────────────────────────────────────────────────
	Describe("suspension", func() {
		It("should not create a ClusterScan when suspended", func() {
			schedule := &clamavv1alpha1.ScanSchedule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-schedule-suspended",
					Namespace: defaultNamespace,
				},
				Spec: clamavv1alpha1.ScanScheduleSpec{
					Schedule: "* * * * *", // every minute — would fire quickly if not suspended
					Suspend:  true,
					ClusterScan: clamavv1alpha1.ClusterScanSpec{
						Priority:   "low",
						Concurrent: 1,
					},
				},
			}

			By("creating the suspended ScanSchedule")
			Expect(k8sClient.Create(ctx, schedule)).To(Succeed())
			key := types.NamespacedName{Name: schedule.Name, Namespace: schedule.Namespace}

			// Wait a brief moment and assert nothing was created
			By("waiting for controller to process the schedule")
			Eventually(func() bool {
				ss := &clamavv1alpha1.ScanSchedule{}
				return k8sClient.Get(ctx, key, ss) == nil
			}, eventuallyTimeout, eventuallyInterval).Should(BeTrue())

			Consistently(func() int {
				ss := &clamavv1alpha1.ScanSchedule{}
				if err := k8sClient.Get(ctx, key, ss); err != nil {
					return -1
				}
				return len(ss.Status.Active)
			}, 5*eventuallyInterval, eventuallyInterval).Should(Equal(0),
				"suspended schedule must not create ClusterScans")

			By("cleaning up")
			ss := &clamavv1alpha1.ScanSchedule{}
			Expect(k8sClient.Get(ctx, key, ss)).To(Succeed())
			Expect(k8sClient.Delete(ctx, ss)).To(Succeed())
		})
	})
})
