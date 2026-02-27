// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/config"
)

// resourceResponse is the shape returned by the openchoreo-api
type resourceResponse struct {
	Data struct {
		UID string `json:"uid"`
	} `json:"data"`
}

const cachedUID = "cached-uid"

// newMockAPIServer creates an httptest.Server that responds with the given UID for any path.
func newMockAPIServer(t *testing.T, uid string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusOK {
			resp := resourceResponse{}
			resp.Data.UID = uid
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
}

// newMockTokenServer creates an httptest.Server that returns a fake OAuth2 token.
func newMockTokenServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "test-token",
			"token_type":   "bearer",
			"expires_in":   3600,
		})
	}))
}

func newTestResolver(t *testing.T, apiURL, tokenURL string) *ResourceResolver {
	t.Helper()
	cfg := &config.ResolverConfig{
		OpenChoreoAPIURL:  apiURL,
		OAuthTokenURL:     tokenURL,
		OAuthClientID:     "test-client",
		OAuthClientSecret: "test-secret",
		Timeout:           5 * time.Second,
		CacheTTL:          1 * time.Minute,
	}
	return NewResourceResolver(cfg, slog.Default())
}

// ---------------------------------------------------------------------------
// GetNamespaceUID
// ---------------------------------------------------------------------------

func TestResourceResolver_GetNamespaceUID_Empty(t *testing.T) {
	resolver := newTestResolver(t, "", "")
	uid := resolver.GetNamespaceUID("")
	if uid != "" {
		t.Errorf("expected empty string for empty namespace, got %q", uid)
	}
}

func TestResourceResolver_GetNamespaceUID_Success(t *testing.T) {
	tokenSrv := newMockTokenServer(t)
	defer tokenSrv.Close()

	apiSrv := newMockAPIServer(t, "ns-uid-123", http.StatusOK)
	defer apiSrv.Close()

	resolver := newTestResolver(t, apiSrv.URL, tokenSrv.URL)

	uid := resolver.GetNamespaceUID("my-namespace")
	if uid != "ns-uid-123" {
		t.Errorf("uid = %q, want %q", uid, "ns-uid-123")
	}
}

func TestResourceResolver_GetNamespaceUID_APIError_FallsBackToName(t *testing.T) {
	tokenSrv := newMockTokenServer(t)
	defer tokenSrv.Close()

	apiSrv := newMockAPIServer(t, "", http.StatusNotFound)
	defer apiSrv.Close()

	resolver := newTestResolver(t, apiSrv.URL, tokenSrv.URL)

	uid := resolver.GetNamespaceUID("fallback-ns")
	if uid != "fallback-ns" {
		t.Errorf("uid = %q, want %q (fallback to name)", uid, "fallback-ns")
	}
}

func TestResourceResolver_GetNamespaceUID_NoAPIURL_FallsBackToName(t *testing.T) {
	resolver := newTestResolver(t, "", "")

	uid := resolver.GetNamespaceUID("my-ns")
	if uid != "my-ns" {
		t.Errorf("uid = %q, want %q (fallback)", uid, "my-ns")
	}
}

func TestResourceResolver_GetNamespaceUID_Caching(t *testing.T) {
	callCount := 0
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		resp := resourceResponse{}
		resp.Data.UID = cachedUID
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer apiSrv.Close()

	tokenSrv := newMockTokenServer(t)
	defer tokenSrv.Close()

	resolver := newTestResolver(t, apiSrv.URL, tokenSrv.URL)

	// Call twice — second should hit cache, not API
	uid1 := resolver.GetNamespaceUID("ns")
	uid2 := resolver.GetNamespaceUID("ns")

	if uid1 != cachedUID || uid2 != cachedUID {
		t.Errorf("expected cached-uid, got %q and %q", uid1, uid2)
	}
	// Token server is called once; API server should be called once for the UID
	if callCount != 1 {
		t.Errorf("API called %d times, want 1 (second call should use cache)", callCount)
	}
}

