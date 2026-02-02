/*
Copyright 2025 Platform Team - Numspot.

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
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
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

// +kubebuilder:webhook:path=/validate-clamav-platform-numspot-com-v1alpha1-nodescan,mutating=false,failurePolicy=fail,sideEffects=None,groups=clamav.platform.numspot.com,resources=nodescans,verbs=create;update,versions=v1alpha1,name=vnodescan.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &NodeScan{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *NodeScan) ValidateCreate() (admission.Warnings, error) {
	nodescanlog.Info("validate create", "name", r.Name)

	if r.Spec.NodeName == "" {
		return nil, fmt.Errorf("nodeName is required")
	}

	if r.Spec.MaxConcurrent < 1 || r.Spec.MaxConcurrent > 20 {
		return nil, fmt.Errorf("maxConcurrent must be between 1 and 20")
	}

	if r.Spec.FileTimeout != 0 && r.Spec.FileTimeout < 1000 {
		return nil, fmt.Errorf("fileTimeout must be at least 1000ms (1 second)")
	}

	if r.Spec.MaxFileSize != 0 && r.Spec.MaxFileSize < 0 {
		return nil, fmt.Errorf("maxFileSize must be positive")
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *NodeScan) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	nodescanlog.Info("validate update", "name", r.Name)

	oldNodeScan := old.(*NodeScan)

	// Prevent changing nodeName after creation
	if r.Spec.NodeName != oldNodeScan.Spec.NodeName {
		return nil, fmt.Errorf("nodeName cannot be changed after creation")
	}

	// Validate other fields like in create
	if r.Spec.MaxConcurrent < 1 || r.Spec.MaxConcurrent > 20 {
		return nil, fmt.Errorf("maxConcurrent must be between 1 and 20")
	}

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *NodeScan) ValidateDelete() (admission.Warnings, error) {
	nodescanlog.Info("validate delete", "name", r.Name)

	// No validation needed for delete
	return nil, nil
}
