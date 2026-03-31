// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

// coverage_gaps_test.go closes the remaining coverage gaps identified after the
// initial test run: UpdateIncident authorization/error paths, additional
// ValidateIncidentsQueryRequest cases, and QuerySpansForTrace validation errors.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/store/incidententry"
)

// UpdateIncident additional error paths (authz, invalid transition, generic, nil service).

func TestUpdateIncident_AuthzForbidden(t *testing.T) {
	t.Parallel()

	updater := &fakeAlertIncidentService{updateErr: observerAuthz.ErrAuthzForbidden}
	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: updater,
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/incidents/inc-1",
		bytes.NewReader([]byte(`{"status":"active"}`)))
	req.SetPathValue("incidentId", "inc-1")
	rr := httptest.NewRecorder()

	h.UpdateIncident(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestUpdateIncident_AuthzUnauthorized(t *testing.T) {
	t.Parallel()

	updater := &fakeAlertIncidentService{updateErr: observerAuthz.ErrAuthzUnauthorized}
	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: updater,
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/incidents/inc-1",
		bytes.NewReader([]byte(`{"status":"active"}`)))
	req.SetPathValue("incidentId", "inc-1")
	rr := httptest.NewRecorder()

	h.UpdateIncident(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestUpdateIncident_AuthzServiceUnavailable(t *testing.T) {
	t.Parallel()

	updater := &fakeAlertIncidentService{updateErr: observerAuthz.ErrAuthzServiceUnavailable}
	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: updater,
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/incidents/inc-1",
		bytes.NewReader([]byte(`{"status":"active"}`)))
	req.SetPathValue("incidentId", "inc-1")
	rr := httptest.NewRecorder()

	h.UpdateIncident(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
}

func TestUpdateIncident_InvalidStatusTransition(t *testing.T) {
	t.Parallel()

	updater := &fakeAlertIncidentService{updateErr: incidententry.ErrInvalidStatusTransition}
	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: updater,
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/incidents/inc-1",
		bytes.NewReader([]byte(`{"status":"active"}`)))
	req.SetPathValue("incidentId", "inc-1")
	rr := httptest.NewRecorder()

	h.UpdateIncident(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "INVALID_STATUS_TRANSITION")
}

func TestUpdateIncident_GenericError(t *testing.T) {
	t.Parallel()

	updater := &fakeAlertIncidentService{updateErr: fmt.Errorf("db timeout")}
	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: updater,
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/incidents/inc-1",
		bytes.NewReader([]byte(`{"status":"active"}`)))
	req.SetPathValue("incidentId", "inc-1")
	rr := httptest.NewRecorder()

	h.UpdateIncident(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "UPDATE_INCIDENT_FAILED")
}

func TestUpdateIncident_ServiceNotInitialized(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: nil,
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/incidents/inc-1",
		bytes.NewReader([]byte(`{"status":"active"}`)))
	req.SetPathValue("incidentId", "inc-1")
	rr := httptest.NewRecorder()

	h.UpdateIncident(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "SERVICE_NOT_READY")
}

func TestUpdateIncident_EmptyIncidentID(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: &fakeAlertIncidentService{},
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/incidents/",
		bytes.NewReader([]byte(`{"status":"active"}`)))
	req.SetPathValue("incidentId", "   ") // whitespace only
	rr := httptest.NewRecorder()

	h.UpdateIncident(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "INVALID_INCIDENT_ID")
}

// ValidateIncidentsQueryRequest additional cases.

