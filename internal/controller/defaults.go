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
//
// STANDALONE MODE MEMORY MODEL
// In standalone mode the scanner spawns a new /usr/bin/clamscan subprocess for
// every file it scans. Each subprocess loads the full ClamAV signature database
// (~300-400 MB) into its own address space before scanning the file. With
// MaxConcurrent=N you therefore have N simultaneous database copies in memory:
//
//	memory_needed ≈ N × 400 MB + 300 MB (Node.js runtime + OS buffers)
//
// The previous defaults (512 Mi limit, 3 concurrent) guaranteed an OOMKill on
// virtually every standalone scan. The corrected defaults below assume
// MaxConcurrent=1 (the standalone default), which keeps peak memory under 1 Gi.
// If MaxConcurrent is raised, raise the memory limits proportionally.
//
// DAEMON MODE MEMORY MODEL
// In daemon mode the clamd process loads the signature database once on startup.
// File-scan requests are forwarded over a Unix socket — no per-file subprocess
// is spawned. Memory consumption is much lower and is dominated by the daemon
// pod, not the scanner Job. The scanner Job itself needs only ~256 Mi.
var (
	// DefaultScannerResources defines the default resource requirements for scanner jobs.
	// Sized for standalone mode with MaxConcurrent=1. Override via ScanPolicy.spec.resources.
	DefaultScannerResources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:              resource.MustParse("250m"),
			corev1.ResourceMemory:           resource.MustParse("1Gi"),
			corev1.ResourceEphemeralStorage: resource.MustParse("256Mi"),
		},
		Limits: corev1.ResourceList{
			// 1 core: clamscan is CPU-intensive but capping at 1 core prevents
			// scan jobs from starving other workloads on the same node.
			corev1.ResourceCPU: resource.MustParse("1000m"),
			// 2 Gi: 1× database load (~400 Mi) + Node.js runtime (~200 Mi)
			// + headroom for archive decompression (clamscan unpacks ZIPs/TARs
			// into memory before scanning their contents).
			corev1.ResourceMemory: resource.MustParse("2Gi"),
			// Cap ephemeral storage so a runaway scan can't fill the node's disk.
			corev1.ResourceEphemeralStorage: resource.MustParse("2Gi"),
		},
	}

	// HighPriorityScannerResources defines resources for high-priority scans.
	// Used when NodeScan.Spec.Priority is set to "high".
	// Allows MaxConcurrent=2 in standalone mode (2 × 400 Mi ≈ 800 Mi peak DB load).
	HighPriorityScannerResources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:              resource.MustParse("500m"),
			corev1.ResourceMemory:           resource.MustParse("1500Mi"),
			corev1.ResourceEphemeralStorage: resource.MustParse("512Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:              resource.MustParse("2000m"),
			corev1.ResourceMemory:           resource.MustParse("3Gi"),
			corev1.ResourceEphemeralStorage: resource.MustParse("4Gi"),
		},
	}

	// LowPriorityScannerResources defines resources for low-priority/background scans.
	// Used when NodeScan.Spec.Priority is set to "low".
	// MaxConcurrent MUST be 1 with these limits — a second clamscan subprocess
	// loading the DB would push total memory over the 2 Gi limit.
	LowPriorityScannerResources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:              resource.MustParse("100m"),
			corev1.ResourceMemory:           resource.MustParse("768Mi"),
			corev1.ResourceEphemeralStorage: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:              resource.MustParse("500m"),
			corev1.ResourceMemory:           resource.MustParse("2Gi"),
			corev1.ResourceEphemeralStorage: resource.MustParse("1Gi"),
		},
	}
)

// Default scan configuration values
const (
	// DefaultMaxConcurrent is the default number of files to scan in parallel.
	//
	// In standalone mode each concurrent slot spawns a separate /usr/bin/clamscan
	// subprocess that loads the full signature database (~300-400 Mi) into its own
	// address space. Setting this to 3 therefore triples peak memory consumption
	// and virtually guarantees OOMKill with the default resource limits.
	//
	// Default: 1 (safe for both standalone and daemon modes).
	// Raise only when resources.limits.memory is increased proportionally:
	//   MaxConcurrent=2 → limits.memory ≥ 2.5 Gi
	//   MaxConcurrent=3 → limits.memory ≥ 3.5 Gi
	DefaultMaxConcurrent = 1

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
