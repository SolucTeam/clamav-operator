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
	"context"
	"fmt"

	clamavv1alpha1 "github.com/SolucTeam/clamav-operator/api/v1alpha1"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// listPoliciesIn is the input of the list_policies tool.
type listPoliciesIn struct {
	// Namespace to list policies in. Omit for all namespaces (unless the server was started with --namespace).
	Namespace string `json:"namespace,omitempty"`
}

// policySummary is a compact view of a ScanPolicy.
type policySummary struct {
	Namespace       string   `json:"namespace"`
	Name            string   `json:"name"`
	Paths           []string `json:"paths"`
	ExcludePatterns []string `json:"excludePatterns,omitempty"`
	MaxConcurrent   int32    `json:"maxConcurrent,omitempty"`
	MaxFileSizeMB   float64  `json:"maxFileSizeMB,omitempty"`
	FileTimeoutSec  int64    `json:"fileTimeoutSeconds,omitempty"`
	Quarantine      string   `json:"quarantine,omitempty"`
	Notifications   []string `json:"notifications,omitempty"`
	UsageCount      int64    `json:"usageCount,omitempty"`
	LastUsed        *string  `json:"lastUsed,omitempty"`
}

type listPoliciesOut struct {
	Count    int             `json:"count"`
	Policies []policySummary `json:"policies"`
}

func (s *Server) listPolicies(ctx context.Context, req *mcp.CallToolRequest, in listPoliciesIn) (*mcp.CallToolResult, listPoliciesOut, error) {
	ns := s.resolveNamespace(in.Namespace)

	// ScanPolicy only exists in v1alpha1 (no v1beta1 version yet).
	var list clamavv1alpha1.ScanPolicyList
	if err := s.client.List(ctx, &list, client.InNamespace(ns)); err != nil {
		return nil, listPoliciesOut{}, fmt.Errorf("listing ScanPolicies: %w", err)
	}

	out := listPoliciesOut{}
	for i := range list.Items {
		p := &list.Items[i]
		psum := policySummary{
			Namespace:       p.Namespace,
			Name:            p.Name,
			Paths:           p.Spec.Paths,
			ExcludePatterns: p.Spec.ExcludePatterns,
			MaxConcurrent:   p.Spec.MaxConcurrent,
			MaxFileSizeMB:   float64(p.Spec.MaxFileSize) / (1024 * 1024),
			FileTimeoutSec:  p.Spec.FileTimeout / 1000,
			UsageCount:      p.Status.UsageCount,
			LastUsed:        timeStr(p.Status.LastUsed),
		}
		if q := p.Spec.Quarantine; q != nil {
			psum.Quarantine = q.Action
			if q.Action == "move" && q.QuarantineDir != "" {
				psum.Quarantine = fmt.Sprintf("%s (dir: %s)", q.Action, q.QuarantineDir)
			}
		}
		if n := p.Spec.Notifications; n != nil {
			if n.Slack != nil && n.Slack.Enabled {
				psum.Notifications = append(psum.Notifications, "slack")
			}
			if n.Email != nil && n.Email.Enabled {
				psum.Notifications = append(psum.Notifications, "email")
			}
			if n.Webhook != nil && n.Webhook.Enabled {
				psum.Notifications = append(psum.Notifications, "webhook")
			}
			if n.Teams != nil && n.Teams.Enabled {
				psum.Notifications = append(psum.Notifications, "teams")
			}
		}
		out.Policies = append(out.Policies, psum)
	}
	out.Count = len(out.Policies)
	return nil, out, nil
}
