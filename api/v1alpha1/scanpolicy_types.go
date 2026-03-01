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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScanPolicySpec defines the desired state of ScanPolicy
type ScanPolicySpec struct {
	// Paths to scan on each node
	// +kubebuilder:validation:MinItems=1
	Paths []string `json:"paths"`

	// ExcludePatterns are regex patterns for paths to exclude from scanning
	// +optional
	ExcludePatterns []string `json:"excludePatterns,omitempty"`

	// MaxConcurrent files to scan in parallel
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=20
	// +kubebuilder:default=5
	// +optional
	MaxConcurrent int32 `json:"maxConcurrent,omitempty"`

	// FileTimeout in milliseconds for scanning each file
	// +kubebuilder:default=300000
	// +optional
	FileTimeout int64 `json:"fileTimeout,omitempty"`

	// MaxFileSize in bytes - files larger than this will be skipped
	// +kubebuilder:default=104857600
	// +optional
	MaxFileSize int64 `json:"maxFileSize,omitempty"`

	// ConnectTimeout in milliseconds for connecting to ClamAV
	// +kubebuilder:default=60000
	// +optional
	ConnectTimeout int64 `json:"connectTimeout,omitempty"`

	// Resources for scan jobs
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Notifications configuration
	// +optional
	Notifications *NotificationConfig `json:"notifications,omitempty"`

	// Quarantine configuration
	// +optional
	Quarantine *QuarantineConfig `json:"quarantine,omitempty"`
}

// NotificationConfig defines notification settings
type NotificationConfig struct {
	// Slack notification settings
	// +optional
	Slack *SlackConfig `json:"slack,omitempty"`

	// Email notification settings
	// +optional
	Email *EmailConfig `json:"email,omitempty"`

	// Webhook notification settings
	// +optional
	Webhook *WebhookConfig `json:"webhook,omitempty"`
}

// SlackConfig defines Slack notification settings
type SlackConfig struct {
	// Enabled indicates if Slack notifications are enabled
	Enabled bool `json:"enabled"`

	// WebhookURL is the Slack webhook URL
	// Should be stored in a Secret and referenced
	// +optional
	WebhookURL string `json:"webhookURL,omitempty"`

	// WebhookSecretRef references a Secret containing the webhook URL
	// +optional
	WebhookSecretRef *corev1.SecretKeySelector `json:"webhookSecretRef,omitempty"`

	// Channel to send notifications to
	// +optional
	Channel string `json:"channel,omitempty"`

	// OnlyOnInfection sends notifications only when malware is detected
	// +kubebuilder:default=true
	// +optional
	OnlyOnInfection bool `json:"onlyOnInfection,omitempty"`
}

// EmailConfig defines email notification settings
type EmailConfig struct {
	// Enabled indicates if email notifications are enabled
	Enabled bool `json:"enabled"`

	// SMTPServer is the SMTP server address (host:port)
	SMTPServer string `json:"smtpServer"`

	// SMTPAuthSecretRef references a Secret containing SMTP credentials
	// Expected keys: username, password
	// +optional
	SMTPAuthSecretRef *corev1.SecretReference `json:"smtpAuthSecretRef,omitempty"`

	// From is the sender email address
	From string `json:"from"`

	// Recipients is the list of recipient email addresses
	// +kubebuilder:validation:MinItems=1
	Recipients []string `json:"recipients"`

	// OnlyOnInfection sends emails only when malware is detected
	// +kubebuilder:default=true
	// +optional
	OnlyOnInfection bool `json:"onlyOnInfection,omitempty"`
}

// WebhookConfig defines webhook notification settings
type WebhookConfig struct {
	// URL to send webhook notifications to
	URL string `json:"url"`

	// Headers to include in webhook requests
	// +optional
	Headers map[string]string `json:"headers,omitempty"`

	// SecretRef references a Secret containing auth headers
	// +optional
	SecretRef *corev1.SecretReference `json:"secretRef,omitempty"`

	// OnlyOnInfection sends webhooks only when malware is detected
	// +kubebuilder:default=true
	// +optional
	OnlyOnInfection bool `json:"onlyOnInfection,omitempty"`
}

// QuarantineConfig defines quarantine settings for infected files
type QuarantineConfig struct {
	// Enabled indicates if quarantine is enabled
	Enabled bool `json:"enabled"`

	// Action to take on infected files
	// +kubebuilder:validation:Enum=move;delete;alert-only
	// +kubebuilder:default=alert-only
	Action string `json:"action"`

	// QuarantineDir is the directory to move infected files to
	// Only used when Action is "move"
	// +optional
	QuarantineDir string `json:"quarantineDir,omitempty"`

	// NotifyAdmin sends notification when files are quarantined
	// +kubebuilder:default=true
	// +optional
	NotifyAdmin bool `json:"notifyAdmin,omitempty"`
}

// ScanPolicyStatus defines the observed state of ScanPolicy
type ScanPolicyStatus struct {
	// LastUsed is the last time this policy was used for a scan
	// +optional
	LastUsed *metav1.Time `json:"lastUsed,omitempty"`

	// UsageCount is how many times this policy has been used
	// +optional
	UsageCount int64 `json:"usageCount,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=sp;scanpolicy
// +kubebuilder:printcolumn:name="Paths",type=string,JSONPath=`.spec.paths`
// +kubebuilder:printcolumn:name="MaxConcurrent",type=integer,JSONPath=`.spec.maxConcurrent`
// +kubebuilder:printcolumn:name="UsageCount",type=integer,JSONPath=`.status.usageCount`
// +kubebuilder:printcolumn:name="LastUsed",type=date,JSONPath=`.status.lastUsed`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ScanPolicy is the Schema for the scanpolicies API
type ScanPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScanPolicySpec   `json:"spec,omitempty"`
	Status ScanPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ScanPolicyList contains a list of ScanPolicy
type ScanPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScanPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ScanPolicy{}, &ScanPolicyList{})
}
