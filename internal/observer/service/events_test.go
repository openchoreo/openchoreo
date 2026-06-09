// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

// fakeEventsAdapter implements observability.EventsAdapter for tests.
type fakeEventsAdapter struct {
	componentResult *observability.ComponentEventsResult
	componentErr    error
	workflowResult  *observability.WorkflowEventsResult
	workflowErr     error

	componentCalled bool
	workflowCalled  bool
	lastComponent   observability.ComponentEventsParams
	lastWorkflow    observability.WorkflowEventsParams
}

func (f *fakeEventsAdapter) GetComponentEvents(_ context.Context,
	params observability.ComponentEventsParams,
) (*observability.ComponentEventsResult, error) {
	f.componentCalled = true
	f.lastComponent = params
	return f.componentResult, f.componentErr
}

func (f *fakeEventsAdapter) GetWorkflowEvents(_ context.Context,
	params observability.WorkflowEventsParams,
) (*observability.WorkflowEventsResult, error) {
	f.workflowCalled = true
	f.lastWorkflow = params
	return f.workflowResult, f.workflowErr
}

func newEventsServiceForTest(t *testing.T, adapter observability.EventsAdapter) *EventsService {
	t.Helper()
	svc, err := NewEventsService(
		adapter,
		nil, // resolver — not used for workflow scope tests
		&config.Config{},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	require.NoError(t, err)
	return svc
}

func TestNewEventsService(t *testing.T) {
	t.Run("rejects nil adapter", func(t *testing.T) {
		_, err := NewEventsService(nil, nil, &config.Config{},
			slog.New(slog.NewTextHandler(io.Discard, nil)))
		require.Error(t, err)
	})

	t.Run("accepts non-nil adapter", func(t *testing.T) {
		svc, err := NewEventsService(&fakeEventsAdapter{}, nil, &config.Config{},
			slog.New(slog.NewTextHandler(io.Discard, nil)))
		require.NoError(t, err)
		require.NotNil(t, svc)
	})
}

func TestEventsServiceQueryWorkflowEvents(t *testing.T) {
	ts := time.Date(2026, 6, 5, 3, 7, 12, 0, time.UTC)
	adapter := &fakeEventsAdapter{
		workflowResult: &observability.WorkflowEventsResult{
			Events: []observability.EventEntry{{
				Timestamp:       ts,
				Message:         "Saw completed job",
				Type:            "Normal",
				Reason:          "SawCompletedJob",
				ObjectKind:      "CronJob",
				ObjectName:      "x",
				ObjectNamespace: "ns",
			}},
			TotalCount: 24,
			Took:       57,
		},
	}
	svc := newEventsServiceForTest(t, adapter)

	resp, err := svc.QueryEvents(context.Background(), &types.EventsQueryRequest{
		SearchScope: &types.SearchScope{
			Workflow: &types.WorkflowSearchScope{Namespace: "default", WorkflowRunName: "run-1", TaskName: "migrate"},
		},
		StartTime: "2026-06-05T02:58:31Z",
		EndTime:   "2026-06-05T03:08:31Z",
		Limit:     50,
		SortOrder: "asc",
	})
	require.NoError(t, err)
	require.True(t, adapter.workflowCalled)
	require.False(t, adapter.componentCalled)
	assert.Equal(t, "run-1", adapter.lastWorkflow.WorkflowRunName)
	assert.Equal(t, "migrate", adapter.lastWorkflow.TaskName)
	assert.Equal(t, 50, adapter.lastWorkflow.Limit)

	require.Len(t, resp.Events, 1)
	assert.Equal(t, "SawCompletedJob", resp.Events[0].Reason)
	assert.Equal(t, "Saw completed job", resp.Events[0].Message)
	assert.Equal(t, "Normal", resp.Events[0].Type)
	// Workflow-scoped events omit the OpenChoreo resource metadata block.
	assert.Nil(t, resp.Events[0].Metadata)
	assert.Equal(t, ts.Format(time.RFC3339), resp.Events[0].Timestamp)
	assert.Equal(t, 24, resp.Total)
	assert.Equal(t, 57, resp.TookMs)
}

func TestEventsServiceQueryEventsErrors(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		svc := newEventsServiceForTest(t, &fakeEventsAdapter{})
		_, err := svc.QueryEvents(context.Background(), nil)
		require.Error(t, err)
	})

	t.Run("adapter error wrapped as retrieval", func(t *testing.T) {
		adapter := &fakeEventsAdapter{workflowErr: errors.New("boom")}
		svc := newEventsServiceForTest(t, adapter)
		_, err := svc.QueryEvents(context.Background(), &types.EventsQueryRequest{
			SearchScope: &types.SearchScope{
				Workflow: &types.WorkflowSearchScope{Namespace: "default", WorkflowRunName: "r"},
			},
			StartTime: "2026-06-05T02:58:31Z",
			EndTime:   "2026-06-05T03:08:31Z",
		})
		require.ErrorIs(t, err, ErrEventsRetrieval)
	})

	t.Run("invalid start time", func(t *testing.T) {
		svc := newEventsServiceForTest(t, &fakeEventsAdapter{})
		_, err := svc.QueryEvents(context.Background(), &types.EventsQueryRequest{
			SearchScope: &types.SearchScope{
				Workflow: &types.WorkflowSearchScope{Namespace: "default", WorkflowRunName: "r"},
			},
			StartTime: "bad",
			EndTime:   "2026-06-05T03:08:31Z",
		})
		require.Error(t, err)
	})
}
