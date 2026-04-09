// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestFilterNewEntries(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Hour)
	later := now.Add(time.Hour)

	entry := func(ts time.Time, log string) gen.WorkflowRunLogEntry {
		return gen.WorkflowRunLogEntry{Timestamp: &ts, Log: log}
	}

	tests := []struct {
		name      string
		entries   []gen.WorkflowRunLogEntry
		lastSeen  time.Time
		wantCount int
		wantLogs  []string
	}{
		{
			name:      "zero lastSeen returns all",
			entries:   []gen.WorkflowRunLogEntry{entry(now, "a"), entry(later, "b")},
			lastSeen:  time.Time{},
			wantCount: 2,
			wantLogs:  []string{"a", "b"},
		},
		{
			name:      "filters entries before lastSeen",
			entries:   []gen.WorkflowRunLogEntry{entry(earlier, "old"), entry(now, "current"), entry(later, "new")},
			lastSeen:  now,
			wantCount: 1,
			wantLogs:  []string{"new"},
		},
		{
			name:      "all entries before lastSeen",
			entries:   []gen.WorkflowRunLogEntry{entry(earlier, "old")},
			lastSeen:  now,
			wantCount: 0,
		},
		{
			name:      "empty entries",
			entries:   nil,
			lastSeen:  now,
			wantCount: 0,
		},
		{
			name:      "entry with nil timestamp is skipped",
			entries:   []gen.WorkflowRunLogEntry{{Log: "no-ts"}, entry(later, "new")},
			lastSeen:  now,
			wantCount: 1,
			wantLogs:  []string{"new"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterNewEntries(tt.entries, tt.lastSeen)
			require.Len(t, got, tt.wantCount)
			for i, log := range tt.wantLogs {
				assert.Equal(t, log, got[i].Log)
			}
		})
	}
}

func TestLogs_ValidationError(t *testing.T) {
	wr := New(nil) // client is never called when validation fails

	t.Run("missing namespace", func(t *testing.T) {
		err := wr.Logs(LogsParams{WorkflowRunName: "run-1"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "Missing required parameter: --namespace")
	})
}

func TestLogs_InvalidSince(t *testing.T) {
	wr := New(nil) // client is never called before --since validation

	t.Run("invalid duration", func(t *testing.T) {
		err := wr.Logs(LogsParams{Namespace: "ns", WorkflowRunName: "run-1", Since: "notaduration"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --since value")
	})

	t.Run("negative duration", func(t *testing.T) {
		err := wr.Logs(LogsParams{Namespace: "ns", WorkflowRunName: "run-1", Since: "-5m"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --since value")
	})

	t.Run("zero duration", func(t *testing.T) {
		err := wr.Logs(LogsParams{Namespace: "ns", WorkflowRunName: "run-1", Since: "0s"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duration must be positive")
	})
}

// --- Logs: GetWorkflowRunStatus error ---

func TestLogs_StatusError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetWorkflowRunStatus(mock.Anything, "ns", "run-1").Return(nil, fmt.Errorf("status unavailable"))

	wr := New(mc)
	err := wr.Logs(LogsParams{Namespace: "ns", WorkflowRunName: "run-1"})
	assert.ErrorContains(t, err, "failed to get workflow run status")
	assert.ErrorContains(t, err, "status unavailable")
}

// --- Logs: live path ---

func TestLogs_LiveLogs_Success(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetWorkflowRunStatus(mock.Anything, "ns", "run-1").Return(
		&gen.WorkflowRunStatusResponse{HasLiveObservability: true}, nil)
	mc.EXPECT().GetWorkflowRunLogs(mock.Anything, "ns", "run-1", mock.Anything).Return(
		[]gen.WorkflowRunLogEntry{
			{Timestamp: &now, Log: "step started"},
			{Log: "no timestamp line"},
		}, nil)

	wr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, wr.Logs(LogsParams{Namespace: "ns", WorkflowRunName: "run-1"}))
	})
	assert.Contains(t, out, "step started")
	assert.Contains(t, out, "no timestamp line")
}

func TestLogs_LiveLogs_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetWorkflowRunStatus(mock.Anything, "ns", "run-1").Return(
		&gen.WorkflowRunStatusResponse{HasLiveObservability: true}, nil)
	mc.EXPECT().GetWorkflowRunLogs(mock.Anything, "ns", "run-1", mock.Anything).Return(
		nil, fmt.Errorf("log fetch failed"))

	wr := New(mc)
	err := wr.Logs(LogsParams{Namespace: "ns", WorkflowRunName: "run-1"})
	assert.ErrorContains(t, err, "failed to get live logs")
	assert.ErrorContains(t, err, "log fetch failed")
}

func TestLogs_LiveLogs_WithSince(t *testing.T) {
	now := time.Now()
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetWorkflowRunStatus(mock.Anything, "ns", "run-1").Return(
		&gen.WorkflowRunStatusResponse{HasLiveObservability: true}, nil)
	mc.EXPECT().GetWorkflowRunLogs(mock.Anything, "ns", "run-1", mock.MatchedBy(func(p *gen.GetWorkflowRunLogsParams) bool {
		return p != nil && p.SinceSeconds != nil && *p.SinceSeconds == 300 // 5m
	})).Return([]gen.WorkflowRunLogEntry{{Timestamp: &now, Log: "recent"}}, nil)

	wr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, wr.Logs(LogsParams{Namespace: "ns", WorkflowRunName: "run-1", Since: "5m"}))
	})
	assert.Contains(t, out, "recent")
}