func TestValidateIncidentsQueryRequest_ComponentWithoutProject(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	comp := "comp"
	req := &gen.IncidentsQueryRequest{
		StartTime: now.Add(-time.Hour),
		EndTime:   now,
		SearchScope: gen.ComponentSearchScope{
			Namespace: "ns",
			Component: &comp,
		},
	}

	err := ValidateIncidentsQueryRequest(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project is required when")
}

func TestValidateIncidentsQueryRequest_BadLimit(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	lim := -1
	req := &gen.IncidentsQueryRequest{
		StartTime:   now.Add(-time.Hour),
		EndTime:     now,
		SearchScope: gen.ComponentSearchScope{Namespace: "ns"},
		Limit:       &lim,
	}

	err := ValidateIncidentsQueryRequest(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "positive integer")
}

func TestValidateIncidentsQueryRequest_InvalidSortOrder(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	order := gen.IncidentsQueryRequestSortOrder("random")
	req := &gen.IncidentsQueryRequest{
		StartTime:   now.Add(-time.Hour),
		EndTime:     now,
		SearchScope: gen.ComponentSearchScope{Namespace: "ns"},
		SortOrder:   &order,
	}

	err := ValidateIncidentsQueryRequest(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sortOrder must be either")
}

// UpdateIncident: invalid JSON body and validation error paths.

func TestUpdateIncident_InvalidBody(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: &fakeAlertIncidentService{},
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/incidents/inc-1",
		bytes.NewReader([]byte("{bad json")))
	req.SetPathValue("incidentId", "inc-1")
	rr := httptest.NewRecorder()

	h.UpdateIncident(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "INVALID_REQUEST_BODY")
}

func TestUpdateIncident_ValidationError(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: &fakeAlertIncidentService{},
	}

	// status "pending" is not a valid value → ValidateIncidentPutRequest returns error.
	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/incidents/inc-1",
		bytes.NewReader([]byte(`{"status":"pending"}`)))
	req.SetPathValue("incidentId", "inc-1")
	rr := httptest.NewRecorder()

	h.UpdateIncident(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "VALIDATION_ERROR")
}

// UpdateAlertRule: invalid JSON body path.

func TestUpdateAlertRule_InvalidBody(t *testing.T) {
	t.Parallel()

	h := newInternalHandler(&fakeAlertRuleService{})
	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/alerts/sources/log/rules/r1",
		bytes.NewReader([]byte("{bad")))
	req.SetPathValue("sourceType", "log")
	req.SetPathValue("ruleName", "r1")
	rr := httptest.NewRecorder()

	h.UpdateAlertRule(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "INVALID_REQUEST_BODY")
}

func TestUpdateAlertRule_ValidationError(t *testing.T) {
	t.Parallel()

	h := newInternalHandler(&fakeAlertRuleService{})
	// Missing metadata.name → validation error.
	raw, _ := json.Marshal(map[string]any{
		"metadata":  map[string]any{"name": ""},
		"source":    map[string]any{"type": "log"},
		"condition": map[string]any{},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/alerts/sources/log/rules/r1",
		bytes.NewReader(raw))
	req.SetPathValue("sourceType", "log")
	req.SetPathValue("ruleName", "r1")
	rr := httptest.NewRecorder()

	h.UpdateAlertRule(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "VALIDATION_ERROR")
}

// QuerySpansForTrace: validation error path (already covered by scope_auth but missing
// the body-binding error).

func TestQuerySpansForTrace_InvalidBody(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: &fakeTracesQuerier{},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/trace-1/spans/query",
		bytes.NewReader([]byte("{bad")))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("traceId", "trace-1")
	rr := httptest.NewRecorder()

	h.QuerySpansForTrace(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestQuerySpansForTrace_ValidationError(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:   baseHandler{logger: noopLogger()},
		tracesService: &fakeTracesQuerier{},
	}

	raw, _ := json.Marshal(map[string]any{
		"startTime":   "2024-01-01T00:00:00Z",
		"endTime":     "2024-01-02T00:00:00Z",
		"searchScope": map[string]any{}, // missing namespace
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/traces/trace-1/spans/query",
		bytes.NewReader(raw))
	req.SetPathValue("traceId", "trace-1")
	rr := httptest.NewRecorder()

	h.QuerySpansForTrace(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// QueryIncidents: authz service unavailable path.

func TestQueryIncidents_AuthzServiceUnavailable(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: &configAlertIncidentService{incidentsErr: observerAuthz.ErrAuthzServiceUnavailable},
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/incidents/query", validIncidentsRequestBody(t))
	rr := httptest.NewRecorder()

	h.QueryIncidents(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
}
