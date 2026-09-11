// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/pkg/observability"
)

func auditWindow() (time.Time, time.Time) {
	return time.Date(2026, 8, 14, 16, 30, 0, 0, time.UTC),
		time.Date(2026, 8, 14, 17, 30, 0, 0, time.UTC)
}

// fullAuditLogsParams populates every filter, so the request-contract test below
// fails if any of them stops being mapped onto the wire.
func fullAuditLogsParams() observability.AuditLogsParams {
	start, end := auditWindow()
	return observability.AuditLogsParams{
		StartTime: start,
		EndTime:   end,
		Actor: observability.AuditLogsActorFilter{
			IDs:          []string{"alice@example.com"},
			Types:        []string{"user", "service_account"},
			Issuers:      []string{"https://idp.example.com"},
			SessionIDs:   []string{"a3f9c1d2"},
			Entitlements: []string{"platform-engineer"},
		},
		Resource: observability.AuditLogsResourceFilter{
			Types:        []string{"project"},
			Namespaces:   []string{"default"},
			Environments: []string{"default/production"},
			Projects:     []string{"payments"},
			Components:   []string{"checkout"},
			Names:        []string{"checkout"},
		},
		Actions:          []string{"create_project"},
		Categories:       []string{"authorization"},
		Results:          []string{"denied"},
		Producers:        []string{"openchoreo-api"},
		Surfaces:         []string{"rest"},
		OperationIDs:     []string{"CreateProject"},
		RequestIDs:       []string{"req-1"},
		EventIDs:         []string{"evt-1"},
		SourceIPs:        []string{"10.42.0.7"},
		UserAgents:       []string{"occ/1.2.0"},
		SearchPhrase:     "reconcile",
		Limit:            50,
		SortOrder:        "asc",
		Cursor:           "opaque-token",
		IncludeTimeline:  true,
		TimelineInterval: "15m",
	}
}

func emptyAuditLogsResponse() map[string]any {
	return map[string]any{
		"records": []any{}, "total": 0, "totalRelation": "eq", "tookMs": 3,
	}
}

// Pins what the observer puts on the wire: the path, and every filter mapped
// onto the body field the spec declares.
func TestLogsAdapter_GetAuditLogs_RequestContract(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	var gotMethod, gotPath string
	server := platformLogsServer(t, http.StatusOK, emptyAuditLogsResponse(),
		&gotBody, &gotMethod, &gotPath)
	defer server.Close()

	_, err := newTestPlatformLogsAdapter(t, server.URL).
		GetAuditLogs(context.Background(), fullAuditLogsParams())
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/api/v1alpha1/audit-logs/query", gotPath)

	assert.Equal(t, "2026-08-14T16:30:00Z", gotBody["startTime"])
	assert.Equal(t, "2026-08-14T17:30:00Z", gotBody["endTime"])

	actor, ok := gotBody["actor"].(map[string]any)
	require.True(t, ok, "actor group must be sent nested, not flattened")
	assert.Equal(t, []any{"alice@example.com"}, actor["id"])
	assert.Equal(t, []any{"user", "service_account"}, actor["type"])
	assert.Equal(t, []any{"https://idp.example.com"}, actor["issuer"])
	assert.Equal(t, []any{"a3f9c1d2"}, actor["session_id"])
	assert.Equal(t, []any{"platform-engineer"}, actor["entitlements"])

	resource, ok := gotBody["resource"].(map[string]any)
	require.True(t, ok, "resource group must be sent nested, not flattened")
	assert.Equal(t, []any{"project"}, resource["type"])
	assert.Equal(t, []any{"default"}, resource["namespace"])
	assert.Equal(t, []any{"default/production"}, resource["environment"])
	assert.Equal(t, []any{"payments"}, resource["project"])
	assert.Equal(t, []any{"checkout"}, resource["component"])
	assert.Equal(t, []any{"checkout"}, resource["name"])

	assert.Equal(t, []any{"create_project"}, gotBody["action"])
	assert.Equal(t, []any{"authorization"}, gotBody["category"])
	assert.Equal(t, []any{"denied"}, gotBody["result"])
	assert.Equal(t, []any{"openchoreo-api"}, gotBody["producer"])
	assert.Equal(t, []any{"rest"}, gotBody["surface"])
	assert.Equal(t, []any{"CreateProject"}, gotBody["operation_id"])
	assert.Equal(t, []any{"req-1"}, gotBody["request_id"])
	assert.Equal(t, []any{"evt-1"}, gotBody["event_id"])
	assert.Equal(t, []any{"10.42.0.7"}, gotBody["source_ip"])
	assert.Equal(t, []any{"occ/1.2.0"}, gotBody["user_agent"])
	assert.Equal(t, "reconcile", gotBody["searchPhrase"])
	assert.Equal(t, float64(50), gotBody["limit"])
	assert.Equal(t, "asc", gotBody["sortOrder"])
	assert.Equal(t, "opaque-token", gotBody["cursor"])
	assert.Equal(t, true, gotBody["includeTimeline"])
	assert.Equal(t, "15m", gotBody["timelineInterval"])
}