// --- resolveObserverURL ---

func TestResolveObserverURL_ViaWorkflowPlane(t *testing.T) {
	url := "http://observer.example.com"
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetWorkflow(mock.Anything, "ns", "my-wf").Return(&gen.Workflow{
		Spec: &gen.WorkflowSpec{
			WorkflowPlaneRef: &gen.WorkflowPlaneRef{Kind: gen.WorkflowPlaneRefKindWorkflowPlane, Name: "my-wp"},
		},
	}, nil)
	obsRef := &gen.ObservabilityPlaneRef{Kind: gen.ObservabilityPlaneRefKindObservabilityPlane, Name: "my-obs"}
	mc.EXPECT().GetWorkflowPlane(mock.Anything, "ns", "my-wp").Return(
		&gen.WorkflowPlane{Spec: &gen.WorkflowPlaneSpec{ObservabilityPlaneRef: obsRef}}, nil)
	mc.EXPECT().GetObservabilityPlane(mock.Anything, "ns", "my-obs").Return(
		&gen.ObservabilityPlane{Spec: &gen.ObservabilityPlaneSpec{ObserverURL: &url}}, nil)

	got, err := resolveObserverURL(context.TODO(), mc, "ns", "my-wf")
	require.NoError(t, err)
	assert.Equal(t, url, got)
}

func TestResolveObserverURL_FallbackToDefault(t *testing.T) {
	url := "http://default-observer.example.com"
	mc := mocks.NewMockInterface(t)
	// Workflow not found — falls through to default plane lookup
	mc.EXPECT().GetWorkflow(mock.Anything, "ns", "missing-wf").Return(nil, fmt.Errorf("not found"))
	mc.EXPECT().GetObservabilityPlane(mock.Anything, "ns", "default").Return(
		&gen.ObservabilityPlane{Spec: &gen.ObservabilityPlaneSpec{ObserverURL: &url}}, nil)

	got, err := resolveObserverURL(context.TODO(), mc, "ns", "missing-wf")
	require.NoError(t, err)
	assert.Equal(t, url, got)
}

// --- followLiveLogs: context cancellation ---

func TestFollowLiveLogs_ContextCancelled(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetWorkflowRunLogs(mock.Anything, "ns", "run-1", mock.Anything).Return(
		[]gen.WorkflowRunLogEntry{}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so the poll loop exits right away

	wr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, wr.followLiveLogs(ctx, mc, LogsParams{Namespace: "ns", WorkflowRunName: "run-1"}, 0))
	})
	assert.Contains(t, out, "Stopping log streaming...")
}

