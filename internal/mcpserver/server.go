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

// Package mcpserver implements a read-only MCP (Model Context Protocol)
// server that exposes the ClamAV Operator custom resources to AI agents.
//
// Two transports are supported:
//
//   - stdio: a local process launched by an MCP client (Claude Code,
//     Claude Desktop, OpenCode, ...), authenticated with the ambient
//     kubeconfig or in-cluster credentials.
//   - http: a Streamable HTTP server meant to run in-cluster as a shared
//     team service. Callers present a Bearer token which is validated via
//     the Kubernetes TokenReview API; every request is then executed with
//     a Kubernetes client impersonating that token, so each user is bound
//     by their own RBAC.
//
// The server is strictly read-only in both modes. Logging goes to stderr
// on stdio (stdout carries the MCP protocol) and to stdout on HTTP so
// container log collectors can pick it up.
package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// version is overridden at build time via -ldflags.
var version = "dev"

// SetVersion sets the version reported through the MCP server identity.
// Call it before Run or RunHTTP (main.go overrides it at build time).
func SetVersion(v string) { version = v }

// Server carries the per-instance state of one MCP server. In HTTP mode,
// one Server is built per authenticated request so that every call runs
// against a Kubernetes client bound to the caller's identity.
type Server struct {
	client client.Client

	// fixedNamespace restricts every tool to a single namespace when set
	// (via the --namespace flag). Empty means all namespaces the caller's
	// RBAC allows are readable.
	fixedNamespace string

	// identity is the authenticated principal behind this server instance
	// (from TokenReview in HTTP mode, "stdio" in stdio mode). It is used
	// only for audit logging.
	identity string
}

// newMCPServer builds an MCP server instance bound to the given Kubernetes
// client, with the six read-only tools registered.
func newMCPServer(c client.Client, identity string, fixedNamespace string) *mcp.Server {
	s := &Server{client: c, identity: identity, fixedNamespace: fixedNamespace}

	impl := &mcp.Implementation{Name: "clamav-operator", Version: version}
	mcpServer := mcp.NewServer(impl, nil)

	// Scans
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "list_scans",
		Description: "List antivirus scans. kind=node lists NodeScans (one per node), kind=cluster lists ClusterScan fan-outs. Optionally filter by phase (Pending, Running, Completed, Failed).",
	}, audit(s, "list_scans", s.listScans))
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "get_scan",
		Description: "Get a single scan with its full status: phase, timing, files scanned/infected, per-node breakdown (ClusterScan) or failure reason (NodeScan).",
	}, audit(s, "get_scan", s.getScan))

	// Infections
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "list_infections",
		Description: "List infected files found by antivirus scans, with node, path, detected viruses, size and detection time. Aggregated across NodeScans.",
	}, audit(s, "list_infections", s.listInfections))
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "get_cluster_status",
		Description: "Operational overview of the ClamAV operator in the cluster: scans by phase, total files scanned/infected, infected nodes, active schedules.",
	}, audit(s, "get_cluster_status", s.getClusterStatus))

	// Schedules & policies
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "list_schedules",
		Description: "List scheduled recurring scans (ScanSchedule): cron expression, suspension state, last and next schedule times.",
	}, audit(s, "list_schedules", s.listSchedules))
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "list_policies",
		Description: "List scan policies (ScanPolicy): scanned paths, exclusion patterns, concurrency, file limits and quarantine configuration.",
	}, audit(s, "list_policies", s.listPolicies))

	return mcpServer
}

// Run serves the MCP server over the stdio transport until the context is
// cancelled, using the ambient Kubernetes credentials.
func Run(ctx context.Context, c client.Client, fixedNamespace string) error {
	logf := log.Log.WithName("mcp")
	logf.Info("starting MCP server", "version", version, "transport", "stdio", "namespace", fixedNamespace)

	if err := newMCPServer(c, identityStdio, fixedNamespace).Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("MCP server stopped: %w", err)
	}
	return nil
}

// identityStdio is the audit identity used when running over stdio, where
// the caller is the local user behind the kubeconfig.
const identityStdio = "stdio"

// resolveNamespace returns the effective namespace for a request: the
// fixed one when the server was started with --namespace, otherwise the
// per-request value (RBAC still scopes what is actually readable).
func (s *Server) resolveNamespace(requested string) string {
	if s.fixedNamespace != "" {
		return s.fixedNamespace
	}
	return requested
}
