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

// Package v1alpha1 implements the conversion hub-spoke pattern.
//
// v1beta1 is the hub (storage version). v1alpha1 types are the spokes and
// must implement ConvertTo (spoke → hub) and ConvertFrom (hub → spoke).
//
// Because v1alpha1 and v1beta1 are structurally identical at this point, the
// conversions are simple field-for-field copies.  When either version gains new
// fields in the future, the corresponding conversion direction must be updated
// to handle defaulting / migration.
package v1alpha1

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1beta1 "github.com/SolucTeam/clamav-operator/api/v1beta1"
)

// ─── NodeScan ────────────────────────────────────────────────────────────────

// ConvertTo converts this NodeScan (v1alpha1) to the Hub version (v1beta1).
func (r *NodeScan) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*v1beta1.NodeScan)
	if !ok {
		return fmt.Errorf("expected *v1beta1.NodeScan, got %T", dstRaw)
	}

	dst.ObjectMeta = r.ObjectMeta

	// Spec
	dst.Spec.NodeName = r.Spec.NodeName
	dst.Spec.ScanPolicy = r.Spec.ScanPolicy
	dst.Spec.Priority = r.Spec.Priority
	dst.Spec.Paths = r.Spec.Paths
	dst.Spec.ExcludePatterns = r.Spec.ExcludePatterns
	dst.Spec.MaxConcurrent = r.Spec.MaxConcurrent
	dst.Spec.FileTimeout = r.Spec.FileTimeout
	dst.Spec.MaxFileSize = r.Spec.MaxFileSize
	dst.Spec.Resources = r.Spec.Resources
	dst.Spec.TTLSecondsAfterFinished = r.Spec.TTLSecondsAfterFinished
	dst.Spec.Strategy = v1beta1.ScanStrategy(r.Spec.Strategy)
	dst.Spec.ForceFullScan = r.Spec.ForceFullScan
	if r.Spec.IncrementalConfig != nil {
		// Field mapping v1alpha1 → v1beta1:
		//   BaselineInterval → FullScanInterval
		//   MaxAge           → MaxFileAgeHours
		dst.Spec.IncrementalConfig = &v1beta1.IncrementalScanConfig{
			FullScanInterval:   r.Spec.IncrementalConfig.BaselineInterval,
			MaxFileAgeHours:    r.Spec.IncrementalConfig.MaxAge,
			SkipUnchangedFiles: r.Spec.IncrementalConfig.SkipUnchangedFiles,
		}
	}

	// Status
	dst.Status.ObservedGeneration = r.Status.ObservedGeneration
	dst.Status.Phase = v1beta1.NodeScanPhase(r.Status.Phase)
	dst.Status.StartTime = r.Status.StartTime
	dst.Status.CompletionTime = r.Status.CompletionTime
	dst.Status.Duration = r.Status.Duration
	dst.Status.FilesScanned = r.Status.FilesScanned
	dst.Status.FilesInfected = r.Status.FilesInfected
	dst.Status.FilesSkipped = r.Status.FilesSkipped
	dst.Status.ErrorCount = r.Status.ErrorCount
	dst.Status.JobRef = r.Status.JobRef
	dst.Status.Conditions = r.Status.Conditions
	dst.Status.ReportPath = r.Status.ReportPath
	dst.Status.LastTransitionTime = r.Status.LastTransitionTime
	dst.Status.StrategyUsed = v1beta1.ScanStrategy(r.Status.StrategyUsed)
	dst.Status.FilesSkippedIncremental = r.Status.FilesSkippedIncremental
	dst.Status.CacheHitRate = r.Status.CacheHitRate
	dst.Status.TimeSaved = r.Status.TimeSaved
	dst.Status.FailureReason = r.Status.FailureReason
	dst.Status.ExitCode = r.Status.ExitCode

	for _, f := range r.Status.InfectedFiles {
		dst.Status.InfectedFiles = append(dst.Status.InfectedFiles, v1beta1.InfectedFile{
			Path:       f.Path,
			Viruses:    f.Viruses,
			Size:       f.Size,
			DetectedAt: f.DetectedAt,
		})
	}
	dst.Status.InfectedFilesTruncated = r.Status.InfectedFilesTruncated
	dst.Status.ResultsPartial = r.Status.ResultsPartial

	return nil
}