func TestFollowLiveLogs_InitialFetchError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetWorkflowRunLogs(mock.Anything, "ns", "run-1", mock.Anything).Return(
		nil, fmt.Errorf("initial fetch failed"))

	ctx := context.Background()
	wr := New(mc)
	err := wr.followLiveLogs(ctx, mc, LogsParams{Namespace: "ns", WorkflowRunName: "run-1"}, 0)
	assert.ErrorContains(t, err, "failed to get live logs")
}

func TestFollowLiveLogs_RunCompleted(t *testing.T) {
	// Initial fetch returns empty, then on first poll status shows no live observability
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetWorkflowRunLogs(mock.Anything, "ns", "run-1", mock.Anything).Return(
		[]gen.WorkflowRunLogEntry{}, nil).Once()
	mc.EXPECT().GetWorkflowRunStatus(mock.Anything, "ns", "run-1").Return(
		&gen.WorkflowRunStatusResponse{HasLiveObservability: false}, nil)

	wr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, wr.followLiveLogs(context.Background(), mc, LogsParams{Namespace: "ns", WorkflowRunName: "run-1"}, 0))
	})
	assert.Contains(t, out, "Workflow run completed. Live logs are no longer available.")
}

func TestFollowLiveLogs_PollStatusError_ThenCancel(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetWorkflowRunLogs(mock.Anything, "ns", "run-1", mock.Anything).Return(
		[]gen.WorkflowRunLogEntry{}, nil).Once()

	ctx, cancel := context.WithCancel(context.Background())
	// Status call returns error and context is already cancelled
	mc.EXPECT().GetWorkflowRunStatus(mock.Anything, "ns", "run-1").RunAndReturn(
		func(c context.Context, ns, run string) (*gen.WorkflowRunStatusResponse, error) {
			cancel()
			return nil, fmt.Errorf("status error")
		})

	wr := New(mc)
	require.NoError(t, wr.followLiveLogs(ctx, mc, LogsParams{Namespace: "ns", WorkflowRunName: "run-1"}, 0))
}

// --- resolveObserverURLFromObsRef ---

func TestResolveObserverURLFromObsRef_NamespacedObsPlane(t *testing.T) {
	url := "http://observer.example.com"
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetObservabilityPlane(mock.Anything, "ns", "my-obs").Return(
		&gen.ObservabilityPlane{Spec: &gen.ObservabilityPlaneSpec{ObserverURL: &url}}, nil)

	obsRef := &gen.ObservabilityPlaneRef{Kind: gen.ObservabilityPlaneRefKindObservabilityPlane, Name: "my-obs"}
	got, err := resolveObserverURLFromObsRef(context.TODO(), mc, "ns", obsRef, nil)
	require.NoError(t, err)
	assert.Equal(t, url, got)
}

func TestResolveObserverURLFromObsRef_ClusterObsPlaneViaObsRef(t *testing.T) {
	url := "http://cluster-observer.example.com"
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetClusterObservabilityPlane(mock.Anything, "my-cluster-obs").Return(
		&gen.ClusterObservabilityPlane{Spec: &gen.ClusterObservabilityPlaneSpec{ObserverURL: &url}}, nil)

	obsRef := &gen.ObservabilityPlaneRef{Kind: gen.ObservabilityPlaneRefKindClusterObservabilityPlane, Name: "my-cluster-obs"}
	got, err := resolveObserverURLFromObsRef(context.TODO(), mc, "ns", obsRef, nil)
	require.NoError(t, err)
	assert.Equal(t, url, got)
}

func TestResolveObserverURLFromObsRef_ClusterObsPlaneViaClusterRef(t *testing.T) {
	url := "http://cluster-observer.example.com"
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetClusterObservabilityPlane(mock.Anything, "my-cluster-obs").Return(
		&gen.ClusterObservabilityPlane{Spec: &gen.ClusterObservabilityPlaneSpec{ObserverURL: &url}}, nil)

	clusterRef := &gen.ClusterObservabilityPlaneRef{Name: "my-cluster-obs"}
	got, err := resolveObserverURLFromObsRef(context.TODO(), mc, "ns", nil, clusterRef)
	require.NoError(t, err)
	assert.Equal(t, url, got)
}

