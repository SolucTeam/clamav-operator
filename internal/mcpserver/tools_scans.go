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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// listScansIn is the input of the list_scans tool.
type listScansIn struct {
	// Kind of scan to list: "cluster" (ClusterScan, default) or "node" (NodeScan).
	// Enum: cluster, node. Validated by the handler.
	Kind string `json:"kind,omitempty" jsonschema:"scan kind: cluster (ClusterScan, default) or node (NodeScan)"`
	// Namespace to list scans in. Omit for all namespaces (unless the server was started with --namespace).
	Namespace string `json:"namespace,omitempty"`
	// Filter by phase: Pending, Running, Completed or Failed.
	Phase string `json:"phase,omitempty" jsonschema:"filter by phase: Pending, Running, Completed or Failed"`
}

// scanSummary is a compact view of a NodeScan or ClusterScan.
type scanSummary struct {
	Kind           string  `json:"kind"`
	Namespace      string  `json:"namespace"`
	Name           string  `json:"name"`
	Phase          string  `json:"phase,omitempty"`
	Node           string  `json:"node,omitempty"`           // NodeScan only
	FilesScanned   int64   `json:"filesScanned,omitempty"`   // NodeScan: total; ClusterScan: sum
	FilesInfected  int64   `json:"filesInfected,omitempty"`  // NodeScan: total; ClusterScan: sum
	TotalNodes     int32   `json:"totalNodes,omitempty"`     // ClusterScan only
	CompletedNodes int32   `json:"completedNodes,omitempty"` // ClusterScan only
	FailedNodes    int32   `json:"failedNodes,omitempty"`    // ClusterScan only
	InfectedNodes  int32   `json:"infectedNodes,omitempty"`  // ClusterScan only
	StartTime      *string `json:"startTime,omitempty"`
	CompletionTime *string `json:"completionTime,omitempty"`
	Age            string  `json:"age,omitempty"`
}

type listScansOut struct {
	Count int           `json:"count"`
	Scans []scanSummary `json:"scans"`
}

func (s *Server) listScans(ctx context.Context, req *mcp.CallToolRequest, in listScansIn) (*mcp.CallToolResult, listScansOut, error) {
	kind := in.Kind
	if kind == "" {
		kind = "cluster"
	}
	if kind != "node" && kind != "cluster" {
		return nil, listScansOut{}, fmt.Errorf("kind must be \"node\" or \"cluster\", got %q", kind)
	}
	if in.Phase != "" && !validPhase(in.Phase) {
		return nil, listScansOut{}, fmt.Errorf("phase must be one of Pending, Running, Completed, Failed (got %q)", in.Phase)
	}
	ns := s.resolveNamespace(in.Namespace)

	switch kind {
	case "node":
		var list clamavv1beta1.NodeScanList
		if err := s.client.List(ctx, &list, client.InNamespace(ns)); err != nil {
			return nil, listScansOut{}, fmt.Errorf("listing NodeScans: %w", err)
		}
		out := listScansOut{}
		for i := range list.Items {
			ns := &list.Items[i]
			if in.Phase != "" && string(ns.Status.Phase) != in.Phase {
				continue
			}
			out.Scans = append(out.Scans, nodeScanSummary(ns))
		}
		out.Count = len(out.Scans)
		return nil, out, nil
	default: // cluster
		var list clamavv1beta1.ClusterScanList
		if err := s.client.List(ctx, &list, client.InNamespace(ns)); err != nil {
			return nil, listScansOut{}, fmt.Errorf("listing ClusterScans: %w", err)
		}
		out := listScansOut{}
		for i := range list.Items {
			cs := &list.Items[i]
			if in.Phase != "" && string(cs.Status.Phase) != in.Phase {
				continue
			}
			out.Scans = append(out.Scans, clusterScanSummary(cs))
		}
		out.Count = len(out.Scans)
		return nil, out, nil
	}
}

// getScanIn is the input of the get_scan tool.
type getScanIn struct {
	// Kind of scan: "cluster" (ClusterScan, default) or "node" (NodeScan).
	Kind string `json:"kind,omitempty" jsonschema:"scan kind: cluster (ClusterScan, default) or node (NodeScan)"`
	// Namespace of the scan. Required if multi-namespace.
	Namespace string `json:"namespace,omitempty"`
	// Name of the scan resource.
	Name string `json:"name" jsonschema:"name of the scan resource"`
}

