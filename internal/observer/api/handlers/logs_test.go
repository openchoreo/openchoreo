// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// fakeLogsQuerier implements service.LogsQuerier for tests.
type fakeLogsQuerier struct {
	resp *types.LogsQueryResponse
	err  error
}

func (f *fakeLogsQuerier) QueryLogs(_ context.Context, _ *types.LogsQueryRequest) (*types.LogsQueryResponse, error) {
	return f.resp, f.err
}

func TestQueryLogs_Success(t *testing.T) {
	t.Parallel()

	svc := &fakeLogsQuerier{
		resp: &types.LogsQueryResponse{
			Logs:   []types.LogEntry{{Timestamp: "2024-01-01T00:00:00Z", Log: "hello"}},
			Total:  1,
			TookMs: 5,
		},
	}
	h := &Handler{
		baseHandler: baseHandler{logger: noopLogger()},
		logsService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/query", validLogsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryLogs(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"total":1`)
}

func TestQueryLogs_InvalidBody(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler: baseHandler{logger: noopLogger()},
		logsService: &fakeLogsQuerier{},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/query", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.QueryLogs(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestQueryLogs_ValidationError(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler: baseHandler{logger: noopLogger()},
		logsService: &fakeLogsQuerier{},
	}

	// Missing searchScope → validation failure.
	body := `{"startTime":"2024-01-01T00:00:00Z","endTime":"2024-01-02T00:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.QueryLogs(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestQueryLogs_ServiceNotInitialized(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler: baseHandler{logger: noopLogger()},
		logsService: nil,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/query", validLogsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryLogs(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1LogsServiceNotReady)
}

func TestQueryLogs_AuthzForbidden(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler: baseHandler{logger: noopLogger()},
		logsService: &fakeLogsQuerier{err: observerAuthz.ErrAuthzForbidden},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/query", validLogsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryLogs(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestQueryLogs_AuthzUnauthorized(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler: baseHandler{logger: noopLogger()},
		logsService: &fakeLogsQuerier{err: observerAuthz.ErrAuthzUnauthorized},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/query", validLogsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryLogs(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestQueryLogs_ResolveSearchScopeError(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler: baseHandler{logger: noopLogger()},
		logsService: &fakeLogsQuerier{err: fmt.Errorf("%w: resolver failed", service.ErrLogsResolveSearchScope)},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/query", validLogsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryLogs(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1LogsResolverFailed)
}

func TestQueryLogs_RetrievalError(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler: baseHandler{logger: noopLogger()},
		logsService: &fakeLogsQuerier{err: fmt.Errorf("%w: backend error", service.ErrLogsRetrieval)},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/query", validLogsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryLogs(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1LogsRetrievalFailed)
}

func TestQueryLogs_GenericError(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler: baseHandler{logger: noopLogger()},
		logsService: &fakeLogsQuerier{err: errors.New("unexpected error")},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/query", validLogsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryLogs(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1LogsInternalGeneric)
}
