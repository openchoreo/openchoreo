// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

// stubAuditLogsAdapter records the params it was called with and returns a
// canned result or error, per operation.
type stubAuditLogsAdapter struct {
	gotQuery     observability.AuditLogsParams
	queryResult  *observability.AuditLogsResult
	queryErr     error
	gotValues    observability.AuditLogFilterValuesParams
	valuesResult *observability.AuditLogFilterValuesResult
	valuesErr    error
}

func (s *stubAuditLogsAdapter) GetAuditLogs(
	_ context.Context, params observability.AuditLogsParams,
) (*observability.AuditLogsResult, error) {
	s.gotQuery = params
	return s.queryResult, s.queryErr
}

func (s *stubAuditLogsAdapter) GetAuditLogFilterValues(
	_ context.Context, params observability.AuditLogFilterValuesParams,
) (*observability.AuditLogFilterValuesResult, error) {
	s.gotValues = params
	return s.valuesResult, s.valuesErr
}

func auditLogsRequest() *types.AuditLogsQueryRequest {
	return &types.AuditLogsQueryRequest{
		StartTime: "2026-08-14T16:30:00Z",
		EndTime:   "2026-08-14T17:30:00Z",
		Actor:     types.AuditLogsActorFilter{IDs: []string{"alice@example.com"}},
		Resource:  types.AuditLogsResourceFilter{Namespaces: []string{"default"}},
		Results:   []string{"denied"},
		Limit:     100,
		SortOrder: "desc",
	}
}

func TestAuditLogsService_QueryAuditLogs(t *testing.T) {
	t.Parallel()

	recorded := time.Date(2026, 8, 14, 16, 45, 0, 0, time.UTC)
	adapter := &stubAuditLogsAdapter{queryResult: &observability.AuditLogsResult{
		Records: []observability.AuditLogRecord{{
			SchemaVersion: "1.0",
			EventID:       "evt-1",
			EventTime:     recorded,
			Actor: observability.AuditLogActor{
				Type: "user", ID: "alice@example.com", Issuer: "https://idp.example.com",
			},
			Action: "create_project", Category: "management", Result: "success",
			UserAgent: "occ/1.2.0", Surface: "rest", Producer: "openchoreo-api",
			Resource:  &observability.AuditLogResource{Type: "project", Name: "payments"},
			Collector: &observability.AuditLogCollectorInfo{ContainerName: "api-server"},
		}},
		TotalCount:    1,
		TotalRelation: observability.AuditLogsTotalEq,
		Took:          7,
		NextCursor:    "next-token",
	}}

	svc := NewAuditLogsService(adapter, testLogger())
	resp, err := svc.QueryAuditLogs(context.Background(), auditLogsRequest())
	require.NoError(t, err)

	// The window reaches the adapter parsed, and the filter groups stay grouped.
	assert.Equal(t, time.Date(2026, 8, 14, 16, 30, 0, 0, time.UTC), adapter.gotQuery.StartTime.UTC())
	assert.Equal(t, []string{"alice@example.com"}, adapter.gotQuery.Actor.IDs)
	assert.Equal(t, []string{"default"}, adapter.gotQuery.Resource.Namespaces)
	assert.Equal(t, []string{"denied"}, adapter.gotQuery.Results)

	require.Len(t, resp.Records, 1)
	assert.Equal(t, "evt-1", resp.Records[0].EventID)
	assert.Equal(t, "2026-08-14T16:45:00Z", resp.Records[0].EventTime)
	assert.Equal(t, "occ/1.2.0", resp.Records[0].UserAgent)
	assert.Equal(t, "rest", resp.Records[0].Surface)
	require.NotNil(t, resp.Records[0].Collector)
	assert.Equal(t, "api-server", resp.Records[0].Collector.ContainerName)
	assert.Equal(t, int64(1), resp.Total)
	assert.Equal(t, "eq", resp.TotalRelation)
	assert.Equal(t, "next-token", resp.NextCursor)
}

// TestAuditLogsService_TimelineOnlyWhenRequested: an adapter that computes a
// timeline unconditionally must not make every caller carry it.
func TestAuditLogsService_TimelineOnlyWhenRequested(t *testing.T) {
	t.Parallel()

	timeline := &observability.AuditLogTimeline{
		Interval: "15m",
		Buckets: []observability.AuditLogTimelineBucket{{
			StartTime: time.Date(2026, 8, 14, 16, 30, 0, 0, time.UTC),
			Total:     4,
			Counts:    map[string]int64{"success": 3, "denied": 1},
		}},
	}

	t.Run("omitted when not asked for", func(t *testing.T) {
		t.Parallel()
		adapter := &stubAuditLogsAdapter{queryResult: &observability.AuditLogsResult{Timeline: timeline}}
		svc := NewAuditLogsService(adapter, testLogger())
		resp, err := svc.QueryAuditLogs(context.Background(), auditLogsRequest())
		require.NoError(t, err)
		assert.Nil(t, resp.Timeline)
	})

	t.Run("mapped when asked for", func(t *testing.T) {
		t.Parallel()
		adapter := &stubAuditLogsAdapter{queryResult: &observability.AuditLogsResult{Timeline: timeline}}
		svc := NewAuditLogsService(adapter, testLogger())
		req := auditLogsRequest()
		req.IncludeTimeline = true
		resp, err := svc.QueryAuditLogs(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp.Timeline)
		assert.Equal(t, "15m", resp.Timeline.Interval)
		require.Len(t, resp.Timeline.Buckets, 1)
		assert.Equal(t, "2026-08-14T16:30:00Z", resp.Timeline.Buckets[0].StartTime)
		assert.Equal(t, int64(4), resp.Timeline.Buckets[0].Total)
		assert.Equal(t, map[string]int64{"success": 3, "denied": 1}, resp.Timeline.Buckets[0].Counts)
	})

	t.Run("nil adapter timeline stays nil even when asked for", func(t *testing.T) {
		t.Parallel()
		adapter := &stubAuditLogsAdapter{queryResult: &observability.AuditLogsResult{}}
		svc := NewAuditLogsService(adapter, testLogger())
		req := auditLogsRequest()
		req.IncludeTimeline = true
		resp, err := svc.QueryAuditLogs(context.Background(), req)
		require.NoError(t, err)
		assert.Nil(t, resp.Timeline, "nil means unknown, not no activity")
	})
}

