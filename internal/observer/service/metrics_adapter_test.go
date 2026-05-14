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

func TestMetricsAdapter_QueryRuntimeTopology_NilRequest(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	adapter := NewMetricsAdapter("http://localhost:9099", 30*time.Second, nil, logger)

	result, err := adapter.QueryRuntimeTopology(context.Background(), nil)

	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request must not be nil")
}

func TestMetricsAdapter_QueryRuntimeTopology_ValidationErrors(t *testing.T) {
	t.Parallel()

	validStart := "2026-01-01T00:00:00Z"
	validEnd := "2026-01-01T01:00:00Z"

	tests := []struct {
		name        string
		req         *types.RuntimeTopologyRequest
		expectedErr error
		errContains string
	}{
		{
			name: "missing project",
			req: &types.RuntimeTopologyRequest{
				SearchScope: types.ComponentSearchScope{
					Namespace:   "ns",
					Environment: "env",
				},
				StartTime: validStart,
				EndTime:   validEnd,
			},
			expectedErr: ErrRuntimeTopologyInvalidRequest,
			errContains: "searchScope.project is required",
		},
		{
			name: "missing environment",
			req: &types.RuntimeTopologyRequest{
				SearchScope: types.ComponentSearchScope{
					Namespace: "ns",
					Project:   "proj",
				},
				StartTime: validStart,
				EndTime:   validEnd,
			},
			expectedErr: ErrRuntimeTopologyInvalidRequest,
			errContains: "searchScope.environment is required",
		},
		{
			name: "missing startTime",
			req: &types.RuntimeTopologyRequest{
				SearchScope: types.ComponentSearchScope{
					Namespace:   "ns",
					Project:     "proj",
					Environment: "env",
				},
				EndTime: validEnd,
			},
			expectedErr: ErrRuntimeTopologyInvalidRequest,
			errContains: "startTime and endTime are required",
		},
		{
			name: "missing endTime",
			req: &types.RuntimeTopologyRequest{
				SearchScope: types.ComponentSearchScope{
					Namespace:   "ns",
					Project:     "proj",
					Environment: "env",
				},
				StartTime: validStart,
			},
			expectedErr: ErrRuntimeTopologyInvalidRequest,
			errContains: "startTime and endTime are required",
		},
		{
			name: "invalid startTime format",
			req: &types.RuntimeTopologyRequest{
				SearchScope: types.ComponentSearchScope{
					Namespace:   "ns",
					Project:     "proj",
					Environment: "env",
				},
				StartTime: "not-a-time",
				EndTime:   validEnd,
			},
			expectedErr: ErrRuntimeTopologyInvalidRequest,
			errContains: "invalid startTime",
		},
		{
			name: "invalid endTime format",
			req: &types.RuntimeTopologyRequest{
				SearchScope: types.ComponentSearchScope{
					Namespace:   "ns",
					Project:     "proj",
					Environment: "env",
				},
				StartTime: validStart,
				EndTime:   "not-a-time",
			},
			expectedErr: ErrRuntimeTopologyInvalidRequest,
			errContains: "invalid endTime",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			adapter := NewMetricsAdapter("http://localhost:9099", 30*time.Second, nil, logger)

			result, err := adapter.QueryRuntimeTopology(context.Background(), tt.req)

			assert.Nil(t, result)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.expectedErr)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestMetricsAdapter_QueryRuntimeTopology_HTTPErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		statusCode   int
		responseBody string
	}{
		{
			name:         "404 not found",
			statusCode:   http.StatusNotFound,
			responseBody: `{"error":"not found"}`,
		},
		{
			name:         "500 internal server error",
			statusCode:   http.StatusInternalServerError,
			responseBody: `{"error":"internal error"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, err := w.Write([]byte(tt.responseBody))
				require.NoError(t, err)
			}))
			defer server.Close()

			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			resolver, cleanup := newMockUIDResolver("proj-uid", "", "env-uid", nil)
			defer cleanup()
			adapter := NewMetricsAdapter(server.URL, 30*time.Second, resolver, logger)

			req := &types.RuntimeTopologyRequest{
				SearchScope: types.ComponentSearchScope{
					Namespace:   "ns",
					Project:     "proj",
					Environment: "env",
				},
				StartTime: "2026-01-01T00:00:00Z",
				EndTime:   "2026-01-01T01:00:00Z",
			}

			result, err := adapter.QueryRuntimeTopology(context.Background(), req)

			assert.Nil(t, result)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrRuntimeTopologyRetrieval)
		})
	}
}

func TestMetricsAdapter_QueryRuntimeTopology_InvalidJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("not json"))
		require.NoError(t, err)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver, cleanup := newMockUIDResolver("proj-uid", "", "env-uid", nil)
	defer cleanup()
	adapter := NewMetricsAdapter(server.URL, 30*time.Second, resolver, logger)

	req := &types.RuntimeTopologyRequest{
		SearchScope: types.ComponentSearchScope{
			Namespace:   "ns",
			Project:     "proj",
			Environment: "env",
		},
		StartTime: "2026-01-01T00:00:00Z",
		EndTime:   "2026-01-01T01:00:00Z",
	}

	result, err := adapter.QueryRuntimeTopology(context.Background(), req)

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRuntimeTopologyRetrieval)
	assert.Contains(t, err.Error(), "failed to decode response")
}

func TestMetricsAdapter_QueryRuntimeTopology_NetworkError(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver, cleanup := newMockUIDResolver("proj-uid", "", "env-uid", nil)
	defer cleanup()
	adapter := NewMetricsAdapter("http://invalid-host-that-does-not-exist:9999", 1*time.Second, resolver, logger)

	req := &types.RuntimeTopologyRequest{
		SearchScope: types.ComponentSearchScope{
			Namespace:   "ns",
			Project:     "proj",
			Environment: "env",
		},
		StartTime: "2026-01-01T00:00:00Z",
		EndTime:   "2026-01-01T01:00:00Z",
	}

	result, err := adapter.QueryRuntimeTopology(context.Background(), req)

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRuntimeTopologyRetrieval)
}

func TestMetricsAdapter_QueryRuntimeTopology_Success(t *testing.T) {
	t.Parallel()

	includeGateways := true
	includeExternal := false

	var capturedRequest runtimeTopologyAdapterRequest

	now := time.Now().UTC()
	adapterResponse := runtimeTopologyAdapterResponse{
		Nodes: []runtimeTopologyAdapterNode{
			{
				Kind:         "component",
				Component:    "comp-1",
				ComponentUID: "comp-uid-1",
				ProjectUID:   "proj-uid",
				Namespace:    "ns",
				Metrics: &runtimeTopologyAdapterMetrics{
					RequestCount:             ptrFloat64(100),
					UnsuccessfulRequestCount: ptrFloat64(5),
					MeanLatency:              ptrFloat64(50.0),
				},
			},
			{Kind: "gateway", GatewayName: "ingress-gw"},
			{Kind: "external", ExternalHost: "api.example.com"},
		},
		Edges: []runtimeTopologyAdapterEdge{
			{
				ID: "comp-uid-1->external:api.example.com",
				Source: runtimeTopologyAdapterNodeRef{
					Kind:         "component",
					Component:    "comp-1",
					ComponentUID: "comp-uid-1",
					ProjectUID:   "proj-uid",
				},
				Target: runtimeTopologyAdapterNodeRef{
					Kind:         "external",
					ExternalHost: "api.example.com",
				},
				Protocol: "http",
				Metrics: &runtimeTopologyAdapterMetrics{
					RequestCount: ptrFloat64(50),
				},
			},
		},
		Summary: runtimeTopologyAdapterSummary{
			StartTime:   now.Add(-1 * time.Hour),
			EndTime:     now,
			GeneratedAt: now,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1alpha1/metrics/runtime-topology", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &capturedRequest))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(adapterResponse))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver, cleanup := newMockUIDResolver("proj-uid", "", "env-uid", nil)
	defer cleanup()
	adapter := NewMetricsAdapter(server.URL, 30*time.Second, resolver, logger)

	req := &types.RuntimeTopologyRequest{
		SearchScope: types.ComponentSearchScope{
			Namespace:   "ns",
			Project:     "proj",
			Environment: "env",
		},
		StartTime:       "2026-01-01T00:00:00Z",
		EndTime:         "2026-01-01T01:00:00Z",
		IncludeGateways: &includeGateways,
		IncludeExternal: &includeExternal,
	}

	result, err := adapter.QueryRuntimeTopology(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, result)

	require.NotNil(t, capturedRequest.SearchScope.ProjectUID)
	assert.Equal(t, "proj-uid", *capturedRequest.SearchScope.ProjectUID)
	require.NotNil(t, capturedRequest.SearchScope.EnvironmentUID)
	assert.Equal(t, "env-uid", *capturedRequest.SearchScope.EnvironmentUID)
	assert.Nil(t, capturedRequest.SearchScope.ComponentUID)
	require.NotNil(t, capturedRequest.IncludeGateways)
	assert.True(t, *capturedRequest.IncludeGateways)
	require.NotNil(t, capturedRequest.IncludeExternal)
	assert.False(t, *capturedRequest.IncludeExternal)

	require.Len(t, result.Nodes, 3)
	require.Len(t, result.Edges, 1)
	assert.Equal(t, "comp-uid-1->external:api.example.com", result.Edges[0].ID)
	assert.Equal(t, types.RuntimeTopologyProtocol("http"), result.Edges[0].Protocol)
	assert.Equal(t, "comp-1", result.Edges[0].Source.Component)
	assert.Equal(t, "comp-uid-1", result.Edges[0].Source.ComponentUID)
}

func TestMetricsAdapter_QueryRuntimeTopology_WithComponent(t *testing.T) {
	t.Parallel()

	var capturedRequest runtimeTopologyAdapterRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &capturedRequest))

		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(runtimeTopologyAdapterResponse{
			Summary: runtimeTopologyAdapterSummary{GeneratedAt: time.Now().UTC()},
		}))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver, cleanup := newMockUIDResolver("proj-uid", "comp-uid", "env-uid", nil)
	defer cleanup()
	adapter := NewMetricsAdapter(server.URL, 30*time.Second, resolver, logger)

	req := &types.RuntimeTopologyRequest{
		SearchScope: types.ComponentSearchScope{
			Namespace:   "ns",
			Project:     "proj",
			Environment: "env",
			Component:   "comp",
		},
		StartTime: "2026-01-01T00:00:00Z",
		EndTime:   "2026-01-01T01:00:00Z",
	}

	result, err := adapter.QueryRuntimeTopology(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, capturedRequest.SearchScope.ComponentUID)
	assert.Equal(t, "comp-uid", *capturedRequest.SearchScope.ComponentUID)
}

func TestMetricsAdapter_QueryRuntimeTopology_EmptySummaryFallback(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		require.NoError(t, json.NewEncoder(w).Encode(runtimeTopologyAdapterResponse{}))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver, cleanup := newMockUIDResolver("proj-uid", "", "env-uid", nil)
	defer cleanup()
	adapter := NewMetricsAdapter(server.URL, 30*time.Second, resolver, logger)

	req := &types.RuntimeTopologyRequest{
		SearchScope: types.ComponentSearchScope{
			Namespace:   "ns",
			Project:     "proj",
			Environment: "env",
		},
		StartTime: "2026-01-01T00:00:00Z",
		EndTime:   "2026-01-01T01:00:00Z",
	}

	result, err := adapter.QueryRuntimeTopology(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Summary.StartTime.IsZero(), "StartTime should fall back to the request value")
	assert.False(t, result.Summary.EndTime.IsZero(), "EndTime should fall back to the request value")
	assert.False(t, result.Summary.GeneratedAt.IsZero(), "GeneratedAt should fall back to now")
}

func TestConvertTopologyAdapterMetrics(t *testing.T) {
	t.Parallel()

	t.Run("nil input returns nil", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, convertTopologyAdapterMetrics(nil))
	})

	t.Run("all fields populated", func(t *testing.T) {
		t.Parallel()
		in := &runtimeTopologyAdapterMetrics{
			RequestCount:             ptrFloat64(10),
			UnsuccessfulRequestCount: ptrFloat64(2),
			MeanLatency:              ptrFloat64(5.0),
			LatencyP50:               ptrFloat64(4.5),
			LatencyP90:               ptrFloat64(8.0),
			LatencyP99:               ptrFloat64(15.0),
		}
		out := convertTopologyAdapterMetrics(in)
		require.NotNil(t, out)
		assert.InDelta(t, 10.0, out.RequestCount, 0.001)
		assert.InDelta(t, 2.0, out.UnsuccessfulRequestCount, 0.001)
		assert.InDelta(t, 5.0, out.MeanLatency, 0.001)
		assert.InDelta(t, 4.5, out.LatencyP50, 0.001)
		assert.InDelta(t, 8.0, out.LatencyP90, 0.001)
		assert.InDelta(t, 15.0, out.LatencyP99, 0.001)
	})

	t.Run("nil pointer fields are skipped", func(t *testing.T) {
		t.Parallel()
		in := &runtimeTopologyAdapterMetrics{}
		out := convertTopologyAdapterMetrics(in)
		require.NotNil(t, out)
		assert.InDelta(t, 0.0, out.RequestCount, 0.001)
	})
}

func TestConvertTopologyAdapterNodeRef(t *testing.T) {
	t.Parallel()

	t.Run("component kind", func(t *testing.T) {
		t.Parallel()
		in := runtimeTopologyAdapterNodeRef{
			Kind:         "component",
			Component:    "comp-1",
			ComponentUID: "comp-uid-1",
			ProjectUID:   "proj-uid",
			Namespace:    "ns",
		}
		out := convertTopologyAdapterNodeRef(in)
		assert.Equal(t, types.RuntimeTopologyNodeKindComponent, out.Kind)
		assert.Equal(t, "comp-1", out.Component)
		assert.Equal(t, "comp-uid-1", out.ComponentUID)
		assert.Equal(t, "proj-uid", out.ProjectUID)
		assert.Equal(t, "ns", out.Namespace)
	})

	t.Run("gateway kind", func(t *testing.T) {
		t.Parallel()
		in := runtimeTopologyAdapterNodeRef{
			Kind:        "gateway",
			GatewayName: "ingress-gw",
			ProjectUID:  "proj-uid",
		}
		out := convertTopologyAdapterNodeRef(in)
		assert.Equal(t, types.RuntimeTopologyNodeKindGateway, out.Kind)
		assert.Equal(t, "ingress-gw", out.Name)
		assert.Equal(t, "proj-uid", out.ProjectUID)
	})

	t.Run("external kind", func(t *testing.T) {
		t.Parallel()
		in := runtimeTopologyAdapterNodeRef{
			Kind:         "external",
			ExternalHost: "api.example.com",
			Component:    "ext-comp",
			ComponentUID: "ext-comp-uid",
			ProjectUID:   "ext-proj-uid",
		}
		out := convertTopologyAdapterNodeRef(in)
		assert.Equal(t, types.RuntimeTopologyNodeKindExternal, out.Kind)
		assert.Equal(t, "api.example.com", out.Host)
		assert.Equal(t, "ext-comp", out.Component)
		assert.Equal(t, "ext-comp-uid", out.ComponentUID)
		assert.Equal(t, "ext-proj-uid", out.ProjectUID)
	})

	t.Run("component with empty project UID", func(t *testing.T) {
		t.Parallel()
		in := runtimeTopologyAdapterNodeRef{
			Kind:         "component",
			Component:    "comp-1",
			ComponentUID: "comp-uid-1",
		}
		out := convertTopologyAdapterNodeRef(in)
		assert.Equal(t, types.RuntimeTopologyNodeKindComponent, out.Kind)
		assert.Empty(t, out.ProjectUID)
	})
}

func TestMetricsAdapter_QueryRuntimeTopology_ResolverErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		projectUID  string
		envUID      string
		errPath     func(path string) bool
		errContains string
	}{
		{
			name:        "project resolution fails",
			errPath:     func(path string) bool { return strings.Contains(path, "/projects/") },
			errContains: "failed to get project UID",
		},
		{
			name:        "environment resolution fails",
			projectUID:  "proj-uid",
			errPath:     func(path string) bool { return strings.Contains(path, "/environments/") },
			errContains: "failed to get environment UID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			resolver, cleanup := newMockUIDResolver(tt.projectUID, "", tt.envUID, tt.errPath)
			defer cleanup()
			adapter := NewMetricsAdapter("http://localhost:9099", 30*time.Second, resolver, logger)

			req := &types.RuntimeTopologyRequest{
				SearchScope: types.ComponentSearchScope{
					Namespace:   "ns",
					Project:     "proj",
					Environment: "env",
				},
				StartTime: "2026-01-01T00:00:00Z",
				EndTime:   "2026-01-01T01:00:00Z",
			}

			result, err := adapter.QueryRuntimeTopology(context.Background(), req)

			assert.Nil(t, result)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrRuntimeTopologyResolveSearchScope)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestPtrStringIfNonEmpty(t *testing.T) {
	t.Parallel()

	assert.Nil(t, ptrStringIfNonEmpty(""))
	p := ptrStringIfNonEmpty("hello")
	require.NotNil(t, p)
	assert.Equal(t, "hello", *p)
}

func TestCoalesceTime(t *testing.T) {
	t.Parallel()

	fallback := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("zero time returns fallback", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, fallback, coalesceTime(time.Time{}, fallback))
	})

	t.Run("non-zero time returns itself", func(t *testing.T) {
		t.Parallel()
		nonZero := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
		assert.Equal(t, nonZero, coalesceTime(nonZero, fallback))
	})
}

func ptrFloat64(v float64) *float64 {
	return &v
}
