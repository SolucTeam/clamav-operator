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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NodeScanSpec defines the desired state of NodeScan
type NodeScanSpec struct {
	// NodeName is the name of the node to scan
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	NodeName string `json:"nodeName"`

	// ScanPolicy references a ScanPolicy to use for this scan
	// If not specified, default scan parameters will be used
	// +optional
	ScanPolicy string `json:"scanPolicy,omitempty"`

	// Priority of the scan (high, medium, low)
	// Affects scheduling and resource allocation
	// +kubebuilder:validation:Enum=high;medium;low
	// +kubebuilder:default=medium
	// +optional
	Priority string `json:"priority,omitempty"`

	// Paths to scan on the node
	// If not specified, uses paths from ScanPolicy or defaults
	// +optional
	Paths []string `json:"paths,omitempty"`

	// ExcludePatterns are regex patterns for paths to exclude
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

	// Resources for the scan job
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// TTLSecondsAfterFinished limits the lifetime of a Job that has finished
	// execution (either Complete or Failed). If this field is set,
	// ttlSecondsAfterFinished after the Job finishes, it is eligible to be
	// automatically deleted.
	// +kubebuilder:default=86400
	// +optional
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`

	// Strategy defines the scan strategy to use
	// +kubebuilder:validation:Enum=full;incremental;modified-only;smart
	// +kubebuilder:default=full
	// +optional
	Strategy ScanStrategy `json:"strategy,omitempty"`

	// IncrementalConfig configures incremental scan behavior
	// +optional
	IncrementalConfig *IncrementalScanConfig `json:"incrementalConfig,omitempty"`

	// ForceFullScan forces a full scan even if incremental is enabled
	// +optional
	ForceFullScan bool `json:"forceFullScan,omitempty"`
}

// NodeScanPhase represents the current phase of a NodeScan
// +kubebuilder:validation:Enum=Pending;Running;Completed;Failed
type NodeScanPhase string

const (
	// NodeScanPhasePending means the scan has been created but not yet started
	NodeScanPhasePending NodeScanPhase = "Pending"
	// NodeScanPhaseRunning means the scan is currently executing
	NodeScanPhaseRunning NodeScanPhase = "Running"
	// NodeScanPhaseCompleted means the scan has finished successfully
	NodeScanPhaseCompleted NodeScanPhase = "Completed"
	// NodeScanPhaseFailed means the scan has failed
	NodeScanPhaseFailed NodeScanPhase = "Failed"
)

// InfectedFile represents a file found to be infected with malware
type InfectedFile struct {
	// Path to the infected file on the node
	Path string `json:"path"`

	// Viruses detected in the file
	Viruses []string `json:"viruses"`

	// Size of the infected file in bytes
	// +optional
	Size int64 `json:"size,omitempty"`

	// DetectedAt is when the infection was detected
	// +optional
	DetectedAt metav1.Time `json:"detectedAt,omitempty"`
}

// NodeScanStatus defines the observed state of NodeScan
type NodeScanStatus struct {
	// Phase of the scan
	// +optional
	Phase NodeScanPhase `json:"phase,omitempty"`

	// StartTime of the scan
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime of the scan
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Duration of the scan in seconds
	// +optional
	Duration int64 `json:"duration,omitempty"`

	// FilesScanned is the total number of files scanned
	// +optional
	FilesScanned int64 `json:"filesScanned,omitempty"`

	// FilesInfected is the number of infected files found
	// +optional
	FilesInfected int64 `json:"filesInfected,omitempty"`

	// FilesSkipped is the number of files skipped
	// +optional
	FilesSkipped int64 `json:"filesSkipped,omitempty"`

	// ErrorCount is the number of errors encountered during scan
	// +optional
	ErrorCount int64 `json:"errorCount,omitempty"`

	// InfectedFiles contains details of infected files
	// Limited to first 100 for performance
	// +optional
	InfectedFiles []InfectedFile `json:"infectedFiles,omitempty"`

	// JobRef is a reference to the created Job
	// +optional
	JobRef *corev1.ObjectReference `json:"jobRef,omitempty"`

	// Conditions represent the latest available observations of the NodeScan's state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// ReportPath is the path to the detailed scan report on the node
	// +optional
	ReportPath string `json:"reportPath,omitempty"`

	// LastTransitionTime is the last time the phase transitioned
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`

	// StrategyUsed is the actual strategy that was used for this scan
	// +optional
	StrategyUsed ScanStrategy `json:"strategyUsed,omitempty"`

	// FilesSkippedIncremental is the number of files skipped due to incremental scan
	// +optional
	FilesSkippedIncremental int64 `json:"filesSkippedIncremental,omitempty"`

	// CacheHitRate is the percentage of files that were skipped (0-100)
	// +optional
	CacheHitRate float64 `json:"cacheHitRate,omitempty"`

	// TimeSaved is the estimated time saved by incremental scanning (in seconds)
	// +optional
	TimeSaved int64 `json:"timeSaved,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=ns;nodescan
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Scanned",type=integer,JSONPath=`.status.filesScanned`
// +kubebuilder:printcolumn:name="Infected",type=integer,JSONPath=`.status.filesInfected`
// +kubebuilder:printcolumn:name="Duration",type=integer,JSONPath=`.status.duration`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// NodeScan is the Schema for the nodescans API
type NodeScan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeScanSpec   `json:"spec,omitempty"`
	Status NodeScanStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodeScanList contains a list of NodeScan
type NodeScanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeScan `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeScan{}, &NodeScanList{})
}
