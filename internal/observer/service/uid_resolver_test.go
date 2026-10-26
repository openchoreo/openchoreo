// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/config"
)

// uidResponse is a helper to build the JSON body returned by openchoreo-api.
func uidResponse(uid string) string {
	body, _ := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{"uid": uid},
	})
	return string(body)
}

// newTestResolver builds a ResourceUIDResolver whose HTTP client targets the
// given test server and uses the supplied config.
func newTestResolver(t *testing.T, apiServer *httptest.Server, tokenServer *httptest.Server, cfg *config.UIDResolverConfig) *ResourceUIDResolver {
	t.Helper()
	if cfg == nil {
		cfg = &config.UIDResolverConfig{}
	}
	cfg.OpenChoreoAPIURL = apiServer.URL
	if tokenServer != nil {
		cfg.OAuthTokenURL = tokenServer.URL + "/token"
		cfg.OAuthClientID = "test-client"
		cfg.OAuthClientSecret = "test-secret"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	r := NewResourceUIDResolver(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return r
}

// tokenServer creates an httptest.Server that always returns a valid token response.
func newAlwaysOKTokenServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"valid-token","expires_in":3600}`))
	}))
}

// newBadTokenServer creates an httptest.Server that always returns 401 for token requests.
func newBadTokenServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
	}))
}

// TestFetchResourceUID_HappyPath verifies that a 200 response is parsed correctly
// and the token is reused on subsequent calls (fetched only once).
func TestFetchResourceUID_HappyPath(t *testing.T) {
	t.Parallel()

	var tokenFetches int32

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tokenFetches, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-1","expires_in":3600}`))
	}))
	defer tokenSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(uidResponse("uid-abc")))
	}))
	defer apiSrv.Close()

	resolver := newTestResolver(t, apiSrv, tokenSrv, &config.UIDResolverConfig{MaxAuthRetry: 1})

	uid, err := resolver.GetNamespaceUID(context.Background(), "my-ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uid != "uid-abc" {
		t.Fatalf("expected uid-abc, got %q", uid)
	}

	// Second call: token must still be cached — no additional fetch.
	uid2, err := resolver.GetNamespaceUID(context.Background(), "my-ns")
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if uid2 != "uid-abc" {
		t.Fatalf("expected uid-abc on second call, got %q", uid2)
	}

	if fetches := atomic.LoadInt32(&tokenFetches); fetches != 1 {
		t.Errorf("expected token fetched once, but got %d fetch(es)", fetches)
	}
}

// TestFetchResourceUID_404ReturnsNotFound verifies that a 404 from openchoreo-api
// is mapped to ErrResourceNotFound and does NOT trigger a retry.
func TestFetchResourceUID_404ReturnsNotFound(t *testing.T) {
	t.Parallel()

	var apiCalls int32

	tokenSrv := newAlwaysOKTokenServer(t)
	defer tokenSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&apiCalls, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer apiSrv.Close()

	resolver := newTestResolver(t, apiSrv, tokenSrv, &config.UIDResolverConfig{MaxAuthRetry: 2})

	_, err := resolver.GetNamespaceUID(context.Background(), "missing")
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("expected ErrResourceNotFound, got %v", err)
	}

	// 404 must not be retried — exactly one API call.
	if n := atomic.LoadInt32(&apiCalls); n != 1 {
		t.Errorf("expected exactly 1 API call for 404, got %d", n)
	}
}

