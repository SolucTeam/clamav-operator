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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScanScheduleSpec defines the desired state of ScanSchedule
type ScanScheduleSpec struct {
	// Schedule in Cron format
	// See https://en.wikipedia.org/wiki/Cron
	// +kubebuilder:validation:Required
	Schedule string `json:"schedule"`

	// ClusterScan template for scheduled scans
	// +kubebuilder:validation:Required
	ClusterScan ClusterScanSpec `json:"clusterScan"`

	// Suspend tells the controller to suspend subsequent executions
	// Defaults to false
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// SuccessfulScansHistoryLimit is the number of successful scans to retain
	// Defaults to 10
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=10
	// +optional
	SuccessfulScansHistoryLimit *int32 `json:"successfulScansHistoryLimit,omitempty"`

	// FailedScansHistoryLimit is the number of failed scans to retain
	// Defaults to 3
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=3
	// +optional
	FailedScansHistoryLimit *int32 `json:"failedScansHistoryLimit,omitempty"`

	// ConcurrencyPolicy specifies how to treat concurrent executions
	// +kubebuilder:validation:Enum=Allow;Forbid;Replace
	// +kubebuilder:default=Forbid
	// +optional
	ConcurrencyPolicy string `json:"concurrencyPolicy,omitempty"`

	// StartingDeadlineSeconds is optional deadline in seconds for starting
	// the scan if it misses scheduled time for any reason
	// +optional
	StartingDeadlineSeconds *int64 `json:"startingDeadlineSeconds,omitempty"`
}

// ScanScheduleStatus defines the observed state of ScanSchedule
type ScanScheduleStatus struct {
	// Active is a list of currently running scans
	// +optional
	Active []corev1.ObjectReference `json:"active,omitempty"`

	// LastScheduleTime is the last time a scan was scheduled
	// +optional
	LastScheduleTime *metav1.Time `json:"lastScheduleTime,omitempty"`

	// LastSuccessfulTime is the last time a scan completed successfully
	// +optional
	LastSuccessfulTime *metav1.Time `json:"lastSuccessfulTime,omitempty"`

	// NextScheduleTime is the next time a scan is scheduled to run
	// +optional
	NextScheduleTime *metav1.Time `json:"nextScheduleTime,omitempty"`

	// LastClusterScan is the name of the last created ClusterScan
	// +optional
	LastClusterScan string `json:"lastClusterScan,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=ss;scanschedule
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.spec.schedule`
// +kubebuilder:printcolumn:name="Suspend",type=boolean,JSONPath=`.spec.suspend`
// +kubebuilder:printcolumn:name="Active",type=integer,JSONPath=`.status.active`
// +kubebuilder:printcolumn:name="LastSchedule",type=date,JSONPath=`.status.lastScheduleTime`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ScanSchedule is the Schema for the scanschedules API
type ScanSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScanScheduleSpec   `json:"spec,omitempty"`
	Status ScanScheduleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ScanScheduleList contains a list of ScanSchedule
type ScanScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScanSchedule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ScanSchedule{}, &ScanScheduleList{})
}
