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

package v1alpha1

import (
	"context"
	"fmt"

	"github.com/robfig/cron/v3"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var scanschedulelog = logf.Log.WithName("scanschedule-resource")

// SetupWebhookWithManager sets up the webhook with the Manager.
func (r *ScanSchedule) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &ScanSchedule{}).
		WithValidator(r).
		Complete()
}

// +kubebuilder:webhook:path=/validate-clamav-io-v1alpha1-scanschedule,mutating=false,failurePolicy=fail,sideEffects=None,groups=clamav.io,resources=scanschedules,verbs=create;update,versions=v1alpha1,name=vscanschedule.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*ScanSchedule] = &ScanSchedule{}

// ValidateCreate validates a new ScanSchedule at admission time.
func (r *ScanSchedule) ValidateCreate(ctx context.Context, obj *ScanSchedule) (admission.Warnings, error) {
	scanschedulelog.Info("validate create", "name", obj.Name)
	if errs := obj.validateScanSchedule(); len(errs) > 0 {
		return nil, errs.ToAggregate()
	}
	return nil, nil
}

// ValidateUpdate validates an updated ScanSchedule at admission time.
func (r *ScanSchedule) ValidateUpdate(ctx context.Context, oldObj, newObj *ScanSchedule) (admission.Warnings, error) {
	scanschedulelog.Info("validate update", "name", newObj.Name)
	if errs := newObj.validateScanSchedule(); len(errs) > 0 {
		return nil, errs.ToAggregate()
	}
	return nil, nil
}

// ValidateDelete is a no-op; no admission checks needed on deletion.
func (r *ScanSchedule) ValidateDelete(ctx context.Context, obj *ScanSchedule) (admission.Warnings, error) {
	scanschedulelog.Info("validate delete", "name", obj.Name)
	return nil, nil
}

// validateScanSchedule performs all validation logic for a ScanSchedule.
func (r *ScanSchedule) validateScanSchedule() field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	// Validate cron expression at admission time so that users get an
	// immediate, human-readable error rather than a cryptic reconciler error.
	if _, err := cron.ParseStandard(r.Spec.Schedule); err != nil {
		allErrs = append(allErrs, field.Invalid(
			specPath.Child("schedule"),
			r.Spec.Schedule,
			fmt.Sprintf("invalid cron expression: %v", err),
		))
	}

	// Validate history limits
	if r.Spec.SuccessfulScansHistoryLimit != nil && *r.Spec.SuccessfulScansHistoryLimit < 0 {
		allErrs = append(allErrs, field.Invalid(
			specPath.Child("successfulScansHistoryLimit"),
			*r.Spec.SuccessfulScansHistoryLimit,
			"must be non-negative",
		))
	}
	if r.Spec.FailedScansHistoryLimit != nil && *r.Spec.FailedScansHistoryLimit < 0 {
		allErrs = append(allErrs, field.Invalid(
			specPath.Child("failedScansHistoryLimit"),
			*r.Spec.FailedScansHistoryLimit,
			"must be non-negative",
		))
	}

	// Validate ClusterScan template fields
	clusterScanPath := specPath.Child("clusterScan")
	allErrs = append(allErrs, ValidateClusterScanConcurrent(r.Spec.ClusterScan.Concurrent, clusterScanPath.Child("concurrent"))...)
	allErrs = append(allErrs, ValidatePriority(r.Spec.ClusterScan.Priority, clusterScanPath.Child("priority"))...)

	return allErrs
}
