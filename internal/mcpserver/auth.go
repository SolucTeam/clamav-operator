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
	"strings"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AuthMode selects how HTTP callers are authenticated.
type AuthMode string

const (
	// AuthTokenReview validates the caller's Bearer token via the
	// Kubernetes TokenReview API, then executes every Kubernetes call of
	// the request with a client bound to that token: each user is bound
	// by their own RBAC.
	AuthTokenReview AuthMode = "tokenreview"

	// AuthNone disables caller authentication: all requests share the
	// ambient credentials (the pod's ServiceAccount in-cluster). Intended
	// for local development behind a port-forward.
	AuthNone AuthMode = "none"
)

// ParseAuthMode validates a raw --auth flag value.
func ParseAuthMode(s string) (AuthMode, error) {
	switch AuthMode(s) {
	case AuthTokenReview, AuthNone:
		return AuthMode(s), nil
	default:
		return "", fmt.Errorf("invalid --auth %q (expected \"tokenreview\" or \"none\")", s)
	}
}

// AuthenticatedRequest carries the result of authenticating an HTTP
// request: the caller identity (for audit) and the Kubernetes client to
// use for every call of this request.
type AuthenticatedRequest struct {
	// Identity is the authenticated principal, e.g.
	// "system:serviceaccount:security:alice".
	Identity string

	// Client executes calls with the caller's credentials. In AuthNone
	// mode it is the ambient client.
	Client client.Client
}

// Authenticator turns HTTP Bearer tokens into per-caller clients.
type Authenticator struct {
	// mode selects the authentication strategy.
	mode AuthMode

	// ambient is the client running with the server's own credentials
	// (pod ServiceAccount in-cluster, kubeconfig locally). It is used to
	// create TokenReviews and, in AuthNone mode, to serve all requests.
	ambient client.Client

	// ambientConfig is the rest.Config behind ambient, cloned per request
	// and re-bound to the caller's token.
	ambientConfig *rest.Config

	// createTokenReview validates a token against the API server. It is a
	// field (not a direct call) so tests can inject a fake reviewer.
	createTokenReview func(ctx context.Context, token string) (*authenticationv1.TokenReview, error)

	// buildCallerClient binds the ambient config to the caller's token.
	// A field so tests can inject a fake client.
	buildCallerClient func(token string) (client.Client, error)
}

// NewAuthenticator builds an Authenticator from the ambient configuration.
func NewAuthenticator(mode AuthMode, ambient client.Client, ambientConfig *rest.Config) *Authenticator {
	a := &Authenticator{mode: mode, ambient: ambient, ambientConfig: ambientConfig}
	a.createTokenReview = a.tokenReview
	a.buildCallerClient = a.callerClient
	return a
}

// callerClient clones the ambient rest.Config and re-binds it to the
// caller's token. BearerTokenFile must be cleared so the pod's own token
// never leaks into requests.
func (a *Authenticator) callerClient(token string) (client.Client, error) {
	cfg := rest.CopyConfig(a.ambientConfig)
	cfg.BearerToken = token
	cfg.BearerTokenFile = ""
	cfg.Username = ""
	cfg.Password = ""
	cfg.ExecProvider = nil
	cfg.AuthProvider = nil

	c, err := client.New(cfg, client.Options{Scheme: Scheme()})
	if err != nil {
		return nil, fmt.Errorf("building per-caller Kubernetes client: %w", err)
	}
	return c, nil
}

// tokenReview creates a TokenReview with the server's own credentials.
// A forbidden error means the MCP ServiceAccount lacks
// system:auth-delegator — an operator misconfiguration, not a caller
// problem.
func (a *Authenticator) tokenReview(ctx context.Context, token string) (*authenticationv1.TokenReview, error) {
	cs, err := kubernetes.NewForConfig(a.ambientConfig)
	if err != nil {
		return nil, fmt.Errorf("building authentication client: %w", err)
	}
	review, err := cs.AuthenticationV1().TokenReviews().Create(ctx, &authenticationv1.TokenReview{
		Spec: authenticationv1.TokenReviewSpec{Token: token},
	}, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("token review failed (check the MCP ServiceAccount has system:auth-delegator): %w", err)
	}
	return review, nil
}

// Authenticate resolves the caller behind an HTTP request.
//
// In AuthTokenReview mode, the Authorization header must carry a Bearer
// token. The token is validated through the TokenReview API (its expiry
// and audience constraints are enforced by the API server), and the
// returned client is bound to that exact token so Kubernetes RBAC applies
// per user. Tokens are never logged or stored.
func (a *Authenticator) Authenticate(ctx context.Context, bearer string) (*AuthenticatedRequest, error) {
	if a.mode == AuthNone {
		return &AuthenticatedRequest{Identity: "anonymous", Client: a.ambient}, nil
	}

	token, ok := parseBearer(bearer)
	if !ok {
		return nil, fmt.Errorf("missing or malformed Authorization header (expected \"Bearer <token>\")")
	}

	// Validate the token with the server's own credentials.
	review, err := a.createTokenReview(ctx, token)
	if err != nil {
		return nil, err
	}
	if !review.Status.Authenticated {
		reason := review.Status.Error
		if reason == "" {
			reason = "invalid or expired token"
		}
		return nil, fmt.Errorf("unauthenticated: %s", reason)
	}

	// Re-bind the ambient config to the caller's token and build the
	// per-caller client.
	callerClient, err := a.buildCallerClient(token)
	if err != nil {
		return nil, err
	}

	return &AuthenticatedRequest{Identity: review.Status.User.Username, Client: callerClient}, nil
}

// parseBearer extracts the token from an "Authorization: Bearer <token>"
// header value.
func parseBearer(header string) (string, bool) {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" || strings.ContainsAny(token, " \t\r\n") {
		return "", false
	}
	return token, true
}
