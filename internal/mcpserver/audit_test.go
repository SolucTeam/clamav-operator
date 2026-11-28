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
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// setupAuditObserver redirects the audit logger to an in-memory buffer and
// returns a function returning the written log lines (one JSON per call).
// The logger is assigned directly to auditLogger (not through the global
// SetLogger delegate, which is effectively one-shot) so every test gets a
// fresh JSON writer.
func setupAuditObserver(t *testing.T) func() []string {
	t.Helper()
	buf := &bytes.Buffer{}
	auditLogger = zap.New(zap.UseFlagOptions(&zap.Options{DestWriter: buf})).WithName("mcp-audit")
	t.Cleanup(func() {
		auditLogger = logf.Log.WithName("mcp-audit")
	})
	return func() []string {
		var lines []string
		for _, l := range strings.Split(buf.String(), "\n") {
			if strings.TrimSpace(l) != "" {
				lines = append(lines, l)
			}
		}
		return lines
	}
}

func TestAudit_WrapsHandlerAndLogs(t *testing.T) {
	entries := setupAuditObserver(t)

	s := &Server{identity: "alice"}
	handler := audit(s, "list_scans", func(ctx context.Context, req *mcp.CallToolRequest, in listScansIn) (*mcp.CallToolResult, listScansOut, error) {
		return nil, listScansOut{Count: 3, Scans: make([]scanSummary, 3)}, nil
	})

	ctx := WithIdentity(context.Background(), "bob")
	_, out, err := handler(ctx, nil, listScansIn{Kind: "node"})
	require.NoError(t, err)
	assert.Equal(t, 3, out.Count)

	logs := entries()
	require.Len(t, logs, 1)
	entry := logs[0]
	assert.Contains(t, entry, `"tool":"list_scans"`)
	assert.Contains(t, entry, `"result":"ok"`)
	assert.Contains(t, entry, `"count":3`)
	assert.Contains(t, entry, `"user":"bob"`) // ctx identity wins over s.identity
	assert.Contains(t, entry, `"arguments"`)
	assert.Contains(t, entry, `"durationMs"`)
}

func TestAudit_IdentityFallsBackToServer(t *testing.T) {
	entries := setupAuditObserver(t)

	s := &Server{identity: identityStdio}
	handler := audit(s, "get_scan", func(ctx context.Context, req *mcp.CallToolRequest, in getScanIn) (*mcp.CallToolResult, getScanOut, error) {
		return nil, getScanOut{Name: "cs-1", Phase: "Completed"}, nil
	})

	_, _, err := handler(context.Background(), nil, getScanIn{Name: "cs-1"})
	require.NoError(t, err)

	logs := entries()
	require.Len(t, logs, 1)
	assert.Contains(t, logs[0], `"user":"stdio"`)
	assert.Contains(t, logs[0], `"phase":"Completed"`)
}

func TestAudit_LogsErrors(t *testing.T) {
	entries := setupAuditObserver(t)

	s := &Server{identity: "alice"}
	handler := audit(s, "get_scan", func(ctx context.Context, req *mcp.CallToolRequest, in getScanIn) (*mcp.CallToolResult, getScanOut, error) {
		return nil, getScanOut{}, errors.New("NodeScan missing not found")
	})

	_, _, err := handler(context.Background(), nil, getScanIn{Name: "missing"})
	require.Error(t, err)

	logs := entries()
	require.Len(t, logs, 1)
	assert.Contains(t, logs[0], `"result":"error"`)
	assert.Contains(t, logs[0], "NodeScan missing not found")
}

func TestAudit_SessionIDFromRequest(t *testing.T) {
	entries := setupAuditObserver(t)

	s := &Server{identity: "alice"}
	handler := audit(s, "list_policies", func(ctx context.Context, req *mcp.CallToolRequest, in listPoliciesIn) (*mcp.CallToolResult, listPoliciesOut, error) {
		return nil, listPoliciesOut{}, nil
	})

	req := &mcp.CallToolRequest{}
	req.Params = &mcp.CallToolParamsRaw{}
	req.Params.Meta = map[string]any{"ai.opencode/sessionID": "ses_123"}

	_, _, err := handler(context.Background(), req, listPoliciesIn{})
	require.NoError(t, err)

	logs := entries()
	require.Len(t, logs, 1)
	assert.Contains(t, logs[0], `"sessionID":"ses_123"`)
}

func TestIdentityFromContext(t *testing.T) {
	assert.Equal(t, "unknown", IdentityFromContext(context.Background()))
	assert.Equal(t, "alice", IdentityFromContext(WithIdentity(context.Background(), "alice")))
	assert.Equal(t, "unknown", IdentityFromContext(WithIdentity(context.Background(), "")))
}