func TestAuditLogsService_QueryAuditLogs_TimeParsing(t *testing.T) {
	t.Parallel()

	svc := NewAuditLogsService(&stubAuditLogsAdapter{}, testLogger())

	req := auditLogsRequest()
	req.StartTime = "not-a-time"
	_, err := svc.QueryAuditLogs(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start time")

	req = auditLogsRequest()
	req.EndTime = "not-a-time"
	_, err = svc.QueryAuditLogs(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "end time")
}

// TestAuditLogsService_SentinelPassThrough: the handler maps these onto specific
// statuses, so wrapping them as a retrieval failure would turn a 501 or a
// restart-the-query 400 into a 500.
func TestAuditLogsService_SentinelPassThrough(t *testing.T) {
	t.Parallel()

	t.Run("records: not supported and cursor expired pass through", func(t *testing.T) {
		t.Parallel()
		for _, sentinel := range []error{ErrAuditLogsNotSupported, ErrAuditLogsCursorExpired} {
			adapter := &stubAuditLogsAdapter{queryErr: sentinel}
			svc := NewAuditLogsService(adapter, testLogger())
			_, err := svc.QueryAuditLogs(context.Background(), auditLogsRequest())
			require.Error(t, err)
			assert.ErrorIs(t, err, sentinel)
			assert.NotErrorIs(t, err, ErrAuditLogsRetrieval)
		}
	})

	t.Run("records: anything else is a retrieval failure", func(t *testing.T) {
		t.Parallel()
		adapter := &stubAuditLogsAdapter{queryErr: errors.New("connection refused")}
		svc := NewAuditLogsService(adapter, testLogger())
		_, err := svc.QueryAuditLogs(context.Background(), auditLogsRequest())
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAuditLogsRetrieval)
		assert.NotErrorIs(t, err, ErrAuditLogsNotSupported)
	})

	// A cursor cannot expire on a filter-values query, so that sentinel is not
	// pass-through there: it would be a bug in the adapter, not a client error.
	t.Run("filter values: only its own sentinel passes through", func(t *testing.T) {
		t.Parallel()
		adapter := &stubAuditLogsAdapter{valuesErr: ErrAuditLogFilterValuesNotSupported}
		svc := NewAuditLogsService(adapter, testLogger())
		_, err := svc.QueryAuditLogFilterValues(context.Background(), auditLogFilterValuesRequest())
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAuditLogFilterValuesNotSupported)

		adapter = &stubAuditLogsAdapter{valuesErr: ErrAuditLogsCursorExpired}
		svc = NewAuditLogsService(adapter, testLogger())
		_, err = svc.QueryAuditLogFilterValues(context.Background(), auditLogFilterValuesRequest())
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAuditLogsRetrieval)
	})
}

func auditLogFilterValuesRequest() *types.AuditLogFilterValuesRequest {
	return &types.AuditLogFilterValuesRequest{
		Query:       *auditLogsRequest(),
		Filter:      "actor.id",
		ValueSearch: "ali",
		MaxValues:   25,
	}
}

func TestAuditLogsService_QueryAuditLogFilterValues(t *testing.T) {
	t.Parallel()

	adapter := &stubAuditLogsAdapter{valuesResult: &observability.AuditLogFilterValuesResult{
		Filter: "actor.id",
		Values: []observability.AuditLogFilterValue{
			{Value: "alice@example.com", Count: 412},
			{Value: "bob@example.com", Count: 17},
		},
		TotalValues:   128,
		TotalRelation: observability.AuditLogsTotalGTE,
		Took:          9,
	}}

	svc := NewAuditLogsService(adapter, testLogger())
	resp, err := svc.QueryAuditLogFilterValues(context.Background(), auditLogFilterValuesRequest())
	require.NoError(t, err)

	// The nested query reaches the adapter parsed and filtered, so values are
	// scoped to the same records the record query would return.
	assert.Equal(t, time.Date(2026, 8, 14, 16, 30, 0, 0, time.UTC), adapter.gotValues.Query.StartTime.UTC())
	assert.Equal(t, []string{"alice@example.com"}, adapter.gotValues.Query.Actor.IDs)
	assert.Equal(t, "actor.id", adapter.gotValues.Filter)
	assert.Equal(t, "ali", adapter.gotValues.ValueSearch)
	assert.Equal(t, 25, adapter.gotValues.MaxValues)

	assert.Equal(t, "actor.id", resp.Filter)
	require.Len(t, resp.Values, 2)
	assert.Equal(t, int64(412), resp.Values[0].Count)
	assert.Equal(t, int64(128), resp.TotalValues)
	assert.Equal(t, "gte", resp.TotalRelation)
	assert.Equal(t, int64(9), resp.TookMs)
}
