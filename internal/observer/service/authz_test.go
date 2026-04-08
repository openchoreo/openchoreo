// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	coremocks "github.com/openchoreo/openchoreo/internal/authz/core/mocks"
	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service/mocks"
	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// authedCtx returns a context with a valid SubjectContext set.
func authedCtx() context.Context {
	return auth.SetSubjectContext(context.Background(), &auth.SubjectContext{
		ID:   "test-user",
		Type: "user",
	})
}

// mockPDPAllow creates a PDP mock that allows the request exactly once.
func mockPDPAllow(t *testing.T) *coremocks.MockPDP {
	pdp := coremocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(&authzcore.Decision{Decision: true}, nil).Once()
	return pdp
}

// mockPDPDeny creates a PDP mock that denies the request exactly once.
func mockPDPDeny(t *testing.T) *coremocks.MockPDP {
	pdp := coremocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).Return(&authzcore.Decision{Decision: false}, nil).Once()
	return pdp
}

// --- AlertIncidentService Authz Tests ---

func TestAlertIncidentAuthz_QueryAlerts_NilPDP(t *testing.T) {
	inner := mocks.NewMockAlertIncidentService(t)
	expected := &gen.AlertsQueryResponse{}
	inner.EXPECT().QueryAlerts(mock.Anything, mock.Anything).Return(expected, nil)

	svc := NewAlertIncidentServiceWithAuthz(inner, nil, testLogger())
	req := gen.AlertsQueryRequest{SearchScope: gen.ComponentSearchScope{Namespace: "ns"}}

	resp, err := svc.QueryAlerts(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, expected, resp)
}

func TestAlertIncidentAuthz_QueryAlerts_Allowed(t *testing.T) {
	inner := mocks.NewMockAlertIncidentService(t)
	expected := &gen.AlertsQueryResponse{}
	inner.EXPECT().QueryAlerts(mock.Anything, mock.Anything).Return(expected, nil)

	svc := NewAlertIncidentServiceWithAuthz(inner, mockPDPAllow(t), testLogger())
	req := gen.AlertsQueryRequest{SearchScope: gen.ComponentSearchScope{Namespace: "ns"}}

	resp, err := svc.QueryAlerts(authedCtx(), req)
	require.NoError(t, err)
	assert.Equal(t, expected, resp)
}

func TestAlertIncidentAuthz_QueryAlerts_Denied(t *testing.T) {
	inner := mocks.NewMockAlertIncidentService(t)

	svc := NewAlertIncidentServiceWithAuthz(inner, mockPDPDeny(t), testLogger())
	req := gen.AlertsQueryRequest{SearchScope: gen.ComponentSearchScope{Namespace: "ns"}}

	_, err := svc.QueryAlerts(authedCtx(), req)
	assert.ErrorIs(t, err, observerAuthz.ErrAuthzForbidden)
}

func TestAlertIncidentAuthz_QueryIncidents_NilPDP(t *testing.T) {
	inner := mocks.NewMockAlertIncidentService(t)
	expected := &gen.IncidentsQueryResponse{}
	inner.EXPECT().QueryIncidents(mock.Anything, mock.Anything).Return(expected, nil)

	svc := NewAlertIncidentServiceWithAuthz(inner, nil, testLogger())
	req := gen.IncidentsQueryRequest{SearchScope: gen.ComponentSearchScope{Namespace: "ns"}}

	resp, err := svc.QueryIncidents(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, expected, resp)
}

func TestAlertIncidentAuthz_QueryIncidents_Denied(t *testing.T) {
	inner := mocks.NewMockAlertIncidentService(t)

	svc := NewAlertIncidentServiceWithAuthz(inner, mockPDPDeny(t), testLogger())
	req := gen.IncidentsQueryRequest{SearchScope: gen.ComponentSearchScope{Namespace: "ns"}}

	_, err := svc.QueryIncidents(authedCtx(), req)
	assert.ErrorIs(t, err, observerAuthz.ErrAuthzForbidden)
}

