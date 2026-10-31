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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1beta1 "github.com/SolucTeam/clamav-operator/api/v1beta1"
)

// TestNodeScanConversion_RoundTripIncrementalConfig verifies that a NodeScan
// created in v1alpha1 keeps its full IncrementalConfig after a
// v1alpha1 → v1beta1 → v1alpha1 round-trip (the conversion webhook path used
// on every read/write when both versions are served).
func TestNodeScanConversion_RoundTripIncrementalConfig(t *testing.T) {
	f := true
	alpha := &NodeScan{}
	alpha.Name = "node-1"
	alpha.Namespace = "default"
	alpha.Spec = NodeScanSpec{
		NodeName: "node-1",
		Strategy: ScanStrategySmart,
		IncrementalConfig: &IncrementalScanConfig{
			Enabled:             true,
			Strategy:            ScanStrategySmart,
			BaselineInterval:    7,
			MaxAge:              168,
			MinTimeBetweenScans: 6,
			CacheExpiration:     120,
			SkipUnchangedFiles:  &f,
		},
	}

	hub := &v1beta1.NodeScan{}
	require.NoError(t, alpha.ConvertTo(hub))

	// Annotations must carry all v1alpha1-only fields.
	assert.Equal(t, "true", hub.Annotations[annEnabled])
	assert.Equal(t, "smart", hub.Annotations[annIncrementalStrategy])
	assert.Equal(t, "6", hub.Annotations[annMinTimeBetweenScans])
	assert.Equal(t, "120", hub.Annotations[annCacheExpiration])
	require.NotNil(t, hub.Spec.IncrementalConfig)
	assert.Equal(t, int32(7), hub.Spec.IncrementalConfig.FullScanInterval)

	back := &NodeScan{}
	require.NoError(t, back.ConvertFrom(hub))

	require.NotNil(t, back.Spec.IncrementalConfig)
	assert.True(t, back.Spec.IncrementalConfig.Enabled, "Enabled must survive the round-trip")
	assert.Equal(t, ScanStrategySmart, back.Spec.IncrementalConfig.Strategy)
	assert.Equal(t, int32(7), back.Spec.IncrementalConfig.BaselineInterval)
	assert.Equal(t, int32(168), back.Spec.IncrementalConfig.MaxAge)
	assert.Equal(t, int32(6), back.Spec.IncrementalConfig.MinTimeBetweenScans)
	assert.Equal(t, int32(120), back.Spec.IncrementalConfig.CacheExpiration)
	require.NotNil(t, back.Spec.IncrementalConfig.SkipUnchangedFiles)
	assert.True(t, *back.Spec.IncrementalConfig.SkipUnchangedFiles)

	// Conversion annotations must not leak onto the spoke object.
	assert.NotContains(t, back.Annotations, annEnabled)
}

// TestNodeScanConversion_NativeV1Beta1Incremental verifies the regression fixed
// by this change: an object created NATIVELY in v1beta1 (no conversion
// annotations) with an incrementalConfig and a non-full strategy must convert
// to a v1alpha1 view where Enabled=true — previously Enabled=false silently
// disabled incremental scanning in the controllers.
func TestNodeScanConversion_NativeV1Beta1Incremental(t *testing.T) {
	hub := &v1beta1.NodeScan{}
	hub.Name = "node-2"
	hub.Spec.Strategy = v1beta1.ScanStrategySmart
	hub.Spec.IncrementalConfig = &v1beta1.IncrementalScanConfig{
		FullScanInterval: 10,
		MaxFileAgeHours:  168,
	}

	alpha := &NodeScan{}
	require.NoError(t, alpha.ConvertFrom(hub))

	require.NotNil(t, alpha.Spec.IncrementalConfig)
	assert.True(t, alpha.Spec.IncrementalConfig.Enabled,
		"non-full strategy on a native v1beta1 object must derive Enabled=true")
	assert.Equal(t, ScanStrategySmart, alpha.Spec.IncrementalConfig.Strategy)
	assert.Equal(t, int32(10), alpha.Spec.IncrementalConfig.BaselineInterval)
	assert.Equal(t, int32(168), alpha.Spec.IncrementalConfig.MaxAge)
	// v1alpha1-only timers fall back to CRD defaults, not 0.
	assert.Equal(t, int32(6), alpha.Spec.IncrementalConfig.MinTimeBetweenScans)
	assert.Equal(t, int32(168), alpha.Spec.IncrementalConfig.CacheExpiration)
}

