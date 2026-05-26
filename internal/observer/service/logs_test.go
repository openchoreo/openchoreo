// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

// Sample UUIDs used as resource identifiers in tests.
const (
	sampleProjectUID     = "5f3d8a1b-2c4e-4d7f-9a6b-1e2c3d4f5a6b"
	sampleComponentUID   = "a1b2c3d4-e5f6-4a7b-8c9d-0e1f2a3b4c5d"
	sampleEnvironmentUID = "b2c3d4e5-f6a7-4b8c-9d0e-1f2a3b4c5d6e"
)

// fakeLogsAdapter implements observability.LogsAdapter for tests.
type fakeLogsAdapter struct {
	componentResult *observability.ComponentApplicationLogsResult
	componentErr    error
	workflowResult  *observability.WorkflowLogsResult
	workflowErr     error

	componentCalled bool
	workflowCalled  bool
	lastComponent   observability.ComponentApplicationLogsParams
	lastWorkflow    observability.WorkflowLogsParams
}

func (f *fakeLogsAdapter) GetComponentApplicationLogs(_ context.Context,
	params observability.ComponentApplicationLogsParams,
) (*observability.ComponentApplicationLogsResult, error) {
	f.componentCalled = true
	f.lastComponent = params
	return f.componentResult, f.componentErr
}

func (f *fakeLogsAdapter) GetWorkflowLogs(_ context.Context,
	params observability.WorkflowLogsParams,
) (*observability.WorkflowLogsResult, error) {
	f.workflowCalled = true
	f.lastWorkflow = params
	return f.workflowResult, f.workflowErr
}

