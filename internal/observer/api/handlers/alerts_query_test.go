// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
)

// configAlertIncidentService is a fully configurable fake that implements
// service.AlertIncidentService. Unlike fakeAlertIncidentService in incidents_test.go,
// this one does not panic on QueryAlerts/QueryIncidents.
type configAlertIncidentService struct {
	alertsResp    *gen.AlertsQueryResponse
	alertsErr     error
	incidentsResp *gen.IncidentsQueryResponse
	incidentsErr  error
	updateResp    *gen.IncidentPutResponse
	updateErr     error
}

func (f *configAlertIncidentService) QueryAlerts(_ context.Context, _ gen.AlertsQueryRequest) (*gen.AlertsQueryResponse, error) {
	return f.alertsResp, f.alertsErr
}

func (f *configAlertIncidentService) QueryIncidents(_ context.Context, _ gen.IncidentsQueryRequest) (*gen.IncidentsQueryResponse, error) {
	return f.incidentsResp, f.incidentsErr
}

func (f *configAlertIncidentService) UpdateIncident(_ context.Context, _ string, _ gen.IncidentPutRequest) (*gen.IncidentPutResponse, error) {
	return f.updateResp, f.updateErr
}

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

	svc := &configAlertIncidentService{alertsResp: &gen.AlertsQueryResponse{}}
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
		alertIncidentService: &configAlertIncidentService{},
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
		alertIncidentService: &configAlertIncidentService{},
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

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: &configAlertIncidentService{alertsErr: observerAuthz.ErrAuthzForbidden},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/query", validAlertsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryAlerts(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestQueryAlerts_AuthzUnauthorized(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: &configAlertIncidentService{alertsErr: observerAuthz.ErrAuthzUnauthorized},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/query", validAlertsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryAlerts(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestQueryAlerts_AuthzServiceUnavailable(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: &configAlertIncidentService{alertsErr: observerAuthz.ErrAuthzServiceUnavailable},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/query", validAlertsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryAlerts(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
}

func TestQueryAlerts_AuthzTimeout(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: &configAlertIncidentService{alertsErr: observerAuthz.ErrAuthzTimeout},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/query", validAlertsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryAlerts(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
}

func TestQueryAlerts_ScopeNotFound(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("%w: %w", service.ErrAlertsResolveSearchScope, service.ErrScopeNotFound)
	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: &configAlertIncidentService{alertsErr: err},
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
	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: &configAlertIncidentService{alertsErr: err},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/query", validAlertsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryAlerts(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "RESOLVE_SCOPE_FAILED")
}

func TestQueryAlerts_GenericError(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: &configAlertIncidentService{alertsErr: errors.New("boom")},
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

	svc := &configAlertIncidentService{incidentsResp: &gen.IncidentsQueryResponse{}}
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
		alertIncidentService: &configAlertIncidentService{},
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
		alertIncidentService: &configAlertIncidentService{},
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

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: &configAlertIncidentService{incidentsErr: observerAuthz.ErrAuthzForbidden},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/incidents/query", validIncidentsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryIncidents(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestQueryIncidents_AuthzUnauthorized(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: &configAlertIncidentService{incidentsErr: observerAuthz.ErrAuthzUnauthorized},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/incidents/query", validIncidentsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryIncidents(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestQueryIncidents_GenericError(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: &configAlertIncidentService{incidentsErr: errors.New("db error")},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/incidents/query", validIncidentsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryIncidents(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "QUERY_INCIDENTS_FAILED")
}
