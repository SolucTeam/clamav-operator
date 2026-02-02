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
var clusterscanlog = logf.Log.WithName("clusterscan-resource")

// SetupWebhookWithManager sets up the webhook with the Manager
func (r *ClusterScan) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/validate-clamav-platform-numspot-com-v1alpha1-clusterscan,mutating=false,failurePolicy=fail,sideEffects=None,groups=clamav.platform.numspot.com,resources=clusterscans,verbs=create;update,versions=v1alpha1,name=vclusterscan.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &ClusterScan{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *ClusterScan) ValidateCreate() (admission.Warnings, error) {
	clusterscanlog.Info("validate create", "name", r.Name)

	if r.Spec.Concurrent < 1 || r.Spec.Concurrent > 50 {
		return nil, fmt.Errorf("concurrent must be between 1 and 50")
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *ClusterScan) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	clusterscanlog.Info("validate update", "name", r.Name)

	if r.Spec.Concurrent < 1 || r.Spec.Concurrent > 50 {
		return nil, fmt.Errorf("concurrent must be between 1 and 50")
	}

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *ClusterScan) ValidateDelete() (admission.Warnings, error) {
	clusterscanlog.Info("validate delete", "name", r.Name)

	// No validation needed for delete
	return nil, nil
}
