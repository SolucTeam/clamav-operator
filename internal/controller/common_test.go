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
	"strings"
	"testing"
)

func TestSanitizeLabelValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		// wantLen checks the output is within the 63-char limit.
		wantMaxLen int
		// wantValidRFC1123 checks start/end chars are alphanumeric.
		wantValidRFC1123 bool
		// wantContains is a substring that must appear in the output (for
		// short inputs that should be returned unchanged).
		wantContains string
	}{
		{
			name:             "short string returned unchanged",
			input:            "nodescan-foo",
			wantMaxLen:       63,
			wantValidRFC1123: true,
			wantContains:     "nodescan-foo",
		},
		{
			name:             "exactly 63 chars returned unchanged",
			input:            strings.Repeat("a", 63),
			wantMaxLen:       63,
			wantValidRFC1123: true,
			wantContains:     strings.Repeat("a", 63),
		},
		{
			name: "long name truncated to ≤63 chars",
			// Simulates a real FQDN node: nodescan-default-schedule-1777324116-ip-10-3-78-130.zad-sandbox.eu-west-2.numspot.internal
			input:            "nodescan-default-schedule-1777324116-ip-10-3-78-130.zad-sandbox.eu-west-2.numspot.internal",
			wantMaxLen:       63,
			wantValidRFC1123: true,
		},
		{
			name: "truncation ending on dot must not produce trailing dot before hash",
			// Craft a string where position 52 lands on a '.'
			// "nodescan-default-schedule-1777324116-ip-10-3-78-130." is 52 chars
			input:            "nodescan-default-schedule-1777324116-ip-10-3-78-130.zad-sandbox",
			wantMaxLen:       63,
			wantValidRFC1123: true,
		},
		{
			name: "truncation ending on hyphen must not produce trailing hyphen before hash",
			// Craft a string where position 52 lands on a '-'
			input:            "nodescan-default-schedule-1777324116-ip-10-3-78-130-zad-sandbox",
			wantMaxLen:       63,
			wantValidRFC1123: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeLabelValue(tt.input)

			if len(got) > tt.wantMaxLen {
				t.Errorf("sanitizeLabelValue(%q) len = %d, want ≤ %d; got %q", tt.input, len(got), tt.wantMaxLen, got)
			}

			if tt.wantValidRFC1123 {
				if len(got) == 0 {
					t.Errorf("sanitizeLabelValue(%q) returned empty string", tt.input)
					return
				}
				first := got[0]
				last := got[len(got)-1]
				isAlNum := func(c byte) bool {
					return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
				}
				if !isAlNum(first) {
					t.Errorf("sanitizeLabelValue(%q) = %q: first char %q is not alphanumeric (RFC 1123)", tt.input, got, first)
				}
				if !isAlNum(last) {
					t.Errorf("sanitizeLabelValue(%q) = %q: last char %q is not alphanumeric (RFC 1123)", tt.input, got, last)
				}
				// No two adjacent separators (e.g. '.-' or '--')
				if strings.Contains(got, ".-") || strings.Contains(got, "-.") {
					t.Errorf("sanitizeLabelValue(%q) = %q: contains invalid separator sequence", tt.input, got)
				}
			}

			if tt.wantContains != "" && !strings.Contains(got, tt.wantContains) {
				t.Errorf("sanitizeLabelValue(%q) = %q: does not contain %q", tt.input, got, tt.wantContains)
			}
		})
	}
}
