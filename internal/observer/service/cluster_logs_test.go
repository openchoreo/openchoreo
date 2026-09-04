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

// stubClusterLogsAdapter records the params it was called with and returns a
// canned result or error.
type stubClusterLogsAdapter struct {
	got    observability.ClusterLogsParams
	result *observability.ClusterLogsResult
	err    error
}

func (s *stubClusterLogsAdapter) GetClusterLogs(
	_ context.Context, params observability.ClusterLogsParams,
) (*observability.ClusterLogsResult, error) {
	s.got = params
	return s.result, s.err
}

func TestClusterLogsService_QueryClusterLogs(t *testing.T) {
	t.Parallel()

	collected := time.Date(2026, 8, 14, 16, 31, 0, 0, time.UTC)
	adapter := &stubClusterLogsAdapter{result: &observability.ClusterLogsResult{
		Logs: []observability.ClusterLogEntry{{
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

	svc := NewClusterLogsService(adapter, testLogger())
	resp, err := svc.QueryClusterLogs(context.Background(), &types.ClusterLogsQueryRequest{
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

// TestClusterLogsService_NotSupportedPassesThrough pins that a 501 from the adapter
// reaches the handler unwrapped, so it can answer 501 rather than reporting a failure.
func TestClusterLogsService_NotSupportedPassesThrough(t *testing.T) {
	t.Parallel()

	adapter := &stubClusterLogsAdapter{err: ErrClusterLogsNotSupported}
	svc := NewClusterLogsService(adapter, testLogger())

	_, err := svc.QueryClusterLogs(context.Background(), &types.ClusterLogsQueryRequest{
		StartTime: "2026-08-14T16:30:00Z",
		EndTime:   "2026-08-14T17:30:00Z",
	})
	require.ErrorIs(t, err, ErrClusterLogsNotSupported)
	assert.NotErrorIs(t, err, ErrClusterLogsRetrieval)
}

func TestClusterLogsService_RetrievalFailureIsWrapped(t *testing.T) {
	t.Parallel()

	adapter := &stubClusterLogsAdapter{err: errors.New("connection refused")}
	svc := NewClusterLogsService(adapter, testLogger())

	_, err := svc.QueryClusterLogs(context.Background(), &types.ClusterLogsQueryRequest{
		StartTime: "2026-08-14T16:30:00Z",
		EndTime:   "2026-08-14T17:30:00Z",
	})
	require.ErrorIs(t, err, ErrClusterLogsRetrieval)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestClusterLogsService_InvalidTimeRange(t *testing.T) {
	t.Parallel()

	adapter := &stubClusterLogsAdapter{}
	svc := NewClusterLogsService(adapter, testLogger())

	_, err := svc.QueryClusterLogs(context.Background(), &types.ClusterLogsQueryRequest{
		StartTime: "not-a-time",
		EndTime:   "2026-08-14T17:30:00Z",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse start time")
}
