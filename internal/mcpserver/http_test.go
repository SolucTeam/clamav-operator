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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newHTTPTestServer builds an httptest server with the same wiring as
// RunHTTP: auth middleware followed by the Streamable HTTP handler.
func newHTTPTestServer(t *testing.T, mode AuthMode, identity string) *httptest.Server {
	t.Helper()
	ambient := fake.NewClientBuilder().WithScheme(Scheme()).Build()
	auth := NewAuthenticator(mode, ambient, &rest.Config{Host: "https://api.example.com"})

	if mode == AuthTokenReview {
		auth.createTokenReview = func(ctx context.Context, token string) (*authenticationv1.TokenReview, error) {
			if token == "valid-token" {
				return &authenticationv1.TokenReview{
					Status: authenticationv1.TokenReviewStatus{
						Authenticated: true,
						User:          authenticationv1.UserInfo{Username: identity},
					},
				}, nil
			}
			return &authenticationv1.TokenReview{Status: authenticationv1.TokenReviewStatus{
				Authenticated: false,
				Error:         "invalid token",
			}}, nil
		}
	}

	handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		if srv, ok := req.Context().Value(mcpServerContextKey{}).(*mcp.Server); ok {
			return srv
		}
		return nil
	}, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handleHealth)
	mux.HandleFunc("/readyz", handleHealth)
	mux.Handle("/mcp", authMiddleware(auth, "", handler))

	return httptest.NewServer(mux)
}

func TestHTTP_HealthEndpoints(t *testing.T) {
	ts := newHTTPTestServer(t, AuthNone, "anon")
	defer ts.Close()

	for _, path := range []string{"/healthz", "/readyz"} {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+path, nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, path)
	}
}

func TestHTTP_MCP_NoAuthRequiredInNoneMode(t *testing.T) {
	ts := newHTTPTestServer(t, AuthNone, "anon")
	defer ts.Close()

	body := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/mcp", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHTTP_MCP_TokenReview(t *testing.T) {
	ts := newHTTPTestServer(t, AuthTokenReview, "system:serviceaccount:security:alice")
	defer ts.Close()

	// Missing Authorization -> 401 before any MCP processing.
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/mcp", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	// Invalid token -> 401.
	req, _ = http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/mcp", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer bad-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	// Valid token -> MCP handshake succeeds.
	req, _ = http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`))
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	assert.Contains(t, buf.String(), "clamav-operator")
}

func TestHTTP_MCP_ToolCallEndToEnd(t *testing.T) {
	// Full path: auth -> per-caller server -> audited tool call against
	// the fake client.
	ambient := fake.NewClientBuilder().WithScheme(Scheme()).WithObjects(
		testNodeScan("scan-a", "node-a", "Completed", 100, 0),
	).Build()

	auth := NewAuthenticator(AuthTokenReview, ambient, &rest.Config{})
	auth.createTokenReview = func(ctx context.Context, token string) (*authenticationv1.TokenReview, error) {
		return &authenticationv1.TokenReview{
			Status: authenticationv1.TokenReviewStatus{
				Authenticated: true,
				User:          authenticationv1.UserInfo{Username: "alice"},
			},
		}, nil
	}
	auth.buildCallerClient = func(token string) (client.Client, error) {
		return ambient, nil
	}

	handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		if srv, ok := req.Context().Value(mcpServerContextKey{}).(*mcp.Server); ok {
			return srv
		}
		return nil
	}, nil)

	mux := http.NewServeMux()
	mux.Handle("/mcp", authMiddleware(auth, "", handler))
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// 1. initialize
	resp := postJSON(t, ts.URL+"/mcp", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`, "valid-token")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	sessionID := resp.Header.Get("Mcp-Session-Id")
	resp.Body.Close()
	require.NotEmpty(t, sessionID, "stateful handler must return a session ID")

	// 2. initialized notification (same session)
	notifResp := postJSONSession(t, ts.URL+"/mcp", `{"jsonrpc":"2.0","method":"notifications/initialized"}`, "valid-token", sessionID)
	notifResp.Body.Close()

	// 3. tools/call (same session) -> audited list_scans against the fake client
	callResp := postJSONSession(t, ts.URL+"/mcp",
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_scans","arguments":{"kind":"node"}}}`,
		"valid-token", sessionID)
	defer callResp.Body.Close()
	require.Equal(t, http.StatusOK, callResp.StatusCode)

	raw := new(bytes.Buffer)
	_, _ = raw.ReadFrom(callResp.Body)
	payload := raw.String()
	if strings.Contains(callResp.Header.Get("Content-Type"), "text/event-stream") {
		// SSE framing: take the payload of the first data: line.
		for _, line := range strings.Split(payload, "\n") {
			if data, ok := strings.CutPrefix(line, "data:"); ok {
				payload = strings.TrimSpace(data)
				break
			}
		}
	}
	var rpc struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(payload), &rpc))
	require.NotEmpty(t, rpc.Result.Content)
	assert.Contains(t, rpc.Result.Content[0].Text, "scan-a")
}

func postJSON(t *testing.T, url, body, token string) *http.Response {
	return postJSONSession(t, url, body, token, "")
}

func postJSONSession(t *testing.T, url, body, token, sessionID string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	// The Streamable HTTP spec requires clients to accept both formats.
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}