// ConvertFrom converts from the Hub version (v1beta1) to this NodeScan (v1alpha1).
func (r *NodeScan) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*v1beta1.NodeScan)
	if !ok {
		return fmt.Errorf("expected *v1beta1.NodeScan, got %T", srcRaw)
	}

	r.ObjectMeta = src.ObjectMeta

	// Spec
	r.Spec.NodeName = src.Spec.NodeName
	r.Spec.ScanPolicy = src.Spec.ScanPolicy
	r.Spec.Priority = src.Spec.Priority
	r.Spec.Paths = src.Spec.Paths
	r.Spec.ExcludePatterns = src.Spec.ExcludePatterns
	r.Spec.MaxConcurrent = src.Spec.MaxConcurrent
	r.Spec.FileTimeout = src.Spec.FileTimeout
	r.Spec.MaxFileSize = src.Spec.MaxFileSize
	r.Spec.Resources = src.Spec.Resources
	r.Spec.TTLSecondsAfterFinished = src.Spec.TTLSecondsAfterFinished
	r.Spec.Strategy = ScanStrategy(src.Spec.Strategy)
	r.Spec.ForceFullScan = src.Spec.ForceFullScan
	if src.Spec.IncrementalConfig != nil {
		// Field mapping v1beta1 → v1alpha1 (reverse of ConvertTo):
		//   FullScanInterval → BaselineInterval
		//   MaxFileAgeHours  → MaxAge
		r.Spec.IncrementalConfig = &IncrementalScanConfig{
			BaselineInterval:   src.Spec.IncrementalConfig.FullScanInterval,
			MaxAge:             src.Spec.IncrementalConfig.MaxFileAgeHours,
			SkipUnchangedFiles: src.Spec.IncrementalConfig.SkipUnchangedFiles,
		}
	}

	// Status
	r.Status.ObservedGeneration = src.Status.ObservedGeneration
	r.Status.Phase = NodeScanPhase(src.Status.Phase)
	r.Status.StartTime = src.Status.StartTime
	r.Status.CompletionTime = src.Status.CompletionTime
	r.Status.Duration = src.Status.Duration
	r.Status.FilesScanned = src.Status.FilesScanned
	r.Status.FilesInfected = src.Status.FilesInfected
	r.Status.FilesSkipped = src.Status.FilesSkipped
	r.Status.ErrorCount = src.Status.ErrorCount
	r.Status.JobRef = src.Status.JobRef
	r.Status.Conditions = src.Status.Conditions
	r.Status.ReportPath = src.Status.ReportPath
	r.Status.LastTransitionTime = src.Status.LastTransitionTime
	r.Status.StrategyUsed = ScanStrategy(src.Status.StrategyUsed)
	r.Status.FilesSkippedIncremental = src.Status.FilesSkippedIncremental
	r.Status.CacheHitRate = src.Status.CacheHitRate
	r.Status.TimeSaved = src.Status.TimeSaved
	r.Status.FailureReason = src.Status.FailureReason
	r.Status.ExitCode = src.Status.ExitCode

	for _, f := range src.Status.InfectedFiles {
		r.Status.InfectedFiles = append(r.Status.InfectedFiles, InfectedFile{
			Path:       f.Path,
			Viruses:    f.Viruses,
			Size:       f.Size,
			DetectedAt: f.DetectedAt,
		})
	}
	r.Status.InfectedFilesTruncated = src.Status.InfectedFilesTruncated
	r.Status.ResultsPartial = src.Status.ResultsPartial

	return nil
}

// ─── ClusterScan ─────────────────────────────────────────────────────────────

