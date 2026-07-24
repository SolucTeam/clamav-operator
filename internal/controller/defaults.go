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
)

// Default scan configuration values
const (
	// DefaultMaxConcurrent is the default number of files to scan in parallel.
	//
	// In standalone mode each concurrent slot spawns a separate /usr/bin/clamscan
	// subprocess that loads the full signature database (~300-400 Mi) into its own
	// address space. Setting this too high can cause OOMKill with default resource
	// limits.
	//
	// Default: 5 (aligned with the kubebuilder CRD default).
	// For standalone mode with limited memory, consider lowering and increasing
	// resources.limits.memory proportionally:
	//   MaxConcurrent=1 → limits.memory ≥ 1.5 Gi
	//   MaxConcurrent=2 → limits.memory ≥ 2.5 Gi
	//   MaxConcurrent=3 → limits.memory ≥ 3.5 Gi
	DefaultMaxConcurrent = 5

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

// GetResourcesForPriority returns an empty ResourceRequirements so that no
// resource constraints are imposed by the operator itself. Resources must be
// defined explicitly by the user via one of the following (in priority order):
//
//  1. NodeScan.spec.resources (per-scan override)
//  2. ScanPolicy.spec.resources (policy-level, set via Helm defaultScanPolicy.spec.resources)
//
// If neither is set the pod runs without resource limits, relying on the
// cluster's LimitRange or namespace defaults. This intentional design keeps
// the operator unopinionated about sizing — the user owns that decision.
func GetResourcesForPriority(_ string) corev1.ResourceRequirements {
	return corev1.ResourceRequirements{}
}
