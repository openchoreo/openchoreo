// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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

// helpers -----------------------------------------------------------------------

func validAlertsRequestBody(t *testing.T) io.Reader {
	t.Helper()
	now := time.Now().UTC()
	raw := map[string]any{
		"startTime":   now.Add(-1 * time.Hour).Format(time.RFC3339),
		"endTime":     now.Format(time.RFC3339),
		"searchScope": map[string]any{"namespace": "test-ns"},
	}
	b, err := json.Marshal(raw)
	require.NoError(t, err)
	return bytes.NewReader(b)
}

func validIncidentsRequestBody(t *testing.T) io.Reader {
	t.Helper()
	now := time.Now().UTC()
	raw := map[string]any{
		"startTime":   now.Add(-1 * time.Hour).Format(time.RFC3339),
		"endTime":     now.Format(time.RFC3339),
		"searchScope": map[string]any{"namespace": "test-ns"},
	}
	b, err := json.Marshal(raw)
	require.NoError(t, err)
	return bytes.NewReader(b)
}

// QueryAlerts tests -------------------------------------------------------------

func TestQueryAlerts_Success(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertIncidentService(t)
	svc.On("QueryAlerts", mock.Anything, mock.Anything).Return(&gen.AlertsQueryResponse{}, nil)

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/query", validAlertsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryAlerts(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestQueryAlerts_InvalidBody(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: servicemocks.NewMockAlertIncidentService(t),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/query", bytes.NewReader([]byte("{bad")))
	rr := httptest.NewRecorder()

	h.QueryAlerts(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestQueryAlerts_ValidationError(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: servicemocks.NewMockAlertIncidentService(t),
	}

	// Missing namespace → validation failure.
	raw := map[string]any{
		"startTime":   "2024-01-01T00:00:00Z",
		"endTime":     "2024-01-02T00:00:00Z",
		"searchScope": map[string]any{},
	}
	b, _ := json.Marshal(raw)
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/query", bytes.NewReader(b))
	rr := httptest.NewRecorder()

	h.QueryAlerts(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "VALIDATION_ERROR")
}

func TestQueryAlerts_ServiceNotInitialized(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: nil,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/query", validAlertsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryAlerts(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "SERVICE_NOT_READY")
}

func TestQueryAlerts_AuthzForbidden(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertIncidentService(t)
	svc.On("QueryAlerts", mock.Anything, mock.Anything).Return(nil, observerAuthz.ErrAuthzForbidden)

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/query", validAlertsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryAlerts(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestQueryAlerts_AuthzUnauthorized(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertIncidentService(t)
	svc.On("QueryAlerts", mock.Anything, mock.Anything).Return(nil, observerAuthz.ErrAuthzUnauthorized)

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/query", validAlertsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryAlerts(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestQueryAlerts_AuthzServiceUnavailable(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertIncidentService(t)
	svc.On("QueryAlerts", mock.Anything, mock.Anything).Return(nil, observerAuthz.ErrAuthzServiceUnavailable)

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/query", validAlertsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryAlerts(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
}

func TestQueryAlerts_AuthzTimeout(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertIncidentService(t)
	svc.On("QueryAlerts", mock.Anything, mock.Anything).Return(nil, observerAuthz.ErrAuthzTimeout)

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/query", validAlertsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryAlerts(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
}

func TestQueryAlerts_ScopeNotFound(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("%w: %w", service.ErrAlertsResolveSearchScope, service.ErrScopeNotFound)

	svc := servicemocks.NewMockAlertIncidentService(t)
	svc.On("QueryAlerts", mock.Anything, mock.Anything).Return(nil, err)

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/query", validAlertsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryAlerts(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "SCOPE_NOT_FOUND")
}

func TestQueryAlerts_ResolveScopeFailed(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("%w: infra error", service.ErrAlertsResolveSearchScope)

	svc := servicemocks.NewMockAlertIncidentService(t)
	svc.On("QueryAlerts", mock.Anything, mock.Anything).Return(nil, err)

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/query", validAlertsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryAlerts(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "RESOLVE_SCOPE_FAILED")
}

func TestQueryAlerts_GenericError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertIncidentService(t)
	svc.On("QueryAlerts", mock.Anything, mock.Anything).Return(nil, errors.New("internal server error"))

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/query", validAlertsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryAlerts(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "QUERY_ALERTS_FAILED")
}

// QueryIncidents tests ----------------------------------------------------------

func TestQueryIncidents_Success(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertIncidentService(t)
	svc.On("QueryIncidents", mock.Anything, mock.Anything).Return(&gen.IncidentsQueryResponse{}, nil)

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/incidents/query", validIncidentsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryIncidents(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestQueryIncidents_InvalidBody(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: servicemocks.NewMockAlertIncidentService(t),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/incidents/query", bytes.NewReader([]byte("!!!")))
	rr := httptest.NewRecorder()

	h.QueryIncidents(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestQueryIncidents_ValidationError(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: servicemocks.NewMockAlertIncidentService(t),
	}

	raw := map[string]any{
		"startTime":   "2024-01-01T00:00:00Z",
		"endTime":     "2024-01-02T00:00:00Z",
		"searchScope": map[string]any{}, // missing namespace
	}
	b, _ := json.Marshal(raw)
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/incidents/query", bytes.NewReader(b))
	rr := httptest.NewRecorder()

	h.QueryIncidents(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestQueryIncidents_ServiceNotInitialized(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: nil,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/incidents/query", validIncidentsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryIncidents(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "SERVICE_NOT_READY")
}

func TestQueryIncidents_AuthzForbidden(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertIncidentService(t)
	svc.On("QueryIncidents", mock.Anything, mock.Anything).Return(nil, observerAuthz.ErrAuthzForbidden)

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/incidents/query", validIncidentsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryIncidents(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestQueryIncidents_AuthzUnauthorized(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertIncidentService(t)
	svc.On("QueryIncidents", mock.Anything, mock.Anything).Return(nil, observerAuthz.ErrAuthzUnauthorized)

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/incidents/query", validIncidentsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryIncidents(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestQueryIncidents_GenericError(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertIncidentService(t)
	svc.On("QueryIncidents", mock.Anything, mock.Anything).Return(nil, errors.New("db error"))

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: svc,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/incidents/query", validIncidentsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryIncidents(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "QUERY_INCIDENTS_FAILED")
}
