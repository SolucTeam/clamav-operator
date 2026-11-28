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
	"fmt"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// mcpServerContextKey carries the per-request *mcp.Server built after
// authentication, so the handler's getServer callback never has to
// re-authenticate or build clients mid-request.
type mcpServerContextKey struct{}

// RunHTTP serves the MCP server over Streamable HTTP, meant for in-cluster
// deployment as a shared team service.
//
// Endpoints:
//   - /mcp      the MCP Streamable HTTP endpoint
//   - /healthz  liveness probe
//   - /readyz   readiness probe
//
// Authentication is enforced before the MCP handler sees the request: on
// success a per-caller Kubernetes client is built and a dedicated MCP
// server instance (with the caller's identity for audit) is attached to
// the request context. Tokens are never logged or persisted.
func RunHTTP(ctx context.Context, addr string, mode AuthMode, ambient client.Client, ambientConfig *rest.Config, fixedNamespace string) error {
	log := logf.Log.WithName("mcp")
	log.Info("starting MCP server", "version", version, "transport", "http",
		"addr", addr, "auth", mode, "namespace", fixedNamespace)

	auth := NewAuthenticator(mode, ambient, ambientConfig)

	// getServer retrieves the per-request MCP server built by the auth
	// middleware; unauthenticated requests never reach it (the middleware
	// rejects them with 401 first).
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(req *http.Request) *mcp.Server {
			if srv, ok := req.Context().Value(mcpServerContextKey{}).(*mcp.Server); ok {
				return srv
			}
			return nil
		}, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handleHealth)
	mux.HandleFunc("/readyz", handleHealth)
	mux.Handle("/mcp", authMiddleware(auth, fixedNamespace, mcpHandler))

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      0, // SSE streams stay open; do not time out responses.
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("HTTP server failed: %w", err)
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// authMiddleware authenticates the request, builds the per-caller MCP
// server and stores it in the request context. Failed authentication
// answers 401 with a JSON body, before any MCP processing.
func authMiddleware(auth *Authenticator, fixedNamespace string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ar, err := auth.Authenticate(req.Context(), req.Header.Get("Authorization"))
		if err != nil {
			logf.Log.WithName("mcp-auth").Info("authentication rejected", "remote", req.RemoteAddr, "reason", err.Error())
			writeJSONError(w, http.StatusUnauthorized, err.Error())
			return
		}

		srv := newMCPServer(ar.Client, ar.Identity, fixedNamespace)
		ctx := WithIdentity(req.Context(), ar.Identity)
		ctx = context.WithValue(ctx, mcpServerContextKey{}, srv)
		next.ServeHTTP(w, req.WithContext(ctx))
	})
}

// handleHealth serves both /healthz and /readyz. The process is ready as
// soon as it listens; Kubernetes calls are made per request at tool time.
func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// writeJSONError writes a minimal JSON error with the given status.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Encoding through encoding/json keeps the message safe whatever it
	// contains; nothing useful to do on a marshal failure.
	payload, err := json.Marshal(map[string]string{"error": msg})
	if err != nil {
		return
	}
	_, _ = w.Write(append(payload, '\n'))
}
