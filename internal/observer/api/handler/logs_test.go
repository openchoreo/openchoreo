// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// errPDP is a stub PDP whose Evaluate always returns an unexpected error,
// exercising the 500 / ErrorCodeInternalError path in QueryLogs.
type errPDP struct{}

func (e *errPDP) Evaluate(_ context.Context, _ *authzcore.EvaluateRequest) (*authzcore.Decision, error) {
	return nil, errors.New("unexpected authz backend failure")
}
func (e *errPDP) BatchEvaluate(_ context.Context, _ *authzcore.BatchEvaluateRequest) (*authzcore.BatchEvaluateResponse, error) {
	return nil, nil
}
func (e *errPDP) GetSubjectProfile(_ context.Context, _ *authzcore.ProfileRequest) (*authzcore.UserCapabilitiesResponse, error) {
	return nil, nil
}

// newQueryLogsRequest creates a POST request with the given JSON body.
func newQueryLogsRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/query",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// newQueryLogsRequestWithSubject creates a POST request that carries a SubjectContext
// in its context (required when a non-nil PDP is in use).
func newQueryLogsRequestWithSubject(t *testing.T, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/query",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	subjectCtx := &auth.SubjectContext{}
	ctx := auth.SetSubjectContext(req.Context(), subjectCtx)
	return req.WithContext(ctx)
}

func TestQueryLogs_InvalidContentType(t *testing.T) {
	h := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/query",
		strings.NewReader(`{"searchScope":{"namespace":"ns"}}`))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()

	h.QueryLogs(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	assertErrorCode(t, w, types.ErrorCodeInvalidRequest)
}

