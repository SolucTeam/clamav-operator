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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	client "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var scanschedulelog = logf.Log.WithName("scanschedule-resource")

type scanscheduleValidator struct {
	client.Client
}

// SetupWebhookWithManager sets up the webhook with the Manager.
func (r *ScanSchedule) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &ScanSchedule{}).
		WithValidator(&scanscheduleValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-clamav-io-v1alpha1-scanschedule,mutating=false,failurePolicy=fail,sideEffects=None,groups=clamav.io,resources=scanschedules,verbs=create;update,versions=v1alpha1,name=vscanschedule.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*ScanSchedule] = &scanscheduleValidator{}

// ValidateCreate validates a new ScanSchedule at admission time.
func (v *scanscheduleValidator) ValidateCreate(ctx context.Context, obj *ScanSchedule) (admission.Warnings, error) {
	scanschedulelog.Info("validate create", "name", obj.Name)
	if errs := obj.validateScanSchedule(ctx, v.Client); len(errs) > 0 {
		return nil, errs.ToAggregate()
	}
	return nil, nil
}

// ValidateUpdate validates an updated ScanSchedule at admission time.
func (v *scanscheduleValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *ScanSchedule) (admission.Warnings, error) {
	scanschedulelog.Info("validate update", "name", newObj.Name)
	if errs := newObj.validateScanSchedule(ctx, v.Client); len(errs) > 0 {
		return nil, errs.ToAggregate()
	}
	return nil, nil
}

// ValidateDelete is a no-op; no admission checks needed on deletion.
func (v *scanscheduleValidator) ValidateDelete(ctx context.Context, obj *ScanSchedule) (admission.Warnings, error) {
	scanschedulelog.Info("validate delete", "name", obj.Name)
	return nil, nil
}

// validateScanSchedule performs all validation logic for a ScanSchedule.
func (r *ScanSchedule) validateScanSchedule(ctx context.Context, c client.Client) field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	if _, err := cron.ParseStandard(r.Spec.Schedule); err != nil {
		allErrs = append(allErrs, field.Invalid(
			specPath.Child("schedule"),
			r.Spec.Schedule,
			fmt.Sprintf("invalid cron expression: %v", err),
		))
	}

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

	clusterScanPath := specPath.Child("clusterScan")
	allErrs = append(allErrs, ValidateClusterScanConcurrent(r.Spec.ClusterScan.Concurrent, clusterScanPath.Child("concurrent"))...)
	allErrs = append(allErrs, ValidatePriority(r.Spec.ClusterScan.Priority, clusterScanPath.Child("priority"))...)

	if r.Spec.ClusterScan.NodeScanTemplate != nil {
		templatePath := clusterScanPath.Child("nodeScanTemplate")

		if len(r.Spec.ClusterScan.NodeScanTemplate.Paths) > 0 {
			allErrs = append(allErrs, ValidatePaths(r.Spec.ClusterScan.NodeScanTemplate.Paths, templatePath.Child("paths"))...)
		}

		if len(r.Spec.ClusterScan.NodeScanTemplate.ExcludePatterns) > 0 {
			allErrs = append(allErrs, ValidateExcludePatterns(
				r.Spec.ClusterScan.NodeScanTemplate.ExcludePatterns,
				templatePath.Child("excludePatterns"))...)
		}

		allErrs = append(allErrs, ValidateNodeScanConcurrent(
			r.Spec.ClusterScan.NodeScanTemplate.MaxConcurrent,
			templatePath.Child("maxConcurrent"))...)

		if r.Spec.ClusterScan.NodeScanTemplate.Resources != nil {
			allErrs = append(allErrs, validateResources(
				r.Spec.ClusterScan.NodeScanTemplate.Resources,
				templatePath.Child("resources"))...)
		}
	}

	if r.Spec.ClusterScan.NodeSelector != nil {
		if len(r.Spec.ClusterScan.NodeSelector.MatchLabels) == 0 && len(r.Spec.ClusterScan.NodeSelector.MatchExpressions) == 0 {
			allErrs = append(allErrs, field.Invalid(
				clusterScanPath.Child("nodeSelector"),
				r.Spec.ClusterScan.NodeSelector,
				"nodeSelector must have at least one matchLabel or matchExpression"))
		}
	}

	if r.Spec.ClusterScan.ScanPolicy != "" && c != nil {
		scanPolicy := &ScanPolicy{}
		err := c.Get(ctx, types.NamespacedName{Name: r.Spec.ClusterScan.ScanPolicy, Namespace: r.Namespace}, scanPolicy)
		if err != nil {
			if apierrors.IsNotFound(err) {
				allErrs = append(allErrs, field.NotFound(
					clusterScanPath.Child("scanPolicy"),
					r.Spec.ClusterScan.ScanPolicy))
			} else {
				allErrs = append(allErrs, field.InternalError(
					clusterScanPath.Child("scanPolicy"),
					err))
			}
		}
	}

	return allErrs
}
