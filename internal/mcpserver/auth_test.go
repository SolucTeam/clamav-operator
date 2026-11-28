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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestParseAuthMode(t *testing.T) {
	m, err := ParseAuthMode("tokenreview")
	require.NoError(t, err)
	assert.Equal(t, AuthTokenReview, m)

	m, err = ParseAuthMode("none")
	require.NoError(t, err)
	assert.Equal(t, AuthNone, m)

	_, err = ParseAuthMode("basic")
	require.Error(t, err)
}

func TestParseBearer(t *testing.T) {
	tok, ok := parseBearer("Bearer abc123")
	assert.True(t, ok)
	assert.Equal(t, "abc123", tok)

	// Case-insensitive scheme, per RFC 6750 practice.
	tok, ok = parseBearer("bearer abc123")
	assert.True(t, ok)
	assert.Equal(t, "abc123", tok)

	_, ok = parseBearer("")
	assert.False(t, ok)
	_, ok = parseBearer("abc123") // no scheme
	assert.False(t, ok)
	_, ok = parseBearer("Basic abc123")
	assert.False(t, ok)
	_, ok = parseBearer("Bearer ") // empty token
	assert.False(t, ok)
	_, ok = parseBearer("Bearer a b") // whitespace in token
	assert.False(t, ok)
}

func TestAuthenticate_NoneMode(t *testing.T) {
	ambient := fake.NewClientBuilder().WithScheme(Scheme()).Build()
	a := NewAuthenticator(AuthNone, ambient, &rest.Config{})

	ar, err := a.Authenticate(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, "anonymous", ar.Identity)
	// The ambient client is shared as-is.
	assert.Equal(t, ambient, ar.Client)
}

func TestAuthenticate_TokenReview_Rejects(t *testing.T) {
	ambient := fake.NewClientBuilder().WithScheme(Scheme()).Build()
	a := NewAuthenticator(AuthTokenReview, ambient, &rest.Config{})

	// No header -> rejected without hitting the reviewer.
	_, err := a.Authenticate(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing or malformed")

	// Reviewer reports an unauthenticated token.
	a.createTokenReview = func(ctx context.Context, token string) (*authenticationv1.TokenReview, error) {
		return &authenticationv1.TokenReview{Status: authenticationv1.TokenReviewStatus{Authenticated: false}}, nil
	}
	_, err = a.Authenticate(context.Background(), "Bearer bad-token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unauthenticated")

	// Reviewer error (e.g. forbidden -> missing auth-delegator) is propagated.
	a.createTokenReview = func(ctx context.Context, token string) (*authenticationv1.TokenReview, error) {
		return nil, assert.AnError
	}
	_, err = a.Authenticate(context.Background(), "Bearer whatever")
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestAuthenticate_TokenReview_Success(t *testing.T) {
	ambient := fake.NewClientBuilder().WithScheme(Scheme()).Build()
	cfg := &rest.Config{Host: "https://api.example.com", BearerToken: "pod-token"}
	a := NewAuthenticator(AuthTokenReview, ambient, cfg)

	var receivedToken string
	a.createTokenReview = func(ctx context.Context, token string) (*authenticationv1.TokenReview, error) {
		receivedToken = token
		return &authenticationv1.TokenReview{
			Status: authenticationv1.TokenReviewStatus{
				Authenticated: true,
				User:          authenticationv1.UserInfo{Username: "system:serviceaccount:security:alice"},
			},
		}, nil
	}

	ar, err := a.Authenticate(context.Background(), "Bearer user-token-xyz")
	require.NoError(t, err)
	assert.Equal(t, "user-token-xyz", receivedToken)
	assert.Equal(t, "system:serviceaccount:security:alice", ar.Identity)
	require.NotNil(t, ar.Client)

	// The per-caller config must carry the caller's token, never the
	// pod's, and must have credential files/providers cleared.
	// (Indirect check: the client exists and the original config is not
	// mutated.)
	assert.Equal(t, "pod-token", cfg.BearerToken, "ambient config must not be mutated")
}