// ConvertTo converts this ClusterScan (v1alpha1) to the Hub version (v1beta1).
func (r *ClusterScan) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*v1beta1.ClusterScan)
	if !ok {
		return fmt.Errorf("expected *v1beta1.ClusterScan, got %T", dstRaw)
	}
	dst.ObjectMeta = r.ObjectMeta
	dst.Spec.NodeSelector = r.Spec.NodeSelector
	dst.Spec.ScanPolicy = r.Spec.ScanPolicy
	dst.Spec.Concurrent = r.Spec.Concurrent
	dst.Spec.Priority = r.Spec.Priority
	// NodeScanTemplate conversion delegated inline
	if r.Spec.NodeScanTemplate != nil {
		t := &v1beta1.NodeScanSpec{
			NodeName:                r.Spec.NodeScanTemplate.NodeName,
			ScanPolicy:              r.Spec.NodeScanTemplate.ScanPolicy,
			Priority:                r.Spec.NodeScanTemplate.Priority,
			Paths:                   r.Spec.NodeScanTemplate.Paths,
			ExcludePatterns:         r.Spec.NodeScanTemplate.ExcludePatterns,
			MaxConcurrent:           r.Spec.NodeScanTemplate.MaxConcurrent,
			FileTimeout:             r.Spec.NodeScanTemplate.FileTimeout,
			MaxFileSize:             r.Spec.NodeScanTemplate.MaxFileSize,
			Resources:               r.Spec.NodeScanTemplate.Resources,
			TTLSecondsAfterFinished: r.Spec.NodeScanTemplate.TTLSecondsAfterFinished,
			Strategy:                v1beta1.ScanStrategy(r.Spec.NodeScanTemplate.Strategy),
			ForceFullScan:           r.Spec.NodeScanTemplate.ForceFullScan,
		}
		if r.Spec.NodeScanTemplate.IncrementalConfig != nil {
			t.IncrementalConfig = &v1beta1.IncrementalScanConfig{
				FullScanInterval:   r.Spec.NodeScanTemplate.IncrementalConfig.BaselineInterval,
				MaxFileAgeHours:    r.Spec.NodeScanTemplate.IncrementalConfig.MaxAge,
				SkipUnchangedFiles: r.Spec.NodeScanTemplate.IncrementalConfig.SkipUnchangedFiles,
			}
		}
		dst.Spec.NodeScanTemplate = t
	}
	dst.Status.ObservedGeneration = r.Status.ObservedGeneration
	dst.Status.Phase = v1beta1.ClusterScanPhase(r.Status.Phase)
	dst.Status.StartTime = r.Status.StartTime
	dst.Status.CompletionTime = r.Status.CompletionTime
	dst.Status.TotalNodes = r.Status.TotalNodes
	dst.Status.CompletedNodes = r.Status.CompletedNodes
	dst.Status.RunningNodes = r.Status.RunningNodes
	dst.Status.FailedNodes = r.Status.FailedNodes
	dst.Status.InfectedNodes = r.Status.InfectedNodes
	dst.Status.TotalFilesScanned = r.Status.TotalFilesScanned
	dst.Status.TotalFilesInfected = r.Status.TotalFilesInfected
	dst.Status.Conditions = r.Status.Conditions
	for _, ns := range r.Status.NodeScans {
		dst.Status.NodeScans = append(dst.Status.NodeScans, v1beta1.NodeScanReference{
			Name: ns.Name, NodeName: ns.NodeName,
			Phase:          v1beta1.NodeScanPhase(ns.Phase),
			FilesInfected:  ns.FilesInfected,
			FilesScanned:   ns.FilesScanned,
			StartTime:      ns.StartTime,
			CompletionTime: ns.CompletionTime,
		})
	}
	return nil
}

// ConvertFrom converts from the Hub version (v1beta1) to this ClusterScan (v1alpha1).
func (r *ClusterScan) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*v1beta1.ClusterScan)
	if !ok {
		return fmt.Errorf("expected *v1beta1.ClusterScan, got %T", srcRaw)
	}
	r.ObjectMeta = src.ObjectMeta
	r.Spec.NodeSelector = src.Spec.NodeSelector
	r.Spec.ScanPolicy = src.Spec.ScanPolicy
	r.Spec.Concurrent = src.Spec.Concurrent
	r.Spec.Priority = src.Spec.Priority
	if src.Spec.NodeScanTemplate != nil {
		t := &NodeScanSpec{
			NodeName:                src.Spec.NodeScanTemplate.NodeName,
			ScanPolicy:              src.Spec.NodeScanTemplate.ScanPolicy,
			Priority:                src.Spec.NodeScanTemplate.Priority,
			Paths:                   src.Spec.NodeScanTemplate.Paths,
			ExcludePatterns:         src.Spec.NodeScanTemplate.ExcludePatterns,
			MaxConcurrent:           src.Spec.NodeScanTemplate.MaxConcurrent,
			FileTimeout:             src.Spec.NodeScanTemplate.FileTimeout,
			MaxFileSize:             src.Spec.NodeScanTemplate.MaxFileSize,
			Resources:               src.Spec.NodeScanTemplate.Resources,
			TTLSecondsAfterFinished: src.Spec.NodeScanTemplate.TTLSecondsAfterFinished,
			Strategy:                ScanStrategy(src.Spec.NodeScanTemplate.Strategy),
			ForceFullScan:           src.Spec.NodeScanTemplate.ForceFullScan,
		}
		if src.Spec.NodeScanTemplate.IncrementalConfig != nil {
			t.IncrementalConfig = &IncrementalScanConfig{
				BaselineInterval:   src.Spec.NodeScanTemplate.IncrementalConfig.FullScanInterval,
				MaxAge:             src.Spec.NodeScanTemplate.IncrementalConfig.MaxFileAgeHours,
				SkipUnchangedFiles: src.Spec.NodeScanTemplate.IncrementalConfig.SkipUnchangedFiles,
			}
		}
		r.Spec.NodeScanTemplate = t
	}
	r.Status.ObservedGeneration = src.Status.ObservedGeneration
	r.Status.Phase = ClusterScanPhase(src.Status.Phase)
	r.Status.StartTime = src.Status.StartTime
	r.Status.CompletionTime = src.Status.CompletionTime
	r.Status.TotalNodes = src.Status.TotalNodes
	r.Status.CompletedNodes = src.Status.CompletedNodes
	r.Status.RunningNodes = src.Status.RunningNodes
	r.Status.FailedNodes = src.Status.FailedNodes
	r.Status.InfectedNodes = src.Status.InfectedNodes
	r.Status.TotalFilesScanned = src.Status.TotalFilesScanned
	r.Status.TotalFilesInfected = src.Status.TotalFilesInfected
	r.Status.Conditions = src.Status.Conditions
	for _, ns := range src.Status.NodeScans {
		r.Status.NodeScans = append(r.Status.NodeScans, NodeScanReference{
			Name: ns.Name, NodeName: ns.NodeName,
			Phase:          NodeScanPhase(ns.Phase),
			FilesInfected:  ns.FilesInfected,
			FilesScanned:   ns.FilesScanned,
			StartTime:      ns.StartTime,
			CompletionTime: ns.CompletionTime,
		})
	}
	return nil
}

