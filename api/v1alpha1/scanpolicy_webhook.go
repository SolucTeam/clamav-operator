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

	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var scanpolicylog = logf.Log.WithName("scanpolicy-resource")

// SetupWebhookWithManager sets up the webhook with the Manager.
func (r *ScanPolicy) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &ScanPolicy{}).
		WithValidator(r).
		Complete()
}

// +kubebuilder:webhook:path=/validate-clamav-io-v1alpha1-scanpolicy,mutating=false,failurePolicy=fail,sideEffects=None,groups=clamav.io,resources=scanpolicies,verbs=create;update,versions=v1alpha1,name=vscanpolicy.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*ScanPolicy] = &ScanPolicy{}

// ValidateCreate validates a new ScanPolicy at admission time.
func (r *ScanPolicy) ValidateCreate(ctx context.Context, obj *ScanPolicy) (admission.Warnings, error) {
	scanpolicylog.Info("validate create", "name", obj.Name)
	if errs := obj.validateScanPolicy(); len(errs) > 0 {
		return nil, errs.ToAggregate()
	}
	return nil, nil
}

// ValidateUpdate validates an updated ScanPolicy at admission time.
func (r *ScanPolicy) ValidateUpdate(ctx context.Context, oldObj, newObj *ScanPolicy) (admission.Warnings, error) {
	scanpolicylog.Info("validate update", "name", newObj.Name)
	if errs := newObj.validateScanPolicy(); len(errs) > 0 {
		return nil, errs.ToAggregate()
	}
	return nil, nil
}

// ValidateDelete is a no-op; no admission checks needed on deletion.
func (r *ScanPolicy) ValidateDelete(ctx context.Context, obj *ScanPolicy) (admission.Warnings, error) {
	scanpolicylog.Info("validate delete", "name", obj.Name)
	return nil, nil
}

// validateScanPolicy performs all validation logic for a ScanPolicy.
func (r *ScanPolicy) validateScanPolicy() field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	allErrs = append(allErrs, ValidatePaths(r.Spec.Paths, specPath.Child("paths"))...)

	if len(r.Spec.ExcludePatterns) > 0 {
		allErrs = append(allErrs, ValidateExcludePatterns(r.Spec.ExcludePatterns, specPath.Child("excludePatterns"))...)
	}

	allErrs = append(allErrs, ValidateNodeScanConcurrent(r.Spec.MaxConcurrent, specPath.Child("maxConcurrent"))...)
	allErrs = append(allErrs, ValidateFileTimeout(r.Spec.FileTimeout, specPath.Child("fileTimeout"))...)
	allErrs = append(allErrs, ValidateMaxFileSize(r.Spec.MaxFileSize, specPath.Child("maxFileSize"))...)

	if r.Spec.Resources != nil {
		allErrs = append(allErrs, validateResources(r.Spec.Resources, specPath.Child("resources"))...)
	}

	if r.Spec.Notifications != nil {
		notifPath := specPath.Child("notifications")
		if r.Spec.Notifications.Slack != nil && r.Spec.Notifications.Slack.Enabled {
			if r.Spec.Notifications.Slack.WebhookURL == "" && r.Spec.Notifications.Slack.WebhookSecretRef == nil {
				allErrs = append(allErrs, field.Required(
					notifPath.Child("slack").Child("webhookURL"),
					"webhookURL or webhookSecretRef is required when Slack is enabled"))
			}
		}
		if r.Spec.Notifications.Email != nil && r.Spec.Notifications.Email.Enabled {
			emailPath := notifPath.Child("email")
			if r.Spec.Notifications.Email.SMTPServer == "" {
				allErrs = append(allErrs, field.Required(emailPath.Child("smtpServer"), "smtpServer is required when Email is enabled"))
			}
			if r.Spec.Notifications.Email.From == "" {
				allErrs = append(allErrs, field.Required(emailPath.Child("from"), "from is required when Email is enabled"))
			}
			if len(r.Spec.Notifications.Email.Recipients) == 0 {
				allErrs = append(allErrs, field.Required(emailPath.Child("recipients"), "at least one recipient is required when Email is enabled"))
			}
		}
		if r.Spec.Notifications.Webhook != nil && r.Spec.Notifications.Webhook.Enabled {
			if r.Spec.Notifications.Webhook.URL == "" {
				allErrs = append(allErrs, field.Required(
					notifPath.Child("webhook").Child("url"),
					"url is required when Webhook is enabled"))
			}
		}
		if r.Spec.Notifications.Teams != nil && r.Spec.Notifications.Teams.Enabled {
			if r.Spec.Notifications.Teams.WebhookURL == "" && r.Spec.Notifications.Teams.WebhookSecretRef == nil {
				allErrs = append(allErrs, field.Required(
					notifPath.Child("teams").Child("webhookURL"),
					"webhookURL or webhookSecretRef is required when Teams is enabled"))
			}
		}
	}

	return allErrs
}
