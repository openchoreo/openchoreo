// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	servicemocks "github.com/openchoreo/openchoreo/internal/observer/service/mocks"
	"github.com/openchoreo/openchoreo/internal/observer/store/incidententry"
)

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestUpdateIncident_Success(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 7, 10, 21, 0, 0, time.UTC)
	triggered := now.Add(-time.Minute)

	respBody := &gen.IncidentPutResponse{
		IncidentId:                    ptrString("inc-1"),
		AlertId:                       ptrString("a-1"),
		Status:                        ptrIncidentPutStatus(gen.IncidentPutResponseStatusAcknowledged),
		Notes:                         ptrString("notes"),
		Description:                   ptrString("desc"),
		IncidentTriggerAiRca:          ptrBool(true),
		IncidentTriggerAiCostAnalysis: ptrBool(false),
		TriggeredAt:                   &triggered,
		AcknowledgedAt:                &now,
	}

	var capturedID string
	var capturedReq gen.IncidentPutRequest

	svc := servicemocks.NewMockAlertIncidentService(t)
	svc.On("UpdateIncident", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			capturedID = args.String(1)
			capturedReq = args.Get(2).(gen.IncidentPutRequest)
		}).
		Return(respBody, nil)

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: svc,
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

	// Assert the mock received the correct ID and request.
	assert.Equal(t, "inc-1", capturedID)
	assert.Equal(t, gen.IncidentPutRequestStatusAcknowledged, capturedReq.Status)
}

func TestUpdateIncident_NotFound(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAlertIncidentService(t)
	svc.On("UpdateIncident", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, incidententry.ErrIncidentNotFound)

	h := &Handler{
		baseHandler:          baseHandler{logger: noopLogger()},
		alertIncidentService: svc,
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
