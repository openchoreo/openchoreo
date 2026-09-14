// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service"
	servicemocks "github.com/openchoreo/openchoreo/internal/observer/service/mocks"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

const (
	auditLogsWindow  = `"startTime":"2026-08-14T16:30:00Z","endTime":"2026-08-14T17:30:00Z"`
	auditLogsBody    = `{` + auditLogsWindow + `}`
	filterValuesBody = `{"query":{` + auditLogsWindow + `},"filter":"actor.id"}`
)

func auditLogsHandler(t *testing.T, svc service.AuditLogsQuerier) *Handler {
	t.Helper()
	return &Handler{
		baseHandler:      baseHandler{logger: noopLogger()},
		auditLogsService: svc,
	}
}

func postAuditLogs(t *testing.T, h *Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	return serve(t, h, req)
}

func queryAuditLogs(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	return postAuditLogs(t, h, "/api/v1alpha1/audit-logs/query", body)
}

func queryFilterValues(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	return postAuditLogs(t, h, "/api/v1alpha1/audit-logs/filter-values", body)
}

// Pins the request mapping end to end, including that the nested groups arrive
// nested.
func TestQueryAuditLogs_FiltersReachTheService(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAuditLogsQuerier(t)
	var got *types.AuditLogsQueryRequest
	svc.EXPECT().QueryAuditLogs(mock.Anything, mock.Anything).
		Run(func(_ context.Context, req *types.AuditLogsQueryRequest) { got = req }).
		Return(&types.AuditLogsResponse{Records: []types.AuditLogRecord{}}, nil)

	body := `{` + auditLogsWindow + `,
		"actor":{"id":["alice@example.com"],"issuer":["https://idp.example.com"],
		         "session_id":["a3f9"],"entitlements":["platform-engineer"]},
		"resource":{"namespace":["default"],"environment":["default/production"]},
		"result":["denied"],"surface":["rest"],"operation_id":["CreateProject"],
		"request_id":["req-1"],"event_id":["evt-1"],"source_ip":["10.42.0.7"],
		"user_agent":["occ/1.2.0"],"includeTimeline":true,"timelineInterval":"15m"}`

	rr := queryAuditLogs(t, auditLogsHandler(t, svc), body)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	require.NotNil(t, got)
	assert.Equal(t, []string{"alice@example.com"}, got.Actor.IDs)
	assert.Equal(t, []string{"https://idp.example.com"}, got.Actor.Issuers)
	assert.Equal(t, []string{"a3f9"}, got.Actor.SessionIDs)
	assert.Equal(t, []string{"platform-engineer"}, got.Actor.Entitlements)
	assert.Equal(t, []string{"default"}, got.Resource.Namespaces)
	assert.Equal(t, []string{"default/production"}, got.Resource.Environments)
	assert.Equal(t, []string{"denied"}, got.Results)
	assert.Equal(t, []string{"rest"}, got.Surfaces)
	assert.Equal(t, []string{"CreateProject"}, got.OperationIDs)
	assert.Equal(t, []string{"req-1"}, got.RequestIDs)
	assert.Equal(t, []string{"evt-1"}, got.EventIDs)
	assert.Equal(t, []string{"10.42.0.7"}, got.SourceIPs)
	assert.Equal(t, []string{"occ/1.2.0"}, got.UserAgents)
	assert.True(t, got.IncludeTimeline)
	assert.Equal(t, "15m", got.TimelineInterval)
}