// TestFetchResourceUID_401RefreshRecovery verifies the primary fix: when openchoreo-api
// returns 401 on the first attempt, the cached token is cleared and a new token is
// fetched, and the retry succeeds.
func TestFetchResourceUID_401RefreshRecovery(t *testing.T) {
	t.Parallel()

	var tokenFetches int32
	var apiCalls int32

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tokenFetches, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"refreshed-token","expires_in":3600}`))
	}))
	defer tokenSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&apiCalls, 1)
		if n == 1 {
			// Simulate expired / invalidated token on the first request.
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Second request succeeds.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(uidResponse("uid-xyz")))
	}))
	defer apiSrv.Close()

	resolver := newTestResolver(t, apiSrv, tokenSrv, &config.UIDResolverConfig{MaxAuthRetry: 1})

	uid, err := resolver.GetNamespaceUID(context.Background(), "my-ns")
	if err != nil {
		t.Fatalf("expected successful recovery, got error: %v", err)
	}
	if uid != "uid-xyz" {
		t.Fatalf("expected uid-xyz, got %q", uid)
	}

	if n := atomic.LoadInt32(&apiCalls); n != 2 {
		t.Errorf("expected 2 API calls (1 initial + 1 retry), got %d", n)
	}

	// After a 401 the cached token is cleared, so a fresh token is fetched.
	// Two token fetches: one for the initial (cached) + one after cache invalidation.
	if n := atomic.LoadInt32(&tokenFetches); n != 2 {
		t.Errorf("expected 2 token fetches, got %d", n)
	}
}

// TestFetchResourceUID_401AfterAllRetries verifies that ErrScopeAuthFailed is returned
// when all configured retries still receive 401.
func TestFetchResourceUID_401AfterAllRetries(t *testing.T) {
	t.Parallel()

	var apiCalls int32

	tokenSrv := newAlwaysOKTokenServer(t)
	defer tokenSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&apiCalls, 1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer apiSrv.Close()

	const maxRetry = 2
	resolver := newTestResolver(t, apiSrv, tokenSrv, &config.UIDResolverConfig{MaxAuthRetry: maxRetry})

	_, err := resolver.GetNamespaceUID(context.Background(), "my-ns")
	if !errors.Is(err, ErrScopeAuthFailed) {
		t.Fatalf("expected ErrScopeAuthFailed, got %v", err)
	}

	// Should have attempted maxRetry+1 times total.
	expected := int32(maxRetry + 1)
	if n := atomic.LoadInt32(&apiCalls); n != expected {
		t.Errorf("expected %d API calls, got %d", expected, n)
	}
}

// TestFetchResourceUID_MaxAuthRetryZero verifies that when MaxAuthRetry is 0
// there is exactly one attempt and 401 still leads to token cache invalidation.
func TestFetchResourceUID_MaxAuthRetryZero(t *testing.T) {
	t.Parallel()

	var apiCalls int32

	tokenSrv := newAlwaysOKTokenServer(t)
	defer tokenSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&apiCalls, 1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer apiSrv.Close()

	resolver := newTestResolver(t, apiSrv, tokenSrv, &config.UIDResolverConfig{MaxAuthRetry: 0})

	_, err := resolver.GetNamespaceUID(context.Background(), "my-ns")
	if !errors.Is(err, ErrScopeAuthFailed) {
		t.Fatalf("expected ErrScopeAuthFailed, got %v", err)
	}

	// Zero retries → exactly one attempt.
	if n := atomic.LoadInt32(&apiCalls); n != 1 {
		t.Errorf("expected 1 API call with MaxAuthRetry=0, got %d", n)
	}

	// The cached token must have been cleared — verify by checking tokenEntry is nil.
	resolver.tokenMu.RLock()
	entry := resolver.tokenEntry
	resolver.tokenMu.RUnlock()
	if entry != nil {
		t.Error("expected tokenEntry to be nil after 401 with MaxAuthRetry=0")
	}
}

// TestFetchResourceUID_BadClientCredentials verifies that when the token endpoint
// itself rejects the client credentials, ErrScopeAuthFailed is returned (not a
// plain "failed to get access token" error).
func TestFetchResourceUID_BadClientCredentials(t *testing.T) {
	t.Parallel()

	badTokenSrv := newBadTokenServer(t)
	defer badTokenSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should never be reached because the token fetch will fail first.
		t.Error("API server should not be called when token fetch fails")
		w.WriteHeader(http.StatusOK)
	}))
	defer apiSrv.Close()

	resolver := newTestResolver(t, apiSrv, badTokenSrv, &config.UIDResolverConfig{MaxAuthRetry: 1})

	_, err := resolver.GetNamespaceUID(context.Background(), "my-ns")
	if !errors.Is(err, ErrScopeAuthFailed) {
		t.Fatalf("expected ErrScopeAuthFailed for bad client credentials, got %v", err)
	}
}

// TestFetchAccessToken_IncludesScope verifies that when OAuthScope is configured,
// the scope parameter is included in the token request body.
func TestFetchAccessToken_IncludesScope(t *testing.T) {
	t.Parallel()

	var capturedScope string

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		params, _ := url.ParseQuery(string(body))
		capturedScope = params.Get("scope")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
	}))
	defer tokenSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(uidResponse("uid-1")))
	}))
	defer apiSrv.Close()

	cfg := &config.UIDResolverConfig{OAuthScope: "api:read", MaxAuthRetry: 0}
	resolver := newTestResolver(t, apiSrv, tokenSrv, cfg)

	_, err := resolver.GetNamespaceUID(context.Background(), "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedScope != "api:read" {
		t.Errorf("expected scope %q in token request, got %q", "api:read", capturedScope)
	}
}

// TestFetchAccessToken_OmitsScopeWhenEmpty verifies that when OAuthScope is empty,
// the scope parameter is not included in the token request body.
func TestFetchAccessToken_OmitsScopeWhenEmpty(t *testing.T) {
	t.Parallel()

	var capturedScope string

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		params, _ := url.ParseQuery(string(body))
		capturedScope = params.Get("scope")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
	}))
	defer tokenSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(uidResponse("uid-1")))
	}))
	defer apiSrv.Close()

	cfg := &config.UIDResolverConfig{OAuthScope: "", MaxAuthRetry: 0}
	resolver := newTestResolver(t, apiSrv, tokenSrv, cfg)

	_, err := resolver.GetNamespaceUID(context.Background(), "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedScope != "" {
		t.Errorf("expected no scope in token request, got %q", capturedScope)
	}
}

// TestFetchResourceUID_BadClientCredentialsAfterInvalidation verifies that when a
// 401 from openchoreo-api triggers a cache clear and then the subsequent token fetch
// also fails (bad credentials), ErrScopeAuthFailed is returned.
func TestFetchResourceUID_BadClientCredentialsAfterInvalidation(t *testing.T) {
	t.Parallel()

	var tokenCalls int32

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&tokenCalls, 1)
		if n == 1 {
			// First fetch succeeds — returns a "valid" token.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"initial-token","expires_in":3600}`))
			return
		}
		// Subsequent fetches fail — simulates credentials becoming invalid.
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer tokenSrv.Close()

	var apiCalls int32
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&apiCalls, 1)
		// Always returns 401 — causes cache clear → retry → token fetch failure.
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer apiSrv.Close()

	resolver := newTestResolver(t, apiSrv, tokenSrv, &config.UIDResolverConfig{MaxAuthRetry: 1})

	_, err := resolver.GetNamespaceUID(context.Background(), "my-ns")
	if !errors.Is(err, ErrScopeAuthFailed) {
		t.Fatalf("expected ErrScopeAuthFailed, got %v", err)
	}

	// 1 API call then token refresh fails, so only 1 API call total.
	if n := atomic.LoadInt32(&apiCalls); n != 1 {
		t.Errorf("expected 1 API call, got %d", n)
	}
}