func TestResolveObserverURLFromObsRef_FallbackToDefaultObsPlane(t *testing.T) {
	url := "http://default-observer.example.com"
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetObservabilityPlane(mock.Anything, "ns", "default").Return(
		&gen.ObservabilityPlane{Spec: &gen.ObservabilityPlaneSpec{ObserverURL: &url}}, nil)

	got, err := resolveObserverURLFromObsRef(context.TODO(), mc, "ns", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, url, got)
}

func TestResolveObserverURLFromObsRef_FallbackToDefaultClusterObsPlane(t *testing.T) {
	url := "http://default-cluster-observer.example.com"
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetObservabilityPlane(mock.Anything, "ns", "default").Return(nil, fmt.Errorf("not found"))
	mc.EXPECT().GetClusterObservabilityPlane(mock.Anything, "default").Return(
		&gen.ClusterObservabilityPlane{Spec: &gen.ClusterObservabilityPlaneSpec{ObserverURL: &url}}, nil)

	got, err := resolveObserverURLFromObsRef(context.TODO(), mc, "ns", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, url, got)
}

func TestResolveObserverURLFromObsRef_NoObserverConfigured(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetObservabilityPlane(mock.Anything, "ns", "default").Return(nil, fmt.Errorf("not found"))
	mc.EXPECT().GetClusterObservabilityPlane(mock.Anything, "default").Return(nil, fmt.Errorf("not found"))

	_, err := resolveObserverURLFromObsRef(context.TODO(), mc, "ns", nil, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "no observer URL configured")
}

func TestResolveObserverURLFromObsRef_NamespacedObsPlaneError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetObservabilityPlane(mock.Anything, "ns", "bad-obs").Return(nil, fmt.Errorf("forbidden"))

	obsRef := &gen.ObservabilityPlaneRef{Kind: gen.ObservabilityPlaneRefKindObservabilityPlane, Name: "bad-obs"}
	_, err := resolveObserverURLFromObsRef(context.TODO(), mc, "ns", obsRef, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to get observability plane")
}

// --- resolveWorkflowPlaneObsRef ---

func TestResolveWorkflowPlaneObsRef_WorkflowPlane(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetWorkflow(mock.Anything, "ns", "my-wf").Return(&gen.Workflow{
		Spec: &gen.WorkflowSpec{
			WorkflowPlaneRef: &gen.WorkflowPlaneRef{Kind: gen.WorkflowPlaneRefKindWorkflowPlane, Name: "my-wp"},
		},
	}, nil)
	obsRef := &gen.ObservabilityPlaneRef{Kind: gen.ObservabilityPlaneRefKindObservabilityPlane, Name: "my-obs"}
	mc.EXPECT().GetWorkflowPlane(mock.Anything, "ns", "my-wp").Return(&gen.WorkflowPlane{
		Spec: &gen.WorkflowPlaneSpec{ObservabilityPlaneRef: obsRef},
	}, nil)

	gotObs, gotCluster := resolveWorkflowPlaneObsRef(context.TODO(), mc, "ns", "my-wf")
	require.NotNil(t, gotObs)
	assert.Equal(t, "my-obs", gotObs.Name)
	assert.Nil(t, gotCluster)
}

func TestResolveWorkflowPlaneObsRef_ClusterWorkflowPlane(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetWorkflow(mock.Anything, "ns", "my-wf").Return(&gen.Workflow{
		Spec: &gen.WorkflowSpec{
			WorkflowPlaneRef: &gen.WorkflowPlaneRef{Kind: gen.WorkflowPlaneRefKindClusterWorkflowPlane, Name: "my-cwp"},
		},
	}, nil)
	clusterRef := &gen.ClusterObservabilityPlaneRef{Name: "my-cluster-obs"}
	mc.EXPECT().GetClusterWorkflowPlane(mock.Anything, "my-cwp").Return(&gen.ClusterWorkflowPlane{
		Spec: &gen.ClusterWorkflowPlaneSpec{ObservabilityPlaneRef: clusterRef},
	}, nil)

	gotObs, gotCluster := resolveWorkflowPlaneObsRef(context.TODO(), mc, "ns", "my-wf")
	assert.Nil(t, gotObs)
	require.NotNil(t, gotCluster)
	assert.Equal(t, "my-cluster-obs", gotCluster.Name)
}