// ---------------------------------------------------------------------------
// GetProjectUID
// ---------------------------------------------------------------------------

func TestResourceResolver_GetProjectUID_Empty(t *testing.T) {
	resolver := newTestResolver(t, "", "")
	uid := resolver.GetProjectUID("ns", "")
	if uid != "" {
		t.Errorf("expected empty string for empty project, got %q", uid)
	}
}

func TestResourceResolver_GetProjectUID_Success(t *testing.T) {
	tokenSrv := newMockTokenServer(t)
	defer tokenSrv.Close()

	apiSrv := newMockAPIServer(t, "proj-uid-456", http.StatusOK)
	defer apiSrv.Close()

	resolver := newTestResolver(t, apiSrv.URL, tokenSrv.URL)

	uid := resolver.GetProjectUID("ns", "my-project")
	if uid != "proj-uid-456" {
		t.Errorf("uid = %q, want %q", uid, "proj-uid-456")
	}
}

func TestResourceResolver_GetProjectUID_Fallback(t *testing.T) {
	resolver := newTestResolver(t, "", "")

	uid := resolver.GetProjectUID("ns", "proj")
	if uid != "proj" {
		t.Errorf("uid = %q, want %q (fallback)", uid, "proj")
	}
}

// ---------------------------------------------------------------------------
// GetComponentUID
// ---------------------------------------------------------------------------

func TestResourceResolver_GetComponentUID_Empty(t *testing.T) {
	resolver := newTestResolver(t, "", "")
	uid := resolver.GetComponentUID("ns", "proj", "")
	if uid != "" {
		t.Errorf("expected empty string for empty component, got %q", uid)
	}
}

func TestResourceResolver_GetComponentUID_Success(t *testing.T) {
	tokenSrv := newMockTokenServer(t)
	defer tokenSrv.Close()

	apiSrv := newMockAPIServer(t, "comp-uid-789", http.StatusOK)
	defer apiSrv.Close()

	resolver := newTestResolver(t, apiSrv.URL, tokenSrv.URL)

	uid := resolver.GetComponentUID("ns", "proj", "my-component")
	if uid != "comp-uid-789" {
		t.Errorf("uid = %q, want %q", uid, "comp-uid-789")
	}
}

func TestResourceResolver_GetComponentUID_Fallback(t *testing.T) {
	resolver := newTestResolver(t, "", "")

	uid := resolver.GetComponentUID("ns", "proj", "comp")
	if uid != "comp" {
		t.Errorf("uid = %q, want %q (fallback)", uid, "comp")
	}
}

// ---------------------------------------------------------------------------
// GetEnvironmentUID
// ---------------------------------------------------------------------------

func TestResourceResolver_GetEnvironmentUID_Empty(t *testing.T) {
	resolver := newTestResolver(t, "", "")
	uid := resolver.GetEnvironmentUID("ns", "")
	if uid != "" {
		t.Errorf("expected empty string for empty environment, got %q", uid)
	}
}

func TestResourceResolver_GetEnvironmentUID_Success(t *testing.T) {
	tokenSrv := newMockTokenServer(t)
	defer tokenSrv.Close()

	apiSrv := newMockAPIServer(t, "env-uid-abc", http.StatusOK)
	defer apiSrv.Close()

	resolver := newTestResolver(t, apiSrv.URL, tokenSrv.URL)

	uid := resolver.GetEnvironmentUID("ns", "dev")
	if uid != "env-uid-abc" {
		t.Errorf("uid = %q, want %q", uid, "env-uid-abc")
	}
}

func TestResourceResolver_GetEnvironmentUID_Fallback(t *testing.T) {
	resolver := newTestResolver(t, "", "")

	uid := resolver.GetEnvironmentUID("ns", "prod")
	if uid != "prod" {
		t.Errorf("uid = %q, want %q (fallback)", uid, "prod")
	}
}