// An empty filter must not be sent as an empty array, which a backend could
// read as "match nothing" rather than "no filter".
func TestLogsAdapter_GetAuditLogs_OmitsUnsetFilters(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	server := platformLogsServer(t, http.StatusOK, emptyAuditLogsResponse(), &gotBody, nil, nil)
	defer server.Close()

	start, end := auditWindow()
	_, err := newTestPlatformLogsAdapter(t, server.URL).GetAuditLogs(context.Background(),
		observability.AuditLogsParams{StartTime: start, EndTime: end})
	require.NoError(t, err)

	for _, key := range []string{
		"actor", "resource", "action", "category", "result", "producer", "surface",
		"operation_id", "request_id", "event_id", "source_ip", "user_agent",
		"searchPhrase", "cursor", "includeTimeline", "timelineInterval",
	} {
		assert.NotContains(t, gotBody, key, "unset filter %q must be omitted", key)
	}
}

func TestLogsAdapter_GetAuditLogs_MapsResponse(t *testing.T) {
	t.Parallel()

	server := platformLogsServer(t, http.StatusOK, map[string]any{
		"records": []any{map[string]any{
			"schema_version": "1.0",
			"event_id":       "evt-1",
			"event_time":     "2026-08-14T16:45:00Z",
			"actor": map[string]any{
				"type": "user", "id": "alice@example.com",
				"issuer": "https://idp.example.com", "session_id": "a3f9c1d2",
				"entitlements": map[string]any{"groups": []any{"platform-engineer"}},
			},
			"action": "create_project", "category": "management", "result": "success",
			"request_id": "req-1", "source_ip": "10.42.0.7", "user_agent": "occ/1.2.0",
			"producer": "openchoreo-api", "surface": "rest", "operation_id": "CreateProject",
			"http":     map[string]any{"method": "POST", "path": "/api/v1/namespaces/default/projects"},
			"resource": map[string]any{"type": "project", "namespace": "default", "name": "payments"},
			"collector": map[string]any{
				"namespaceName": "openchoreo-control-plane", "podName": "api-0", "containerName": "api-server",
			},
			"log": `{"action":"create_project"}`,
		}},
		"total": 1, "totalRelation": "gte", "tookMs": 7, "nextCursor": "next-token",
		"timeline": map[string]any{
			"interval": "15m",
			"buckets": []any{
				map[string]any{
					"startTime": "2026-08-14T16:30:00Z", "total": 1,
					"counts": map[string]any{"success": 1},
				},
				map[string]any{"startTime": "2026-08-14T16:45:00Z", "total": 0},
			},
		},
	}, nil, nil, nil)
	defer server.Close()

	params := fullAuditLogsParams()
	result, err := newTestPlatformLogsAdapter(t, server.URL).
		GetAuditLogs(context.Background(), params)
	require.NoError(t, err)

	require.Len(t, result.Records, 1)
	rec := result.Records[0]
	assert.Equal(t, "1.0", rec.SchemaVersion)
	assert.Equal(t, "evt-1", rec.EventID)
	assert.Equal(t, time.Date(2026, 8, 14, 16, 45, 0, 0, time.UTC), rec.EventTime.UTC())
	assert.Equal(t, "alice@example.com", rec.Actor.ID)
	assert.Equal(t, "https://idp.example.com", rec.Actor.Issuer)
	assert.Equal(t, "a3f9c1d2", rec.Actor.SessionID)
	assert.Equal(t, map[string][]string{"groups": {"platform-engineer"}}, rec.Actor.Entitlements)
	assert.Equal(t, "occ/1.2.0", rec.UserAgent)
	assert.Equal(t, "rest", rec.Surface)
	require.NotNil(t, rec.HTTP)
	assert.Equal(t, "POST", rec.HTTP.Method)
	require.NotNil(t, rec.Resource)
	assert.Equal(t, "payments", rec.Resource.Name)
	require.NotNil(t, rec.Collector)
	assert.Equal(t, "api-server", rec.Collector.ContainerName)
	assert.Equal(t, `{"action":"create_project"}`, rec.Log)

	assert.Equal(t, int64(1), result.TotalCount)
	assert.Equal(t, observability.AuditLogsTotalGTE, result.TotalRelation)
	assert.Equal(t, int64(7), result.Took)
	assert.Equal(t, "next-token", result.NextCursor)

	require.NotNil(t, result.Timeline)
	assert.Equal(t, "15m", result.Timeline.Interval)
	require.Len(t, result.Timeline.Buckets, 2)
	assert.Equal(t, map[string]int64{"success": 1}, result.Timeline.Buckets[0].Counts)
	// Dropping an empty bucket would let a client draw a continuous chart
	// across a gap in activity.
	assert.Equal(t, int64(0), result.Timeline.Buckets[1].Total)
	assert.Nil(t, result.Timeline.Buckets[1].Counts)
}

