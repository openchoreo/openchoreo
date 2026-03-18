// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

// scope_auth_test.go verifies that when a service returns service.ErrScopeAuthFailed
// the handler responds with HTTP 500 and the OBS-V1-SCOPE-AUTH-FAILED error code.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// ---- fake services returning ErrScopeAuthFailed --------------------------------

type fakeScopeAuthFailedLogsService struct{}

func (f *fakeScopeAuthFailedLogsService) QueryLogs(_ context.Context, _ *types.LogsQueryRequest) (*types.LogsQueryResponse, error) {
	return nil, fmt.Errorf("%w: token expired after idle", service.ErrScopeAuthFailed)
}

type fakeScopeAuthFailedMetricsService struct{}

func (f *fakeScopeAuthFailedMetricsService) QueryMetrics(_ context.Context, _ *types.MetricsQueryRequest) (any, error) {
	return nil, fmt.Errorf("%w: token expired after idle", service.ErrScopeAuthFailed)
}

type fakeScopeAuthFailedTracesService struct{}

func (f *fakeScopeAuthFailedTracesService) QueryTraces(_ context.Context, _ *types.TracesQueryRequest) (*types.TracesQueryResponse, error) {
	return nil, fmt.Errorf("%w: token expired after idle", service.ErrScopeAuthFailed)
}

func (f *fakeScopeAuthFailedTracesService) QuerySpans(_ context.Context, _ string, _ *types.TracesQueryRequest) (*types.SpansQueryResponse, error) {
	return nil, fmt.Errorf("%w: token expired after idle", service.ErrScopeAuthFailed)
}

func (f *fakeScopeAuthFailedTracesService) GetSpanDetails(_ context.Context, _, _ string) (*types.SpanInfo, error) {
	return nil, fmt.Errorf("%w: token expired after idle", service.ErrScopeAuthFailed)
}

// ---- helpers -------------------------------------------------------------------

// validLogsRequestBody returns a minimal valid logs query request JSON.
func validLogsRequestBody(t *testing.T) io.Reader {
	t.Helper()
	now := time.Now().UTC()
	req := map[string]any{
		"startTime": now.Add(-1 * time.Hour).Format(time.RFC3339),
		"endTime":   now.Format(time.RFC3339),
		"searchScope": map[string]any{
			"namespace": "test-ns",
			"project":   "test-project",
			"component": "test-component",
		},
	}
	b, err := json.Marshal(req)
	require.NoError(t, err, "failed to marshal logs request")
	return bytes.NewReader(b)
}

// validMetricsRequestBody returns a minimal valid metrics query request JSON.
func validMetricsRequestBody(t *testing.T) io.Reader {
	t.Helper()
	now := time.Now().UTC()
	req := map[string]any{
		"metric":    "resource",
		"startTime": now.Add(-1 * time.Hour).Format(time.RFC3339),
		"endTime":   now.Format(time.RFC3339),
		"searchScope": map[string]any{
			"namespace": "test-ns",
		},
	}
	b, err := json.Marshal(req)
	require.NoError(t, err, "failed to marshal metrics request")
	return bytes.NewReader(b)
}

// validTracesRequestBody returns a minimal valid traces query request JSON.
func validTracesRequestBody(t *testing.T) io.Reader {
	t.Helper()
	now := time.Now().UTC()
	req := map[string]any{
		"startTime": now.Add(-1 * time.Hour).Format(time.RFC3339),
		"endTime":   now.Format(time.RFC3339),
		"searchScope": map[string]any{
			"namespace": "test-ns",
		},
	}
	b, err := json.Marshal(req)
	require.NoError(t, err, "failed to marshal traces request")
	return bytes.NewReader(b)
}

// assertScopeAuthFailedResponse checks that the response is HTTP 500 with the
// OBS-V1-SCOPE-AUTH-FAILED error code.
func assertScopeAuthFailedResponse(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	body, err := io.ReadAll(rr.Body)
	require.NoError(t, err, "failed to read response body")

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(body, &errResp), "failed to unmarshal error response, body: %s", string(body))

	got, _ := errResp["errorCode"].(string)
	assert.Equal(t, types.ErrorCodeV1ScopeAuthFailed, got, "full body: %s", string(body))
}

// ---- test cases ----------------------------------------------------------------

func TestQueryLogs_ScopeAuthFailed_Returns500WithCode(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler: baseHandler{logger: noopLogger()},
		logsService: &fakeScopeAuthFailedLogsService{},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/query", validLogsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryLogs(rr, req)

	assertScopeAuthFailedResponse(t, rr)
}

func TestQueryMetrics_ScopeAuthFailed_Returns500WithCode(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:    baseHandler{logger: noopLogger()},
		metricsService: &fakeScopeAuthFailedMetricsService{},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/query", validMetricsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryMetrics(rr, req)

	assertScopeAuthFailedResponse(t, rr)
}

func TestQueryTraces_ScopeAuthFailed_Returns500WithCode(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: &fakeScopeAuthFailedTracesService{},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/query", validTracesRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryTraces(rr, req)

	assertScopeAuthFailedResponse(t, rr)
}

func TestQuerySpans_ScopeAuthFailed_Returns500WithCode(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: &fakeScopeAuthFailedTracesService{},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/trace-1/spans/query", validTracesRequestBody(t))
	req.SetPathValue("traceId", "trace-1")
	rr := httptest.NewRecorder()

	h.QuerySpansForTrace(rr, req)

	assertScopeAuthFailedResponse(t, rr)
}

func TestGetSpanDetails_ScopeAuthFailed_Returns500WithCode(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: &fakeScopeAuthFailedTracesService{},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/traces/trace-1/spans/span-1", nil)
	req.SetPathValue("traceId", "trace-1")
	req.SetPathValue("spanId", "span-1")
	rr := httptest.NewRecorder()

	h.GetSpanDetailsForTrace(rr, req)

	assertScopeAuthFailedResponse(t, rr)
}
