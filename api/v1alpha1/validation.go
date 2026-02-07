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
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// Validation constants
const (
	// MaxPathLength is the maximum length for a scan path
	MaxPathLength = 4096

	// MaxPaths is the maximum number of paths that can be specified
	MaxPaths = 100

	// MaxExcludePatterns is the maximum number of exclude patterns
	MaxExcludePatterns = 200

	// MaxExcludePatternLength is the maximum length for an exclude pattern
	MaxExcludePatternLength = 1024

	// MinFileTimeout is the minimum file timeout in milliseconds
	MinFileTimeout = 1000 // 1 second

	// MaxFileTimeout is the maximum file timeout in milliseconds
	MaxFileTimeout = 3600000 // 1 hour

	// MinMaxFileSize is the minimum value for maxFileSize
	MinMaxFileSize = 1024 // 1KB

	// MaxMaxFileSize is the maximum value for maxFileSize (10GB)
	MaxMaxFileSize = 10737418240

	// MinConcurrent is the minimum concurrent value
	MinConcurrent = 1

	// MaxConcurrent is the maximum concurrent value
	MaxConcurrent = 50

	// MinNodeScanConcurrent is the minimum concurrent value for NodeScan
	MinNodeScanConcurrent = 1

	// MaxNodeScanConcurrent is the maximum concurrent value for NodeScan
	MaxNodeScanConcurrent = 20
)

// Dangerous path prefixes that should not be scanned
var dangerousPaths = []string{
	"/proc",
	"/sys",
	"/dev",
}

// ValidatePaths validates a list of scan paths
func ValidatePaths(paths []string, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if len(paths) > MaxPaths {
		allErrs = append(allErrs, field.TooMany(fldPath, len(paths), MaxPaths))
	}

	for i, path := range paths {
		pathField := fldPath.Index(i)

		// Check path length
		if len(path) > MaxPathLength {
			allErrs = append(allErrs, field.TooLong(pathField, path, MaxPathLength))
		}

		// Check for empty path
		if strings.TrimSpace(path) == "" {
			allErrs = append(allErrs, field.Invalid(pathField, path, "path cannot be empty"))
		}

		// Check for absolute path
		if !strings.HasPrefix(path, "/") {
			allErrs = append(allErrs, field.Invalid(pathField, path, "path must be absolute (start with /)"))
		}

		// Check for path traversal attempts
		if strings.Contains(path, "..") {
			allErrs = append(allErrs, field.Invalid(pathField, path, "path cannot contain '..' (path traversal)"))
		}

		// Warn about dangerous paths (but allow with /host prefix for container mounts)
		for _, dangerous := range dangerousPaths {
			if path == dangerous || strings.HasPrefix(path, dangerous+"/") {
				allErrs = append(allErrs, field.Invalid(pathField, path,
					fmt.Sprintf("scanning %s is not recommended and may cause system issues", dangerous)))
			}
		}
	}

	return allErrs
}

// ValidateExcludePatterns validates exclude patterns
func ValidateExcludePatterns(patterns []string, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if len(patterns) > MaxExcludePatterns {
		allErrs = append(allErrs, field.TooMany(fldPath, len(patterns), MaxExcludePatterns))
	}

	for i, pattern := range patterns {
		patternField := fldPath.Index(i)

		// Check pattern length
		if len(pattern) > MaxExcludePatternLength {
			allErrs = append(allErrs, field.TooLong(patternField, pattern, MaxExcludePatternLength))
		}

		// Check for empty pattern
		if strings.TrimSpace(pattern) == "" {
			allErrs = append(allErrs, field.Invalid(patternField, pattern, "pattern cannot be empty"))
		}

		// Validate the pattern: support both glob patterns (*.tmp, /var/lib/docker/*)
		// and regex patterns (^/tmp/.*\.log$).
		// A pattern is treated as regex only if it contains explicit regex anchors
		// or constructs that are not valid in globs.
		if isRegexPattern(pattern) {
			_, err := regexp.Compile(pattern)
			if err != nil {
				allErrs = append(allErrs, field.Invalid(patternField, pattern,
					fmt.Sprintf("invalid regex pattern: %v", err)))
			}
		} else {
			// Validate as a glob pattern
			_, err := filepath.Match(pattern, "")
			if err != nil {
				allErrs = append(allErrs, field.Invalid(patternField, pattern,
					fmt.Sprintf("invalid glob pattern: %v", err)))
			}
		}
	}

	return allErrs
}

// isRegexPattern returns true if the pattern looks like an intentional regex
// (contains anchors or regex-specific constructs) rather than a simple glob.
// Glob patterns like "*.tmp" or "/var/lib/docker/*" are NOT treated as regex.
func isRegexPattern(pattern string) bool {
	// Patterns starting with ^ or ending with $ are clearly regex
	if strings.HasPrefix(pattern, "^") || strings.HasSuffix(pattern, "$") {
		return true
	}
	// Patterns containing regex-specific constructs: character classes with
	// quantifiers, alternation, groups, look-aheads, etc.
	// Note: [] and {} are valid in globs too, but + and | are regex-only.
	if strings.ContainsAny(pattern, "+|") {
		return true
	}
	// Escaped characters like \d, \w, \s indicate regex
	if strings.Contains(pattern, `\d`) || strings.Contains(pattern, `\w`) || strings.Contains(pattern, `\s`) {
		return true
	}
	return false
}

