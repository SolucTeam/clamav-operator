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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterScanSpec defines the desired state of ClusterScan
type ClusterScanSpec struct {
	// NodeSelector selects which nodes to scan
	// If not specified, all nodes will be scanned
	// +optional
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`

	// ScanPolicy references a ScanPolicy to use for all node scans
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
	NodeScanTemplate *NodeScanSpec `json:"nodeScanTemplate,omitempty"`
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
	// Name of the NodeScan
	Name string `json:"name"`

	// NodeName is the node being scanned
	NodeName string `json:"nodeName"`

	// Phase of the NodeScan
	Phase NodeScanPhase `json:"phase"`

	// FilesInfected on this node
	// +optional
	FilesInfected int64 `json:"filesInfected,omitempty"`

	// FilesScanned on this node
	// +optional
	FilesScanned int64 `json:"filesScanned,omitempty"`

	// StartTime of the node scan
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime of the node scan
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

// ClusterScanStatus defines the observed state of ClusterScan
type ClusterScanStatus struct {
	// Phase of the cluster scan
	// +optional
	Phase ClusterScanPhase `json:"phase,omitempty"`

	// StartTime of the cluster scan
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime of the cluster scan
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// TotalNodes is the total number of nodes to scan
	// +optional
	TotalNodes int32 `json:"totalNodes,omitempty"`

	// CompletedNodes is the number of nodes that have completed scanning
	// +optional
	CompletedNodes int32 `json:"completedNodes,omitempty"`

	// RunningNodes is the number of nodes currently being scanned
	// +optional
	RunningNodes int32 `json:"runningNodes,omitempty"`

	// FailedNodes is the number of nodes that failed to scan
	// +optional
	FailedNodes int32 `json:"failedNodes,omitempty"`

	// InfectedNodes is the number of nodes with infected files
	// +optional
	InfectedNodes int32 `json:"infectedNodes,omitempty"`

	// TotalFilesScanned across all nodes
	// +optional
	TotalFilesScanned int64 `json:"totalFilesScanned,omitempty"`

	// TotalFilesInfected across all nodes
	// +optional
	TotalFilesInfected int64 `json:"totalFilesInfected,omitempty"`

	// NodeScans contains references to individual node scans
	// +optional
	NodeScans []NodeScanReference `json:"nodeScans,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=cs;clusterscan
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Total",type=integer,JSONPath=`.status.totalNodes`
// +kubebuilder:printcolumn:name="Completed",type=integer,JSONPath=`.status.completedNodes`
// +kubebuilder:printcolumn:name="Running",type=integer,JSONPath=`.status.runningNodes`
// +kubebuilder:printcolumn:name="Failed",type=integer,JSONPath=`.status.failedNodes`
// +kubebuilder:printcolumn:name="Infected",type=integer,JSONPath=`.status.totalFilesInfected`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ClusterScan is the Schema for the clusterscans API
type ClusterScan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterScanSpec   `json:"spec,omitempty"`
	Status ClusterScanStatus `json:"status,omitempty"`
}

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