// TestGetComponentUIDRejectsAComponentOfAnotherProject pins the membership check.
// Components are namespace-scoped, so the name alone addresses one whatever
// project the caller names -- and the project the caller names is what the
// authorization decision was made on, a project-scoped grant matching every
// component path beneath it. Resolving a component owned elsewhere would hand
// back a UID the grant never covered.
func TestGetComponentUIDRejectsAComponentOfAnotherProject(t *testing.T) {
	tokens := newAlwaysOKTokenServer(t)
	defer tokens.Close()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// The component exists in the namespace, but project p2 owns it.
		_, _ = w.Write([]byte(`{
			"metadata": {"uid": "comp-uid-1"},
			"spec": {"owner": {"projectName": "p2"}}
		}`))
	}))
	defer api.Close()

	r := newTestResolver(t, api, tokens, nil)

	uid, err := r.GetComponentUID(context.Background(), "acme", "p2", "payments-api")
	if err != nil {
		t.Fatalf("the owning project must resolve: %v", err)
	}
	if uid != "comp-uid-1" {
		t.Fatalf("expected comp-uid-1, got %q", uid)
	}

	_, err = r.GetComponentUID(context.Background(), "acme", "p1", "payments-api")
	if err == nil {
		t.Fatal("a project that does not own the component must not resolve it")
	}
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("expected ErrResourceNotFound, got %v", err)
	}
	var notFound *NotFoundError
	if !errors.As(err, &notFound) || notFound.Error() != `component "payments-api" not found` {
		t.Fatalf("expected the component named as not found, without its owner, got %v", err)
	}
}

// newCapturingTokenServer creates an httptest.Server that records the form parameters of
// the most recent token request and always returns a valid token response.
func newCapturingTokenServer(t *testing.T, captured *url.Values) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		params, _ := url.ParseQuery(string(body))
		*captured = params
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
	}))
}

