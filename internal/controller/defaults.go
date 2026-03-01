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

package controller

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
			corev1.ResourceCPU:              resource.MustParse("100m"),
			corev1.ResourceMemory:           resource.MustParse("256Mi"),
			corev1.ResourceEphemeralStorage: resource.MustParse("256Mi"),
		},
		Limits: corev1.ResourceList{
			// 1 core max to prevent scans from starving the node
			corev1.ResourceCPU:    resource.MustParse("1000m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
			// Cap ephemeral storage so a runaway scan can't fill the node's disk
			corev1.ResourceEphemeralStorage: resource.MustParse("2Gi"),
		},
	}

	// HighPriorityScannerResources defines resources for high-priority scans.
	// Used when NodeScan.Spec.Priority is set to "high".
	HighPriorityScannerResources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:              resource.MustParse("500m"),
			corev1.ResourceMemory:           resource.MustParse("512Mi"),
			corev1.ResourceEphemeralStorage: resource.MustParse("512Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:              resource.MustParse("2000m"),
			corev1.ResourceMemory:           resource.MustParse("1Gi"),
			corev1.ResourceEphemeralStorage: resource.MustParse("4Gi"),
		},
	}

	// LowPriorityScannerResources defines resources for low-priority/background scans.
	// Used when NodeScan.Spec.Priority is set to "low".
	LowPriorityScannerResources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:              resource.MustParse("50m"),
			corev1.ResourceMemory:           resource.MustParse("128Mi"),
			corev1.ResourceEphemeralStorage: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:              resource.MustParse("500m"),
			corev1.ResourceMemory:           resource.MustParse("256Mi"),
			corev1.ResourceEphemeralStorage: resource.MustParse("1Gi"),
		},
	}
)

// Default scan configuration values
const (
	// DefaultMaxConcurrent is the default number of files to scan in parallel.
	// Reduced to 3 to limit CPU pressure on busy nodes.
	DefaultMaxConcurrent = 3

	// DefaultFileTimeout is the default timeout for scanning a single file (ms)
	DefaultFileTimeout = 300000 // 5 minutes

	// DefaultMaxFileSize is the default maximum file size to scan (bytes)
	DefaultMaxFileSize = 104857600 // 100MB

	// DefaultConnectTimeout is the default timeout for connecting to ClamAV (ms)
	DefaultConnectTimeout = 60000 // 60 seconds

	// DefaultTTLSecondsAfterFinished is the default TTL for completed jobs
	DefaultTTLSecondsAfterFinished = 86400 // 24 hours

	// TTLSecondsAfterSucceeded is the TTL applied to succeeded jobs.
	// Kept short: the NodeScan CRD is the source of truth — the Job pod is disposable.
	TTLSecondsAfterSucceeded = 3600 // 1 hour

	// TTLSecondsAfterFailed is the TTL applied to failed jobs.
	// Kept longer to allow post-mortem log inspection.
	TTLSecondsAfterFailed = 86400 // 24 hours

	// JobActiveDeadlineSeconds is the hard wall-clock deadline for a scan Job.
	// Prevents pods from blocking a worker goroutine (or a node) indefinitely.
	JobActiveDeadlineSeconds = 7200 // 2 hours

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
