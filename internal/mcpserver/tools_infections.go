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
	"sort"
	"strings"

	clamavv1beta1 "github.com/SolucTeam/clamav-operator/api/v1beta1"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// listInfectionsIn is the input of the list_infections tool.
type listInfectionsIn struct {
	// Namespace to search. Omit for all namespaces (unless the server was started with --namespace).
	Namespace string `json:"namespace,omitempty"`
	// Restrict to a single NodeScan name.
	ScanName string `json:"scanName,omitempty"`
	// Restrict to a single node name.
	NodeName string `json:"nodeName,omitempty"`
	// Restrict to a single virus signature name (substring match).
	Virus string `json:"virus,omitempty"`
}

// infection is an infected file found on a node.
type infection struct {
	Node       string   `json:"node"`
	Namespace  string   `json:"namespace"`
	ScanName   string   `json:"scanName"`
	Path       string   `json:"path"`
	Viruses    []string `json:"viruses"`
	SizeBytes  int64    `json:"sizeBytes,omitempty"`
	DetectedAt *string  `json:"detectedAt,omitempty"`
}

type listInfectionsOut struct {
	Count int `json:"count"`
	// Truncated is true when infected-file lists were capped in the NodeScan status (first 100 kept by the operator).
	Truncated  bool        `json:"truncated,omitempty"`
	Infections []infection `json:"infections"`
}

func (s *Server) listInfections(ctx context.Context, req *mcp.CallToolRequest, in listInfectionsIn) (*mcp.CallToolResult, listInfectionsOut, error) {
	ns := s.resolveNamespace(in.Namespace)

	var list clamavv1beta1.NodeScanList
	if err := s.client.List(ctx, &list, client.InNamespace(ns)); err != nil {
		return nil, listInfectionsOut{}, fmt.Errorf("listing NodeScans: %w", err)
	}

	out := listInfectionsOut{}
	for i := range list.Items {
		scan := &list.Items[i]
		if in.ScanName != "" && scan.Name != in.ScanName {
			continue
		}
		if in.NodeName != "" && scan.Spec.NodeName != in.NodeName {
			continue
		}
		if scan.Status.FilesInfected == 0 {
			continue
		}
		if scan.Status.InfectedFilesTruncated {
			out.Truncated = true
		}
		for _, f := range scan.Status.InfectedFiles {
			if in.Virus != "" && !matchesVirus(f.Viruses, in.Virus) {
				continue
			}
			out.Infections = append(out.Infections, infection{
				Node:       scan.Spec.NodeName,
				Namespace:  scan.Namespace,
				ScanName:   scan.Name,
				Path:       f.Path,
				Viruses:    f.Viruses,
				SizeBytes:  f.Size,
				DetectedAt: timeStr(&f.DetectedAt),
			})
		}
	}
	out.Count = len(out.Infections)

	sort.Slice(out.Infections, func(a, b int) bool {
		return out.Infections[a].Node < out.Infections[b].Node ||
			(out.Infections[a].Node == out.Infections[b].Node &&
				out.Infections[a].Path < out.Infections[b].Path)
	})
	return nil, out, nil
}

