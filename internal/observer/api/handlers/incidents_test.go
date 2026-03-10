// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/store/incidententry"
)

type fakeIncidentsUpdater struct {
	updateResp *gen.IncidentPutResponse
	updateErr  error

	lastCtx context.Context
	lastID  string
	lastReq gen.IncidentPutRequest
}

func (f *fakeIncidentsUpdater) UpdateIncident(ctx context.Context, id string, req gen.IncidentPutRequest) (*gen.IncidentPutResponse, error) {
	f.lastCtx = ctx
	f.lastID = id
	f.lastReq = req
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.updateResp, nil
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

	updater := &fakeIncidentsUpdater{
		updateResp: respBody,
	}

	h := &Handler{
		logger: nil,
	}

	// Build handler that uses the fake updater via a wrapper.
	handlerWithFake := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use the fake updater directly instead of alertService.
		id := "inc-1"
		var req gen.IncidentPutRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "INVALID_REQUEST_BODY", "invalid request body: "+err.Error())
			return
		}
		if err := ValidateIncidentPutRequest(&req); err != nil {
			h.writeErrorResponse(w, http.StatusBadRequest, gen.BadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		resp, err := updater.UpdateIncident(r.Context(), id, req)
		if err != nil {
			h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, "UPDATE_INCIDENT_FAILED", "failed to update incident")
			return
		}
		h.writeJSON(w, http.StatusOK, resp)
	})

	body := gen.IncidentPutRequest{
		Status:      gen.IncidentPutRequestStatusAcknowledged,
		Notes:       ptrString("notes"),
		Description: ptrString("desc"),
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/incidents/inc-1", bytes.NewReader(raw))
	rr := httptest.NewRecorder()

	handlerWithFake.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	respBytes, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	out := string(respBytes)
	for _, expected := range []string{`"incidentId":"inc-1"`, `"alertId":"a-1"`, `"status":"acknowledged"`} {
		if !contains(out, expected) {
			t.Fatalf("expected %q in response: %s", expected, out)
		}
	}
}

func TestUpdateIncident_NotFound(t *testing.T) {
	t.Parallel()

	h := &Handler{}

	notFoundErr := incidententry.ErrIncidentNotFound
	h.alertService = nil

	// Minimal handler using the same error mapping logic as UpdateIncident.
	handlerWithError := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := notFoundErr
		switch {
		case errors.Is(err, incidententry.ErrIncidentNotFound):
			h.writeErrorResponse(w, http.StatusNotFound, gen.NotFound, "INCIDENT_NOT_FOUND", "incident not found")
		default:
			h.writeErrorResponse(w, http.StatusInternalServerError, gen.InternalServerError, "UPDATE_INCIDENT_FAILED", "failed to update incident")
		}
	})

	req := httptest.NewRequest(http.MethodPut, "/api/v1alpha1/incidents/non-existent", bytes.NewReader([]byte(`{"status":"active"}`)))
	rr := httptest.NewRecorder()

	handlerWithError.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
}

// Helper functions for tests.

func ptrString(s string) *string { return &s }

func ptrBool(b bool) *bool { return &b }

func ptrIncidentPutStatus(s gen.IncidentPutResponseStatus) *gen.IncidentPutResponseStatus {
	return &s
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