// writeAssertionFile writes a client assertion to a file in a temp dir and returns its path.
func writeAssertionFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "azure-identity-token")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("failed to write assertion file: %v", err)
	}
	return path
}

// TestFetchAccessToken_UsesClientAssertion verifies that when a client assertion file is
// configured, the token request carries the RFC 7523 assertion parameters and omits the
// client secret even though one is set.
func TestFetchAccessToken_UsesClientAssertion(t *testing.T) {
	t.Parallel()

	var captured url.Values

	tokenSrv := newCapturingTokenServer(t, &captured)
	defer tokenSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(uidResponse("uid-1")))
	}))
	defer apiSrv.Close()

	cfg := &config.UIDResolverConfig{
		OAuthClientAssertionFile: writeAssertionFile(t, "header.payload.signature\n"),
		MaxAuthRetry:             0,
	}
	resolver := newTestResolver(t, apiSrv, tokenSrv, cfg)

	if _, err := resolver.GetNamespaceUID(context.Background(), "ns"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := captured.Get("client_assertion_type"); got != clientAssertionType {
		t.Errorf("expected client_assertion_type %q, got %q", clientAssertionType, got)
	}
	if got := captured.Get("client_assertion"); got != "header.payload.signature" {
		t.Errorf("expected trimmed assertion in token request, got %q", got)
	}
	if got := captured.Get("client_secret"); got != "" {
		t.Errorf("expected no client_secret when an assertion file is configured, got %q", got)
	}
	if got := captured.Get("grant_type"); got != "client_credentials" {
		t.Errorf("expected grant_type client_credentials, got %q", got)
	}
}

// TestFetchAccessToken_RereadsClientAssertion verifies that the assertion file is read on
// every token request, so a token rotated in place by the platform is picked up.
func TestFetchAccessToken_RereadsClientAssertion(t *testing.T) {
	t.Parallel()

	var captured url.Values

	tokenSrv := newCapturingTokenServer(t, &captured)
	defer tokenSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(uidResponse("uid-1")))
	}))
	defer apiSrv.Close()

	assertionFile := writeAssertionFile(t, "first-assertion")
	cfg := &config.UIDResolverConfig{OAuthClientAssertionFile: assertionFile, MaxAuthRetry: 0}
	resolver := newTestResolver(t, apiSrv, tokenSrv, cfg)

	if _, _, err := resolver.fetchAccessToken(context.Background()); err != nil {
		t.Fatalf("unexpected error on first fetch: %v", err)
	}
	if got := captured.Get("client_assertion"); got != "first-assertion" {
		t.Fatalf("expected first-assertion, got %q", got)
	}

	// Simulate the platform rotating the projected token in place.
	if err := os.WriteFile(assertionFile, []byte("second-assertion"), 0o600); err != nil {
		t.Fatalf("failed to rotate assertion file: %v", err)
	}

	if _, _, err := resolver.fetchAccessToken(context.Background()); err != nil {
		t.Fatalf("unexpected error on second fetch: %v", err)
	}
	if got := captured.Get("client_assertion"); got != "second-assertion" {
		t.Errorf("expected rotated assertion to be re-read, got %q", got)
	}
}

// TestFetchAccessToken_ExpandsClientAssertionPath verifies that an environment variable
// reference in the assertion file path is resolved, as Azure Workload Identity exposes its
// per-pod token path only through AZURE_FEDERATED_TOKEN_FILE.
func TestFetchAccessToken_ExpandsClientAssertionPath(t *testing.T) {
	var captured url.Values

	tokenSrv := newCapturingTokenServer(t, &captured)
	defer tokenSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(uidResponse("uid-1")))
	}))
	defer apiSrv.Close()

	t.Setenv("TEST_FEDERATED_TOKEN_FILE", writeAssertionFile(t, "projected-assertion"))
	cfg := &config.UIDResolverConfig{
		OAuthClientAssertionFile: "${TEST_FEDERATED_TOKEN_FILE}",
		MaxAuthRetry:             0,
	}
	resolver := newTestResolver(t, apiSrv, tokenSrv, cfg)

	if _, _, err := resolver.fetchAccessToken(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := captured.Get("client_assertion"); got != "projected-assertion" {
		t.Errorf("expected assertion from the expanded path, got %q", got)
	}
}

