// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

// insights_test.go drives both Delivery Insights operations through the generated
// public router, so the spec's routes, body decoding and the handler's status
// mapping are all exercised together.
//
// The status mapping is the part worth pinning. Both operations declare 503 for an
// authorization-service outage, which the pre-generated-routing handler returned
// without the spec saying so; TestQueryDora*_AuthzUnavailable fails if either the
// mapping or the spec declaration is dropped.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	servicemocks "github.com/openchoreo/openchoreo/internal/observer/service/mocks"
)

const (
	doraMetricsPath     = "/api/v1alpha1/insights/dora/query"
	doraDeploymentsPath = "/api/v1alpha1/insights/dora/deployments/query"
)

// helpers -----------------------------------------------------------------------

func validDoraRequestBody(t *testing.T) io.Reader {
	t.Helper()
	now := time.Now().UTC()
	raw := map[string]any{
		"startTime":   now.Add(-24 * time.Hour).Format(time.RFC3339),
		"endTime":     now.Format(time.RFC3339),
		"searchScope": map[string]any{"namespace": "test-ns"},
	}
	b, err := json.Marshal(raw)
	require.NoError(t, err)
	return bytes.NewReader(b)
}

func insightsHandler(t *testing.T, svc service.InsightsService) *Handler {
	t.Helper()
	return &Handler{
		baseHandler:     baseHandler{logger: noopLogger()},
		insightsService: svc,
	}
}

// QueryDoraMetrics tests --------------------------------------------------------

func TestQueryDoraMetrics_Success(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockInsightsService(t)
	svc.On("QueryDoraMetrics", mock.Anything, mock.Anything).
		Return(&gen.DoraMetricsQueryResponse{}, nil)

	req := httptest.NewRequest(http.MethodPost, doraMetricsPath, validDoraRequestBody(t))
	rr := serve(t, insightsHandler(t, svc), req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestQueryDoraMetrics_InvalidBody(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, doraMetricsPath, bytes.NewReader([]byte("{bad")))
	rr := serve(t, insightsHandler(t, servicemocks.NewMockInsightsService(t)), req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestQueryDoraMetrics_ValidationError(t *testing.T) {
	t.Parallel()

	// Missing namespace → validation failure.
	raw := map[string]any{
		"startTime":   "2026-01-01T00:00:00Z",
		"endTime":     "2026-01-02T00:00:00Z",
		"searchScope": map[string]any{},
	}
	b, err := json.Marshal(raw)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, doraMetricsPath, bytes.NewReader(b))
	rr := serve(t, insightsHandler(t, servicemocks.NewMockInsightsService(t)), req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "VALIDATION_ERROR")
}

func TestQueryDoraMetrics_ServiceNotInitialized(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, doraMetricsPath, validDoraRequestBody(t))
	rr := serve(t, insightsHandler(t, nil), req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "SERVICE_NOT_READY")
}

func TestQueryDoraMetrics_AuthzForbidden(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockInsightsService(t)
	svc.On("QueryDoraMetrics", mock.Anything, mock.Anything).
		Return(nil, observerAuthz.ErrAuthzForbidden)

	req := httptest.NewRequest(http.MethodPost, doraMetricsPath, validDoraRequestBody(t))
	rr := serve(t, insightsHandler(t, svc), req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestQueryDoraMetrics_AuthzUnavailable(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockInsightsService(t)
	svc.On("QueryDoraMetrics", mock.Anything, mock.Anything).
		Return(nil, observerAuthz.ErrAuthzServiceUnavailable)

	req := httptest.NewRequest(http.MethodPost, doraMetricsPath, validDoraRequestBody(t))
	rr := serve(t, insightsHandler(t, svc), req)

	require.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.Contains(t, rr.Body.String(), "AUTHZ_UNAVAILABLE")
}

func TestQueryDoraMetrics_ScopeNotFound(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockInsightsService(t)
	svc.On("QueryDoraMetrics", mock.Anything, mock.Anything).
		Return(nil, service.ErrScopeNotFound)

	req := httptest.NewRequest(http.MethodPost, doraMetricsPath, validDoraRequestBody(t))
	rr := serve(t, insightsHandler(t, svc), req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "SCOPE_NOT_FOUND")
}

// TestQueryDoraMetrics_NilResponse covers the errNilInsightsResponse guard: a
// service returning neither a response nor an error must not dereference nil.
func TestQueryDoraMetrics_NilResponse(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockInsightsService(t)
	svc.On("QueryDoraMetrics", mock.Anything, mock.Anything).Return(nil, nil)

	req := httptest.NewRequest(http.MethodPost, doraMetricsPath, validDoraRequestBody(t))
	rr := serve(t, insightsHandler(t, svc), req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "QUERY_DORA_METRICS_FAILED")
}

// QueryDoraDeployments tests ----------------------------------------------------

func TestQueryDoraDeployments_Success(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockInsightsService(t)
	svc.On("QueryDoraDeployments", mock.Anything, mock.Anything).
		Return(&gen.DoraDeploymentsQueryResponse{}, nil)

	req := httptest.NewRequest(http.MethodPost, doraDeploymentsPath, validDoraRequestBody(t))
	rr := serve(t, insightsHandler(t, svc), req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestQueryDoraDeployments_ValidationError(t *testing.T) {
	t.Parallel()

	// limit above the cap → validation failure.
	raw := map[string]any{
		"startTime":   "2026-01-01T00:00:00Z",
		"endTime":     "2026-01-02T00:00:00Z",
		"searchScope": map[string]any{"namespace": "test-ns"},
		"limit":       1_000_000,
	}
	b, err := json.Marshal(raw)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, doraDeploymentsPath, bytes.NewReader(b))
	rr := serve(t, insightsHandler(t, servicemocks.NewMockInsightsService(t)), req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestQueryDoraDeployments_ServiceNotInitialized(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, doraDeploymentsPath, validDoraRequestBody(t))
	rr := serve(t, insightsHandler(t, nil), req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "SERVICE_NOT_READY")
}

func TestQueryDoraDeployments_AuthzUnavailable(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockInsightsService(t)
	svc.On("QueryDoraDeployments", mock.Anything, mock.Anything).
		Return(nil, observerAuthz.ErrAuthzTimeout)

	req := httptest.NewRequest(http.MethodPost, doraDeploymentsPath, validDoraRequestBody(t))
	rr := serve(t, insightsHandler(t, svc), req)

	require.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.Contains(t, rr.Body.String(), "AUTHZ_UNAVAILABLE")
}

func TestQueryDoraDeployments_NilResponse(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockInsightsService(t)
	svc.On("QueryDoraDeployments", mock.Anything, mock.Anything).Return(nil, nil)

	req := httptest.NewRequest(http.MethodPost, doraDeploymentsPath, validDoraRequestBody(t))
	rr := serve(t, insightsHandler(t, svc), req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "QUERY_DORA_DEPLOYMENTS_FAILED")
}
