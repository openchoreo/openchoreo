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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// newMockUIDResolver creates a ResourceUIDResolver backed by a test HTTP server
// that returns the specified UIDs. The returnErr function, if non-nil, is called
// with the request path to determine whether to return an error for that path.
func newMockUIDResolver(projectUID, componentUID, environmentUID string, returnErr func(path string) bool) (*ResourceUIDResolver, func()) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle OAuth token endpoint
		if strings.Contains(r.URL.Path, "/token") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			tokenResp := map[string]interface{}{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			}
			_ = json.NewEncoder(w).Encode(tokenResp)
			return
		}

		if returnErr != nil && returnErr(r.URL.Path) {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		var uid string
		// Determine which UID to return based on the URL path
		if strings.Contains(r.URL.Path, "/projects/") {
			uid = projectUID
		} else if strings.Contains(r.URL.Path, "/components/") {
			uid = componentUID
		} else if strings.Contains(r.URL.Path, "/environments/") {
			uid = environmentUID
		} else {
			uid = projectUID // default
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"metadata": map[string]interface{}{
				"uid": uid,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))

	cfg := &config.UIDResolverConfig{
		OpenChoreoAPIURL:      server.URL,
		OAuthTokenURL:         server.URL + "/token",
		OAuthClientID:         "test-client",
		OAuthClientSecret:     "test-secret",
		Timeout:               5 * time.Second,
		TLSInsecureSkipVerify: true,
	}

	resolver := NewResourceUIDResolver(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return resolver, server.Close
}

func TestNewMetricsAdapter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		baseURL         string
		timeout         time.Duration
		expectedTimeout time.Duration
	}{
		{
			name:            "default timeout",
			baseURL:         "http://localhost:9099",
			timeout:         0,
			expectedTimeout: 30 * time.Second,
		},
		{
			name:            "custom timeout",
			baseURL:         "http://localhost:9099",
			timeout:         60 * time.Second,
			expectedTimeout: 60 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			resolver, cleanup := newMockUIDResolver("", "", "", nil)
			defer cleanup()

			adapter := NewMetricsAdapter(tt.baseURL, tt.timeout, resolver, logger)

			require.NotNil(t, adapter)
			assert.Equal(t, tt.baseURL, adapter.baseURL)
			require.NotNil(t, adapter.httpClient)
			assert.Equal(t, tt.expectedTimeout, adapter.httpClient.Timeout)
			require.NotNil(t, adapter.resolver)
			assert.Equal(t, logger, adapter.logger)
		})
	}
}

func TestMetricsAdapter_QueryMetrics_NilRequest(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver, cleanup := newMockUIDResolver("", "", "", nil)
	defer cleanup()
	adapter := NewMetricsAdapter("http://localhost:9099", 30*time.Second, resolver, logger)

	result, err := adapter.QueryMetrics(context.Background(), nil)

	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request must not be nil")
}