func TestResolveWorkflowPlaneObsRef_WorkflowNotFound(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetWorkflow(mock.Anything, "ns", "missing-wf").Return(nil, fmt.Errorf("not found"))

	gotObs, gotCluster := resolveWorkflowPlaneObsRef(context.TODO(), mc, "ns", "missing-wf")
	assert.Nil(t, gotObs)
	assert.Nil(t, gotCluster)
}

func TestFollowLiveLogs_PollNewLogs(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Second)
	mc := mocks.NewMockInterface(t)

	// Initial fetch: one entry
	mc.EXPECT().GetWorkflowRunLogs(mock.Anything, "ns", "run-1", mock.Anything).Return(
		[]gen.WorkflowRunLogEntry{{Timestamp: &now, Log: "first"}}, nil).Once()

	// Poll: status still live, both entries returned (simulating overlap), context cancelled inside RunAndReturn
	ctx, cancel := context.WithCancel(context.Background())
	mc.EXPECT().GetWorkflowRunStatus(mock.Anything, "ns", "run-1").Return(
		&gen.WorkflowRunStatusResponse{HasLiveObservability: true}, nil).Once()
	mc.EXPECT().GetWorkflowRunLogs(mock.Anything, "ns", "run-1", mock.Anything).RunAndReturn(
		func(_ context.Context, _, _ string, _ *gen.GetWorkflowRunLogsParams) ([]gen.WorkflowRunLogEntry, error) {
			cancel()
			return []gen.WorkflowRunLogEntry{
				{Timestamp: &now, Log: "first"},
				{Timestamp: &later, Log: "second"},
			}, nil
		}).Once()

	wr := New(mc)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, wr.followLiveLogs(ctx, mc, LogsParams{Namespace: "ns", WorkflowRunName: "run-1"}, 0))
	})
	assert.Equal(t, 1, strings.Count(out, "first"), "duplicate suppression: 'first' should appear exactly once")
	assert.Contains(t, out, "second")
	assert.Contains(t, out, "Stopping log streaming...")
}

func TestFollowLiveLogs_PollFetchError_ThenCancel(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetWorkflowRunLogs(mock.Anything, "ns", "run-1", mock.Anything).Return(
		[]gen.WorkflowRunLogEntry{}, nil).Once()

	ctx, cancel := context.WithCancel(context.Background())
	mc.EXPECT().GetWorkflowRunStatus(mock.Anything, "ns", "run-1").Return(
		&gen.WorkflowRunStatusResponse{HasLiveObservability: true}, nil).Once()
	mc.EXPECT().GetWorkflowRunLogs(mock.Anything, "ns", "run-1", mock.Anything).RunAndReturn(
		func(c context.Context, _, _ string, _ *gen.GetWorkflowRunLogsParams) ([]gen.WorkflowRunLogEntry, error) {
			cancel()
			return nil, fmt.Errorf("transient error")
		}).Once()

	wr := New(mc)
	require.NoError(t, wr.followLiveLogs(ctx, mc, LogsParams{Namespace: "ns", WorkflowRunName: "run-1"}, 0))
}

func TestParseSinceToSeconds(t *testing.T) {
	tests := []struct {
		name  string
		since string
		want  int64
	}{
		{name: "empty string", since: "", want: 0},
		{name: "5 minutes", since: "5m", want: 300},
		{name: "1 hour", since: "1h", want: 3600},
		{name: "30 seconds", since: "30s", want: 30},
		{name: "invalid returns 0", since: "notaduration", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseSinceToSeconds(tt.since))
		})
	}
}
