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
}

func (s *stubPlatformLogsAdapter) GetPlatformLogs(
	_ context.Context, params observability.PlatformLogsParams,
) (*observability.PlatformLogsResult, error) {
	s.got = params
	return s.result, s.err
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

// facetedResult is an adapter answer carrying facets, for the pass-through tests.
func facetedResult() *observability.PlatformLogsResult {
	return &observability.PlatformLogsResult{
		TotalCount: 0,
		Took:       3,
		Facets: &observability.PlatformLogFacets{
			Namespaces: []observability.PlatformLogFacetValue{
				{Value: "openchoreo-control-plane", Count: 412},
			},
			PodNames: []observability.PlatformLogFacetValue{
				{Value: "controller-manager-abc", Count: 412},
			},
		},
	}
}

func facetRequest(includeFacets bool) *types.PlatformLogsQueryRequest {
	return &types.PlatformLogsQueryRequest{
		StartTime:     "2026-08-14T16:30:00Z",
		EndTime:       "2026-08-14T17:30:00Z",
		IncludeFacets: includeFacets,
	}
}

// TestPlatformLogsService_ForwardsFacetRequest pins that the opt-in reaches the
// adapter and that what comes back is re-keyed onto the response field names the
// client filters by.
func TestPlatformLogsService_ForwardsFacetRequest(t *testing.T) {
	t.Parallel()

	adapter := &stubPlatformLogsAdapter{result: facetedResult()}
	svc := NewPlatformLogsService(adapter, testLogger())

	resp, err := svc.QueryPlatformLogs(context.Background(), facetRequest(true))
	require.NoError(t, err)

	assert.True(t, adapter.got.IncludeFacets)
	require.NotNil(t, resp.Facets)
	assert.Equal(t, []types.PlatformLogFacetValue{
		{Value: "openchoreo-control-plane", Count: 412},
	}, resp.Facets.Namespace)
	assert.Equal(t, []types.PlatformLogFacetValue{
		{Value: "controller-manager-abc", Count: 412},
	}, resp.Facets.PodName)
	// Coordinates the adapter said nothing about stay absent rather than empty.
	assert.Nil(t, resp.Facets.ClusterInstance)
	assert.Nil(t, resp.Facets.ContainerName)
}

// TestPlatformLogsService_DropsUnrequestedFacets pins that facets are answered only
// when asked for. An adapter that aggregates unconditionally must not make every
// caller pay to carry the result.
func TestPlatformLogsService_DropsUnrequestedFacets(t *testing.T) {
	t.Parallel()

	adapter := &stubPlatformLogsAdapter{result: facetedResult()}
	svc := NewPlatformLogsService(adapter, testLogger())

	resp, err := svc.QueryPlatformLogs(context.Background(), facetRequest(false))
	require.NoError(t, err)

	assert.False(t, adapter.got.IncludeFacets)
	assert.Nil(t, resp.Facets)
}

// TestPlatformLogsService_FacetsStayNilWhenUnsupported pins that an adapter which
// cannot aggregate produces an absent facets field rather than an empty one: the
// client has to tell "unknown" apart from "nothing matches".
func TestPlatformLogsService_FacetsStayNilWhenUnsupported(t *testing.T) {
	t.Parallel()

	adapter := &stubPlatformLogsAdapter{result: &observability.PlatformLogsResult{Took: 1}}
	svc := NewPlatformLogsService(adapter, testLogger())

	resp, err := svc.QueryPlatformLogs(context.Background(), facetRequest(true))
	require.NoError(t, err)

	assert.Nil(t, resp.Facets)
}