// ─── ScanSchedule ────────────────────────────────────────────────────────────

// ConvertTo converts this ScanSchedule (v1alpha1) to the Hub version (v1beta1).
func (r *ScanSchedule) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*v1beta1.ScanSchedule)
	if !ok {
		return fmt.Errorf("expected *v1beta1.ScanSchedule, got %T", dstRaw)
	}
	dst.ObjectMeta = r.ObjectMeta
	dst.Spec.Schedule = r.Spec.Schedule
	dst.Spec.Suspend = r.Spec.Suspend
	dst.Spec.SuccessfulScansHistoryLimit = r.Spec.SuccessfulScansHistoryLimit
	dst.Spec.FailedScansHistoryLimit = r.Spec.FailedScansHistoryLimit
	dst.Spec.ConcurrencyPolicy = r.Spec.ConcurrencyPolicy
	dst.Spec.StartingDeadlineSeconds = r.Spec.StartingDeadlineSeconds
	// Inline ClusterScanSpec conversion
	dst.Spec.ClusterScan = v1beta1.ClusterScanSpec{
		NodeSelector: r.Spec.ClusterScan.NodeSelector,
		ScanPolicy:   r.Spec.ClusterScan.ScanPolicy,
		Concurrent:   r.Spec.ClusterScan.Concurrent,
		Priority:     r.Spec.ClusterScan.Priority,
	}
	if r.Spec.ClusterScan.NodeScanTemplate != nil {
		t := &v1beta1.NodeScanSpec{
			NodeName:                r.Spec.ClusterScan.NodeScanTemplate.NodeName,
			ScanPolicy:              r.Spec.ClusterScan.NodeScanTemplate.ScanPolicy,
			Priority:                r.Spec.ClusterScan.NodeScanTemplate.Priority,
			Paths:                   r.Spec.ClusterScan.NodeScanTemplate.Paths,
			ExcludePatterns:         r.Spec.ClusterScan.NodeScanTemplate.ExcludePatterns,
			MaxConcurrent:           r.Spec.ClusterScan.NodeScanTemplate.MaxConcurrent,
			FileTimeout:             r.Spec.ClusterScan.NodeScanTemplate.FileTimeout,
			MaxFileSize:             r.Spec.ClusterScan.NodeScanTemplate.MaxFileSize,
			Resources:               r.Spec.ClusterScan.NodeScanTemplate.Resources,
			TTLSecondsAfterFinished: r.Spec.ClusterScan.NodeScanTemplate.TTLSecondsAfterFinished,
			Strategy:                v1beta1.ScanStrategy(r.Spec.ClusterScan.NodeScanTemplate.Strategy),
			ForceFullScan:           r.Spec.ClusterScan.NodeScanTemplate.ForceFullScan,
		}
		if r.Spec.ClusterScan.NodeScanTemplate.IncrementalConfig != nil {
			t.IncrementalConfig = &v1beta1.IncrementalScanConfig{
				FullScanInterval:   r.Spec.ClusterScan.NodeScanTemplate.IncrementalConfig.BaselineInterval,
				MaxFileAgeHours:    r.Spec.ClusterScan.NodeScanTemplate.IncrementalConfig.MaxAge,
				SkipUnchangedFiles: r.Spec.ClusterScan.NodeScanTemplate.IncrementalConfig.SkipUnchangedFiles,
			}
		}
		dst.Spec.ClusterScan.NodeScanTemplate = t
	}
	dst.Status.Active = r.Status.Active
	dst.Status.LastScheduleTime = r.Status.LastScheduleTime
	dst.Status.LastSuccessfulTime = r.Status.LastSuccessfulTime
	dst.Status.NextScheduleTime = r.Status.NextScheduleTime
	dst.Status.LastClusterScan = r.Status.LastClusterScan
	dst.Status.Conditions = r.Status.Conditions
	return nil
}

