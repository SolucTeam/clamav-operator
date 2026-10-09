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

// ClusterScanSpec defines the desired state of ClusterScan
type ClusterScanSpec struct {
	// NodeSelector selects which nodes to scan
	// +optional
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
	// ScanPolicy references a ScanPolicy
	// +optional
	ScanPolicy string `json:"scanPolicy,omitempty"`
	// Concurrent is the maximum number of nodes to scan in parallel
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=50
	// +kubebuilder:default=3
	// +optional
	Concurrent int32 `json:"concurrent,omitempty"`
	// Priority of all scans in this cluster scan
	// +kubebuilder:validation:Enum=high;medium;low
	// +kubebuilder:default=medium
	// +optional
	Priority string `json:"priority,omitempty"`
	// NodeScanTemplate contains the template for creating NodeScans
	// +optional
	NodeScanTemplate *NodeScanTemplateSpec `json:"nodeScanTemplate,omitempty"`
}

// NodeScanTemplateSpec is the template applied to every NodeScan created from
// a ClusterScan.
//
// It deliberately does not reuse NodeScanSpec: NodeScan requires nodeName (a
// NodeScan targets exactly one node), but inside a template the node name is
// meaningless — the ClusterScan controller always sets the real node name when
// it fans out. Inheriting the required marker made the API server reject any
// ClusterScan/ScanSchedule carrying a nodeScanTemplate without nodeName,
// which broke chart-rendered default ScanSchedules.
type NodeScanTemplateSpec struct {
	// NodeName is the name of the node to scan. Ignored: overwritten by the
	// ClusterScan controller with the name of each scanned node.
	// +optional
	NodeName string `json:"nodeName,omitempty"`

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

	// Strategy defines the scan strategy to use.
	// IMPORTANT: no CRD default here. When unset, the operator falls back to the
	// Helm-level SCANNER_SCAN_STRATEGY env var. A `default=full` marker would make
	// the API server persist "full" on every NodeScan created without an explicit
	// strategy, silently disabling incremental scanning cluster-wide.
	// +kubebuilder:validation:Enum=full;incremental;modified-only;smart
	// +optional
	Strategy ScanStrategy `json:"strategy,omitempty"`

	// IncrementalConfig configures incremental scan behavior
	// +optional
	IncrementalConfig *IncrementalScanConfig `json:"incrementalConfig,omitempty"`

	// ForceFullScan forces a full scan even if incremental is enabled
	// +optional
	ForceFullScan bool `json:"forceFullScan,omitempty"`
}

// ClusterScanPhase represents the current phase of a ClusterScan
// +kubebuilder:validation:Enum=Pending;Running;Completed;Failed;PartiallyCompleted
type ClusterScanPhase string

const (
	ClusterScanPhasePending           ClusterScanPhase = "Pending"
	ClusterScanPhaseRunning           ClusterScanPhase = "Running"
	ClusterScanPhaseCompleted         ClusterScanPhase = "Completed"
	ClusterScanPhaseFailed            ClusterScanPhase = "Failed"
	ClusterScanPhasePartiallyComplete ClusterScanPhase = "PartiallyCompleted"
)

// NodeScanReference references a NodeScan and its status
type NodeScanReference struct {
	Name           string        `json:"name"`
	NodeName       string        `json:"nodeName"`
	Phase          NodeScanPhase `json:"phase"`
	FilesInfected  int64         `json:"filesInfected,omitempty"`
	FilesScanned   int64         `json:"filesScanned,omitempty"`
	StartTime      *metav1.Time  `json:"startTime,omitempty"`
	CompletionTime *metav1.Time  `json:"completionTime,omitempty"`
}

// ClusterScanStatus defines the observed state of ClusterScan
type ClusterScanStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +optional
	Phase ClusterScanPhase `json:"phase,omitempty"`
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
	// +optional
	TotalNodes int32 `json:"totalNodes,omitempty"`
	// +optional
	CompletedNodes int32 `json:"completedNodes,omitempty"`
	// +optional
	RunningNodes int32 `json:"runningNodes,omitempty"`
	// +optional
	FailedNodes int32 `json:"failedNodes,omitempty"`
	// +optional
	InfectedNodes int32 `json:"infectedNodes,omitempty"`
	// +optional
	TotalFilesScanned int64 `json:"totalFilesScanned,omitempty"`
	// +optional
	TotalFilesInfected int64 `json:"totalFilesInfected,omitempty"`
	// +optional
	NodeScans []NodeScanReference `json:"nodeScans,omitempty"`
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Namespaced,shortName=cs;clusterscan
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Total",type=integer,JSONPath=`.status.totalNodes`
// +kubebuilder:printcolumn:name="Completed",type=integer,JSONPath=`.status.completedNodes`
// +kubebuilder:printcolumn:name="Failed",type=integer,JSONPath=`.status.failedNodes`
// +kubebuilder:printcolumn:name="Infected",type=integer,JSONPath=`.status.totalFilesInfected`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ClusterScan is the Schema for the clusterscans API (v1beta1 — stable, preferred version)
type ClusterScan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterScanSpec   `json:"spec,omitempty"`
	Status ClusterScanStatus `json:"status,omitempty"`
}

// Hub marks ClusterScan v1beta1 as the conversion hub.
func (*ClusterScan) Hub() {}

// +kubebuilder:object:root=true

// ClusterScanList contains a list of ClusterScan
type ClusterScanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterScan `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterScan{}, &ClusterScanList{})
}