func TestQueryAuditLogs_ValidationRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "missing window", body: `{}`},
		{
			name: "too many filter values",
			body: `{` + auditLogsWindow + `,"action":[` +
				`"a1","a2","a3","a4","a5","a6","a7","a8","a9","a10",` +
				`"a11","a12","a13","a14","a15","a16","a17","a18","a19","a20","a21"]}`,
		},
		{
			name: "duplicate filter value",
			body: `{` + auditLogsWindow + `,"action":["create_project","create_project"]}`,
		},
		{
			name: "unknown category is a 400, not a filter that matches nothing",
			body: `{` + auditLogsWindow + `,"category":["bogus"]}`,
		},
		{name: "unknown result", body: `{` + auditLogsWindow + `,"result":["maybe"]}`},
		{name: "unknown surface", body: `{` + auditLogsWindow + `,"surface":["api"]}`},
		{
			name: "malformed timeline interval",
			body: `{` + auditLogsWindow + `,"includeTimeline":true,"timelineInterval":"15x"}`,
		},
		{
			// endTime is exclusive, so an equal pair selects nothing. A 400
			// says so rather than answering with an empty page.
			name: "empty window",
			body: `{"startTime":"2026-08-14T16:30:00Z","endTime":"2026-08-14T16:30:00Z"}`,
		},
		{
			// actor.type accepts 4 values, not the 20 most filters take.
			name: "too many values for a narrow filter",
			body: `{` + auditLogsWindow + `,"actor":{"type":["a","b","c","d","e"]}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := servicemocks.NewMockAuditLogsQuerier(t)
			rr := queryAuditLogs(t, auditLogsHandler(t, svc), tt.body)
			assert.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
			svc.AssertNotCalled(t, "QueryAuditLogs", mock.Anything, mock.Anything)
		})
	}
}

// The audit window is not bounded by the 30-day constant the log endpoints
// share.
func TestQueryAuditLogs_WindowCapIsAuditsOwn(t *testing.T) {
	t.Parallel()

	t.Run("a quarter is accepted", func(t *testing.T) {
		t.Parallel()

		svc := servicemocks.NewMockAuditLogsQuerier(t)
		svc.EXPECT().QueryAuditLogs(mock.Anything, mock.Anything).
			Return(&types.AuditLogsResponse{}, nil)

		rr := queryAuditLogs(t, auditLogsHandler(t, svc),
			`{"startTime":"2026-01-01T00:00:00Z","endTime":"2026-04-01T00:00:00Z"}`)
		assert.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	})

	t.Run("beyond a year is rejected", func(t *testing.T) {
		t.Parallel()

		svc := servicemocks.NewMockAuditLogsQuerier(t)
		rr := queryAuditLogs(t, auditLogsHandler(t, svc),
			`{"startTime":"2024-01-01T00:00:00Z","endTime":"2026-01-01T00:00:00Z"}`)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		svc.AssertNotCalled(t, "QueryAuditLogs", mock.Anything, mock.Anything)
	})
}

func TestQueryAuditLogFilterValues_ValidationRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{
			name: "unknown filter",
			body: `{"query":{` + auditLogsWindow + `},"filter":"actor.nickname"}`,
		},
		{
			name: "event_id is deliberately not offered",
			body: `{"query":{` + auditLogsWindow + `},"filter":"event_id"}`,
		},
		{
			name: "the nested query is validated by the same rules",
			body: `{"query":{` + auditLogsWindow + `,"category":["bogus"]},"filter":"actor.id"}`,
		},
		{name: "missing window in the nested query", body: `{"query":{},"filter":"actor.id"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := servicemocks.NewMockAuditLogsQuerier(t)
			rr := queryFilterValues(t, auditLogsHandler(t, svc), tt.body)
			assert.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
			svc.AssertNotCalled(t, "QueryAuditLogFilterValues", mock.Anything, mock.Anything)
		})
	}
}

func TestQueryAuditLogFilterValues_Succeeds(t *testing.T) {
	t.Parallel()

	svc := servicemocks.NewMockAuditLogsQuerier(t)
	var got *types.AuditLogFilterValuesRequest
	svc.EXPECT().QueryAuditLogFilterValues(mock.Anything, mock.Anything).
		Run(func(_ context.Context, req *types.AuditLogFilterValuesRequest) { got = req }).
		Return(&types.AuditLogFilterValuesResponse{
			Filter: "actor.id", Values: []types.AuditLogFilterValue{}, TotalValues: 0,
		}, nil)

	body := `{"query":{` + auditLogsWindow + `,"resource":{"namespace":["default"]}},
		"filter":"actor.id","valueSearch":"ali","maxValues":25}`
	rr := queryFilterValues(t, auditLogsHandler(t, svc), body)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	require.NotNil(t, got)
	assert.Equal(t, "actor.id", got.Filter)
	assert.Equal(t, "ali", got.ValueSearch)
	assert.Equal(t, 25, got.MaxValues)
	assert.Equal(t, []string{"default"}, got.Query.Resource.Namespaces)
}

