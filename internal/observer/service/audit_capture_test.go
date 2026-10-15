// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/observer/service/mocks"
	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

const (
	leakLogLine        = "DB_PASSWORD=hunter2-should-not-leak"
	leakSpanAttribute  = "SELECT * FROM users WHERE token='should-not-leak'"
	captureTestTraceID = "trace-1"
)

func auditedCtx(ctx context.Context) (context.Context, *audit.AuditData) {
	return audit.NewAuditContext(ctx, &audit.Resource{}, audit.RequestInfo{})
}

// publishedRecord renders what the emitter would publish for a successful op.
func publishedRecord(t *testing.T, ctx context.Context, data *audit.AuditData, opID string) string {
	t.Helper()
	policies, errs := audit.NewPolicySet(coreconfig.NewPath("audit"), audit.Settings{Publish: true}, nil)
	require.Empty(t, errs)
	var buf bytes.Buffer
	emitter, err := audit.NewEmitter("observer", policies, audit.NewLogger(&buf))
	require.NoError(t, err)

	op := &audit.Operation{ID: opID, Action: "read", Category: audit.CategoryAccess}
	audit.EmitFromContext(ctx, emitter, "", op, audit.SurfaceREST, audit.ResultSuccess, data, http.Header{}, "")
	return buf.String()
}

func TestQueryAuditLogs_RecordsFiltersAndCount(t *testing.T) {
	inner := mocks.NewMockAuditLogsQuerier(t)
	inner.EXPECT().QueryAuditLogs(mock.Anything, mock.Anything).Return(&types.AuditLogsResponse{
		Records: []types.AuditLogRecord{{EventID: "e1"}, {EventID: "e2"}},
		Total:   40,
	}, nil)
	svc := NewAuditLogsServiceWithAuthz(inner, mockPDPAllow(t), testLogger())

	ctx, data := auditedCtx(authedCtx())
	_, err := svc.QueryAuditLogs(ctx, &types.AuditLogsQueryRequest{
		StartTime:    "2026-10-01T00:00:00Z",
		EndTime:      "2026-10-02T00:00:00Z",
		Actor:        types.AuditLogsActorFilter{IDs: []string{"alice"}},
		Returned:     types.AuditLogsReturnedFilter{Items: types.AuditLogsReturnedItemsFilter{Names: []string{"db-creds"}}},
		Categories:   []string{"access"},
		SearchPhrase: "rotate",
		Limit:        50,
	})
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"startTime":    "2026-10-01T00:00:00Z",
		"endTime":      "2026-10-02T00:00:00Z",
		"actor":        map[string]any{"id": []string{"alice"}},
		"returned":     map[string]any{"items": map[string]any{"name": []string{"db-creds"}}},
		"category":     []string{"access"},
		"searchPhrase": "rotate",
	}, data.Parameters)

	require.NotNil(t, data.Returned)
	// The entries handed back, not every match in the window.
	assert.Equal(t, audit.Returned{Count: 2}, *data.Returned)
}

func TestQueryAuditLogs_DeniedRecordsRequestOnly(t *testing.T) {
	inner := mocks.NewMockAuditLogsQuerier(t)
	svc := NewAuditLogsServiceWithAuthz(inner, mockPDPDeny(t), testLogger())

	ctx, data := auditedCtx(authedCtx())
	_, err := svc.QueryAuditLogs(ctx, &types.AuditLogsQueryRequest{StartTime: "s", EndTime: "e"})
	require.Error(t, err)

	require.NotNil(t, data.Parameters)
	assert.Nil(t, data.Returned)
}

func TestQueryLogs_RecordsScopeSearchAndCount(t *testing.T) {
	inner := mocks.NewMockLogsQuerier(t)
	inner.EXPECT().QueryLogs(mock.Anything, mock.Anything).Return(&types.LogsQueryResponse{
		Logs: []types.LogEntry{{Log: leakLogLine}, {Log: "second"}, {Log: "third"}},
	}, nil)
	svc := NewLogsServiceWithAuthz(inner, mockPDPAllow(t), testLogger())

	ctx, data := auditedCtx(authedCtx())
	_, err := svc.QueryLogs(ctx, &types.LogsQueryRequest{
		SearchScope: &types.SearchScope{Workflow: &types.WorkflowSearchScope{
			Namespace: "acme", WorkflowRunName: "build-42",
		}},
		StartTime:    "2026-10-01T00:00:00Z",
		EndTime:      "2026-10-02T00:00:00Z",
		SearchPhrase: "password",
		LogLevels:    []string{"ERROR"},
	})
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"startTime":    "2026-10-01T00:00:00Z",
		"endTime":      "2026-10-02T00:00:00Z",
		"searchScope":  map[string]any{"namespace": "acme", "workflowRunName": "build-42"},
		"searchPhrase": "password",
		"logLevels":    []string{"ERROR"},
	}, data.Parameters)
	assert.Equal(t, audit.Returned{Count: 3}, *data.Returned)

	record := publishedRecord(t, ctx, data, "QueryLogs")
	assert.Contains(t, record, `"returned":{"count":3}`)
	assert.NotContains(t, record, leakLogLine)
}

func TestQueryLogs_DeniedRecordsRequestOnly(t *testing.T) {
	inner := mocks.NewMockLogsQuerier(t)
	svc := NewLogsServiceWithAuthz(inner, mockPDPDeny(t), testLogger())

	ctx, data := auditedCtx(authedCtx())
	_, err := svc.QueryLogs(ctx, &types.LogsQueryRequest{
		SearchScope: &types.SearchScope{Component: &types.ComponentSearchScope{Namespace: "ns", Component: "c"}},
	})
	require.Error(t, err)

	require.NotNil(t, data.Parameters)
	assert.Nil(t, data.Returned)
}

