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

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScanStrategy defines the strategy to use for scanning
// +kubebuilder:validation:Enum=full;incremental;modified-only;smart
type ScanStrategy string

const (
	ScanStrategyFull         ScanStrategy = "full"
	ScanStrategyIncremental  ScanStrategy = "incremental"
	ScanStrategyModifiedOnly ScanStrategy = "modified-only"
	ScanStrategySmart        ScanStrategy = "smart"
)

// IncrementalScanConfig configures incremental scan behavior
type IncrementalScanConfig struct {
	// FullScanInterval defines how often a full scan runs when strategy is "smart"
	// +kubebuilder:default=10
	// +optional
	FullScanInterval int32 `json:"fullScanInterval,omitempty"`

	// MaxFileAgeHours is the maximum age (in hours) of files to consider for incremental scans
	// +kubebuilder:default=24
	// +optional
	MaxFileAgeHours int32 `json:"maxFileAgeHours,omitempty"`

	// SkipUnchangedFiles skips files whose mtime+size haven't changed since last scan
	// +kubebuilder:default=true
	// +optional
	SkipUnchangedFiles bool `json:"skipUnchangedFiles,omitempty"`
}

// NodeScanSpec defines the desired state of NodeScan
type NodeScanSpec struct {
	// NodeName is the name of the node to scan
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	NodeName string `json:"nodeName"`

	// ScanPolicy references a ScanPolicy to use for this scan
	// +optional
	ScanPolicy string `json:"scanPolicy,omitempty"`

	// Priority of the scan (high, medium, low)
	// +kubebuilder:validation:Enum=high;medium;low
	// +kubebuilder:default=medium
	// +optional
	Priority string `json:"priority,omitempty"`

	// Paths to scan on the node
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

	// MaxFileSize in bytes — files larger than this will be skipped
	// +kubebuilder:default=104857600
	// +optional
	MaxFileSize int64 `json:"maxFileSize,omitempty"`

	// Resources for the scan job
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// TTLSecondsAfterFinished limits the lifetime of a finished Job
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
	NodeScanPhasePending   NodeScanPhase = "Pending"
	NodeScanPhaseRunning   NodeScanPhase = "Running"
	NodeScanPhaseCompleted NodeScanPhase = "Completed"
	NodeScanPhaseFailed    NodeScanPhase = "Failed"
)

// InfectedFile represents a file found to be infected with malware
type InfectedFile struct {
	Path       string      `json:"path"`
	Viruses    []string    `json:"viruses"`
	Size       int64       `json:"size,omitempty"`
	DetectedAt metav1.Time `json:"detectedAt,omitempty"`
}

// NodeScanStatus defines the observed state of NodeScan
type NodeScanStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +optional
	Phase NodeScanPhase `json:"phase,omitempty"`
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
	// +optional
	Duration int64 `json:"duration,omitempty"`
	// +optional
	FilesScanned int64 `json:"filesScanned,omitempty"`
	// +optional
	FilesInfected int64 `json:"filesInfected,omitempty"`
	// +optional
	FilesSkipped int64 `json:"filesSkipped,omitempty"`
	// +optional
	ErrorCount int64 `json:"errorCount,omitempty"`
	// Limited to first 100 for performance
	// +optional
	InfectedFiles []InfectedFile `json:"infectedFiles,omitempty"`
	// InfectedFilesTruncated is true when the scan detected more than 100 infected
	// files. Only the first 100 are stored in InfectedFiles; the full count is
	// still reflected in FilesInfected. Always check this flag before drawing
	// security conclusions from InfectedFiles.
	// +optional
	InfectedFilesTruncated bool `json:"infectedFilesTruncated,omitempty"`
	// ResultsPartial is true when scan result parsing failed after all retries
	// and the status reflects incomplete data. The scan itself succeeded (the
	// scanner Job exited 0) but the controller could not fully parse the output.
	// Do NOT treat this scan as a clean bill of health — re-run the scan.
	// +optional
	ResultsPartial bool `json:"resultsPartial,omitempty"`
	// +optional
	JobRef *corev1.ObjectReference `json:"jobRef,omitempty"`
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
	// +optional
	ReportPath string `json:"reportPath,omitempty"`
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
	// +optional
	StrategyUsed ScanStrategy `json:"strategyUsed,omitempty"`
	// +optional
	FilesSkippedIncremental int64 `json:"filesSkippedIncremental,omitempty"`
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	CacheHitRate int32 `json:"cacheHitRate,omitempty"`
	// +optional
	TimeSaved int64 `json:"timeSaved,omitempty"`
	// +optional
	FailureReason string `json:"failureReason,omitempty"`
	// +optional
	ExitCode int32 `json:"exitCode,omitempty"`
	// +optional
	ScannerMemoryRSSBytes int64 `json:"scannerMemoryRSSBytes,omitempty"`
	// +optional
	ScannerCPUUserSeconds float64 `json:"scannerCPUUserSeconds,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Namespaced,shortName=ns;nodescan
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Scanned",type=integer,JSONPath=`.status.filesScanned`
// +kubebuilder:printcolumn:name="Infected",type=integer,JSONPath=`.status.filesInfected`
// +kubebuilder:printcolumn:name="Duration",type=integer,JSONPath=`.status.duration`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// NodeScan is the Schema for the nodescans API (v1beta1 — stable, preferred version)
type NodeScan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeScanSpec   `json:"spec,omitempty"`
	Status NodeScanStatus `json:"status,omitempty"`
}

// Hub marks NodeScan v1beta1 as the conversion hub.
// All other versions (v1alpha1, …) convert to and from this version.
func (*NodeScan) Hub() {}

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