// ValidateNodeName validates a node name
func ValidateNodeName(nodeName string, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if nodeName == "" {
		allErrs = append(allErrs, field.Required(fldPath, "nodeName is required"))
		return allErrs
	}

	// Check length (DNS-1123 subdomain max is 253)
	if len(nodeName) > 253 {
		allErrs = append(allErrs, field.TooLong(fldPath, nodeName, 253))
	}

	// Basic DNS-1123 validation
	if !isValidDNS1123Name(nodeName) {
		allErrs = append(allErrs, field.Invalid(fldPath, nodeName,
			"nodeName must be a valid DNS-1123 subdomain"))
	}

	return allErrs
}

// ValidatePriority validates a scan priority
func ValidatePriority(priority string, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	validPriorities := map[string]bool{
		"high":   true,
		"medium": true,
		"low":    true,
		"":       true, // empty defaults to medium
	}

	if !validPriorities[priority] {
		allErrs = append(allErrs, field.NotSupported(fldPath, priority, []string{"high", "medium", "low"}))
	}

	return allErrs
}

// ValidateConcurrent validates concurrent value for NodeScan
func ValidateNodeScanConcurrent(concurrent int32, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if concurrent != 0 && (concurrent < MinNodeScanConcurrent || concurrent > MaxNodeScanConcurrent) {
		allErrs = append(allErrs, field.Invalid(fldPath, concurrent,
			fmt.Sprintf("must be between %d and %d", MinNodeScanConcurrent, MaxNodeScanConcurrent)))
	}

	return allErrs
}

// ValidateClusterScanConcurrent validates concurrent value for ClusterScan
func ValidateClusterScanConcurrent(concurrent int32, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if concurrent != 0 && (concurrent < MinConcurrent || concurrent > MaxConcurrent) {
		allErrs = append(allErrs, field.Invalid(fldPath, concurrent,
			fmt.Sprintf("must be between %d and %d", MinConcurrent, MaxConcurrent)))
	}

	return allErrs
}

// ValidateFileTimeout validates file timeout
func ValidateFileTimeout(timeout int64, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if timeout != 0 && (timeout < MinFileTimeout || timeout > MaxFileTimeout) {
		allErrs = append(allErrs, field.Invalid(fldPath, timeout,
			fmt.Sprintf("must be between %d and %d milliseconds", MinFileTimeout, MaxFileTimeout)))
	}

	return allErrs
}

// ValidateMaxFileSize validates max file size
func ValidateMaxFileSize(size int64, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if size != 0 && (size < MinMaxFileSize || size > MaxMaxFileSize) {
		allErrs = append(allErrs, field.Invalid(fldPath, size,
			fmt.Sprintf("must be between %d and %d bytes", MinMaxFileSize, MaxMaxFileSize)))
	}

	return allErrs
}

// isValidDNS1123Name checks if a string is a valid DNS-1123 subdomain name
func isValidDNS1123Name(name string) bool {
	if len(name) == 0 || len(name) > 253 {
		return false
	}

	// Must start and end with alphanumeric
	dns1123Regex := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
	return dns1123Regex.MatchString(name)
}

// validateResources validates resource requirements
func validateResources(resources *corev1.ResourceRequirements, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if resources == nil {
		return allErrs
	}

	// Validate that limits >= requests for CPU
	if cpuRequest, ok := resources.Requests[corev1.ResourceCPU]; ok {
		if cpuLimit, ok := resources.Limits[corev1.ResourceCPU]; ok {
			if cpuLimit.Cmp(cpuRequest) < 0 {
				allErrs = append(allErrs, field.Invalid(
					fldPath.Child("limits").Child("cpu"),
					cpuLimit.String(),
					"CPU limit must be greater than or equal to CPU request"))
			}
		}
	}

	// Validate that limits >= requests for memory
	if memRequest, ok := resources.Requests[corev1.ResourceMemory]; ok {
		if memLimit, ok := resources.Limits[corev1.ResourceMemory]; ok {
			if memLimit.Cmp(memRequest) < 0 {
				allErrs = append(allErrs, field.Invalid(
					fldPath.Child("limits").Child("memory"),
					memLimit.String(),
					"memory limit must be greater than or equal to memory request"))
			}
		}
	}

	// Validate reasonable memory limits (at least 64Mi for scanner)
	if memLimit, ok := resources.Limits[corev1.ResourceMemory]; ok {
		minMem := int64(64 * 1024 * 1024) // 64Mi
		if memLimit.Value() < minMem {
			allErrs = append(allErrs, field.Invalid(
				fldPath.Child("limits").Child("memory"),
				memLimit.String(),
				"memory limit must be at least 64Mi for scanner to function"))
		}
	}

	return allErrs
}
