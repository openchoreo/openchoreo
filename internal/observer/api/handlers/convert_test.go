// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	servicemocks "github.com/openchoreo/openchoreo/internal/observer/service/mocks"
)

// TestAbsentStartTimeKeepsRequiredMessage is the guard for the zero-time trap in
// the gen -> types adapters, and it covers all four bodies that have one.
//
// The generated time fields have no omitempty, so an absent startTime decodes to
// the zero time rather than being missing. Formatting that naively yields
// "0001-01-01T00:00:00Z", which parses fine — so ValidateTimeRange's
// "startTime is required" would never fire and the caller would instead get
// "query time range cannot exceed N days", a different and misleading error for
// the same mistake.
//
// rfc3339OrEmpty maps the zero time to "" so the required-field message wins.
// Without that mapping every case below still returns 400, so only asserting the
// status would not catch the regression — the message is the assertion that
// matters.
func TestAbsentStartTimeKeepsRequiredMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		body string
		// handler wires only the service the operation needs.
		handler func(t *testing.T) *Handler
	}{
		{
			name: "logs",
			url:  "/api/v1/logs/query",
			body: `{"searchScope":{"namespace":"ns"},"endTime":"2024-01-02T00:00:00Z"}`,
			handler: func(t *testing.T) *Handler {
				t.Helper()
				return &Handler{
					baseHandler: baseHandler{logger: noopLogger()},
					logsService: servicemocks.NewMockLogsQuerier(t),
				}
			},
		},
		{
			name: "events",
			url:  "/api/v1/events/query",
			body: `{"searchScope":{"namespace":"ns"},"endTime":"2024-01-02T00:00:00Z"}`,
			handler: func(t *testing.T) *Handler {
				t.Helper()
				return &Handler{
					baseHandler:   baseHandler{logger: noopLogger()},
					eventsService: servicemocks.NewMockEventsQuerier(t),
				}
			},
		},
		{
			name: "metrics",
			url:  "/api/v1/metrics/query",
			body: `{"metric":"resource","searchScope":{"namespace":"ns"},"endTime":"2024-01-02T00:00:00Z"}`,
			handler: func(t *testing.T) *Handler {
				t.Helper()
				return &Handler{
					baseHandler:    baseHandler{logger: noopLogger()},
					metricsService: servicemocks.NewMockMetricsQuerier(t),
				}
			},
		},
		{
			name: "runtime-topology",
			url:  "/api/v1alpha1/metrics/runtime-topology",
			body: `{"searchScope":{"namespace":"ns","project":"p","environment":"e"},` +
				`"endTime":"2024-01-02T00:00:00Z"}`,
			handler: func(t *testing.T) *Handler {
				t.Helper()
				return &Handler{
					baseHandler:    baseHandler{logger: noopLogger()},
					metricsService: servicemocks.NewMockMetricsQuerier(t),
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rr := serve(t, tc.handler(t),
				httptest.NewRequest(http.MethodPost, tc.url, strings.NewReader(tc.body)))

			require.Equal(t, http.StatusBadRequest, rr.Code)
			assert.Contains(t, rr.Body.String(), "startTime is required",
				"an absent startTime must keep its original message, not become a "+
					"time-range error; check rfc3339OrEmpty's zero handling")
		})
	}
}

