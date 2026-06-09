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
	"sync/atomic"
	"testing"
	"time"

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