func TestAlertIncidentAuthz_UpdateIncident_NilPDP(t *testing.T) {
	inner := mocks.NewMockAlertIncidentService(t)
	expected := &gen.IncidentPutResponse{}
	inner.EXPECT().UpdateIncident(mock.Anything, "inc-1", mock.Anything).Return(expected, nil)

	svc := NewAlertIncidentServiceWithAuthz(inner, nil, testLogger())

	resp, err := svc.UpdateIncident(context.Background(), "inc-1", gen.IncidentPutRequest{})
	require.NoError(t, err)
	assert.Equal(t, expected, resp)
}

func TestAlertIncidentAuthz_UpdateIncident_Denied(t *testing.T) {
	inner := mocks.NewMockAlertIncidentService(t)

	svc := NewAlertIncidentServiceWithAuthz(inner, mockPDPDeny(t), testLogger())

	_, err := svc.UpdateIncident(authedCtx(), "inc-1", gen.IncidentPutRequest{})
	assert.ErrorIs(t, err, observerAuthz.ErrAuthzForbidden)
}

// --- LogsQuerier Authz Tests ---

func TestLogsAuthz_QueryLogs_NilPDP(t *testing.T) {
	inner := mocks.NewMockLogsQuerier(t)
	expected := &types.LogsQueryResponse{}
	inner.EXPECT().QueryLogs(mock.Anything, mock.Anything).Return(expected, nil)

	svc := NewLogsServiceWithAuthz(inner, nil, testLogger())
	req := &types.LogsQueryRequest{
		SearchScope: &types.SearchScope{
			Component: &types.ComponentSearchScope{
				Namespace: "ns",
				Project:   "proj",
				Component: "comp",
			},
		},
	}

	resp, err := svc.QueryLogs(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, expected, resp)
}

func TestLogsAuthz_QueryLogs_Denied(t *testing.T) {
	inner := mocks.NewMockLogsQuerier(t)

	svc := NewLogsServiceWithAuthz(inner, mockPDPDeny(t), testLogger())
	req := &types.LogsQueryRequest{
		SearchScope: &types.SearchScope{
			Component: &types.ComponentSearchScope{
				Namespace: "ns",
				Project:   "proj",
				Component: "comp",
			},
		},
	}

	_, err := svc.QueryLogs(authedCtx(), req)
	assert.ErrorIs(t, err, observerAuthz.ErrAuthzForbidden)
}

func TestLogsAuthz_QueryLogs_NilRequest(t *testing.T) {
	inner := mocks.NewMockLogsQuerier(t)

	// PDP with no expectation — nil request fails before auth check
	pdp := coremocks.NewMockPDP(t)
	svc := NewLogsServiceWithAuthz(inner, pdp, testLogger())

	_, err := svc.QueryLogs(authedCtx(), nil)
	assert.Error(t, err)
}

// --- MetricsQuerier Authz Tests ---

func TestMetricsAuthz_QueryMetrics_NilPDP(t *testing.T) {
	inner := mocks.NewMockMetricsQuerier(t)
	inner.EXPECT().QueryMetrics(mock.Anything, mock.Anything).Return("result", nil)

	svc := NewMetricsServiceWithAuthz(inner, nil, testLogger())
	req := &types.MetricsQueryRequest{
		SearchScope: types.ComponentSearchScope{
			Namespace: "ns",
			Project:   "proj",
			Component: "comp",
		},
	}

	resp, err := svc.QueryMetrics(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "result", resp)
}

