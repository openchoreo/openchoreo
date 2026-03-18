// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/store/incidententry"
)

// fakeAlertIncidentService implements service.AlertIncidentService for tests.
// Only UpdateIncident is exercised by the incident handler tests; the alert/incident
// query methods are stubs that panic so accidental calls are caught immediately.
type fakeAlertIncidentService struct {
	updateResp *gen.IncidentPutResponse
	updateErr  error

	lastID  string
	lastReq gen.IncidentPutRequest
}

func (f *fakeAlertIncidentService) QueryAlerts(_ context.Context, _ gen.AlertsQueryRequest) (*gen.AlertsQueryResponse, error) {
	panic("unexpected call to QueryAlerts in incident handler test")
}

func (f *fakeAlertIncidentService) QueryIncidents(_ context.Context, _ gen.IncidentsQueryRequest) (*gen.IncidentsQueryResponse, error) {
	panic("unexpected call to QueryIncidents in incident handler test")
}

func (f *fakeAlertIncidentService) UpdateIncident(_ context.Context, id string, req gen.IncidentPutRequest) (*gen.IncidentPutResponse, error) {
	f.lastID = id
	f.lastReq = req
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.updateResp, nil
}

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestUpdateIncident_Success(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 7, 10, 21, 0, 0, time.UTC)
	triggered := now.Add(-time.Minute)

	respBody := &gen.IncidentPutResponse{
		IncidentId:           ptrString("inc-1"),
		AlertId:              ptrString("a-1"),
		Status:               ptrIncidentPutStatus(gen.IncidentPutResponseStatusAcknowledged),
		Notes:                ptrString("notes"),
		Description:          ptrString("desc"),
		IncidentTriggerAiRca: ptrBool(true),
		TriggeredAt:          &triggered,
		AcknowledgedAt:       &now,
	}

	updater := &fakeAlertIncidentService{
		updateResp: respBody,
	}

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: updater,
	}

	body := gen.IncidentPutRequest{
		Status:      gen.IncidentPutRequestStatusAcknowledged,
		Notes:       ptrString("notes"),
		Description: ptrString("desc"),
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err, "failed to marshal request")

	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/incidents/inc-1", bytes.NewReader(raw))
	req.SetPathValue("incidentId", "inc-1")
	rr := httptest.NewRecorder()

	h.UpdateIncident(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	respBytes, err := io.ReadAll(rr.Body)
	require.NoError(t, err, "failed to read response body")
	out := string(respBytes)
	for _, expected := range []string{`"incidentId":"inc-1"`, `"alertId":"a-1"`, `"status":"acknowledged"`} {
		assert.Contains(t, out, expected)
	}

	// Assert the fake updater received the correct ID and request.
	assert.Equal(t, "inc-1", updater.lastID)
	assert.Equal(t, gen.IncidentPutRequestStatusAcknowledged, updater.lastReq.Status)
}

func TestUpdateIncident_NotFound(t *testing.T) {
	t.Parallel()

	updater := &fakeAlertIncidentService{
		updateErr: incidententry.ErrIncidentNotFound,
	}

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: updater,
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/incidents/non-existent", bytes.NewReader([]byte(`{"status":"active"}`)))
	req.SetPathValue("incidentId", "non-existent")
	rr := httptest.NewRecorder()

	h.UpdateIncident(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
}

// Helper functions for tests.

func ptrString(s string) *string { return &s }

func ptrBool(b bool) *bool { return &b }

func ptrIncidentPutStatus(s gen.IncidentPutResponseStatus) *gen.IncidentPutResponseStatus {
	return &s
}
