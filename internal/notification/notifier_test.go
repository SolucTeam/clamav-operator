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

package notification

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

// TestRedactURLError ensures secrets carried in URL paths (Slack/Teams webhook
// tokens) never reach logs or Kubernetes Events.
func TestRedactURLError(t *testing.T) {
	// Simulate the exact shape of an http.Client failure.
	httpErr := &url.Error{
		Op:  "Post",
		URL: "https://hooks.slack.com/services/T000/B000/SECRET-TOKEN",
		Err: errors.New("context deadline exceeded"),
	}
	redacted := redactURLError(httpErr)

	msg := redacted.Error()
	if strings.Contains(msg, "SECRET-TOKEN") {
		t.Fatalf("redacted error still contains the token: %s", msg)
	}
	if !strings.Contains(msg, "hooks.slack.com") {
		t.Fatalf("redacted error lost the host (useless for debugging): %s", msg)
	}
	if !strings.Contains(msg, "context deadline exceeded") {
		t.Fatalf("redacted error lost the underlying cause: %s", msg)
	}

	// Wrapped chains must be unwrapped too.
	wrapped := fmt.Errorf("send failed: %w", httpErr)
	if strings.Contains(redactURLError(wrapped).Error(), "SECRET-TOKEN") {
		t.Fatal("wrapped url.Error must also be redacted")
	}

	// Non-URL errors pass through unchanged.
	plain := errors.New("smtp auth failed")
	if redactURLError(plain) != plain {
		t.Fatal("plain errors must pass through unchanged")
	}

	// Unparseable URL: target is redacted to a placeholder, cause preserved.
	broken := &url.Error{Op: "Post", URL: "://bad", Err: errors.New("boom")}
	if strings.Contains(redactURLError(broken).Error(), "://bad") {
		t.Fatal("unparseable URL must not appear in the redacted error")
	}
}