func TestQueryPlatformLogs_RecordsCoordinatesAndCount(t *testing.T) {
	inner := mocks.NewMockPlatformLogsQuerier(t)
	inner.EXPECT().QueryPlatformLogs(mock.Anything, mock.Anything).Return(&types.PlatformLogsResponse{
		Logs: []types.PlatformLog{{Log: leakLogLine}},
	}, nil)
	svc := NewPlatformLogsServiceWithAuthz(inner, mockPDPAllow(t), testLogger())

	ctx, data := auditedCtx(authedCtx())
	req := platformLogsRequest()
	req.PodNames = []string{"api-7f9c"}
	req.Labels = map[string]string{"openchoreo.dev/plane": "controlplane", "app": "api"}
	_, err := svc.QueryPlatformLogs(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"startTime": "2026-08-14T16:30:00Z",
		"endTime":   "2026-08-14T17:30:00Z",
		"namespace": []string{"openchoreo-control-plane"},
		"podName":   []string{"api-7f9c"},
		"labels":    "app=api,openchoreo.dev/plane=controlplane",
	}, data.Parameters)
	assert.Equal(t, audit.Returned{Count: 1}, *data.Returned)

	record := publishedRecord(t, ctx, data, "GetPlatformLogs")
	assert.NotContains(t, record, leakLogLine)
}

func TestQuerySpans_RecordsIncludeAttributesAndCount(t *testing.T) {
	inner := mocks.NewMockTracesQuerier(t)
	inner.EXPECT().QuerySpans(mock.Anything, captureTestTraceID, mock.Anything).Return(&types.SpansQueryResponse{
		Spans: []types.SpanInfo{
			{SpanID: "s1", Attributes: map[string]interface{}{"db.statement": leakSpanAttribute}},
			{SpanID: "s2"},
		},
	}, nil)
	svc := NewTracesServiceWithAuthz(inner, mockPDPAllow(t), testLogger())

	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	ctx, data := auditedCtx(authedCtx())
	_, err := svc.QuerySpans(ctx, captureTestTraceID, &types.TracesQueryRequest{
		StartTime:         start,
		EndTime:           start.Add(time.Hour),
		IncludeAttributes: true,
		SearchScope:       types.ComponentSearchScope{Namespace: "ns", Component: "comp"},
	})
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"startTime":         "2026-10-01T00:00:00Z",
		"endTime":           "2026-10-01T01:00:00Z",
		"includeAttributes": true,
		"searchScope":       map[string]any{"namespace": "ns", "component": "comp"},
	}, data.Parameters)
	assert.Equal(t, audit.Returned{Count: 2}, *data.Returned)

	record := publishedRecord(t, ctx, data, "QuerySpansForTrace")
	assert.NotContains(t, record, leakSpanAttribute)
}

func TestQuerySpans_RecordsIncludeAttributesFalse(t *testing.T) {
	inner := mocks.NewMockTracesQuerier(t)
	inner.EXPECT().QuerySpans(mock.Anything, captureTestTraceID, mock.Anything).Return(&types.SpansQueryResponse{}, nil)
	svc := NewTracesServiceWithAuthz(inner, nil, testLogger())

	ctx, data := auditedCtx(context.Background())
	_, err := svc.QuerySpans(ctx, captureTestTraceID, &types.TracesQueryRequest{
		SearchScope: types.ComponentSearchScope{Namespace: "ns"},
	})
	require.NoError(t, err)

	assert.Equal(t, false, data.Parameters["includeAttributes"])
	assert.Equal(t, audit.Returned{Count: 0}, *data.Returned)

	record := publishedRecord(t, ctx, data, "QuerySpansForTrace")
	assert.Contains(t, record, `"includeAttributes":false`)
	assert.Contains(t, record, `"returned":{"count":0}`)
}

func TestGetSpanDetails_RecordsSpanAndCount(t *testing.T) {
	inner := mocks.NewMockTracesQuerier(t)
	span := spanWithScope()
	span.Attributes = map[string]interface{}{"db.statement": leakSpanAttribute}
	inner.EXPECT().GetSpanDetails(mock.Anything, captureTestTraceID, "span-1").Return(span, nil)
	svc := NewTracesServiceWithAuthz(inner, mockPDPAllow(t), testLogger())

	ctx, data := auditedCtx(authedCtx())
	_, err := svc.GetSpanDetails(ctx, captureTestTraceID, "span-1")
	require.NoError(t, err)

	assert.Equal(t, map[string]any{"spanId": "span-1"}, data.Parameters)
	assert.Equal(t, audit.Returned{Count: 1}, *data.Returned)

	record := publishedRecord(t, ctx, data, "GetSpanDetailsForTrace")
	assert.NotContains(t, record, leakSpanAttribute)
}

// The span is fetched before the authz check, but never reaches the caller.
func TestGetSpanDetails_DeniedRecordsRequestOnly(t *testing.T) {
	inner := mocks.NewMockTracesQuerier(t)
	inner.EXPECT().GetSpanDetails(mock.Anything, captureTestTraceID, "span-1").Return(spanWithScope(), nil)
	svc := NewTracesServiceWithAuthz(inner, mockPDPDeny(t), testLogger())

	ctx, data := auditedCtx(authedCtx())
	_, err := svc.GetSpanDetails(ctx, captureTestTraceID, "span-1")
	require.Error(t, err)

	require.NotNil(t, data.Parameters)
	assert.Nil(t, data.Returned)
}