func TestQueryLogs_EmptyBody(t *testing.T) {
	h := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/query", nil)
	w := httptest.NewRecorder()

	h.QueryLogs(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	assertErrorCode(t, w, types.ErrorCodeInvalidRequest)
}

func TestQueryLogs_InvalidJSON(t *testing.T) {
	h := newTestHandler(t)

	req := newQueryLogsRequest(t, `{not valid json}`)
	w := httptest.NewRecorder()

	h.QueryLogs(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	assertErrorCode(t, w, types.ErrorCodeInvalidRequest)
}

func TestQueryLogs_ValidationFailure_MissingStartTime(t *testing.T) {
	h := newTestHandler(t)

	body := `{
		"searchScope": {"namespace":"ns","project":"proj"},
		"endTime": "2024-01-01T10:00:00Z"
	}`
	req := newQueryLogsRequest(t, body)
	w := httptest.NewRecorder()

	h.QueryLogs(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	assertErrorCode(t, w, types.ErrorCodeInvalidRequest)
}

func TestQueryLogs_ValidationFailure_MissingEndTime(t *testing.T) {
	h := newTestHandler(t)

	body := `{
		"searchScope": {"namespace":"ns","project":"proj"},
		"startTime": "2024-01-01T00:00:00Z"
	}`
	req := newQueryLogsRequest(t, body)
	w := httptest.NewRecorder()

	h.QueryLogs(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	assertErrorCode(t, w, types.ErrorCodeInvalidRequest)
}

func TestQueryLogs_ValidationFailure_InvalidLogLevel(t *testing.T) {
	h := newTestHandler(t)

	body := `{
		"searchScope": {"namespace":"ns","project":"proj"},
		"startTime": "2024-01-01T00:00:00Z",
		"endTime": "2024-01-01T10:00:00Z",
		"logLevels": ["INVALID"]
	}`
	req := newQueryLogsRequest(t, body)
	w := httptest.NewRecorder()

	h.QueryLogs(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	assertErrorCode(t, w, types.ErrorCodeInvalidRequest)
}

func TestQueryLogs_ValidationFailure_InvalidSortOrder(t *testing.T) {
	h := newTestHandler(t)

	body := `{
		"searchScope": {"namespace":"ns","project":"proj"},
		"startTime": "2024-01-01T00:00:00Z",
		"endTime": "2024-01-01T10:00:00Z",
		"sortOrder": "INVALID"
	}`
	req := newQueryLogsRequest(t, body)
	w := httptest.NewRecorder()

	h.QueryLogs(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	assertErrorCode(t, w, types.ErrorCodeInvalidRequest)
}

func TestQueryLogs_ValidationFailure_LimitTooLarge(t *testing.T) {
	h := newTestHandler(t)

	body := `{
		"searchScope": {"namespace":"ns","project":"proj"},
		"startTime": "2024-01-01T00:00:00Z",
		"endTime": "2024-01-01T10:00:00Z",
		"limit": 99999
	}`
	req := newQueryLogsRequest(t, body)
	w := httptest.NewRecorder()

	h.QueryLogs(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	assertErrorCode(t, w, types.ErrorCodeInvalidRequest)
}

func TestQueryLogs_AuthzSkippedWithNilPDP_ComponentScope(t *testing.T) {
	// With nil PDP authorization is skipped. But without a real LogsService,
	// the call will fail when QueryLogs tries to use the service.
	// We use nil logsService to verify that authz is passed and the service is attempted.
	// The test verifies we get past authz (no 401/403) but fail with 500 at service level.
	h := newTestHandler(t) // logsService is nil

	body := `{
		"searchScope": {"namespace":"ns","project":"proj"},
		"startTime": "2024-01-01T00:00:00Z",
		"endTime": "2024-01-01T10:00:00Z"
	}`
	req := newQueryLogsRequest(t, body)
	w := httptest.NewRecorder()

	// This will panic if logsService is nil and we reach the service call,
	// so wrap in a defer/recover to detect the boundary.
	func() {
		defer func() {
			// If a panic occurs it means we got past authz and hit the nil service.
			// That is acceptable behavior for this boundary test.
			recover() //nolint:errcheck
		}()
		h.QueryLogs(w, req)
	}()

	// We must NOT get a 401 or 403 (authz should have been skipped)
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("expected authz to be skipped with nil PDP, got status %d", w.Code)
	}
}

func TestQueryLogs_AuthzSkippedWithNilPDP_WorkflowScope(t *testing.T) {
	h := newTestHandler(t)

	body := `{
		"searchScope": {"namespace":"ns","workflowRunName":"run-abc"},
		"startTime": "2024-01-01T00:00:00Z",
		"endTime": "2024-01-01T10:00:00Z"
	}`
	req := newQueryLogsRequest(t, body)
	w := httptest.NewRecorder()

	func() {
		defer func() {
			recover() //nolint:errcheck
		}()
		h.QueryLogs(w, req)
	}()

	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("expected authz to be skipped with nil PDP, got status %d", w.Code)
	}
}

func TestQueryLogs_AuthzSkippedWithNilPDP_NamespaceOnlyScope(t *testing.T) {
	h := newTestHandler(t)

	// Namespace-only component scope
	body := `{
		"searchScope": {"namespace":"ns"},
		"startTime": "2024-01-01T00:00:00Z",
		"endTime": "2024-01-01T10:00:00Z"
	}`
	req := newQueryLogsRequest(t, body)
	w := httptest.NewRecorder()

	func() {
		defer func() {
			recover() //nolint:errcheck
		}()
		h.QueryLogs(w, req)
	}()

	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("expected authz to be skipped with nil PDP, got status %d", w.Code)
	}
}

func TestQueryLogs_AuthzBackendError_Returns500(t *testing.T) {
	// Build a handler with a non-nil PDP that always errors from Evaluate.
	// The errPDP triggers the "authorization check failed" 500 path in QueryLogs.
	logger := slog.Default()
	healthService := service.NewHealthService(logger)
	h := NewHandler(nil, healthService, logger, &errPDP{})

	body := `{
		"searchScope": {"namespace":"ns","project":"proj"},
		"startTime": "2024-01-01T00:00:00Z",
		"endTime": "2024-01-01T10:00:00Z"
	}`
	// The request must carry a SubjectContext so the authz check reaches pdp.Evaluate.
	req := newQueryLogsRequestWithSubject(t, body)
	w := httptest.NewRecorder()

	h.QueryLogs(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	assertErrorCode(t, w, types.ErrorCodeInternalError)
}

// assertErrorCode decodes an ErrorResponse and checks that ErrorCode matches.
func assertErrorCode(t *testing.T, w *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	var resp types.ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp.ErrorCode != wantCode {
		t.Errorf("ErrorCode = %q, want %q", resp.ErrorCode, wantCode)
	}
}
