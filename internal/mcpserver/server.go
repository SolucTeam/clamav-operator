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

	"github.com/modelcontextprotocol/go-sdk/mcp"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// version is overridden at build time via -ldflags.
var version = "dev"

// SetVersion sets the version reported through the MCP server identity.
// Call it before Run (main.go overrides it at build time via -ldflags).
func SetVersion(v string) { version = v }

// Server is the read-only MCP server for the ClamAV Operator.
type Server struct {
	client client.Client

	// fixedNamespace restricts every tool to a single namespace when set
	// (via the --namespace flag). Empty means all namespaces are readable.
	fixedNamespace string
}

// Run creates the MCP server, registers the read-only tools and serves them
// over the stdio transport until the context is cancelled.
func Run(ctx context.Context, c client.Client, fixedNamespace string) error {
	s := &Server{client: c, fixedNamespace: fixedNamespace}

	impl := &mcp.Implementation{Name: "clamav-operator", Version: version}
	mcpServer := mcp.NewServer(impl, nil)

	// Scans
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "list_scans",
		Description: "List antivirus scans. kind=node lists NodeScans (one per node), kind=cluster lists ClusterScan fan-outs. Optionally filter by phase (Pending, Running, Completed, Failed).",
	}, s.listScans)
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "get_scan",
		Description: "Get a single scan with its full status: phase, timing, files scanned/infected, per-node breakdown (ClusterScan) or failure reason (NodeScan).",
	}, s.getScan)

	// Infections
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "list_infections",
		Description: "List infected files found by antivirus scans, with node, path, detected viruses, size and detection time. Aggregated across NodeScans.",
	}, s.listInfections)
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "get_cluster_status",
		Description: "Operational overview of the ClamAV operator in the cluster: scans by phase, total files scanned/infected, infected nodes, active schedules.",
	}, s.getClusterStatus)

	// Schedules & policies
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "list_schedules",
		Description: "List scheduled recurring scans (ScanSchedule): cron expression, suspension state, last and next schedule times.",
	}, s.listSchedules)
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "list_policies",
		Description: "List scan policies (ScanPolicy): scanned paths, exclusion patterns, concurrency, file limits and quarantine configuration.",
	}, s.listPolicies)

	log := logf.Log.WithName("mcp")
	log.Info("starting MCP server", "version", version, "namespace", fixedNamespace, "transport", "stdio")

	if err := mcpServer.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("MCP server stopped: %w", err)
	}
	return nil
}

// resolveNamespace returns the effective namespace for a request: the
// per-request value if allowed, otherwise the fixed one, otherwise "" (all).
func (s *Server) resolveNamespace(requested string) string {
	if s.fixedNamespace != "" {
		return s.fixedNamespace
	}
	return requested
}