// TestNodeScanConversion_NativeV1Beta1FullStrategy: strategy=full must NOT
// enable incremental (v1beta1 semantics).
func TestNodeScanConversion_NativeV1Beta1FullStrategy(t *testing.T) {
	hub := &v1beta1.NodeScan{}
	hub.Spec.Strategy = v1beta1.ScanStrategyFull
	hub.Spec.IncrementalConfig = &v1beta1.IncrementalScanConfig{FullScanInterval: 10}

	alpha := &NodeScan{}
	require.NoError(t, alpha.ConvertFrom(hub))

	require.NotNil(t, alpha.Spec.IncrementalConfig)
	assert.False(t, alpha.Spec.IncrementalConfig.Enabled)
}

// TestNodeScanConversion_HubObjectNotMutated: ConvertFrom must not mutate the
// hub object's annotation map (aliasing regression).
func TestNodeScanConversion_HubObjectNotMutated(t *testing.T) {
	alpha := &NodeScan{}
	alpha.Spec.IncrementalConfig = &IncrementalScanConfig{Enabled: true, MinTimeBetweenScans: 3}

	hub := &v1beta1.NodeScan{}
	require.NoError(t, alpha.ConvertTo(hub))

	// Snapshot the hub annotations AFTER ConvertTo; ConvertFrom must leave
	// them exactly as-is (no aliasing: stripping must not mutate the hub map).
	hubAnnotationsSnapshot := map[string]string{}
	for k, v := range hub.Annotations {
		hubAnnotationsSnapshot[k] = v
	}
	require.Len(t, hubAnnotationsSnapshot, 4) // 4 conversion annotations

	require.NoError(t, alpha.ConvertFrom(hub))

	assert.Equal(t, hubAnnotationsSnapshot, hub.Annotations,
		"hub annotations must be untouched by ConvertFrom")
}

// TestClusterScanConversion_RoundTripIncrementalConfig covers the template path.
func TestClusterScanConversion_RoundTripIncrementalConfig(t *testing.T) {
	alpha := &ClusterScan{}
	alpha.Spec.NodeScanTemplate = &NodeScanTemplateSpec{
		Strategy: ScanStrategySmart,
		IncrementalConfig: &IncrementalScanConfig{
			Enabled:          true,
			Strategy:         ScanStrategySmart,
			BaselineInterval: 7,
		},
	}

	hub := &v1beta1.ClusterScan{}
	require.NoError(t, alpha.ConvertTo(hub))
	assert.Equal(t, "true", hub.Annotations[annEnabled])

	back := &ClusterScan{}
	require.NoError(t, back.ConvertFrom(hub))
	require.NotNil(t, back.Spec.NodeScanTemplate)
	require.NotNil(t, back.Spec.NodeScanTemplate.IncrementalConfig)
	assert.True(t, back.Spec.NodeScanTemplate.IncrementalConfig.Enabled)
	assert.Equal(t, int32(7), back.Spec.NodeScanTemplate.IncrementalConfig.BaselineInterval)
}

// TestScanScheduleConversion_RoundTripIncrementalConfig covers the inline
// ClusterScanSpec template path.
func TestScanScheduleConversion_RoundTripIncrementalConfig(t *testing.T) {
	alpha := &ScanSchedule{}
	alpha.Spec.ClusterScan.NodeScanTemplate = &NodeScanTemplateSpec{
		Strategy: ScanStrategySmart,
		IncrementalConfig: &IncrementalScanConfig{
			Enabled:             true,
			MinTimeBetweenScans: 12,
			CacheExpiration:     48,
		},
	}

	hub := &v1beta1.ScanSchedule{}
	require.NoError(t, alpha.ConvertTo(hub))
	assert.Equal(t, "12", hub.Annotations[annMinTimeBetweenScans])
	assert.Equal(t, "48", hub.Annotations[annCacheExpiration])

	back := &ScanSchedule{}
	require.NoError(t, back.ConvertFrom(hub))
	require.NotNil(t, back.Spec.ClusterScan.NodeScanTemplate)
	require.NotNil(t, back.Spec.ClusterScan.NodeScanTemplate.IncrementalConfig)
	assert.True(t, back.Spec.ClusterScan.NodeScanTemplate.IncrementalConfig.Enabled)
	assert.Equal(t, int32(12), back.Spec.ClusterScan.NodeScanTemplate.IncrementalConfig.MinTimeBetweenScans)
	assert.Equal(t, int32(48), back.Spec.ClusterScan.NodeScanTemplate.IncrementalConfig.CacheExpiration)
}