func newLogsServiceForTest(t *testing.T, adapter observability.LogsAdapter) *LogsService {
	t.Helper()
	svc, err := NewLogsService(
		adapter,
		nil, // resolver — not used for workflow scope tests
		&config.Config{},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	require.NoError(t, err)
	return svc
}

func TestNewLogsService(t *testing.T) {
	t.Parallel()

	t.Run("rejects nil adapter", func(t *testing.T) {
		t.Parallel()
		_, err := NewLogsService(nil, nil, &config.Config{},
			slog.New(slog.NewTextHandler(io.Discard, nil)))
		require.Error(t, err)
	})

	t.Run("accepts non-nil adapter", func(t *testing.T) {
		t.Parallel()
		svc, err := NewLogsService(&fakeLogsAdapter{}, nil, &config.Config{},
			slog.New(slog.NewTextHandler(io.Discard, nil)))
		require.NoError(t, err)
		require.NotNil(t, svc)
	})
}

func TestLogsService_QueryLogs_NilRequest(t *testing.T) {
	t.Parallel()
	svc := newLogsServiceForTest(t, &fakeLogsAdapter{})
	_, err := svc.QueryLogs(context.Background(), nil)
	require.Error(t, err)
}

func TestLogsService_QueryLogs_InvalidStartTime(t *testing.T) {
	t.Parallel()
	svc := newLogsServiceForTest(t, &fakeLogsAdapter{})
	_, err := svc.QueryLogs(context.Background(), &types.LogsQueryRequest{
		SearchScope: &types.SearchScope{
			Workflow: &types.WorkflowSearchScope{
				Namespace:       "ns",
				WorkflowRunName: "wf-1",
			},
		},
		StartTime: "not-a-time",
		EndTime:   "2026-03-07T11:00:00Z",
	})
	require.Error(t, err)
}

func TestLogsService_QueryLogs_InvalidEndTime(t *testing.T) {
	t.Parallel()
	svc := newLogsServiceForTest(t, &fakeLogsAdapter{})
	_, err := svc.QueryLogs(context.Background(), &types.LogsQueryRequest{
		SearchScope: &types.SearchScope{
			Workflow: &types.WorkflowSearchScope{
				Namespace:       "ns",
				WorkflowRunName: "wf-1",
			},
		},
		StartTime: "2026-03-07T10:00:00Z",
		EndTime:   "garbage",
	})
	require.Error(t, err)
}

func TestLogsService_QueryLogs_NilSearchScope(t *testing.T) {
	t.Parallel()
	svc := newLogsServiceForTest(t, &fakeLogsAdapter{})
	_, err := svc.QueryLogs(context.Background(), &types.LogsQueryRequest{
		SearchScope: nil,
		StartTime:   "2026-03-07T10:00:00Z",
		EndTime:     "2026-03-07T11:00:00Z",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrLogsResolveSearchScope)
}

func TestLogsService_QueryLogs_EmptySearchScope(t *testing.T) {
	t.Parallel()
	svc := newLogsServiceForTest(t, &fakeLogsAdapter{})
	_, err := svc.QueryLogs(context.Background(), &types.LogsQueryRequest{
		SearchScope: &types.SearchScope{},
		StartTime:   "2026-03-07T10:00:00Z",
		EndTime:     "2026-03-07T11:00:00Z",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrLogsResolveSearchScope)
}

func TestLogsService_QueryLogs_WorkflowScope_Success(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)
	adapter := &fakeLogsAdapter{
		workflowResult: &observability.WorkflowLogsResult{
			Logs: []observability.WorkflowLogEntry{
				{
					Timestamp: now,
					Log:       "starting step",
					LogLevel:  "INFO",
				},
				{
					Timestamp: now.Add(time.Second),
					Log:       "step done",
					LogLevel:  "INFO",
				},
			},
			TotalCount: 2,
			Took:       12,
		},
	}
	svc := newLogsServiceForTest(t, adapter)

	resp, err := svc.QueryLogs(context.Background(), &types.LogsQueryRequest{
		SearchScope: &types.SearchScope{
			Workflow: &types.WorkflowSearchScope{
				Namespace:       "ns-1",
				WorkflowRunName: "wf-run-1",
				TaskName:        "task-1",
			},
		},
		StartTime:    "2026-03-07T10:00:00Z",
		EndTime:      "2026-03-07T11:00:00Z",
		SearchPhrase: "error",
		LogLevels:    []string{"INFO"},
		Limit:        50,
		SortOrder:    "asc",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, adapter.workflowCalled)
	assert.False(t, adapter.componentCalled)

	assert.Equal(t, "ns-1", adapter.lastWorkflow.Namespace)
	assert.Equal(t, "wf-run-1", adapter.lastWorkflow.WorkflowRunName)
	assert.Equal(t, "task-1", adapter.lastWorkflow.TaskName)
	assert.Equal(t, "error", adapter.lastWorkflow.SearchPhrase)
	assert.Equal(t, []string{"INFO"}, adapter.lastWorkflow.LogLevels)
	assert.Equal(t, 50, adapter.lastWorkflow.Limit)
	assert.Equal(t, "asc", adapter.lastWorkflow.SortOrder)

	assert.Len(t, resp.Logs, 2)
	assert.Equal(t, 2, resp.Total)
	assert.Equal(t, 12, resp.TookMs)
	assert.Equal(t, "starting step", resp.Logs[0].Log)
	assert.Equal(t, "INFO", resp.Logs[0].Level)
}

func TestLogsService_QueryLogs_WorkflowScope_AdapterError(t *testing.T) {
	t.Parallel()
	adapter := &fakeLogsAdapter{workflowErr: errors.New("upstream boom")}
	svc := newLogsServiceForTest(t, adapter)

	_, err := svc.QueryLogs(context.Background(), &types.LogsQueryRequest{
		SearchScope: &types.SearchScope{
			Workflow: &types.WorkflowSearchScope{
				Namespace:       "ns",
				WorkflowRunName: "wf",
			},
		},
		StartTime: "2026-03-07T10:00:00Z",
		EndTime:   "2026-03-07T11:00:00Z",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrLogsRetrieval)
}

func TestLogsService_QueryLogs_WorkflowScope_AdapterReturnsNil(t *testing.T) {
	t.Parallel()
	adapter := &fakeLogsAdapter{workflowResult: nil}
	svc := newLogsServiceForTest(t, adapter)

	_, err := svc.QueryLogs(context.Background(), &types.LogsQueryRequest{
		SearchScope: &types.SearchScope{
			Workflow: &types.WorkflowSearchScope{
				Namespace:       "ns",
				WorkflowRunName: "wf",
			},
		},
		StartTime: "2026-03-07T10:00:00Z",
		EndTime:   "2026-03-07T11:00:00Z",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrLogsRetrieval)
}

func TestLogsService_QueryLogs_ComponentScope_RequiresResolver(t *testing.T) {
	t.Parallel()
	// resolver is nil — component scope should fail at resolution.
	svc := newLogsServiceForTest(t, &fakeLogsAdapter{})

	_, err := svc.QueryLogs(context.Background(), &types.LogsQueryRequest{
		SearchScope: &types.SearchScope{
			Component: &types.ComponentSearchScope{
				Namespace:   "ns",
				Project:     "proj",
				Component:   "comp",
				Environment: "env",
			},
		},
		StartTime: "2026-03-07T10:00:00Z",
		EndTime:   "2026-03-07T11:00:00Z",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrLogsResolveSearchScope)
}

func TestLogsService_QueryLogs_ComponentScope_Success(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)

	tokenSrv := newAlwaysOKTokenServer(t)
	defer tokenSrv.Close()

	// Route by URL path so each resource type returns its own UID,
	// confirming the service wires each lookup to the correct field.
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/projects/"):
			_, _ = w.Write([]byte(uidResponse(sampleProjectUID)))
		case strings.Contains(r.URL.Path, "/components/"):
			_, _ = w.Write([]byte(uidResponse(sampleComponentUID)))
		case strings.Contains(r.URL.Path, "/environments/"):
			_, _ = w.Write([]byte(uidResponse(sampleEnvironmentUID)))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer apiSrv.Close()

	resolver := newTestResolver(t, apiSrv, tokenSrv, &config.UIDResolverConfig{MaxAuthRetry: 1})

	adapter := &fakeLogsAdapter{
		componentResult: &observability.ComponentApplicationLogsResult{
			Logs: []observability.LogEntry{
				{Timestamp: now, Log: "hello", LogLevel: "INFO", ComponentName: "comp"},
			},
			TotalCount: 1,
			Took:       3,
		},
	}
	svc, err := NewLogsService(adapter, resolver, &config.Config{},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	resp, err := svc.QueryLogs(context.Background(), &types.LogsQueryRequest{
		SearchScope: &types.SearchScope{
			Component: &types.ComponentSearchScope{
				Namespace:   "ns",
				Project:     "proj",
				Component:   "comp",
				Environment: "env",
			},
		},
		StartTime: "2026-03-07T10:00:00Z",
		EndTime:   "2026-03-07T11:00:00Z",
		Limit:     25,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, adapter.componentCalled)
	assert.False(t, adapter.workflowCalled)
	assert.Equal(t, "ns", adapter.lastComponent.Namespace)
	assert.Equal(t, sampleProjectUID, adapter.lastComponent.ProjectID)
	assert.Equal(t, sampleComponentUID, adapter.lastComponent.ComponentID)
	assert.Equal(t, sampleEnvironmentUID, adapter.lastComponent.EnvironmentID)
	require.Len(t, resp.Logs, 1)
	assert.Equal(t, "hello", resp.Logs[0].Log)
}

func TestLogsService_ConvertComponentLogsToResponse(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)
	svc := newLogsServiceForTest(t, &fakeLogsAdapter{})

	result := &observability.ComponentApplicationLogsResult{
		Logs: []observability.LogEntry{
			{
				Timestamp:       now,
				Log:             "hello",
				LogLevel:        "INFO",
				ComponentName:   "comp",
				ProjectName:     "proj",
				EnvironmentName: "env",
				NamespaceName:   "ns",
				ComponentID:     sampleComponentUID,
				ProjectID:       sampleProjectUID,
				EnvironmentID:   sampleEnvironmentUID,
				ContainerName:   "main",
				PodName:         "pod-1",
				PodNamespace:    "ns",
			},
		},
		TotalCount: 1,
		Took:       5,
	}
	resp := svc.convertComponentLogsToResponse(result)
	require.NotNil(t, resp)
	require.Len(t, resp.Logs, 1)
	assert.Equal(t, "hello", resp.Logs[0].Log)
	require.NotNil(t, resp.Logs[0].Metadata)
	assert.Equal(t, "comp", resp.Logs[0].Metadata.ComponentName)
	assert.Equal(t, "pod-1", resp.Logs[0].Metadata.PodName)
	assert.Equal(t, sampleComponentUID, resp.Logs[0].Metadata.ComponentUID)
	assert.Equal(t, sampleProjectUID, resp.Logs[0].Metadata.ProjectUID)
	assert.Equal(t, sampleEnvironmentUID, resp.Logs[0].Metadata.EnvironmentUID)
	assert.Equal(t, 1, resp.Total)
	assert.Equal(t, 5, resp.TookMs)
}

func TestLogsService_ConvertWorkflowLogsToResponse(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)
	svc := newLogsServiceForTest(t, &fakeLogsAdapter{})

	result := &observability.WorkflowLogsResult{
		Logs: []observability.WorkflowLogEntry{
			{Timestamp: now, Log: "line-1", LogLevel: "INFO"},
			{Timestamp: now.Add(time.Second), Log: "line-2", LogLevel: "ERROR"},
		},
		TotalCount: 2,
		Took:       7,
	}
	resp := svc.convertWorkflowLogsToResponse(result)
	require.NotNil(t, resp)
	require.Len(t, resp.Logs, 2)
	assert.Equal(t, "line-1", resp.Logs[0].Log)
	assert.Equal(t, "ERROR", resp.Logs[1].Level)
	assert.Equal(t, 2, resp.Total)
	assert.Equal(t, 7, resp.TookMs)
}
