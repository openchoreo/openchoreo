// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

// traces_handler_test.go covers the HTTP handler paths for QueryTraces,
// QuerySpansForTrace, and GetSpanDetailsForTrace that are NOT already covered
// by scope_auth_test.go (scope-auth error) or traces_test.go (conversion functions).

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

// QueryTraces tests -------------------------------------------------------------

func TestQueryTraces_Success(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockTracesQuerier(t)
	svc.On("QueryTraces", mock.Anything, mock.Anything).Return(&types.TracesQueryResponse{}, nil)

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/query", validTracesRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryTraces(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestQueryTraces_InvalidBody(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: servicemocks.NewMockTracesQuerier(t),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/query", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.QueryTraces(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestQueryTraces_ValidationError(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: servicemocks.NewMockTracesQuerier(t),
	}

	// Missing namespace → validation failure.
	body := `{"startTime":"2024-01-01T00:00:00Z","endTime":"2024-01-02T00:00:00Z","searchScope":{}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.QueryTraces(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestQueryTraces_ServiceNotInitialized(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: nil,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/query", validTracesRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryTraces(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1TracesServiceNotReady)
}

func TestQueryTraces_AuthzForbidden(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockTracesQuerier(t)
	svc.On("QueryTraces", mock.Anything, mock.Anything).Return(nil, observerAuthz.ErrAuthzForbidden)

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/query", validTracesRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryTraces(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestQueryTraces_AuthzUnauthorized(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockTracesQuerier(t)
	svc.On("QueryTraces", mock.Anything, mock.Anything).Return(nil, observerAuthz.ErrAuthzUnauthorized)

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/query", validTracesRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryTraces(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestQueryTraces_ResolveSearchScopeError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockTracesQuerier(t)
	svc.On("QueryTraces", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("%w: scope failed", service.ErrTracesResolveSearchScope))

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/query", validTracesRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryTraces(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1TracesResolverFailed)
}

func TestQueryTraces_RetrievalError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockTracesQuerier(t)
	svc.On("QueryTraces", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("%w: backend error", service.ErrTracesRetrieval))

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/query", validTracesRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryTraces(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1TracesRetrievalFailed)
}

func TestQueryTraces_InvalidRequestError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockTracesQuerier(t)
	svc.On("QueryTraces", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("%w: bad param", service.ErrTracesInvalidRequest))

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/query", validTracesRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryTraces(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestQueryTraces_GenericError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockTracesQuerier(t)
	svc.On("QueryTraces", mock.Anything, mock.Anything).Return(nil, errors.New("unexpected"))

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/query", validTracesRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryTraces(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1TracesInternalGeneric)
}

// QuerySpansForTrace tests -------------------------------------------------------

func TestQuerySpansForTrace_Success(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockTracesQuerier(t)
	svc.On("QuerySpans", mock.Anything, mock.Anything, mock.Anything).Return(&types.SpansQueryResponse{}, nil)

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/trace-1/spans/query", validTracesRequestBody(t))
	req.SetPathValue("traceId", "trace-1")
	rr := httptest.NewRecorder()

	h.QuerySpansForTrace(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestQuerySpansForTrace_ServiceNotInitialized(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: nil,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/trace-1/spans/query", validTracesRequestBody(t))
	req.SetPathValue("traceId", "trace-1")
	rr := httptest.NewRecorder()

	h.QuerySpansForTrace(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1TracesServiceNotReady)
}

func TestQuerySpansForTrace_AuthzForbidden(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockTracesQuerier(t)
	svc.On("QuerySpans", mock.Anything, mock.Anything, mock.Anything).Return(nil, observerAuthz.ErrAuthzForbidden)

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/trace-1/spans/query", validTracesRequestBody(t))
	req.SetPathValue("traceId", "trace-1")
	rr := httptest.NewRecorder()

	h.QuerySpansForTrace(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestQuerySpansForTrace_RetrievalError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockTracesQuerier(t)
	svc.On("QuerySpans", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("%w: backend error", service.ErrTracesRetrieval))

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/trace-1/spans/query", validTracesRequestBody(t))
	req.SetPathValue("traceId", "trace-1")
	rr := httptest.NewRecorder()

	h.QuerySpansForTrace(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1TracesRetrievalFailed)
}

func TestQuerySpansForTrace_InvalidRequestError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockTracesQuerier(t)
	svc.On("QuerySpans", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("%w: bad param", service.ErrTracesInvalidRequest))

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/trace-1/spans/query", validTracesRequestBody(t))
	req.SetPathValue("traceId", "trace-1")
	rr := httptest.NewRecorder()

	h.QuerySpansForTrace(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestQuerySpansForTrace_GenericError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockTracesQuerier(t)
	svc.On("QuerySpans", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("unknown"))

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/trace-1/spans/query", validTracesRequestBody(t))
	req.SetPathValue("traceId", "trace-1")
	rr := httptest.NewRecorder()

	h.QuerySpansForTrace(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1TracesInternalGeneric)
}

// GetSpanDetailsForTrace tests ---------------------------------------------------

func TestGetSpanDetailsForTrace_Success(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockTracesQuerier(t)
	svc.On("GetSpanDetails", mock.Anything, mock.Anything, mock.Anything).Return(&types.SpanInfo{SpanID: "span-1"}, nil)

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: svc,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/traces/trace-1/spans/span-1", nil)
	req.SetPathValue("traceId", "trace-1")
	req.SetPathValue("spanId", "span-1")
	rr := httptest.NewRecorder()

	h.GetSpanDetailsForTrace(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestGetSpanDetailsForTrace_EmptyTraceID(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: servicemocks.NewMockTracesQuerier(t),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/traces//spans/span-1", nil)
	req.SetPathValue("traceId", "")
	req.SetPathValue("spanId", "span-1")
	rr := httptest.NewRecorder()

	h.GetSpanDetailsForTrace(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "traceId is required")
}

func TestGetSpanDetailsForTrace_EmptySpanID(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: servicemocks.NewMockTracesQuerier(t),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/traces/trace-1/spans/", nil)
	req.SetPathValue("traceId", "trace-1")
	req.SetPathValue("spanId", "")
	rr := httptest.NewRecorder()

	h.GetSpanDetailsForTrace(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "spanId is required")
}

func TestGetSpanDetailsForTrace_ServiceNotInitialized(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: nil,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/traces/trace-1/spans/span-1", nil)
	req.SetPathValue("traceId", "trace-1")
	req.SetPathValue("spanId", "span-1")
	rr := httptest.NewRecorder()

	h.GetSpanDetailsForTrace(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1TracesServiceNotReady)
}

func TestGetSpanDetailsForTrace_SpanNotFound(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockTracesQuerier(t)
	svc.On("GetSpanDetails", mock.Anything, mock.Anything, mock.Anything).Return(nil, service.ErrSpanNotFound)

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: svc,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/traces/trace-1/spans/span-99", nil)
	req.SetPathValue("traceId", "trace-1")
	req.SetPathValue("spanId", "span-99")
	rr := httptest.NewRecorder()

	h.GetSpanDetailsForTrace(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1TracesSpanNotFound)
}

func TestGetSpanDetailsForTrace_RetrievalError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockTracesQuerier(t)
	svc.On("GetSpanDetails", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("%w: backend", service.ErrTracesRetrieval))

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: svc,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/traces/trace-1/spans/span-1", nil)
	req.SetPathValue("traceId", "trace-1")
	req.SetPathValue("spanId", "span-1")
	rr := httptest.NewRecorder()

	h.GetSpanDetailsForTrace(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1TracesRetrievalFailed)
}

func TestGetSpanDetailsForTrace_GenericError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockTracesQuerier(t)
	svc.On("GetSpanDetails", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("unexpected"))

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: svc,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1alpha1/traces/trace-1/spans/span-1", nil)
	req.SetPathValue("traceId", "trace-1")
	req.SetPathValue("spanId", "span-1")
	rr := httptest.NewRecorder()

	h.GetSpanDetailsForTrace(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1TracesInternalGeneric)
}