// ---------------------------------------------------------------------------
// getAccessToken — OAuth disabled (no token URL)
// ---------------------------------------------------------------------------

func TestResourceResolver_GetAccessToken_NoOAuth(t *testing.T) {
	resolver := newTestResolver(t, "http://api.example.com", "")
	// OAuthTokenURL is empty → getAccessToken returns empty token, no error
	token, err := resolver.getAccessToken()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if token != "" {
		t.Errorf("expected empty token without OAuth config, got %q", token)
	}
}

// ---------------------------------------------------------------------------
// getAccessToken — token caching
// ---------------------------------------------------------------------------

func TestResourceResolver_GetAccessToken_Caching(t *testing.T) {
	callCount := 0
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "cached-token",
			"token_type":   "bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenSrv.Close()

	resolver := newTestResolver(t, "http://api.example.com", tokenSrv.URL)

	t1, err1 := resolver.getAccessToken()
	t2, err2 := resolver.getAccessToken()

	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v, %v", err1, err2)
	}
	if t1 != "cached-token" || t2 != "cached-token" {
		t.Errorf("expected cached-token, got %q and %q", t1, t2)
	}
	if callCount != 1 {
		t.Errorf("token server called %d times, want 1 (cached)", callCount)
	}
}

// ---------------------------------------------------------------------------
// fetchAccessToken — error cases
// ---------------------------------------------------------------------------

func TestResourceResolver_FetchAccessToken_ServerError(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer tokenSrv.Close()

	resolver := newTestResolver(t, "http://api.example.com", tokenSrv.URL)

	_, _, err := resolver.fetchAccessToken()
	if err == nil {
		t.Error("expected error for server error response, got nil")
	}
}

func TestResourceResolver_FetchAccessToken_EmptyToken(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "",
			"expires_in":   3600,
		})
	}))
	defer tokenSrv.Close()

	resolver := newTestResolver(t, "http://api.example.com", tokenSrv.URL)

	_, _, err := resolver.fetchAccessToken()
	if err == nil {
		t.Error("expected error for empty access token, got nil")
	}
}

// ---------------------------------------------------------------------------
// fetchResourceUID — error cases
// ---------------------------------------------------------------------------

func TestResourceResolver_FetchResourceUID_NoURL(t *testing.T) {
	resolver := newTestResolver(t, "", "")

	_, err := resolver.fetchResourceUID("/api/v1/namespaces/ns")
	if err == nil {
		t.Error("expected error when API URL is not configured")
	}
}

func TestResourceResolver_FetchResourceUID_EmptyUID(t *testing.T) {
	tokenSrv := newMockTokenServer(t)
	defer tokenSrv.Close()

	// Server returns data with empty uid
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{"uid": ""},
		})
	}))
	defer apiSrv.Close()

	resolver := newTestResolver(t, apiSrv.URL, tokenSrv.URL)

	_, err := resolver.fetchResourceUID("/api/v1/namespaces/ns")
	if err == nil {
		t.Error("expected error when uid is empty in response")
	}
}

// ---------------------------------------------------------------------------
// cache helpers
// ---------------------------------------------------------------------------

func TestResourceResolver_CacheExpiry(t *testing.T) {
	resolver := newTestResolver(t, "", "")
	// Set a very short TTL-like entry by manipulating cache directly
	key := "namespace:test"

	// Store entry with already-expired time
	resolver.cache.Store(key, cacheEntry{
		uid:       "expired-uid",
		expiresAt: time.Now().Add(-1 * time.Second),
	})

	uid := resolver.getFromCache(key)
	if uid != "" {
		t.Errorf("expected empty string for expired cache entry, got %q", uid)
	}
}

func TestResourceResolver_SetInCache(t *testing.T) {
	resolver := newTestResolver(t, "", "")
	resolver.setInCache("test-key", "test-uid")

	uid := resolver.getFromCache("test-key")
	if uid != "test-uid" {
		t.Errorf("uid = %q, want %q", uid, "test-uid")
	}
}
