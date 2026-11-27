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

package mcpserver

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// timeFormat is the RFC3339 layout used in tool outputs.
const timeFormat = time.RFC3339

// timeStr formats a metav1.Time pointer as an RFC3339 UTC string.
func timeStr(t *metav1.Time) *string {
	if t == nil || t.Time.IsZero() {
		return nil
	}
	s := t.UTC().Format(timeFormat)
	return &s
}

// ageOf returns a human-readable age: time since creation, or the total
// duration when the object already completed.
func ageOf(creation time.Time, completion *metav1.Time) string {
	if creation.IsZero() {
		return ""
	}
	if completion != nil && !completion.Time.IsZero() {
		return humanizeDuration(completion.Time.Sub(creation))
	}
	return humanizeDuration(time.Since(creation))
}

// humanizeDuration renders a duration compactly (e.g. "2m30s", "1h5m", "3d4h").
func humanizeDuration(d time.Duration) string {
	if d < time.Minute {
		return d.Round(time.Second).String()
	}
	if d < time.Hour {
		return d.Round(time.Second).String()
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh%dm", h, m)
	}
	days := int(d.Hours()) / 24
	h := int(d.Hours()) % 24
	if h == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd%dh", days, h)
}
