/*
Copyright 2025 The ClamAV Operator Authors.

Incremental Scan — Types and Definitions
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ScanStrategy defines the scan strategy to use
// +kubebuilder:validation:Enum=full;incremental;modified-only;smart
type ScanStrategy string

const (
	// ScanStrategyFull scans every file on every run
	ScanStrategyFull ScanStrategy = "full"

	// ScanStrategyIncremental scans only files modified since the last successful scan
	ScanStrategyIncremental ScanStrategy = "incremental"

	// ScanStrategyModifiedOnly scans only files modified within the last 24 h
	ScanStrategyModifiedOnly ScanStrategy = "modified-only"

	// ScanStrategySmart alternates between incremental and full scans automatically
	ScanStrategySmart ScanStrategy = "smart"
)

// IncrementalScanConfig configures incremental scan behavior
type IncrementalScanConfig struct {
	// Enabled activates incremental scanning
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Strategy defines the scan strategy
	// +kubebuilder:default=incremental
	// +optional
	Strategy ScanStrategy `json:"strategy,omitempty"`

	// BaselineInterval triggers a full scan every X incremental scans.
	// For example, if set to 7, a full scan is triggered every 7 incremental scans
	// +kubebuilder:default=7
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=30
	// +optional
	BaselineInterval int32 `json:"baselineInterval,omitempty"`

	// MaxAge defines the maximum age (in hours) of files to consider for incremental scans.
	// Used with the modified-only and smart strategies
	// +kubebuilder:default=24
	// +optional
	MaxAge int32 `json:"maxAge,omitempty"`

	// MinTimeBetweenScans defines the minimum delay (in hours) between two scans.
	// Prevents rescanning the same node too frequently
	// +kubebuilder:default=6
	// +optional
	MinTimeBetweenScans int32 `json:"minTimeBetweenScans,omitempty"`

	// CacheExpiration defines the cache validity duration (in hours).
	// After this period a full scan is forced
	// +kubebuilder:default=168
	// +optional
	CacheExpiration int32 `json:"cacheExpiration,omitempty"`

	// SkipUnchangedFiles skips files whose mtime+size have not changed since the last scan
	// +kubebuilder:default=true
	// +optional
	SkipUnchangedFiles bool `json:"skipUnchangedFiles,omitempty"`
}

// FileMetadata holds the metadata of a scanned file
type FileMetadata struct {
	// Path is the absolute path to the file
	Path string `json:"path"`

	// ModTime is the Unix modification timestamp
	ModTime int64 `json:"modTime"`

	// Size is the file size in bytes
	Size int64 `json:"size"`

	// Hash is the SHA256 hash of the file (optional)
	// +optional
	Hash string `json:"hash,omitempty"`

	// LastScanned is the Unix timestamp of the last scan
	LastScanned int64 `json:"lastScanned"`

	// ScanResult is the outcome of the last scan: "clean" or "infected"
	ScanResult string `json:"scanResult"`
}

// ScanCache holds the scan cache for a node
type ScanCache struct {
	// NodeName is the name of the scanned node
	NodeName string `json:"nodeName"`

	// LastFullScan is the Unix timestamp of the last full scan
	LastFullScan int64 `json:"lastFullScan"`

	// LastIncrementalScan is the Unix timestamp of the last incremental scan
	// +optional
	LastIncrementalScan int64 `json:"lastIncrementalScan,omitempty"`

	// ScanCount is the number of scans performed since the last full scan
	ScanCount int32 `json:"scanCount"`

	// Files contains the metadata of scanned files.
	// Capped at 10 000 entries to keep the CustomResource size manageable
	// +optional
	Files []FileMetadata `json:"files,omitempty"`

	// TotalFiles is the total number of tracked files
	TotalFiles int64 `json:"totalFiles"`

	// CacheVersion specifies the cache format version (for future migrations)
	CacheVersion string `json:"cacheVersion"`
}

// ScanCacheStatus defines the observed state of the scan cache
type ScanCacheStatus struct {
	// LastUpdated is the timestamp of the last cache update
	// +optional
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`

	// Size is the cache size in bytes
	// +optional
	Size int64 `json:"size,omitempty"`

	// Compressed indicates whether the cache data is compressed
	// +optional
	Compressed bool `json:"compressed,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=sc;scancache

// ScanCacheResource stores the per-node scan cache
type ScanCacheResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScanCache       `json:"spec,omitempty"`
	Status ScanCacheStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ScanCacheResourceList contains a list of ScanCache
type ScanCacheResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScanCacheResource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ScanCacheResource{}, &ScanCacheResourceList{})
}
