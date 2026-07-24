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
	"crypto/sha256"
	"fmt"
	"math/rand"
	"strings"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

// requeueWithJitter returns a Result that requeues after base duration plus a
// random jitter of up to 20 % of base. This avoids thundering-herd effects when
// many objects are created at the same time and would otherwise all re-reconcile
// simultaneously after each periodic interval.
func requeueWithJitter(base time.Duration) ctrl.Result {
	jitter := time.Duration(rand.Int63n(int64(base / 5))) //nolint:gosec // not security-sensitive
	return ctrl.Result{RequeueAfter: base + jitter}
}

// sanitizeLabelValue ensures s is a valid Kubernetes label value AND DNS
// subdomain name (≤ 63 chars, RFC 1123).
//
// If s already fits it is returned unchanged. Otherwise the first 52 characters
// are kept, any trailing '-' or '.' are stripped (they would produce an invalid
// label like "foo.-abc123"), and a 10-character hex suffix derived from the
// SHA-256 of the full string is appended (separated by "-").
//
//	e.g. "nodescan-...-ip-10-3-78-130." → "nodescan-...-ip-10-3-78-130-<hash>"
func sanitizeLabelValue(s string) string {
	if len(s) <= 63 {
		return s
	}
	sum := sha256.Sum256([]byte(s))
	prefix := strings.TrimRight(s[:52], "-.")
	result := fmt.Sprintf("%s-%x", prefix, sum[:5])
	if len(result) > 0 && (result[0] == '-' || result[0] == '.') {
		result = "x" + result[1:]
	}
	if len(result) > 63 {
		result = result[:63]
	}
	return result
}
