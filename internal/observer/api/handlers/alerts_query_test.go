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
	rr := serve(t, h, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestQueryAlerts_InvalidBody(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: servicemocks.NewMockAlertIncidentService(t),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/alerts/query", bytes.NewReader([]byte("{bad")))
	rr := serve(t, h, req)

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
	rr := serve(t, h, req)

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
	rr := serve(t, h, req)

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
	rr := serve(t, h, req)

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
	rr := serve(t, h, req)

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
	rr := serve(t, h, req)

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
	rr := serve(t, h, req)

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
	rr := serve(t, h, req)

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
	rr := serve(t, h, req)

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
	rr := serve(t, h, req)

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
	rr := serve(t, h, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestQueryIncidents_InvalidBody(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: servicemocks.NewMockAlertIncidentService(t),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/incidents/query", bytes.NewReader([]byte("!!!")))
	rr := serve(t, h, req)

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
	rr := serve(t, h, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestQueryIncidents_ServiceNotInitialized(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: nil,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/incidents/query", validIncidentsRequestBody(t))
	rr := serve(t, h, req)

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
	rr := serve(t, h, req)

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
	rr := serve(t, h, req)

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
	rr := serve(t, h, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "QUERY_INCIDENTS_FAILED")
}

// nil-response guard -------------------------------------------------------------

// TestPublicAlertHandlers_NilServiceResponse covers the errNilServiceResponse
// branch in the three operations that return generated typed responses.
//
// A service returning (nil, nil) violates the AlertIncidentService contract, so
// this is unreachable in practice — but the generated response types are value
// types, so without the guard each of these would panic on a nil dereference.
func TestPublicAlertHandlers_NilServiceResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mockCall  string
		method    string
		url       string
		body      func(*testing.T) io.Reader
		errorCode string
	}{
		{
			name:      "QueryAlerts",
			mockCall:  "QueryAlerts",
			method:    http.MethodPost,
			url:       "/api/v1alpha1/alerts/query",
			body:      validAlertsRequestBody,
			errorCode: "QUERY_ALERTS_FAILED",
		},
		{
			name:      "QueryIncidents",
			mockCall:  "QueryIncidents",
			method:    http.MethodPost,
			url:       "/api/v1alpha1/incidents/query",
			body:      validIncidentsRequestBody,
			errorCode: "QUERY_INCIDENTS_FAILED",
		},
		{
			name:     "UpdateIncident",
			mockCall: "UpdateIncident",
			method:   http.MethodPut,
			url:      "/api/v1alpha1/incidents/inc-1",
			body: func(t *testing.T) io.Reader {
				t.Helper()
				b, err := json.Marshal(gen.IncidentPutRequest{Status: gen.Acknowledged})
				require.NoError(t, err)
				return bytes.NewReader(b)
			},
			errorCode: "UPDATE_INCIDENT_FAILED",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := servicemocks.NewMockAlertIncidentService(t)
			// Registered at both arities, since UpdateIncident takes an
			// incident ID the two query methods do not.
			svc.On(tc.mockCall, mock.Anything, mock.Anything, mock.Anything).
				Maybe().Return(nil, nil)
			svc.On(tc.mockCall, mock.Anything, mock.Anything).
				Maybe().Return(nil, nil)

			h := &Handler{
				baseHandler:          baseHandler{logger: noopLogger()},
				alertIncidentService: svc,
			}

			rr := serve(t, h, httptest.NewRequest(tc.method, tc.url, tc.body(t)))

			require.Equal(t, http.StatusInternalServerError, rr.Code,
				"a nil service response must be a 500, not a panic")
			assert.Contains(t, rr.Body.String(), tc.errorCode)
			// Without the guard the recovery middleware would turn the nil
			// dereference into a plain-text 500, so the body shape is what
			// separates the guard from the panic path.
			assertErrorResponseShape(t, rr)
		})
	}
}
