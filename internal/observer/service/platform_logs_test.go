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

// stubPlatformLogsAdapter records the params it was called with and returns a
// canned result or error.
type stubPlatformLogsAdapter struct {
	got    observability.PlatformLogsParams
	result *observability.PlatformLogsResult
	err    error

	gotFilterValues observability.PlatformLogFilterValuesParams
	filterValues    *observability.PlatformLogFilterValuesResult
}

func (s *stubPlatformLogsAdapter) GetPlatformLogs(
	_ context.Context, params observability.PlatformLogsParams,
) (*observability.PlatformLogsResult, error) {
	s.got = params
	return s.result, s.err
}

func (s *stubPlatformLogsAdapter) GetPlatformLogFilterValues(
	_ context.Context, params observability.PlatformLogFilterValuesParams,
) (*observability.PlatformLogFilterValuesResult, error) {
	s.gotFilterValues = params
	return s.filterValues, s.err
}

func TestPlatformLogsService_QueryPlatformLogs(t *testing.T) {
	t.Parallel()

	collected := time.Date(2026, 8, 14, 16, 31, 0, 0, time.UTC)
	adapter := &stubPlatformLogsAdapter{result: &observability.PlatformLogsResult{
		Logs: []observability.PlatformLogEntry{{
			Timestamp:       collected,
			Log:             "reconcile failed",
			LogLevel:        "ERROR",
			ClusterInstance: "cluster1",
			NamespaceName:   "openchoreo-control-plane",
			PodName:         "controller-manager-7f58b689b5-pwsb5",
			ContainerName:   "manager",
		}},
		TotalCount: 1,
		Took:       7,
	}}

	svc := NewPlatformLogsService(adapter, testLogger())
	resp, err := svc.QueryPlatformLogs(context.Background(), &types.PlatformLogsQueryRequest{
		Namespaces: []string{"openchoreo-control-plane"},
		Labels:     map[string]string{"openchoreo.dev/plane": "controlplane"},
		StartTime:  "2026-08-14T16:30:00Z",
		EndTime:    "2026-08-14T17:30:00Z",
		Limit:      100,
		SortOrder:  "desc",
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"openchoreo-control-plane"}, adapter.got.Namespaces)
	assert.Equal(t, map[string]string{"openchoreo.dev/plane": "controlplane"}, adapter.got.Labels)
	assert.Equal(t, time.Date(2026, 8, 14, 16, 30, 0, 0, time.UTC), adapter.got.StartTime)

	require.Len(t, resp.Logs, 1)
	assert.Equal(t, "2026-08-14T16:31:00Z", resp.Logs[0].Timestamp)
	assert.Equal(t, "ERROR", resp.Logs[0].Level)
	assert.Equal(t, "cluster1", resp.Logs[0].ClusterInstance)
	assert.Equal(t, 1, resp.Total)
	assert.Equal(t, 7, resp.TookMs)
}

// TestPlatformLogsService_NotSupportedPassesThrough pins that a 501 from the adapter
// reaches the handler unwrapped, so it can answer 501 rather than reporting a failure.
func TestPlatformLogsService_NotSupportedPassesThrough(t *testing.T) {
	t.Parallel()

	adapter := &stubPlatformLogsAdapter{err: ErrPlatformLogsNotSupported}
	svc := NewPlatformLogsService(adapter, testLogger())

	_, err := svc.QueryPlatformLogs(context.Background(), &types.PlatformLogsQueryRequest{
		StartTime: "2026-08-14T16:30:00Z",
		EndTime:   "2026-08-14T17:30:00Z",
	})
	require.ErrorIs(t, err, ErrPlatformLogsNotSupported)
	assert.NotErrorIs(t, err, ErrPlatformLogsRetrieval)
}

