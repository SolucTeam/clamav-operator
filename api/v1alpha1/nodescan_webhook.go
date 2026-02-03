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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var nodescanlog = logf.Log.WithName("nodescan-resource")

// SetupWebhookWithManager sets up the webhook with the Manager
func (r *NodeScan) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/validate-clamav-io-v1alpha1-nodescan,mutating=false,failurePolicy=fail,sideEffects=None,groups=clamav.io,resources=nodescans,verbs=create;update,versions=v1alpha1,name=vnodescan.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &NodeScan{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *NodeScan) ValidateCreate() (admission.Warnings, error) {
	nodescanlog.Info("validate create", "name", r.Name)

	allErrs := r.validateNodeScan()

	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *NodeScan) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	nodescanlog.Info("validate update", "name", r.Name)

	oldNodeScan := old.(*NodeScan)

	var allErrs field.ErrorList

	// Prevent changing nodeName after creation
	if r.Spec.NodeName != oldNodeScan.Spec.NodeName {
		allErrs = append(allErrs, field.Forbidden(
			field.NewPath("spec").Child("nodeName"),
			"nodeName cannot be changed after creation"))
	}

	// Validate other fields
	allErrs = append(allErrs, r.validateNodeScan()...)

	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
	}

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *NodeScan) ValidateDelete() (admission.Warnings, error) {
	nodescanlog.Info("validate delete", "name", r.Name)

	// No validation needed for delete
	return nil, nil
}

// validateNodeScan performs comprehensive validation of NodeScan spec
func (r *NodeScan) validateNodeScan() field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	// Validate nodeName
	allErrs = append(allErrs, ValidateNodeName(r.Spec.NodeName, specPath.Child("nodeName"))...)

	// Validate priority
	allErrs = append(allErrs, ValidatePriority(r.Spec.Priority, specPath.Child("priority"))...)

	// Validate paths if specified
	if len(r.Spec.Paths) > 0 {
		allErrs = append(allErrs, ValidatePaths(r.Spec.Paths, specPath.Child("paths"))...)
	}

	// Validate exclude patterns if specified
	if len(r.Spec.ExcludePatterns) > 0 {
		allErrs = append(allErrs, ValidateExcludePatterns(r.Spec.ExcludePatterns, specPath.Child("excludePatterns"))...)
	}

	// Validate maxConcurrent
	allErrs = append(allErrs, ValidateNodeScanConcurrent(r.Spec.MaxConcurrent, specPath.Child("maxConcurrent"))...)

	// Validate fileTimeout
	allErrs = append(allErrs, ValidateFileTimeout(r.Spec.FileTimeout, specPath.Child("fileTimeout"))...)

	// Validate maxFileSize
	allErrs = append(allErrs, ValidateMaxFileSize(r.Spec.MaxFileSize, specPath.Child("maxFileSize"))...)

	// Validate resources if specified
	if r.Spec.Resources != nil {
		allErrs = append(allErrs, validateResources(r.Spec.Resources, specPath.Child("resources"))...)
	}

	// Validate TTL
	if r.Spec.TTLSecondsAfterFinished != nil && *r.Spec.TTLSecondsAfterFinished < 0 {
		allErrs = append(allErrs, field.Invalid(
			specPath.Child("ttlSecondsAfterFinished"),
			*r.Spec.TTLSecondsAfterFinished,
			"must be non-negative"))
	}

	return allErrs
}