func TestMetricsAuthz_QueryMetrics_Denied(t *testing.T) {
	inner := mocks.NewMockMetricsQuerier(t)

	svc := NewMetricsServiceWithAuthz(inner, mockPDPDeny(t), testLogger())
	req := &types.MetricsQueryRequest{
		SearchScope: types.ComponentSearchScope{
			Namespace: "ns",
			Project:   "proj",
			Component: "comp",
		},
	}

	_, err := svc.QueryMetrics(authedCtx(), req)
	assert.ErrorIs(t, err, observerAuthz.ErrAuthzForbidden)
}

func TestMetricsAuthz_QueryMetrics_NilRequest(t *testing.T) {
	inner := mocks.NewMockMetricsQuerier(t)

	svc := NewMetricsServiceWithAuthz(inner, nil, testLogger())

	_, err := svc.QueryMetrics(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "metrics query request is required")
}

// --- TracesQuerier Authz Tests ---

func TestTracesAuthz_QueryTraces_NilPDP(t *testing.T) {
	inner := mocks.NewMockTracesQuerier(t)
	expected := &types.TracesQueryResponse{}
	inner.EXPECT().QueryTraces(mock.Anything, mock.Anything).Return(expected, nil)

	svc := NewTracesServiceWithAuthz(inner, nil, testLogger())
	req := &types.TracesQueryRequest{
		SearchScope: types.ComponentSearchScope{
			Namespace: "ns",
			Project:   "proj",
			Component: "comp",
		},
	}

	resp, err := svc.QueryTraces(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, expected, resp)
}

func TestTracesAuthz_QueryTraces_Denied(t *testing.T) {
	inner := mocks.NewMockTracesQuerier(t)

	svc := NewTracesServiceWithAuthz(inner, mockPDPDeny(t), testLogger())
	req := &types.TracesQueryRequest{
		SearchScope: types.ComponentSearchScope{
			Namespace: "ns",
			Project:   "proj",
			Component: "comp",
		},
	}

	_, err := svc.QueryTraces(authedCtx(), req)
	assert.ErrorIs(t, err, observerAuthz.ErrAuthzForbidden)
}

func TestTracesAuthz_QuerySpans_NilPDP(t *testing.T) {
	inner := mocks.NewMockTracesQuerier(t)
	expected := &types.SpansQueryResponse{}
	inner.EXPECT().QuerySpans(mock.Anything, "trace-1", mock.Anything).Return(expected, nil)

	svc := NewTracesServiceWithAuthz(inner, nil, testLogger())
	req := &types.TracesQueryRequest{
		SearchScope: types.ComponentSearchScope{
			Namespace: "ns",
			Project:   "proj",
			Component: "comp",
		},
	}

	resp, err := svc.QuerySpans(context.Background(), "trace-1", req)
	require.NoError(t, err)
	assert.Equal(t, expected, resp)
}

func TestTracesAuthz_QuerySpans_Denied(t *testing.T) {
	inner := mocks.NewMockTracesQuerier(t)

	svc := NewTracesServiceWithAuthz(inner, mockPDPDeny(t), testLogger())
	req := &types.TracesQueryRequest{
		SearchScope: types.ComponentSearchScope{
			Namespace: "ns",
			Project:   "proj",
			Component: "comp",
		},
	}

	_, err := svc.QuerySpans(authedCtx(), "trace-1", req)
	assert.ErrorIs(t, err, observerAuthz.ErrAuthzForbidden)
}

func TestTracesAuthz_GetSpanDetails_PassThrough(t *testing.T) {
	inner := mocks.NewMockTracesQuerier(t)
	expected := &types.SpanInfo{SpanID: "span-1"}
	inner.EXPECT().GetSpanDetails(mock.Anything, "trace-1", "span-1").Return(expected, nil)

	// PDP with no Evaluate expectation — testify will fail if Evaluate is called
	pdp := coremocks.NewMockPDP(t)
	svc := NewTracesServiceWithAuthz(inner, pdp, testLogger())

	resp, err := svc.GetSpanDetails(context.Background(), "trace-1", "span-1")
	require.NoError(t, err)
	assert.Equal(t, expected, resp)
}
