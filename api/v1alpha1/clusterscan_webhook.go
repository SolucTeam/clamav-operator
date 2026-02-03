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
var clusterscanlog = logf.Log.WithName("clusterscan-resource")

// SetupWebhookWithManager sets up the webhook with the Manager
func (r *ClusterScan) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/validate-clamav-io-v1alpha1-clusterscan,mutating=false,failurePolicy=fail,sideEffects=None,groups=clamav.io,resources=clusterscans,verbs=create;update,versions=v1alpha1,name=vclusterscan.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &ClusterScan{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *ClusterScan) ValidateCreate() (admission.Warnings, error) {
	clusterscanlog.Info("validate create", "name", r.Name)

	allErrs := r.validateClusterScan()

	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *ClusterScan) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	clusterscanlog.Info("validate update", "name", r.Name)

	allErrs := r.validateClusterScan()

	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
	}

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *ClusterScan) ValidateDelete() (admission.Warnings, error) {
	clusterscanlog.Info("validate delete", "name", r.Name)

	// No validation needed for delete
	return nil, nil
}

// validateClusterScan performs comprehensive validation of ClusterScan spec
func (r *ClusterScan) validateClusterScan() field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	// Validate concurrent (1-50)
	allErrs = append(allErrs, ValidateClusterScanConcurrent(r.Spec.Concurrent, specPath.Child("concurrent"))...)

	// Validate priority
	allErrs = append(allErrs, ValidatePriority(r.Spec.Priority, specPath.Child("priority"))...)

	// Validate NodeScanTemplate if provided
	if r.Spec.NodeScanTemplate != nil {
		templatePath := specPath.Child("nodeScanTemplate")

		// Validate paths in template
		if len(r.Spec.NodeScanTemplate.Paths) > 0 {
			allErrs = append(allErrs, ValidatePaths(r.Spec.NodeScanTemplate.Paths, templatePath.Child("paths"))...)
		}

		// Validate maxConcurrent in template
		allErrs = append(allErrs, ValidateNodeScanConcurrent(
			r.Spec.NodeScanTemplate.MaxConcurrent,
			templatePath.Child("maxConcurrent"))...)

		// Validate resources in template
		if r.Spec.NodeScanTemplate.Resources != nil {
			allErrs = append(allErrs, validateResources(
				r.Spec.NodeScanTemplate.Resources,
				templatePath.Child("resources"))...)
		}
	}

	// Validate nodeSelector if provided
	if r.Spec.NodeSelector != nil {
		if len(r.Spec.NodeSelector.MatchLabels) == 0 && len(r.Spec.NodeSelector.MatchExpressions) == 0 {
			allErrs = append(allErrs, field.Invalid(
				specPath.Child("nodeSelector"),
				r.Spec.NodeSelector,
				"nodeSelector must have at least one matchLabel or matchExpression"))
		}
	}

	return allErrs
}