// TestFetchAccessToken_ClientAssertionFileUnusable verifies that an unreadable or empty
// assertion file fails the token request instead of silently falling back to the secret.
func TestFetchAccessToken_ClientAssertionFileUnusable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		assertionFile func(t *testing.T) string
	}{
		{
			name: "missing file",
			assertionFile: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "does-not-exist")
			},
		},
		{
			name: "empty file",
			assertionFile: func(t *testing.T) string {
				return writeAssertionFile(t, "  \n")
			},
		},
		{
			name: "unset path variable",
			assertionFile: func(*testing.T) string {
				return "${OPENCHOREO_TEST_UNSET_TOKEN_FILE}"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tokenSrv := newAlwaysOKTokenServer(t)
			defer tokenSrv.Close()

			apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(uidResponse("uid-1")))
			}))
			defer apiSrv.Close()

			cfg := &config.UIDResolverConfig{
				OAuthClientAssertionFile: tt.assertionFile(t),
				MaxAuthRetry:             0,
			}
			resolver := newTestResolver(t, apiSrv, tokenSrv, cfg)

			_, err := resolver.GetNamespaceUID(context.Background(), "ns")
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !errors.Is(err, ErrScopeAuthFailed) {
				t.Errorf("expected ErrScopeAuthFailed, got %v", err)
			}
		})
	}
}

func TestResolveReleaseLineScope(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/namespaces/ns/components/hello/releaselines/next" {
			_, _ = w.Write([]byte(`{"metadata":{"name":"next","uid":"uid-next"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer apiServer.Close()
	r := newTestResolver(t, apiServer, nil, nil)
	ctx := context.Background()

	scope, err := r.ResolveReleaseLineScope(ctx, "ns", "hello", "", false)
	require.NoError(t, err)
	assert.Equal(t, ReleaseLineScope{}, scope, "no line covers the default line, as before release lines")

	scope, err = r.ResolveReleaseLineScope(ctx, "ns", "hello", "", true)
	require.NoError(t, err)
	assert.Equal(t, ReleaseLineScope{All: true}, scope, "allLines covers every line of the component")

	scope, err = r.ResolveReleaseLineScope(ctx, "ns", "", "", true)
	require.NoError(t, err)
	assert.Equal(t, ReleaseLineScope{}, scope, "a scope without a component already covers every line")

	scope, err = r.ResolveReleaseLineScope(ctx, "ns", "hello", "default", false)
	require.NoError(t, err)
	assert.Equal(t, ReleaseLineScope{}, scope)

	scope, err = r.ResolveReleaseLineScope(ctx, "ns", "hello", "next", false)
	require.NoError(t, err)
	assert.Equal(t, ReleaseLineScope{UID: "uid-next"}, scope)

	_, err = r.ResolveReleaseLineScope(ctx, "ns", "hello", "ghost", false)
	require.ErrorIs(t, err, ErrResourceNotFound)
	var notFound *NotFoundError
	require.ErrorAs(t, err, &notFound)
	assert.Equal(t, `release line "ghost" not found`, notFound.Error())
}

func TestCheckReleaseLineApplied(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces/ns/components/lined/releaselines":
			_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"default"}},{"metadata":{"name":"next"}}]}`))
		case "/api/v1/namespaces/ns/components/plain/releaselines":
			_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"default"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer apiServer.Close()
	r := newTestResolver(t, apiServer, nil, nil)
	ctx := context.Background()

	for name, tc := range map[string]struct {
		component string
		lines     ReleaseLineScope
		applied   bool
		wantErr   bool
	}{
		"default line":                         {component: "lined"},
		"one line, applied":                    {component: "lined", lines: ReleaseLineScope{UID: "uid-next"}, applied: true},
		"one line, not applied":                {component: "lined", lines: ReleaseLineScope{UID: "uid-next"}, wantErr: true},
		"every line, applied":                  {component: "lined", lines: ReleaseLineScope{All: true}, applied: true},
		"every line of a component with lines": {component: "lined", lines: ReleaseLineScope{All: true}, wantErr: true},
		"every line of a component without":    {component: "plain", lines: ReleaseLineScope{All: true}},
	} {
		t.Run(name, func(t *testing.T) {
			err := r.CheckReleaseLineApplied(ctx, "ns", tc.component, tc.lines, tc.applied, "logs adapter")
			if !tc.wantErr {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, ErrReleaseLineNotSupported)
			assert.ErrorContains(t, err, "logs adapter")
		})
	}
}