func TestMetricsAdapter_QueryMetrics_ResolverError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		req         *types.MetricsQueryRequest
		resolverErr func(path string) bool
		expectedErr string
	}{
		{
			name: "project UID resolution failure",
			req: &types.MetricsQueryRequest{
				Metric:    "resource",
				StartTime: "2026-01-01T00:00:00Z",
				EndTime:   "2026-01-01T01:00:00Z",
				SearchScope: types.ComponentSearchScope{
					Namespace: "test-ns",
					Project:   "test-project",
				},
			},
			resolverErr: func(path string) bool { return strings.Contains(path, "/projects/") },
			expectedErr: "failed to get project UID",
		},
		{
			name: "component UID resolution failure",
			req: &types.MetricsQueryRequest{
				Metric:    "resource",
				StartTime: "2026-01-01T00:00:00Z",
				EndTime:   "2026-01-01T01:00:00Z",
				SearchScope: types.ComponentSearchScope{
					Namespace: "test-ns",
					Project:   "test-project",
					Component: "test-component",
				},
			},
			resolverErr: func(path string) bool { return strings.Contains(path, "/components/") },
			expectedErr: "failed to get component UID",
		},
		{
			name: "environment UID resolution failure",
			req: &types.MetricsQueryRequest{
				Metric:    "resource",
				StartTime: "2026-01-01T00:00:00Z",
				EndTime:   "2026-01-01T01:00:00Z",
				SearchScope: types.ComponentSearchScope{
					Namespace:   "test-ns",
					Environment: "test-env",
				},
			},
			resolverErr: func(path string) bool { return strings.Contains(path, "/environments/") },
			expectedErr: "failed to get environment UID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			// Provide valid UIDs so that successful resolutions return proper values
			resolver, cleanup := newMockUIDResolver("project-uid-123", "component-uid-456", "env-uid-789", tt.resolverErr)
			defer cleanup()
			adapter := NewMetricsAdapter("http://localhost:9099", 30*time.Second, resolver, logger)

			result, err := adapter.QueryMetrics(context.Background(), tt.req)

			assert.Nil(t, result)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrMetricsResolveSearchScope)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestMetricsAdapter_QueryMetrics_Success(t *testing.T) {
	t.Parallel()

	step := "5m"
	expectedResponse := map[string]interface{}{
		"cpuUsage": []map[string]interface{}{
			{"timestamp": "2026-01-01T00:00:00Z", "value": 0.5},
			{"timestamp": "2026-01-01T00:05:00Z", "value": 0.6},
		},
	}

	var capturedRequest metricsAdapterRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/metrics/query", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Capture the request body
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &capturedRequest)
		require.NoError(t, err)

		// Return success response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(expectedResponse)
		require.NoError(t, err)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver, resolverCleanup := newMockUIDResolver("project-uid-123", "component-uid-456", "env-uid-789", nil)
	defer resolverCleanup()
	adapter := NewMetricsAdapter(server.URL, 30*time.Second, resolver, logger)

	req := &types.MetricsQueryRequest{
		Metric:    "resource",
		StartTime: "2026-01-01T00:00:00Z",
		EndTime:   "2026-01-01T01:00:00Z",
		Step:      &step,
		SearchScope: types.ComponentSearchScope{
			Namespace:   "test-ns",
			Project:     "test-project",
			Component:   "test-component",
			Environment: "test-env",
		},
	}

	result, err := adapter.QueryMetrics(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the captured request
	assert.Equal(t, "resource", capturedRequest.Metric)
	assert.Equal(t, "2026-01-01T00:00:00Z", capturedRequest.StartTime)
	assert.Equal(t, "2026-01-01T01:00:00Z", capturedRequest.EndTime)
	require.NotNil(t, capturedRequest.Step)
	assert.Equal(t, "5m", *capturedRequest.Step)
	assert.Equal(t, "test-ns", capturedRequest.SearchScope.Namespace)
	require.NotNil(t, capturedRequest.SearchScope.ProjectUID)
	assert.Equal(t, "project-uid-123", *capturedRequest.SearchScope.ProjectUID)
	require.NotNil(t, capturedRequest.SearchScope.ComponentUID)
	assert.Equal(t, "component-uid-456", *capturedRequest.SearchScope.ComponentUID)
	require.NotNil(t, capturedRequest.SearchScope.EnvironmentUID)
	assert.Equal(t, "env-uid-789", *capturedRequest.SearchScope.EnvironmentUID)

	// Verify the response
	resultJSON, err := json.Marshal(result)
	require.NoError(t, err)
	expectedJSON, err := json.Marshal(expectedResponse)
	require.NoError(t, err)
	assert.JSONEq(t, string(expectedJSON), string(resultJSON))
}

func TestMetricsAdapter_QueryMetrics_PartialScope(t *testing.T) {
	t.Parallel()

	var capturedRequest metricsAdapterRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &capturedRequest)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(map[string]interface{}{})
		require.NoError(t, err)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver, resolverCleanup := newMockUIDResolver("project-uid-123", "", "", nil)
	defer resolverCleanup()
	adapter := NewMetricsAdapter(server.URL, 30*time.Second, resolver, logger)

	req := &types.MetricsQueryRequest{
		Metric:    "http",
		StartTime: "2026-01-01T00:00:00Z",
		EndTime:   "2026-01-01T01:00:00Z",
		SearchScope: types.ComponentSearchScope{
			Namespace: "test-ns",
			Project:   "test-project",
		},
	}

	result, err := adapter.QueryMetrics(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify only project UID is set
	assert.Equal(t, "test-ns", capturedRequest.SearchScope.Namespace)
	require.NotNil(t, capturedRequest.SearchScope.ProjectUID)
	assert.Equal(t, "project-uid-123", *capturedRequest.SearchScope.ProjectUID)
	assert.Nil(t, capturedRequest.SearchScope.ComponentUID)
	assert.Nil(t, capturedRequest.SearchScope.EnvironmentUID)
}

func TestMetricsAdapter_QueryMetrics_EmptyScope(t *testing.T) {
	t.Parallel()

	var capturedRequest metricsAdapterRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &capturedRequest)
		require.NoError(t, err)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(map[string]interface{}{})
		require.NoError(t, err)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver, cleanup := newMockUIDResolver("", "", "", nil)
	defer cleanup()
	adapter := NewMetricsAdapter(server.URL, 30*time.Second, resolver, logger)

	req := &types.MetricsQueryRequest{
		Metric:    "resource",
		StartTime: "2026-01-01T00:00:00Z",
		EndTime:   "2026-01-01T01:00:00Z",
		SearchScope: types.ComponentSearchScope{
			Namespace: "test-ns",
		},
	}

	result, err := adapter.QueryMetrics(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify no UIDs are set
	assert.Equal(t, "test-ns", capturedRequest.SearchScope.Namespace)
	assert.Nil(t, capturedRequest.SearchScope.ProjectUID)
	assert.Nil(t, capturedRequest.SearchScope.ComponentUID)
	assert.Nil(t, capturedRequest.SearchScope.EnvironmentUID)
}

func TestMetricsAdapter_QueryMetrics_HTTPErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedErrMsg string
	}{
		{
			name:           "404 not found",
			statusCode:     http.StatusNotFound,
			responseBody:   `{"error": "not found"}`,
			expectedErrMsg: "metrics adapter returned HTTP 404",
		},
		{
			name:           "500 internal server error",
			statusCode:     http.StatusInternalServerError,
			responseBody:   `{"error": "internal error"}`,
			expectedErrMsg: "metrics adapter returned HTTP 500",
		},
		{
			name:           "400 bad request",
			statusCode:     http.StatusBadRequest,
			responseBody:   `{"error": "invalid request"}`,
			expectedErrMsg: "metrics adapter returned HTTP 400",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_, err := w.Write([]byte(tt.responseBody))
				require.NoError(t, err)
			}))
			defer server.Close()

			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			resolver, cleanup := newMockUIDResolver("", "", "", nil)
			defer cleanup()
			adapter := NewMetricsAdapter(server.URL, 30*time.Second, resolver, logger)

			req := &types.MetricsQueryRequest{
				Metric:    "resource",
				StartTime: "2026-01-01T00:00:00Z",
				EndTime:   "2026-01-01T01:00:00Z",
				SearchScope: types.ComponentSearchScope{
					Namespace: "test-ns",
				},
			}

			result, err := adapter.QueryMetrics(context.Background(), req)

			assert.Nil(t, result)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrMetricsRetrieval)
			assert.Contains(t, err.Error(), tt.expectedErrMsg)
		})
	}
}