// A picker repopulates on every keystroke, and the response reports what the cap
// left out, so maxValues is clamped into range instead of rejected.
func TestQueryAuditLogFilterValues_ClampsMaxValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		sent string
		want int
	}{
		{name: "absent falls back to the default", sent: "", want: defaultAuditLogsMaxValues},
		{name: "zero is absent", sent: `,"maxValues":0`, want: defaultAuditLogsMaxValues},
		{name: "negative is absent", sent: `,"maxValues":-5`, want: defaultAuditLogsMaxValues},
		{name: "within range passes through", sent: `,"maxValues":25`, want: 25},
		{name: "over the cap clamps", sent: `,"maxValues":5000`, want: maxAuditLogsMaxValues},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := servicemocks.NewMockAuditLogsQuerier(t)
			var got *types.AuditLogFilterValuesRequest
			svc.EXPECT().QueryAuditLogFilterValues(mock.Anything, mock.Anything).
				Run(func(_ context.Context, req *types.AuditLogFilterValuesRequest) { got = req }).
				Return(&types.AuditLogFilterValuesResponse{Filter: "action"}, nil)

			rr := queryFilterValues(t, auditLogsHandler(t, svc),
				`{"query":{`+auditLogsWindow+`},"filter":"action"`+tt.sent+`}`)
			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			require.NotNil(t, got)
			assert.Equal(t, tt.want, got.MaxValues)
		})
	}
}

func TestAuditLogs_ErrorMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantCode int
		wantBody string
	}{
		{name: "forbidden", err: observerAuthz.ErrAuthzForbidden, wantCode: http.StatusForbidden},
		{
			name:     "unauthorized",
			err:      observerAuthz.ErrAuthzUnauthorized,
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "adapter does not serve the trail",
			err:      service.ErrAuditLogsNotSupported,
			wantCode: http.StatusNotImplemented,
			wantBody: types.ErrorCodeV1AuditLogsNotSupported,
		},
		{
			// Distinct from the above: records may work while aggregation does not.
			name:     "adapter serves records but cannot aggregate",
			err:      service.ErrAuditLogFilterValuesNotSupported,
			wantCode: http.StatusNotImplemented,
			wantBody: types.ErrorCodeV1AuditLogsFilterValuesNotSupported,
		},
		{
			name:     "retrieval failure",
			err:      fmt.Errorf("%w: boom", service.ErrAuditLogsRetrieval),
			wantCode: http.StatusInternalServerError,
			wantBody: types.ErrorCodeV1AuditLogsRetrievalFailed,
		},
		{
			name:     "unclassified failure",
			err:      errors.New("boom"),
			wantCode: http.StatusInternalServerError,
			wantBody: types.ErrorCodeV1AuditLogsInternalGeneric,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := servicemocks.NewMockAuditLogsQuerier(t)
			svc.EXPECT().QueryAuditLogs(mock.Anything, mock.Anything).Return(nil, tt.err)

			rr := queryAuditLogs(t, auditLogsHandler(t, svc), auditLogsBody)

			assert.Equal(t, tt.wantCode, rr.Code)
			if tt.wantBody != "" {
				assert.Contains(t, rr.Body.String(), tt.wantBody)
			}
		})
	}
}

// The state the endpoints sit in before main.go constructs the authz-wrapped
// service.
func TestAuditLogs_ServiceNotInitialized(t *testing.T) {
	t.Parallel()

	rr := queryAuditLogs(t, auditLogsHandler(t, nil), auditLogsBody)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1AuditLogsServiceNotReady)

	rr = queryFilterValues(t, auditLogsHandler(t, nil), filterValuesBody)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), types.ErrorCodeV1AuditLogsServiceNotReady)
}