// matchesVirus reports whether any virus name contains the given substring
// (case-insensitive).
func matchesVirus(viruses []string, needle string) bool {
	if needle == "" {
		return true
	}
	for _, v := range viruses {
		if strings.Contains(strings.ToLower(v), strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

// getClusterStatusIn is the input of the get_cluster_status tool.
type getClusterStatusIn struct {
	// Namespace to inspect. Omit for all namespaces (unless the server was started with --namespace).
	Namespace string `json:"namespace,omitempty"`
}

type phaseCount struct {
	Phase string `json:"phase"`
	Count int    `json:"count"`
}

type getClusterStatusOut struct {
	NodeScansByPhase    []phaseCount `json:"nodeScansByPhase,omitempty"`
	ClusterScansByPhase []phaseCount `json:"clusterScansByPhase,omitempty"`
	TotalNodeScans      int          `json:"totalNodeScans"`
	TotalClusterScans   int          `json:"totalClusterScans"`
	TotalFilesScanned   int64        `json:"totalFilesScanned"`
	TotalFilesInfected  int64        `json:"totalFilesInfected"`
	InfectedNodes       int          `json:"infectedNodes"`
	RunningClusterScans []string     `json:"runningClusterScans,omitempty"`
	ActiveSchedules     int          `json:"activeSchedules"`
	SuspendedSchedules  int          `json:"suspendedSchedules"`
	LastScheduleTime    *string      `json:"lastScheduleTime,omitempty"`
	InfectedNodesList   []string     `json:"infectedNodesList,omitempty"`
}

func (s *Server) getClusterStatus(ctx context.Context, req *mcp.CallToolRequest, in getClusterStatusIn) (*mcp.CallToolResult, getClusterStatusOut, error) {
	ns := s.resolveNamespace(in.Namespace)
	out := getClusterStatusOut{}

	var nodeList clamavv1beta1.NodeScanList
	if err := s.client.List(ctx, &nodeList, client.InNamespace(ns)); err != nil {
		return nil, getClusterStatusOut{}, fmt.Errorf("listing NodeScans: %w", err)
	}
	phaseCounts := map[string]int{}
	infectedNodes := map[string]bool{}
	for i := range nodeList.Items {
		scan := &nodeList.Items[i]
		phase := string(scan.Status.Phase)
		phaseCounts[phase]++
		out.TotalNodeScans++
		out.TotalFilesScanned += scan.Status.FilesScanned
		out.TotalFilesInfected += scan.Status.FilesInfected
		if scan.Status.FilesInfected > 0 {
			infectedNodes[scan.Spec.NodeName] = true
		}
	}
	out.InfectedNodes = len(infectedNodes)
	out.NodeScansByPhase = toPhaseCounts(phaseCounts)

	var clusterList clamavv1beta1.ClusterScanList
	if err := s.client.List(ctx, &clusterList, client.InNamespace(ns)); err != nil {
		return nil, getClusterStatusOut{}, fmt.Errorf("listing ClusterScans: %w", err)
	}
	clusterPhaseCounts := map[string]int{}
	for i := range clusterList.Items {
		scan := &clusterList.Items[i]
		phase := string(scan.Status.Phase)
		clusterPhaseCounts[phase]++
		out.TotalClusterScans++
		if scan.Status.Phase == clamavv1beta1.ClusterScanPhaseRunning {
			out.RunningClusterScans = append(out.RunningClusterScans, scan.Name)
		}
	}
	out.ClusterScansByPhase = toPhaseCounts(clusterPhaseCounts)

	// Latest cluster-scan completion, computed on raw times.
	var latest *clamavv1beta1.ClusterScan
	for i := range clusterList.Items {
		scan := &clusterList.Items[i]
		if scan.Status.CompletionTime == nil {
			continue
		}
		if latest == nil ||
			scan.Status.CompletionTime.After(latest.Status.CompletionTime.Time) {
			latest = scan
		}
	}
	if latest != nil {
		t := latest.Status.CompletionTime.UTC().Format(timeFormat)
		out.LastScheduleTime = &t
	}

	var scheduleList clamavv1beta1.ScanScheduleList
	if err := s.client.List(ctx, &scheduleList, client.InNamespace(ns)); err != nil {
		return nil, getClusterStatusOut{}, fmt.Errorf("listing ScanSchedules: %w", err)
	}
	for i := range scheduleList.Items {
		if scheduleList.Items[i].Spec.Suspend {
			out.SuspendedSchedules++
		} else {
			out.ActiveSchedules++
		}
	}

	if len(infectedNodes) > 0 {
		out.InfectedNodesList = sortedKeys(infectedNodes)
	}
	return nil, out, nil
}

func toPhaseCounts(m map[string]int) []phaseCount {
	out := make([]phaseCount, 0, len(m))
	for phase, count := range m {
		out = append(out, phaseCount{Phase: phase, Count: count})
	}
	// Stable, deterministic order: Pending, Running, Completed, Failed, rest.
	order := map[string]int{"Pending": 0, "Running": 1, "Completed": 2, "Failed": 3, "PartiallyCompleted": 4}
	sort.Slice(out, func(a, b int) bool {
		oa, okA := order[out[a].Phase]
		ob, okB := order[out[b].Phase]
		if !okA {
			oa = 100
		}
		if !okB {
			ob = 100
		}
		if oa != ob {
			return oa < ob
		}
		return out[a].Phase < out[b].Phase
	})
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// auditInfo exposes list_infections counters for the audit trail.
func (o listInfectionsOut) auditInfo() []any { return []any{"count", o.Count} }

// auditInfo exposes get_cluster_status counters for the audit trail.
func (o getClusterStatusOut) auditInfo() []any {
	return []any{"totalFilesInfected", o.TotalFilesInfected, "infectedNodes", o.InfectedNodes}
}
