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
	"encoding/json"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// auditLogger is the dedicated logger for MCP audit events. In stdio mode
// it writes to stderr; in HTTP mode to stdout (container log collection).
var auditLogger = logf.Log.WithName("mcp-audit")

// contextKey is the private key type used to pass the caller identity and
// the OpenCode session correlation ID through request contexts.
type contextKey int

const (
	// identityKey carries the authenticated principal of the caller.
	identityKey contextKey = iota
	// sessionKey carries the opaque MCP session correlation ID when the
	// client provides one (e.g. OpenCode's ai.opencode/sessionID _meta).
	sessionKey
)

// WithIdentity returns a context carrying the caller identity.
func WithIdentity(ctx context.Context, identity string) context.Context {
	return context.WithValue(ctx, identityKey, identity)
}

// IdentityFromContext returns the authenticated principal stored in ctx,
// or "unknown" when absent.
func IdentityFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(identityKey).(string); ok && v != "" {
		return v
	}
	return "unknown"
}

// WithSessionID returns a context carrying the client session correlation
// ID (opaque value, never used for authentication).
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionKey, sessionID)
}

// SessionIDFromContext returns the correlation ID stored in ctx, if any.
func SessionIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(sessionKey).(string); ok {
		return v
	}
	return ""
}

// auditable is implemented by tool outputs that expose summary fields for
// the audit trail (e.g. result counts).
type auditable interface {
	auditInfo() []any
}

// sessionIDFromRequest extracts the OpenCode session correlation ID from
// CallToolRequest _meta, when present. It is treated as an opaque value.
func sessionIDFromRequest(req *mcp.CallToolRequest) string {
	if req == nil || req.Params == nil || req.Params.Meta == nil {
		return ""
	}
	if v, ok := req.Params.Meta["ai.opencode/sessionID"].(string); ok {
		return v
	}
	return ""
}

// audit wraps a typed tool handler with an audit log entry. Every call is
// logged with the caller identity, the tool name, the (read-only) input
// arguments, the outcome and the duration. No secrets are logged: tokens
// and headers never reach this layer.
func audit[In, Out any](s *Server, tool string, h func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error)) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		start := time.Now()
		res, out, err := h(ctx, req, in)

		// Enrich the request context for nested logging if the SDK did
		// not already provide identity (HTTP mode stores it upstream).
		identity := IdentityFromContext(ctx)
		if s != nil && s.identity != "" && identity == "unknown" {
			identity = s.identity
		}

		fields := []any{
			"user", identity,
			"tool", tool,
			"arguments", sanitizeArguments(in),
			"durationMs", time.Since(start).Milliseconds(),
		}
		if sid := SessionIDFromContext(ctx); sid != "" {
			fields = append(fields, "sessionID", sid)
		} else if sid := sessionIDFromRequest(req); sid != "" {
			fields = append(fields, "sessionID", sid)
		}

		if err != nil {
			fields = append(fields, "result", "error", "error", err.Error())
			auditLogger.Error(err, "tool_call", fields...)
			return res, out, err
		}

		fields = append(fields, "result", "ok")
		if a, ok := any(out).(auditable); ok {
			fields = append(fields, a.auditInfo()...)
		}
		auditLogger.Info("tool_call", fields...)
		return res, out, err
	}
}

// sanitizeArguments serializes tool input for the audit trail. Tool inputs
// are plain read filters (names, namespaces, phases) and contain no
// credentials; the JSON form keeps the log compact and greppable.
func sanitizeArguments(in any) json.RawMessage {
	b, err := json.Marshal(in)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}