func TestMetricsAdapter_QueryMetrics_InvalidJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("invalid json"))
		require.NoError(t, err)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver, cleanup := newMockUIDResolver("", "", "", nil)
	defer cleanup()
	adapter := NewMetricsAdapter(server.URL, 30*time.Second, resolver, logger)

	req := &types.MetricsQueryRequest{
		Metric:    "resource",
		StartTime: "2026-01-01T00:00:00Z",
		EndTime:   "2026-01-01T01:00:00Z",
		SearchScope: types.ComponentSearchScope{
			Namespace: "test-ns",
		},
	}

	result, err := adapter.QueryMetrics(context.Background(), req)

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMetricsRetrieval)
	assert.Contains(t, err.Error(), "failed to decode metrics adapter response")
}

func TestMetricsAdapter_QueryMetrics_NetworkError(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver, cleanup := newMockUIDResolver("", "", "", nil)
	defer cleanup()
	// Use an invalid URL to trigger network error
	adapter := NewMetricsAdapter("http://invalid-host-that-does-not-exist:9999", 1*time.Second, resolver, logger)

	req := &types.MetricsQueryRequest{
		Metric:    "resource",
		StartTime: "2026-01-01T00:00:00Z",
		EndTime:   "2026-01-01T01:00:00Z",
		SearchScope: types.ComponentSearchScope{
			Namespace: "test-ns",
		},
	}

	result, err := adapter.QueryMetrics(context.Background(), req)

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMetricsRetrieval)
}

func TestMetricsAdapter_QueryMetrics_ContextCancellation(t *testing.T) {
	t.Parallel()

	// Create a server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver, cleanup := newMockUIDResolver("", "", "", nil)
	defer cleanup()
	adapter := NewMetricsAdapter(server.URL, 30*time.Second, resolver, logger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := &types.MetricsQueryRequest{
		Metric:    "resource",
		StartTime: "2026-01-01T00:00:00Z",
		EndTime:   "2026-01-01T01:00:00Z",
		SearchScope: types.ComponentSearchScope{
			Namespace: "test-ns",
		},
	}

	result, err := adapter.QueryMetrics(ctx, req)

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMetricsRetrieval)
}
