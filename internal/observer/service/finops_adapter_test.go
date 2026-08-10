// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/jwt"
)

const (
	testProjectUID     = "11111111-1111-1111-1111-111111111111"
	testComponentUID   = "22222222-2222-2222-2222-222222222222"
	testEnvironmentUID = "33333333-3333-3333-3333-333333333333"
)

func newTestFinOpsLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewFinOpsAdapter(t *testing.T) {
	t.Parallel()

	resolver, closeResolver := newMockUIDResolver(testProjectUID, testComponentUID, testEnvironmentUID, nil)
	defer closeResolver()

	adapter, err := NewFinOpsAdapter("http://finops-adapter:9101", 0, resolver, newTestFinOpsLogger())
	require.NoError(t, err)
	require.NotNil(t, adapter)
}

func TestFinOpsAdapter_GetComponentCosts_Success(t *testing.T) {
	t.Parallel()

	var capturedMethod, capturedPath, capturedAuth string
	var capturedQuery url.Values
	expectedBody := map[string]any{
		"items": []map[string]any{
			{"componentUid": testComponentUID, "component": "checkout", "cpuCost": 1.25},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMethod = r.Method
		capturedPath = r.URL.Path
		capturedQuery = r.URL.Query()
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(expectedBody)
	}))
	defer server.Close()

	resolver, closeResolver := newMockUIDResolver(testProjectUID, testComponentUID, testEnvironmentUID, nil)
	defer closeResolver()

	adapter, err := NewFinOpsAdapter(server.URL, 30*time.Second, resolver, newTestFinOpsLogger())
	require.NoError(t, err)

	req := &types.CostQueryRequest{
		Namespace:   "default",
		Environment: "production",
		Project:     "gcp-microservices",
		Component:   "checkout",
		StartTime:   "2026-05-23T10:00:01Z",
		EndTime:     "2026-05-24T10:00:01Z",
		Granularity: "1d",
	}

	// Even with a token in context, the costs endpoint must NOT forward it.
	ctx := jwt.ContextWithToken(context.Background(), "caller-jwt-token")
	result, err := adapter.GetComponentCosts(ctx, req)
	require.NoError(t, err)

	// Verify the outbound request used UIDs in path + query.
	assert.Equal(t, http.MethodGet, capturedMethod)
	assert.Equal(t, "/api/v1alpha1/costs/namespaces/default/environments/"+testEnvironmentUID, capturedPath)
	assert.Equal(t, testProjectUID, capturedQuery.Get("projectUid"))
	assert.Equal(t, testComponentUID, capturedQuery.Get("componentUid"))
	assert.Equal(t, "1d", capturedQuery.Get("granularity"), "granularity passes through verbatim")
	assert.NotEmpty(t, capturedQuery.Get("startTime"))
	assert.NotEmpty(t, capturedQuery.Get("endTime"))
	assert.Empty(t, capturedAuth, "costs endpoint must not forward the caller's JWT")

	// Verify raw passthrough of the response body.
	raw, ok := result.(json.RawMessage)
	require.True(t, ok, "result should be raw JSON passthrough")
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Contains(t, got, "items")
}

func TestFinOpsAdapter_GetComponentCosts_NamespaceScopeNoUIDs(t *testing.T) {
	t.Parallel()

	var capturedQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()

	resolver, closeResolver := newMockUIDResolver(testProjectUID, testComponentUID, testEnvironmentUID, nil)
	defer closeResolver()

	adapter, err := NewFinOpsAdapter(server.URL, 30*time.Second, resolver, newTestFinOpsLogger())
	require.NoError(t, err)

	req := &types.CostQueryRequest{
		Namespace:   "default",
		Environment: "production",
		StartTime:   "2026-05-23T10:00:01Z",
		EndTime:     "2026-05-24T10:00:01Z",
	}

	_, err = adapter.GetComponentCosts(context.Background(), req)
	require.NoError(t, err)
	// Without project/component, the UID query params should be absent.
	assert.Empty(t, capturedQuery.Get("projectUid"))
	assert.Empty(t, capturedQuery.Get("componentUid"))
	assert.Empty(t, capturedQuery.Get("granularity"))
}

func TestFinOpsAdapter_GetRecommendations_ForwardsNamesAndUIDs(t *testing.T) {
	t.Parallel()

	var capturedPath string
	var capturedQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.Query()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()

	resolver, closeResolver := newMockUIDResolver(testProjectUID, testComponentUID, testEnvironmentUID, nil)
	defer closeResolver()

	adapter, err := NewFinOpsAdapter(server.URL, 30*time.Second, resolver, newTestFinOpsLogger())
	require.NoError(t, err)

	req := &types.RecommendationQueryRequest{
		Namespace:   "default",
		Environment: "production",
		Project:     "gcp-microservices",
		Component:   "checkout",
		StartTime:   "2026-05-23T10:00:01Z",
		EndTime:     "2026-05-24T10:00:01Z",
	}

	_, err = adapter.GetRecommendations(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t,
		"/api/v1alpha1/costs/namespaces/default/environments/"+testEnvironmentUID+"/recommendations",
		capturedPath)
	// UIDs
	assert.Equal(t, testProjectUID, capturedQuery.Get("projectUid"))
	assert.Equal(t, testComponentUID, capturedQuery.Get("componentUid"))
	// Names forwarded alongside UIDs (adapter needs them for its name-based metrics call).
	assert.Equal(t, "production", capturedQuery.Get("environment"))
	assert.Equal(t, "gcp-microservices", capturedQuery.Get("project"))
	assert.Equal(t, "checkout", capturedQuery.Get("component"))
}

