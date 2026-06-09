// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/observer/config"
)

// ─────────────────────── NewClient ───────────────────────

func TestNewClient_EmptyServiceURL(t *testing.T) {
	cfg := &config.AuthzConfig{
		ServiceURL: "",
		Timeout:    10 * time.Second,
	}
	_, err := NewClient(cfg, noopLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authz service URL is required")
}

func TestNewClient_ZeroTimeout(t *testing.T) {
	cfg := &config.AuthzConfig{
		ServiceURL: "http://localhost:8080",
		Timeout:    0,
	}
	_, err := NewClient(cfg, noopLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authz timeout must be positive")
}

func TestNewClient_NegativeTimeout(t *testing.T) {
	cfg := &config.AuthzConfig{
		ServiceURL: "http://localhost:8080",
		Timeout:    -5 * time.Second,
	}
	_, err := NewClient(cfg, noopLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authz timeout must be positive")
}

func TestNewClient_Valid(t *testing.T) {
	cfg := &config.AuthzConfig{
		ServiceURL: "http://localhost:8080",
		Timeout:    30 * time.Second,
	}
	c, err := NewClient(cfg, noopLogger())
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.Equal(t, "http://localhost:8080", c.baseURL)
}

func TestNewClient_TLSInsecureSkipVerify(t *testing.T) {
	cfg := &config.AuthzConfig{
		ServiceURL:            "https://localhost:8443",
		Timeout:               30 * time.Second,
		TLSInsecureSkipVerify: true,
	}
	c, err := NewClient(cfg, noopLogger())
	require.NoError(t, err)
	require.NotNil(t, c)
	// TLS transport should be configured
	require.NotNil(t, c.httpClient.Transport, "transport should be set when TLS skip verify is enabled")
}

func TestNewClient_NoTLSSkip(t *testing.T) {
	cfg := &config.AuthzConfig{
		ServiceURL:            "https://localhost:8443",
		Timeout:               30 * time.Second,
		TLSInsecureSkipVerify: false,
	}
	c, err := NewClient(cfg, noopLogger())
	require.NoError(t, err)
	require.NotNil(t, c)
	// Default transport (nil) is used when TLS skip verify is disabled
	assert.Nil(t, c.httpClient.Transport)
}

// ─────────────────────── Evaluate ───────────────────────

func TestEvaluate_NilRequest(t *testing.T) {
	c := newTestClient(t, "http://unused")
	_, err := c.Evaluate(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "evaluate request must not be nil")
}

func TestEvaluate_ServerReturns200WithDecision(t *testing.T) {
	decisions := []authzcore.Decision{{Decision: true}}
	srv := newDecisionServer(t, decisions)
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	req := &authzcore.EvaluateRequest{
		Action:   string(ActionViewLogs),
		Resource: authzcore.Resource{Type: string(ResourceTypeComponent), ID: "api"},
	}

	decision, err := c.Evaluate(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.True(t, decision.Decision)
}

func TestEvaluate_ServerReturns200WithDeniedDecision(t *testing.T) {
	decisions := []authzcore.Decision{{Decision: false}}
	srv := newDecisionServer(t, decisions)
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	req := &authzcore.EvaluateRequest{
		Action:   string(ActionViewLogs),
		Resource: authzcore.Resource{Type: string(ResourceTypeComponent), ID: "api"},
	}

	decision, err := c.Evaluate(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.False(t, decision.Decision)
}

func TestEvaluate_ServerReturnsEmptyDecisions(t *testing.T) {
	srv := newDecisionServer(t, []authzcore.Decision{})
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	req := &authzcore.EvaluateRequest{Action: string(ActionViewLogs)}

	_, err := c.Evaluate(context.Background(), req)
	assert.ErrorIs(t, err, ErrAuthzInvalidResponse)
}

func TestEvaluate_ServerReturns401(t *testing.T) {
	srv := newStatusServer(t, http.StatusUnauthorized)
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	req := &authzcore.EvaluateRequest{Action: string(ActionViewLogs)}

	_, err := c.Evaluate(context.Background(), req)
	assert.ErrorIs(t, err, ErrAuthzUnauthorized)
}

func TestEvaluate_ServerReturns403(t *testing.T) {
	srv := newStatusServer(t, http.StatusForbidden)
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	req := &authzcore.EvaluateRequest{Action: string(ActionViewLogs)}

	_, err := c.Evaluate(context.Background(), req)
	assert.ErrorIs(t, err, ErrAuthzForbidden)
}

func TestEvaluate_ServerReturns500(t *testing.T) {
	srv := newStatusServer(t, http.StatusInternalServerError)
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	req := &authzcore.EvaluateRequest{Action: string(ActionViewLogs)}

	_, err := c.Evaluate(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestEvaluate_ServerUnreachable(t *testing.T) {
	c := newTestClient(t, "http://127.0.0.1:1") // port 1 is unreachable
	req := &authzcore.EvaluateRequest{Action: string(ActionViewLogs)}

	_, err := c.Evaluate(context.Background(), req)
	assert.ErrorIs(t, err, ErrAuthzServiceUnavailable)
}

func TestEvaluate_ServerReturnsInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-valid-json"))
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	req := &authzcore.EvaluateRequest{Action: string(ActionViewLogs)}

	_, err := c.Evaluate(context.Background(), req)
	assert.ErrorIs(t, err, ErrAuthzInvalidResponse)
}

func TestEvaluate_NoTokenInContext_NoAuthorizationHeader(t *testing.T) {
	var receivedAuthHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		decisions := []authzcore.Decision{{Decision: true}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(decisions)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	req := &authzcore.EvaluateRequest{Action: string(ActionViewLogs)}

	// No JWT token in context - Authorization header should not be sent
	_, err := c.Evaluate(context.Background(), req)
	require.NoError(t, err)
	assert.Empty(t, receivedAuthHeader, "no Authorization header should be forwarded when context has no token")
}

// ─────────────────────── BatchEvaluate ───────────────────────

func TestBatchEvaluate_NilRequest(t *testing.T) {
	c := newTestClient(t, "http://unused")
	_, err := c.BatchEvaluate(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "batch evaluate request must not be nil")
}

func TestBatchEvaluate_MatchingDecisions(t *testing.T) {
	decisions := []authzcore.Decision{{Decision: true}, {Decision: false}}
	srv := newDecisionServer(t, decisions)
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	batchReq := &authzcore.BatchEvaluateRequest{
		Requests: []authzcore.EvaluateRequest{
			{Action: string(ActionViewLogs)},
			{Action: string(ActionViewMetrics)},
		},
	}

	resp, err := c.BatchEvaluate(context.Background(), batchReq)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Decisions, 2)
	assert.True(t, resp.Decisions[0].Decision)
	assert.False(t, resp.Decisions[1].Decision)
}

func TestBatchEvaluate_MismatchedDecisionsCount(t *testing.T) {
	// Server returns fewer decisions than requested
	decisions := []authzcore.Decision{{Decision: true}}
	srv := newDecisionServer(t, decisions)
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	batchReq := &authzcore.BatchEvaluateRequest{
		Requests: []authzcore.EvaluateRequest{
			{Action: string(ActionViewLogs)},
			{Action: string(ActionViewMetrics)},
		},
	}

	_, err := c.BatchEvaluate(context.Background(), batchReq)
	assert.ErrorIs(t, err, ErrAuthzInvalidResponse)
}

func TestBatchEvaluate_ServerReturns401(t *testing.T) {
	srv := newStatusServer(t, http.StatusUnauthorized)
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	batchReq := &authzcore.BatchEvaluateRequest{
		Requests: []authzcore.EvaluateRequest{{Action: string(ActionViewLogs)}},
	}

	_, err := c.BatchEvaluate(context.Background(), batchReq)
	assert.ErrorIs(t, err, ErrAuthzUnauthorized)
}

func TestBatchEvaluate_ServerUnreachable(t *testing.T) {
	c := newTestClient(t, "http://127.0.0.1:1")
	batchReq := &authzcore.BatchEvaluateRequest{
		Requests: []authzcore.EvaluateRequest{{Action: string(ActionViewLogs)}},
	}

	_, err := c.BatchEvaluate(context.Background(), batchReq)
	assert.ErrorIs(t, err, ErrAuthzServiceUnavailable)
}

// ─────────────────────── GetSubjectProfile ───────────────────────

// ─────────────────────── evaluate() internal paths ───────────────────────

func TestEvaluate_InvalidURLCausesRequestCreationError(t *testing.T) {
	// Bypass NewClient to inject an invalid URL directly, which causes
	// http.NewRequestWithContext to fail.
	c := &Client{
		baseURL:    "://not-a-valid-url",
		httpClient: &http.Client{Timeout: 5 * time.Second},
		logger:     noopLogger(),
	}
	req := &authzcore.EvaluateRequest{Action: string(ActionViewLogs)}

	_, err := c.Evaluate(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create HTTP request")
}

func TestEvaluate_BrokenBodyReturnsInvalidResponse(t *testing.T) {
	// Server sends headers with Content-Length: 100 but writes no body,
	// causing the client to receive an incomplete response.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	req := &authzcore.EvaluateRequest{Action: string(ActionViewLogs)}

	_, err := c.Evaluate(context.Background(), req)
	require.Error(t, err)
}

func TestGetSubjectProfile_AlwaysReturnsError(t *testing.T) {
	c := newTestClient(t, "http://unused")
	_, err := c.GetSubjectProfile(context.Background(), &authzcore.ProfileRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

// ─────────────────────── test helpers ───────────────────────

// newTestClient builds a Client pointing at the given base URL with a short timeout.
func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	cfg := &config.AuthzConfig{
		ServiceURL: baseURL,
		Timeout:    5 * time.Second,
	}
	c, err := NewClient(cfg, noopLogger())
	require.NoError(t, err)
	return c
}

// newDecisionServer starts a test HTTP server that responds with 200 OK and
// a JSON-encoded decisions payload.
func newDecisionServer(t *testing.T, decisions []authzcore.Decision) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, evaluatesEndpoint, r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(decisions)
	}))
}

// newStatusServer starts a test HTTP server that responds with the given status code.
func newStatusServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	}))
}
