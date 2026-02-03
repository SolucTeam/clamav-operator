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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestValidatePaths(t *testing.T) {
	tests := []struct {
		name        string
		paths       []string
		expectError bool
		errorCount  int
	}{
		{
			name:        "valid paths",
			paths:       []string{"/var/lib", "/opt", "/home"},
			expectError: false,
		},
		{
			name:        "empty path",
			paths:       []string{"/var/lib", "", "/opt"},
			expectError: true,
			errorCount:  1,
		},
		{
			name:        "relative path",
			paths:       []string{"var/lib"},
			expectError: true,
			errorCount:  1,
		},
		{
			name:        "path with traversal",
			paths:       []string{"/var/../etc/passwd"},
			expectError: true,
			errorCount:  1,
		},
		{
			name:        "dangerous path /proc",
			paths:       []string{"/proc"},
			expectError: true,
			errorCount:  1,
		},
		{
			name:        "dangerous path /sys",
			paths:       []string{"/sys/kernel"},
			expectError: true,
			errorCount:  1,
		},
		{
			name:        "too many paths",
			paths:       make([]string, MaxPaths+1),
			expectError: true,
		},
		{
			name:        "path too long",
			paths:       []string{"/" + strings.Repeat("a", MaxPathLength)},
			expectError: true,
			errorCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Fill too many paths test case with valid paths
			if tt.name == "too many paths" {
				for i := range tt.paths {
					tt.paths[i] = "/valid/path"
				}
			}

			errs := ValidatePaths(tt.paths, field.NewPath("spec").Child("paths"))

			if tt.expectError {
				assert.NotEmpty(t, errs)
				if tt.errorCount > 0 {
					assert.GreaterOrEqual(t, len(errs), tt.errorCount)
				}
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}

func TestValidateExcludePatterns(t *testing.T) {
	tests := []struct {
		name        string
		patterns    []string
		expectError bool
	}{
		{
			name:        "valid patterns",
			patterns:    []string{"*.tmp", "*.log", "/var/lib/docker/*"},
			expectError: false,
		},
		{
			name:        "valid regex pattern",
			patterns:    []string{"^/tmp/.*\\.log$"},
			expectError: false,
		},
		{
			name:        "invalid regex pattern",
			patterns:    []string{"[invalid(regex"},
			expectError: true,
		},
		{
			name:        "empty pattern",
			patterns:    []string{"*.tmp", "", "*.log"},
			expectError: true,
		},
		{
			name:        "pattern too long",
			patterns:    []string{strings.Repeat("a", MaxExcludePatternLength+1)},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateExcludePatterns(tt.patterns, field.NewPath("spec").Child("excludePatterns"))

			if tt.expectError {
				assert.NotEmpty(t, errs)
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}

func TestValidateNodeName(t *testing.T) {
	tests := []struct {
		name        string
		nodeName    string
		expectError bool
	}{
		{
			name:        "valid simple name",
			nodeName:    "node-1",
			expectError: false,
		},
		{
			name:        "valid with dots",
			nodeName:    "node-1.cluster.local",
			expectError: false,
		},
		{
			name:        "empty name",
			nodeName:    "",
			expectError: true,
		},
		{
			name:        "invalid characters",
			nodeName:    "node_1",
			expectError: true,
		},
		{
			name:        "starts with dash",
			nodeName:    "-node",
			expectError: true,
		},
		{
			name:        "uppercase letters",
			nodeName:    "Node-1",
			expectError: true,
		},
		{
			name:        "too long",
			nodeName:    strings.Repeat("a", 254),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateNodeName(tt.nodeName, field.NewPath("spec").Child("nodeName"))

			if tt.expectError {
				assert.NotEmpty(t, errs)
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}

func TestValidatePriority(t *testing.T) {
	tests := []struct {
		name        string
		priority    string
		expectError bool
	}{
		{name: "high", priority: "high", expectError: false},
		{name: "medium", priority: "medium", expectError: false},
		{name: "low", priority: "low", expectError: false},
		{name: "empty", priority: "", expectError: false},
		{name: "invalid", priority: "critical", expectError: true},
		{name: "uppercase", priority: "HIGH", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidatePriority(tt.priority, field.NewPath("spec").Child("priority"))

			if tt.expectError {
				assert.NotEmpty(t, errs)
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}

func TestValidateNodeScanConcurrent(t *testing.T) {
	tests := []struct {
		name        string
		concurrent  int32
		expectError bool
	}{
		{name: "valid 1", concurrent: 1, expectError: false},
		{name: "valid 10", concurrent: 10, expectError: false},
		{name: "valid 20", concurrent: 20, expectError: false},
		{name: "zero (default)", concurrent: 0, expectError: false},
		{name: "negative", concurrent: -1, expectError: true},
		{name: "too high", concurrent: 21, expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateNodeScanConcurrent(tt.concurrent, field.NewPath("spec").Child("maxConcurrent"))

			if tt.expectError {
				assert.NotEmpty(t, errs)
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}

func TestValidateClusterScanConcurrent(t *testing.T) {
	tests := []struct {
		name        string
		concurrent  int32
		expectError bool
	}{
		{name: "valid 1", concurrent: 1, expectError: false},
		{name: "valid 50", concurrent: 50, expectError: false},
		{name: "zero (default)", concurrent: 0, expectError: false},
		{name: "negative", concurrent: -1, expectError: true},
		{name: "too high", concurrent: 51, expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateClusterScanConcurrent(tt.concurrent, field.NewPath("spec").Child("concurrent"))

			if tt.expectError {
				assert.NotEmpty(t, errs)
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}

func TestValidateFileTimeout(t *testing.T) {
	tests := []struct {
		name        string
		timeout     int64
		expectError bool
	}{
		{name: "valid 1 second", timeout: 1000, expectError: false},
		{name: "valid 5 minutes", timeout: 300000, expectError: false},
		{name: "valid 1 hour", timeout: 3600000, expectError: false},
		{name: "zero (default)", timeout: 0, expectError: false},
		{name: "too short", timeout: 999, expectError: true},
		{name: "too long", timeout: 3600001, expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateFileTimeout(tt.timeout, field.NewPath("spec").Child("fileTimeout"))

			if tt.expectError {
				assert.NotEmpty(t, errs)
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}

func TestValidateMaxFileSize(t *testing.T) {
	tests := []struct {
		name        string
		size        int64
		expectError bool
	}{
		{name: "valid 1KB", size: 1024, expectError: false},
		{name: "valid 100MB", size: 104857600, expectError: false},
		{name: "valid 10GB", size: 10737418240, expectError: false},
		{name: "zero (default)", size: 0, expectError: false},
		{name: "too small", size: 1023, expectError: true},
		{name: "too large", size: 10737418241, expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateMaxFileSize(tt.size, field.NewPath("spec").Child("maxFileSize"))

			if tt.expectError {
				assert.NotEmpty(t, errs)
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}

func TestValidateResources(t *testing.T) {
	tests := []struct {
		name        string
		resources   *corev1.ResourceRequirements
		expectError bool
	}{
		{
			name:        "nil resources",
			resources:   nil,
			expectError: false,
		},
		{
			name: "valid resources",
			resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
			},
			expectError: false,
		},
		{
			name: "CPU limit less than request",
			resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("500m"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("100m"),
				},
			},
			expectError: true,
		},
		{
			name: "memory limit less than request",
			resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			},
			expectError: true,
		},
		{
			name: "memory limit too small",
			resources: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("32Mi"),
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateResources(tt.resources, field.NewPath("spec").Child("resources"))

			if tt.expectError {
				assert.NotEmpty(t, errs)
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}

func TestIsValidDNS1123Name(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{name: "simple name", input: "node-1", valid: true},
		{name: "with dots", input: "node.cluster.local", valid: true},
		{name: "numbers only", input: "123", valid: true},
		{name: "empty", input: "", valid: false},
		{name: "starts with dash", input: "-node", valid: false},
		{name: "ends with dash", input: "node-", valid: false},
		{name: "uppercase", input: "Node", valid: false},
		{name: "underscore", input: "node_1", valid: false},
		{name: "space", input: "node 1", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidDNS1123Name(tt.input)
			assert.Equal(t, tt.valid, result)
		})
	}
}