// ConvertFrom converts from the Hub version (v1beta1) to this ScanSchedule (v1alpha1).
func (r *ScanSchedule) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*v1beta1.ScanSchedule)
	if !ok {
		return fmt.Errorf("expected *v1beta1.ScanSchedule, got %T", srcRaw)
	}
	r.ObjectMeta = src.ObjectMeta
	r.Spec.Schedule = src.Spec.Schedule
	r.Spec.Suspend = src.Spec.Suspend
	r.Spec.SuccessfulScansHistoryLimit = src.Spec.SuccessfulScansHistoryLimit
	r.Spec.FailedScansHistoryLimit = src.Spec.FailedScansHistoryLimit
	r.Spec.ConcurrencyPolicy = src.Spec.ConcurrencyPolicy
	r.Spec.StartingDeadlineSeconds = src.Spec.StartingDeadlineSeconds
	r.Spec.ClusterScan = ClusterScanSpec{
		NodeSelector: src.Spec.ClusterScan.NodeSelector,
		ScanPolicy:   src.Spec.ClusterScan.ScanPolicy,
		Concurrent:   src.Spec.ClusterScan.Concurrent,
		Priority:     src.Spec.ClusterScan.Priority,
	}
	if src.Spec.ClusterScan.NodeScanTemplate != nil {
		t := &NodeScanSpec{
			NodeName:                src.Spec.ClusterScan.NodeScanTemplate.NodeName,
			ScanPolicy:              src.Spec.ClusterScan.NodeScanTemplate.ScanPolicy,
			Priority:                src.Spec.ClusterScan.NodeScanTemplate.Priority,
			Paths:                   src.Spec.ClusterScan.NodeScanTemplate.Paths,
			ExcludePatterns:         src.Spec.ClusterScan.NodeScanTemplate.ExcludePatterns,
			MaxConcurrent:           src.Spec.ClusterScan.NodeScanTemplate.MaxConcurrent,
			FileTimeout:             src.Spec.ClusterScan.NodeScanTemplate.FileTimeout,
			MaxFileSize:             src.Spec.ClusterScan.NodeScanTemplate.MaxFileSize,
			Resources:               src.Spec.ClusterScan.NodeScanTemplate.Resources,
			TTLSecondsAfterFinished: src.Spec.ClusterScan.NodeScanTemplate.TTLSecondsAfterFinished,
			Strategy:                ScanStrategy(src.Spec.ClusterScan.NodeScanTemplate.Strategy),
			ForceFullScan:           src.Spec.ClusterScan.NodeScanTemplate.ForceFullScan,
		}
		if src.Spec.ClusterScan.NodeScanTemplate.IncrementalConfig != nil {
			t.IncrementalConfig = &IncrementalScanConfig{
				BaselineInterval:   src.Spec.ClusterScan.NodeScanTemplate.IncrementalConfig.FullScanInterval,
				MaxAge:             src.Spec.ClusterScan.NodeScanTemplate.IncrementalConfig.MaxFileAgeHours,
				SkipUnchangedFiles: src.Spec.ClusterScan.NodeScanTemplate.IncrementalConfig.SkipUnchangedFiles,
			}
		}
		r.Spec.ClusterScan.NodeScanTemplate = t
	}
	r.Status.Active = src.Status.Active
	r.Status.LastScheduleTime = src.Status.LastScheduleTime
	r.Status.LastSuccessfulTime = src.Status.LastSuccessfulTime
	r.Status.NextScheduleTime = src.Status.NextScheduleTime
	r.Status.LastClusterScan = src.Status.LastClusterScan
	r.Status.Conditions = src.Status.Conditions
	return nil
}
