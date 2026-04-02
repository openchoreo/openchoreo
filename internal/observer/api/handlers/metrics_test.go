// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	servicemocks "github.com/openchoreo/openchoreo/internal/observer/service/mocks"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

func TestQueryMetrics_Success(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockMetricsQuerier(t)
	svc.On("QueryMetrics", mock.Anything, mock.Anything).Return(map[string]any{"data": "ok"}, nil)

	h := &Handler{
		baseHandler:    baseHandler{logger: noopLogger()},
		metricsService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/query", validMetricsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryMetrics(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"data"`)
}

func TestQueryMetrics_InvalidBody(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:    baseHandler{logger: noopLogger()},
		metricsService: servicemocks.NewMockMetricsQuerier(t),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/query", strings.NewReader("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.QueryMetrics(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestQueryMetrics_ValidationError(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:    baseHandler{logger: noopLogger()},
		metricsService: servicemocks.NewMockMetricsQuerier(t),
	}

	// Missing metric field → validation failure.
	body := `{"searchScope":{"namespace":"ns"},"startTime":"2024-01-01T00:00:00Z","endTime":"2024-01-02T00:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.QueryMetrics(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestQueryMetrics_ServiceNotInitialized(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:    baseHandler{logger: noopLogger()},
		metricsService: nil,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/query", validMetricsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryMetrics(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1MetricsServiceNotReady)
}

func TestQueryMetrics_AuthzForbidden(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockMetricsQuerier(t)
	svc.On("QueryMetrics", mock.Anything, mock.Anything).Return(nil, observerAuthz.ErrAuthzForbidden)

	h := &Handler{
		baseHandler:    baseHandler{logger: noopLogger()},
		metricsService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/query", validMetricsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryMetrics(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestQueryMetrics_AuthzUnauthorized(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockMetricsQuerier(t)
	svc.On("QueryMetrics", mock.Anything, mock.Anything).Return(nil, observerAuthz.ErrAuthzUnauthorized)

	h := &Handler{
		baseHandler:    baseHandler{logger: noopLogger()},
		metricsService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/query", validMetricsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryMetrics(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestQueryMetrics_InvalidRequestError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockMetricsQuerier(t)
	svc.On("QueryMetrics", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("%w: bad step", service.ErrMetricsInvalidRequest))

	h := &Handler{
		baseHandler:    baseHandler{logger: noopLogger()},
		metricsService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/query", validMetricsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryMetrics(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestQueryMetrics_ResolveSearchScopeError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockMetricsQuerier(t)
	svc.On("QueryMetrics", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("%w: scope failed", service.ErrMetricsResolveSearchScope))

	h := &Handler{
		baseHandler:    baseHandler{logger: noopLogger()},
		metricsService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/query", validMetricsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryMetrics(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1MetricsResolverFailed)
}

func TestQueryMetrics_RetrievalError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockMetricsQuerier(t)
	svc.On("QueryMetrics", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("%w: backend error", service.ErrMetricsRetrieval))

	h := &Handler{
		baseHandler:    baseHandler{logger: noopLogger()},
		metricsService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/query", validMetricsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryMetrics(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1MetricsRetrievalFailed)
}

func TestQueryMetrics_GenericError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockMetricsQuerier(t)
	svc.On("QueryMetrics", mock.Anything, mock.Anything).Return(nil, errors.New("unexpected"))

	h := &Handler{
		baseHandler:    baseHandler{logger: noopLogger()},
		metricsService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/query", validMetricsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryMetrics(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1MetricsInternalGeneric)
}