// Nil is not the same answer as an empty bucket list.
func TestLogsAdapter_GetAuditLogs_NilTimelinePreserved(t *testing.T) {
	t.Parallel()

	server := platformLogsServer(t, http.StatusOK, emptyAuditLogsResponse(), nil, nil, nil)
	defer server.Close()

	result, err := newTestPlatformLogsAdapter(t, server.URL).
		GetAuditLogs(context.Background(), fullAuditLogsParams())
	require.NoError(t, err)
	assert.Nil(t, result.Timeline)
}

func TestLogsAdapter_GetAuditLogs_StatusMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		wantErr error
		notErrs []error
	}{
		{
			name:    "501 is not-supported, not a failure",
			status:  http.StatusNotImplemented,
			wantErr: ErrAuditLogsNotSupported,
		},
		{
			name:    "410 is an expired cursor, never an empty page",
			status:  http.StatusGone,
			wantErr: ErrAuditLogsCursorExpired,
		},
		{
			name:   "500 is neither sentinel",
			status: http.StatusInternalServerError,
			notErrs: []error{
				ErrAuditLogsNotSupported, ErrAuditLogsCursorExpired,
				ErrAuditLogFilterValuesNotSupported,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := platformLogsServer(t, tt.status, map[string]any{"message": "nope"}, nil, nil, nil)
			defer server.Close()

			_, err := newTestPlatformLogsAdapter(t, server.URL).
				GetAuditLogs(context.Background(), fullAuditLogsParams())
			require.Error(t, err)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			}
			for _, notErr := range tt.notErrs {
				assert.NotErrorIs(t, err, notErr)
			}
		})
	}
}

func TestLogsAdapter_GetAuditLogFilterValues_RequestContract(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	var gotMethod, gotPath string
	server := platformLogsServer(t, http.StatusOK, map[string]any{
		"filter": "actor.id", "values": []any{}, "totalValues": 0,
		"totalRelation": "eq", "tookMs": 2,
	}, &gotBody, &gotMethod, &gotPath)
	defer server.Close()

	_, err := newTestPlatformLogsAdapter(t, server.URL).GetAuditLogFilterValues(
		context.Background(), observability.AuditLogFilterValuesParams{
			Query:       fullAuditLogsParams(),
			Filter:      "actor.id",
			ValueSearch: "ali",
			MaxValues:   25,
		})
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/api/v1alpha1/audit-logs/filter-values", gotPath)
	assert.Equal(t, "actor.id", gotBody["filter"])
	assert.Equal(t, "ali", gotBody["valueSearch"])
	assert.Equal(t, float64(25), gotBody["maxValues"])

	// The whole query travels nested, so the adapter applies the same filters
	// it would to a record query.
	query, ok := gotBody["query"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "2026-08-14T16:30:00Z", query["startTime"])
	actor, ok := query["actor"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []any{"alice@example.com"}, actor["id"])
}

func TestLogsAdapter_GetAuditLogFilterValues_MapsResponse(t *testing.T) {
	t.Parallel()

	server := platformLogsServer(t, http.StatusOK, map[string]any{
		"filter": "actor.id",
		"values": []any{
			map[string]any{"value": "alice@example.com", "count": 412},
			map[string]any{"value": "bob@example.com", "count": 17},
		},
		"totalValues": 128, "totalRelation": "gte", "tookMs": 9,
	}, nil, nil, nil)
	defer server.Close()

	result, err := newTestPlatformLogsAdapter(t, server.URL).GetAuditLogFilterValues(
		context.Background(), observability.AuditLogFilterValuesParams{
			Query: fullAuditLogsParams(), Filter: "actor.id",
		})
	require.NoError(t, err)

	assert.Equal(t, "actor.id", result.Filter)
	require.Len(t, result.Values, 2)
	assert.Equal(t, "alice@example.com", result.Values[0].Value)
	assert.Equal(t, int64(412), result.Values[0].Count)
	assert.Equal(t, int64(128), result.TotalValues)
	assert.Equal(t, observability.AuditLogsTotalGTE, result.TotalRelation)
	assert.Equal(t, int64(9), result.Took)
}

// A module may serve records while unable to aggregate, so the two sentinels
// must stay distinct.
//
// The 501 carries a body because the contract declares one. A bodyless 501 with
// a JSON content type fails the generated parser before the status is read and
// degrades to a 500 — pre-existing, shared with platform logs.
func TestLogsAdapter_GetAuditLogFilterValues_NotSupportedIsItsOwnSentinel(t *testing.T) {
	t.Parallel()

	server := platformLogsServer(t, http.StatusNotImplemented, map[string]any{
		"title": "notImplemented", "errorCode": "",
		"message": "audit log filter values are not supported by this adapter",
	}, nil, nil, nil)
	defer server.Close()

	_, err := newTestPlatformLogsAdapter(t, server.URL).GetAuditLogFilterValues(
		context.Background(), observability.AuditLogFilterValuesParams{
			Query: fullAuditLogsParams(), Filter: "actor.id",
		})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAuditLogFilterValuesNotSupported)
	assert.NotErrorIs(t, err, ErrAuditLogsNotSupported)
}