// TestNilRequestBodyGuards covers the `request.Body == nil` branch in every
// handler that has one.
//
// These are unreachable through the generated router: for a required request
// body the strict layer always sets Body, and a body that fails to decode is
// rejected by StrictRequestErrorHandler before the handler runs. They exist so a
// nil Body cannot panic on dereference, and are called directly here because the
// router cannot produce this input.
func TestNilRequestBodyGuards(t *testing.T) {
	t.Parallel()

	h := &Handler{baseHandler: baseHandler{logger: noopLogger()}}
	ctx := context.Background()

	checks := []struct {
		name string
		call func() (int, error)
	}{
		{"QueryLogs", func() (int, error) {
			resp, err := h.QueryLogs(ctx, gen.QueryLogsRequestObject{})
			return recordStatus(t, resp.VisitQueryLogsResponse), err
		}},
		{"QueryEvents", func() (int, error) {
			resp, err := h.QueryEvents(ctx, gen.QueryEventsRequestObject{})
			return recordStatus(t, resp.VisitQueryEventsResponse), err
		}},
		{"QueryMetrics", func() (int, error) {
			resp, err := h.QueryMetrics(ctx, gen.QueryMetricsRequestObject{})
			return recordStatus(t, resp.VisitQueryMetricsResponse), err
		}},
		{"QueryRuntimeTopology", func() (int, error) {
			resp, err := h.QueryRuntimeTopology(ctx, gen.QueryRuntimeTopologyRequestObject{})
			return recordStatus(t, resp.VisitQueryRuntimeTopologyResponse), err
		}},
		{"QueryTraces", func() (int, error) {
			resp, err := h.QueryTraces(ctx, gen.QueryTracesRequestObject{})
			return recordStatus(t, resp.VisitQueryTracesResponse), err
		}},
		{"QuerySpansForTrace", func() (int, error) {
			resp, err := h.QuerySpansForTrace(ctx, gen.QuerySpansForTraceRequestObject{})
			return recordStatus(t, resp.VisitQuerySpansForTraceResponse), err
		}},
		{"QueryAlerts", func() (int, error) {
			resp, err := h.QueryAlerts(ctx, gen.QueryAlertsRequestObject{})
			return recordStatus(t, resp.VisitQueryAlertsResponse), err
		}},
		{"QueryIncidents", func() (int, error) {
			resp, err := h.QueryIncidents(ctx, gen.QueryIncidentsRequestObject{})
			return recordStatus(t, resp.VisitQueryIncidentsResponse), err
		}},
		{"UpdateIncident", func() (int, error) {
			resp, err := h.UpdateIncident(ctx, gen.UpdateIncidentRequestObject{IncidentId: "inc-1"})
			return recordStatus(t, resp.VisitUpdateIncidentResponse), err
		}},
	}

	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			status, err := tc.call()
			require.NoError(t, err)
			assert.Equal(t, http.StatusBadRequest, status,
				"a nil request body must be a 400, not a panic")
		})
	}
}

// recordStatus runs a Visit*Response writer and returns the status it wrote.
func recordStatus(t *testing.T, visit func(http.ResponseWriter) error) int {
	t.Helper()
	rr := httptest.NewRecorder()
	require.NoError(t, visit(rr))
	return rr.Code
}

// TestRFC3339OrEmpty pins the two behaviors the adapters depend on.
func TestRFC3339OrEmpty(t *testing.T) {
	t.Parallel()

	t.Run("zero maps to empty", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "", rfc3339OrEmpty(time.Time{}))
	})

	t.Run("sub-second precision survives", func(t *testing.T) {
		t.Parallel()
		// RFC3339 (as opposed to RFC3339Nano) would truncate the .5, silently
		// moving a query window boundary.
		ts := time.Date(2024, 1, 1, 0, 0, 0, 500_000_000, time.UTC)
		assert.Equal(t, "2024-01-01T00:00:00.5Z", rfc3339OrEmpty(ts))
	})
}

// TestSearchScopeMixedOneOfStillRejected covers the hand-written oneOf
// discrimination the adapters rely on.
//
// types.SearchScope.UnmarshalJSON rejects a searchScope that mixes
// workflowRunName with component-scope fields. The generated wrapper holds the
// scope as raw JSON and performs no such check, so this only works because
// remarshalJSON re-decodes through the hand-written type rather than mapping
// fields.
func TestSearchScopeMixedOneOfStillRejected(t *testing.T) {
	t.Parallel()

	h := &Handler{
		baseHandler: baseHandler{logger: noopLogger()},
		logsService: servicemocks.NewMockLogsQuerier(t),
	}

	body := `{"searchScope":{"namespace":"ns","workflowRunName":"wf","component":"c"},` +
		`"startTime":"2024-01-01T00:00:00Z","endTime":"2024-01-02T00:00:00Z"}`

	rr := serve(t, h, httptest.NewRequest(http.MethodPost, "/api/v1/logs/query",
		strings.NewReader(body)))

	assert.Equal(t, http.StatusBadRequest, rr.Code,
		"a searchScope mixing workflowRunName with component fields must be rejected")
}
