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

package controllers

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Default resource limits and requests for scan jobs.
// These values are applied when no custom resources are specified
// in NodeScan, ClusterScan, or ScanPolicy resources.
var (
	// DefaultScannerResources defines the default resource requirements for scanner jobs.
	// These values balance scan performance with cluster resource conservation.
	DefaultScannerResources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			// CPU request: minimum CPU guaranteed for the scan job
			corev1.ResourceCPU: resource.MustParse("100m"),
			// Memory request: minimum memory guaranteed for the scan job
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Limits: corev1.ResourceList{
			// CPU limit: maximum CPU the scan job can use
			// Set to 1 core to prevent scans from impacting node performance
			corev1.ResourceCPU: resource.MustParse("1000m"),
			// Memory limit: maximum memory the scan job can use
			// Set to 512Mi to handle large file scanning without OOM
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}

	// HighPriorityScannerResources defines resources for high-priority scans.
	// Used when NodeScan.Spec.Priority is set to "high".
	HighPriorityScannerResources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2000m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
	}

	// LowPriorityScannerResources defines resources for low-priority/background scans.
	// Used when NodeScan.Spec.Priority is set to "low".
	LowPriorityScannerResources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("50m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
	}
)

// Default scan configuration values
const (
	// DefaultMaxConcurrent is the default number of files to scan in parallel
	DefaultMaxConcurrent = 5

	// DefaultFileTimeout is the default timeout for scanning a single file (ms)
	DefaultFileTimeout = 300000 // 5 minutes

	// DefaultMaxFileSize is the default maximum file size to scan (bytes)
	DefaultMaxFileSize = 104857600 // 100MB

	// DefaultConnectTimeout is the default timeout for connecting to ClamAV (ms)
	DefaultConnectTimeout = 60000 // 60 seconds

	// DefaultTTLSecondsAfterFinished is the default TTL for completed jobs
	DefaultTTLSecondsAfterFinished = 86400 // 24 hours

	// DefaultConcurrentClusterScans is the default number of parallel node scans in ClusterScan
	DefaultConcurrentClusterScans = 3
)

// Default paths to scan if none specified
var DefaultScanPaths = []string{
	"/host/var/lib",
	"/host/opt",
}

// GetResourcesForPriority returns the appropriate resource requirements based on scan priority.
func GetResourcesForPriority(priority string) corev1.ResourceRequirements {
	switch priority {
	case "high":
		return HighPriorityScannerResources
	case "low":
		return LowPriorityScannerResources
	default:
		return DefaultScannerResources
	}
}
