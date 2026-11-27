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

	clamavv1beta1 "github.com/SolucTeam/clamav-operator/api/v1beta1"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// listSchedulesIn is the input of the list_schedules tool.
type listSchedulesIn struct {
	// Namespace to list schedules in. Omit for all namespaces (unless the server was started with --namespace).
	Namespace string `json:"namespace,omitempty"`
}

// scheduleSummary is a compact view of a ScanSchedule.
type scheduleSummary struct {
	Namespace          string  `json:"namespace"`
	Name               string  `json:"name"`
	Cron               string  `json:"cron"`
	Suspended          bool    `json:"suspended"`
	ConcurrencyPolicy  string  `json:"concurrencyPolicy,omitempty"`
	ActiveScans        int     `json:"activeScans,omitempty"`
	LastScheduleTime   *string `json:"lastScheduleTime,omitempty"`
	LastSuccessfulTime *string `json:"lastSuccessfulTime,omitempty"`
	NextScheduleTime   *string `json:"nextScheduleTime,omitempty"`
	LastClusterScan    string  `json:"lastClusterScan,omitempty"`
	ScanPolicy         string  `json:"scanPolicy,omitempty"`
}

type listSchedulesOut struct {
	Count     int               `json:"count"`
	Schedules []scheduleSummary `json:"schedules"`
}

func (s *Server) listSchedules(ctx context.Context, req *mcp.CallToolRequest, in listSchedulesIn) (*mcp.CallToolResult, listSchedulesOut, error) {
	ns := s.resolveNamespace(in.Namespace)

	var list clamavv1beta1.ScanScheduleList
	if err := s.client.List(ctx, &list, client.InNamespace(ns)); err != nil {
		return nil, listSchedulesOut{}, fmt.Errorf("listing ScanSchedules: %w", err)
	}

	out := listSchedulesOut{}
	for i := range list.Items {
		sched := &list.Items[i]
		out.Schedules = append(out.Schedules, scheduleSummary{
			Namespace:          sched.Namespace,
			Name:               sched.Name,
			Cron:               sched.Spec.Schedule,
			Suspended:          sched.Spec.Suspend,
			ConcurrencyPolicy:  sched.Spec.ConcurrencyPolicy,
			ActiveScans:        len(sched.Status.Active),
			LastScheduleTime:   timeStr(sched.Status.LastScheduleTime),
			LastSuccessfulTime: timeStr(sched.Status.LastSuccessfulTime),
			NextScheduleTime:   timeStr(sched.Status.NextScheduleTime),
			LastClusterScan:    sched.Status.LastClusterScan,
			ScanPolicy:         sched.Spec.ClusterScan.ScanPolicy,
		})
	}
	out.Count = len(out.Schedules)
	return nil, out, nil
}