func TestPlatformLogsService_RetrievalFailureIsWrapped(t *testing.T) {
	t.Parallel()

	adapter := &stubPlatformLogsAdapter{err: errors.New("connection refused")}
	svc := NewPlatformLogsService(adapter, testLogger())

	_, err := svc.QueryPlatformLogs(context.Background(), &types.PlatformLogsQueryRequest{
		StartTime: "2026-08-14T16:30:00Z",
		EndTime:   "2026-08-14T17:30:00Z",
	})
	require.ErrorIs(t, err, ErrPlatformLogsRetrieval)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestPlatformLogsService_InvalidTimeRange(t *testing.T) {
	t.Parallel()

	adapter := &stubPlatformLogsAdapter{}
	svc := NewPlatformLogsService(adapter, testLogger())

	_, err := svc.QueryPlatformLogs(context.Background(), &types.PlatformLogsQueryRequest{
		StartTime: "not-a-time",
		EndTime:   "2026-08-14T17:30:00Z",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse start time")
}

// --- filter values ---

func filterValuesRequest() *types.PlatformLogFilterValuesRequest {
	return &types.PlatformLogFilterValuesRequest{
		Filter:      "podName",
		ValueSearch: "controller",
		MaxValues:   100,
		Query: types.PlatformLogsQueryRequest{
			Namespaces: []string{"openchoreo-control-plane"},
			PodNames:   []string{"already-selected"},
			Labels:     map[string]string{"openchoreo.dev/plane": "controlplane"},
			StartTime:  "2026-08-14T16:30:00Z",
			EndTime:    "2026-08-14T17:30:00Z",
		},
	}
}

func TestPlatformLogsService_FilterValues_Query(t *testing.T) {
	t.Parallel()

	adapter := &stubPlatformLogsAdapter{
		filterValues: &observability.PlatformLogFilterValuesResult{
			Filter: "podName",
			Values: []observability.PlatformLogFilterValue{
				{Value: "controller-manager-abc", Count: 412},
				{Value: "controller-manager-xyz", Count: 88},
			},
			TotalValues:   2,
			TotalRelation: "eq",
			Took:          7,
		},
	}

	svc := NewPlatformLogsService(adapter, testLogger())
	resp, err := svc.QueryPlatformLogFilterValues(context.Background(), filterValuesRequest())
	require.NoError(t, err)

	assert.Equal(t, "podName", adapter.gotFilterValues.Filter)
	assert.Equal(t, "controller", adapter.gotFilterValues.ValueSearch)
	assert.Equal(t, 100, adapter.gotFilterValues.MaxValues)
	assert.Equal(t, time.Date(2026, 8, 14, 16, 30, 0, 0, time.UTC), adapter.gotFilterValues.Query.StartTime)
	assert.Equal(t, []string{"openchoreo-control-plane"}, adapter.gotFilterValues.Query.Namespaces)
	assert.Equal(t, map[string]string{"openchoreo.dev/plane": "controlplane"}, adapter.gotFilterValues.Query.Labels)

	assert.Equal(t, "podName", resp.Filter)
	require.Len(t, resp.Values, 2)
	assert.Equal(t, types.PlatformLogFilterValue{Value: "controller-manager-abc", Count: 412}, resp.Values[0])
	assert.EqualValues(t, 2, resp.TotalValues)
	assert.Equal(t, "eq", resp.TotalRelation)
	assert.Equal(t, 7, resp.TookMs)
}

// The named filter's own selections go over the wire untouched. Excluding them is the
// adapter's job, and doctoring the query here would leave it unable to tell "not
// selected" from "excluded for this call".
func TestPlatformLogsService_FilterValues_ForwardsNamedFilterSelections(t *testing.T) {
	t.Parallel()

	adapter := &stubPlatformLogsAdapter{
		filterValues: &observability.PlatformLogFilterValuesResult{Filter: "podName", TotalRelation: "eq"},
	}
	svc := NewPlatformLogsService(adapter, testLogger())

	_, err := svc.QueryPlatformLogFilterValues(context.Background(), filterValuesRequest())
	require.NoError(t, err)

	assert.Equal(t, []string{"already-selected"}, adapter.gotFilterValues.Query.PodNames)
}

func TestPlatformLogsService_FilterValues_NotSupportedPassesThrough(t *testing.T) {
	t.Parallel()

	adapter := &stubPlatformLogsAdapter{err: ErrPlatformLogFilterValuesNotSupported}
	svc := NewPlatformLogsService(adapter, testLogger())

	_, err := svc.QueryPlatformLogFilterValues(context.Background(), filterValuesRequest())

	require.ErrorIs(t, err, ErrPlatformLogFilterValuesNotSupported)
	assert.NotErrorIs(t, err, ErrPlatformLogFilterValuesRetrieval)
}

func TestPlatformLogsService_FilterValues_RetrievalFailureIsWrapped(t *testing.T) {
	t.Parallel()

	adapter := &stubPlatformLogsAdapter{err: errors.New("connection refused")}
	svc := NewPlatformLogsService(adapter, testLogger())

	_, err := svc.QueryPlatformLogFilterValues(context.Background(), filterValuesRequest())

	require.ErrorIs(t, err, ErrPlatformLogFilterValuesRetrieval)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestPlatformLogsService_FilterValues_InvalidTimeRange(t *testing.T) {
	t.Parallel()

	svc := NewPlatformLogsService(&stubPlatformLogsAdapter{}, testLogger())
	req := filterValuesRequest()
	req.Query.StartTime = "not-a-time"

	_, err := svc.QueryPlatformLogFilterValues(context.Background(), req)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse start time")
}