// nodeScanDetail reports per-node results of a ClusterScan.
type nodeScanDetail struct {
	Name           string  `json:"name"`
	Node           string  `json:"node"`
	Phase          string  `json:"phase"`
	FilesScanned   int64   `json:"filesScanned,omitempty"`
	FilesInfected  int64   `json:"filesInfected,omitempty"`
	StartTime      *string `json:"startTime,omitempty"`
	CompletionTime *string `json:"completionTime,omitempty"`
}

type getScanOut struct {
	Kind           string           `json:"kind"`
	Namespace      string           `json:"namespace"`
	Name           string           `json:"name"`
	Phase          string           `json:"phase,omitempty"`
	Node           string           `json:"node,omitempty"`
	ScanPolicy     string           `json:"scanPolicy,omitempty"`
	Paths          []string         `json:"paths,omitempty"`
	FilesScanned   int64            `json:"filesScanned,omitempty"`
	FilesInfected  int64            `json:"filesInfected,omitempty"`
	FilesSkipped   int64            `json:"filesSkipped,omitempty"`
	ErrorCount     int64            `json:"errorCount,omitempty"`
	DurationSec    int64            `json:"durationSeconds,omitempty"`
	StrategyUsed   string           `json:"strategyUsed,omitempty"`
	FailureReason  string           `json:"failureReason,omitempty"`
	ExitCode       int32            `json:"exitCode,omitempty"`
	ResultsPartial bool             `json:"resultsPartial,omitempty"`
	ReportPath     string           `json:"reportPath,omitempty"`
	TotalNodes     int32            `json:"totalNodes,omitempty"`
	CompletedNodes int32            `json:"completedNodes,omitempty"`
	RunningNodes   int32            `json:"runningNodes,omitempty"`
	FailedNodes    int32            `json:"failedNodes,omitempty"`
	InfectedNodes  int32            `json:"infectedNodes,omitempty"`
	NodeScans      []nodeScanDetail `json:"nodeScans,omitempty"`
	StartTime      *string          `json:"startTime,omitempty"`
	CompletionTime *string          `json:"completionTime,omitempty"`
	CreatedAt      string           `json:"createdAt,omitempty"`
}

func (s *Server) getScan(ctx context.Context, req *mcp.CallToolRequest, in getScanIn) (*mcp.CallToolResult, getScanOut, error) {
	if in.Name == "" {
		return nil, getScanOut{}, fmt.Errorf("name is required")
	}
	kind := in.Kind
	if kind == "" {
		kind = "cluster"
	}
	if kind != "node" && kind != "cluster" {
		return nil, getScanOut{}, fmt.Errorf("kind must be \"node\" or \"cluster\", got %q", kind)
	}
	ns := s.resolveNamespace(in.Namespace)

	if kind == "node" {
		var nsr clamavv1beta1.NodeScan
		if err := s.client.Get(ctx, client.ObjectKey{Namespace: ns, Name: in.Name}, &nsr); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, getScanOut{}, fmt.Errorf("NodeScan %q not found in namespace %q", in.Name, ns)
			}
			return nil, getScanOut{}, fmt.Errorf("getting NodeScan %s/%s: %w", ns, in.Name, err)
		}
		return nil, nodeScanDetailOut(&nsr), nil
	}

	var cs clamavv1beta1.ClusterScan
	if err := s.client.Get(ctx, client.ObjectKey{Namespace: ns, Name: in.Name}, &cs); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, getScanOut{}, fmt.Errorf("ClusterScan %q not found in namespace %q", in.Name, ns)
		}
		return nil, getScanOut{}, fmt.Errorf("getting ClusterScan %s/%s: %w", ns, in.Name, err)
	}
	return nil, clusterScanDetailOut(&cs), nil
}

// --- summary/detail builders (pure, unit-testable) ---

func validPhase(p string) bool {
	switch p {
	case "Pending", "Running", "Completed", "Failed":
		return true
	}
	return false
}

func nodeScanSummary(n *clamavv1beta1.NodeScan) scanSummary {
	return scanSummary{
		Kind:           "NodeScan",
		Namespace:      n.Namespace,
		Name:           n.Name,
		Phase:          string(n.Status.Phase),
		Node:           n.Spec.NodeName,
		FilesScanned:   n.Status.FilesScanned,
		FilesInfected:  n.Status.FilesInfected,
		StartTime:      timeStr(n.Status.StartTime),
		CompletionTime: timeStr(n.Status.CompletionTime),
		Age:            ageOf(n.CreationTimestamp.Time, n.Status.CompletionTime),
	}
}