func TestFinOpsAdapter_ForwardsBearerToken(t *testing.T) {
	t.Parallel()

	var capturedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer server.Close()

	resolver, closeResolver := newMockUIDResolver(testProjectUID, testComponentUID, testEnvironmentUID, nil)
	defer closeResolver()

	adapter, err := NewFinOpsAdapter(server.URL, 30*time.Second, resolver, newTestFinOpsLogger())
	require.NoError(t, err)

	// The caller's JWT is carried in the context (as the JWT middleware sets it).
	ctx := jwt.ContextWithToken(context.Background(), "caller-jwt-token")
	req := &types.RecommendationQueryRequest{
		Namespace:   "default",
		Environment: "production",
		StartTime:   "2026-05-23T10:00:01Z",
		EndTime:     "2026-05-24T10:00:01Z",
	}

	_, err = adapter.GetRecommendations(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, "Bearer caller-jwt-token", capturedAuth)
}

func TestFinOpsAdapter_NilRequest(t *testing.T) {
	t.Parallel()

	resolver, closeResolver := newMockUIDResolver(testProjectUID, testComponentUID, testEnvironmentUID, nil)
	defer closeResolver()
	adapter, err := NewFinOpsAdapter("http://finops-adapter:9101", 30*time.Second, resolver, newTestFinOpsLogger())
	require.NoError(t, err)

	_, err = adapter.GetComponentCosts(context.Background(), nil)
	assert.ErrorIs(t, err, ErrFinOpsInvalidRequest)

	_, err = adapter.GetRecommendations(context.Background(), nil)
	assert.ErrorIs(t, err, ErrFinOpsInvalidRequest)
}

func TestFinOpsAdapter_InvalidTimes(t *testing.T) {
	t.Parallel()

	resolver, closeResolver := newMockUIDResolver(testProjectUID, testComponentUID, testEnvironmentUID, nil)
	defer closeResolver()
	adapter, err := NewFinOpsAdapter("http://finops-adapter:9101", 30*time.Second, resolver, newTestFinOpsLogger())
	require.NoError(t, err)

	req := &types.CostQueryRequest{
		Namespace:   "default",
		Environment: "production",
		StartTime:   "not-a-time",
		EndTime:     "2026-05-24T10:00:01Z",
	}
	_, err = adapter.GetComponentCosts(context.Background(), req)
	assert.ErrorIs(t, err, ErrFinOpsInvalidRequest)
}

func TestFinOpsAdapter_ResolverFailure(t *testing.T) {
	t.Parallel()

	// Resolver returns 404 for environment lookups.
	resolver, closeResolver := newMockUIDResolver(testProjectUID, testComponentUID, testEnvironmentUID,
		func(path string) bool { return true })
	defer closeResolver()

	adapter, err := NewFinOpsAdapter("http://finops-adapter:9101", 30*time.Second, resolver, newTestFinOpsLogger())
	require.NoError(t, err)

	req := &types.CostQueryRequest{
		Namespace:   "default",
		Environment: "production",
		StartTime:   "2026-05-23T10:00:01Z",
		EndTime:     "2026-05-24T10:00:01Z",
	}
	_, err = adapter.GetComponentCosts(context.Background(), req)
	assert.ErrorIs(t, err, ErrFinOpsResolveScope)
}

func TestFinOpsAdapter_AdapterErrorStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	}))
	defer server.Close()

	resolver, closeResolver := newMockUIDResolver(testProjectUID, testComponentUID, testEnvironmentUID, nil)
	defer closeResolver()

	adapter, err := NewFinOpsAdapter(server.URL, 30*time.Second, resolver, newTestFinOpsLogger())
	require.NoError(t, err)

	req := &types.CostQueryRequest{
		Namespace:   "default",
		Environment: "production",
		StartTime:   "2026-05-23T10:00:01Z",
		EndTime:     "2026-05-24T10:00:01Z",
	}
	_, err = adapter.GetComponentCosts(context.Background(), req)
	assert.ErrorIs(t, err, ErrFinOpsRetrieval)
}

func TestFinOpsAdapter_NetworkError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	baseURL := server.URL
	server.Close() // close immediately so requests fail

	resolver, closeResolver := newMockUIDResolver(testProjectUID, testComponentUID, testEnvironmentUID, nil)
	defer closeResolver()

	adapter, err := NewFinOpsAdapter(baseURL, 2*time.Second, resolver, newTestFinOpsLogger())
	require.NoError(t, err)

	req := &types.CostQueryRequest{
		Namespace:   "default",
		Environment: "production",
		StartTime:   "2026-05-23T10:00:01Z",
		EndTime:     "2026-05-24T10:00:01Z",
	}
	_, err = adapter.GetComponentCosts(context.Background(), req)
	assert.ErrorIs(t, err, ErrFinOpsRetrieval)
}