func clusterScanSummary(c *clamavv1beta1.ClusterScan) scanSummary {
	return scanSummary{
		Kind:           "ClusterScan",
		Namespace:      c.Namespace,
		Name:           c.Name,
		Phase:          string(c.Status.Phase),
		FilesScanned:   c.Status.TotalFilesScanned,
		FilesInfected:  c.Status.TotalFilesInfected,
		TotalNodes:     c.Status.TotalNodes,
		CompletedNodes: c.Status.CompletedNodes,
		FailedNodes:    c.Status.FailedNodes,
		InfectedNodes:  c.Status.InfectedNodes,
		StartTime:      timeStr(c.Status.StartTime),
		CompletionTime: timeStr(c.Status.CompletionTime),
		Age:            ageOf(c.CreationTimestamp.Time, c.Status.CompletionTime),
	}
}

func nodeScanDetailOut(n *clamavv1beta1.NodeScan) getScanOut {
	o := getScanOut{
		Kind:           "NodeScan",
		Namespace:      n.Namespace,
		Name:           n.Name,
		Phase:          string(n.Status.Phase),
		Node:           n.Spec.NodeName,
		ScanPolicy:     n.Spec.ScanPolicy,
		Paths:          n.Spec.Paths,
		FilesScanned:   n.Status.FilesScanned,
		FilesInfected:  n.Status.FilesInfected,
		FilesSkipped:   n.Status.FilesSkipped,
		ErrorCount:     n.Status.ErrorCount,
		DurationSec:    n.Status.Duration,
		StrategyUsed:   string(n.Status.StrategyUsed),
		FailureReason:  n.Status.FailureReason,
		ExitCode:       n.Status.ExitCode,
		ResultsPartial: n.Status.ResultsPartial,
		ReportPath:     n.Status.ReportPath,
		StartTime:      timeStr(n.Status.StartTime),
		CompletionTime: timeStr(n.Status.CompletionTime),
	}
	if !n.CreationTimestamp.Time.IsZero() {
		o.CreatedAt = n.CreationTimestamp.UTC().Format(timeFormat)
	}
	return o
}

func clusterScanDetailOut(c *clamavv1beta1.ClusterScan) getScanOut {
	o := getScanOut{
		Kind:           "ClusterScan",
		Namespace:      c.Namespace,
		Name:           c.Name,
		Phase:          string(c.Status.Phase),
		ScanPolicy:     c.Spec.ScanPolicy,
		FilesScanned:   c.Status.TotalFilesScanned,
		FilesInfected:  c.Status.TotalFilesInfected,
		TotalNodes:     c.Status.TotalNodes,
		CompletedNodes: c.Status.CompletedNodes,
		RunningNodes:   c.Status.RunningNodes,
		FailedNodes:    c.Status.FailedNodes,
		InfectedNodes:  c.Status.InfectedNodes,
		StartTime:      timeStr(c.Status.StartTime),
		CompletionTime: timeStr(c.Status.CompletionTime),
	}
	if c.Spec.NodeScanTemplate != nil {
		o.Paths = c.Spec.NodeScanTemplate.Paths
	}
	if !c.CreationTimestamp.Time.IsZero() {
		o.CreatedAt = c.CreationTimestamp.UTC().Format(timeFormat)
	}
	for _, r := range c.Status.NodeScans {
		o.NodeScans = append(o.NodeScans, nodeScanDetail{
			Name:           r.Name,
			Node:           r.NodeName,
			Phase:          string(r.Phase),
			FilesScanned:   r.FilesScanned,
			FilesInfected:  r.FilesInfected,
			StartTime:      timeStr(r.StartTime),
			CompletionTime: timeStr(r.CompletionTime),
		})
	}
	return o
}

// auditInfo exposes list_scans counters for the audit trail.
func (o listScansOut) auditInfo() []any { return []any{"count", o.Count} }

// auditInfo exposes get_scan counters for the audit trail.
func (o getScanOut) auditInfo() []any {
	return []any{"phase", o.Phase, "filesInfected", o.FilesInfected}
}
