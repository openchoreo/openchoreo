// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	obsgen "github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/service/mocks"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

func TestQueryComponentLogs(t *testing.T) {
	ctx := context.Background()

	t.Run("builds component scope and forwards to logs service", func(t *testing.T) {
		logsSvc := mocks.NewMockLogsQuerier(t)
		logsSvc.EXPECT().
			QueryLogs(mock.Anything, mock.MatchedBy(func(req *types.LogsQueryRequest) bool {
				if req.SearchScope == nil || req.SearchScope.Component == nil {
					return false
				}
				c := req.SearchScope.Component
				return c.Namespace == testNamespace &&
					c.Project == testProject &&
					c.Component == testComponent &&
					c.Environment == testEnvironment &&
					req.StartTime == testStartTime &&
					req.EndTime == testEndTime &&
					req.SearchPhrase == "err" &&
					len(req.LogLevels) == 1 && req.LogLevels[0] == "ERROR" &&
					req.Limit == 50 &&
					req.SortOrder == sortOrderAsc
			})).
			Return(&types.LogsQueryResponse{}, nil)

		h := newTestMCPHandler(t, withLogsService(logsSvc))
		_, err := h.QueryComponentLogs(ctx, testNamespace, testProject, testComponent, testEnvironment,
			testStartTime, testEndTime, "err", []string{"ERROR"}, 50, sortOrderAsc)
		require.NoError(t, err)
	})

	t.Run("zero limit and empty sort use defaults", func(t *testing.T) {
		logsSvc := mocks.NewMockLogsQuerier(t)
		logsSvc.EXPECT().
			QueryLogs(mock.Anything, mock.MatchedBy(func(req *types.LogsQueryRequest) bool {
				return req.Limit == 100 && req.SortOrder == sortOrderDesc && req.LogLevels != nil && len(req.LogLevels) == 0
			})).
			Return(&types.LogsQueryResponse{}, nil)

		h := newTestMCPHandler(t, withLogsService(logsSvc))
		_, err := h.QueryComponentLogs(ctx, testNamespace, "", "", "", testStartTime, testEndTime, "", nil, 0, "")
		require.NoError(t, err)
	})

	t.Run("logs service error propagated", func(t *testing.T) {
		logsSvc := mocks.NewMockLogsQuerier(t)
		logsSvc.EXPECT().QueryLogs(mock.Anything, mock.Anything).Return(nil, errors.New("backend down"))

		h := newTestMCPHandler(t, withLogsService(logsSvc))
		_, err := h.QueryComponentLogs(ctx, testNamespace, testProject, testComponent, testEnvironment,
			testStartTime, testEndTime, "", nil, 10, sortOrderDesc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "backend down")
	})
}

func TestQueryWorkflowLogs(t *testing.T) {
	ctx := context.Background()

	t.Run("builds workflow scope and forwards to logs service", func(t *testing.T) {
		logsSvc := mocks.NewMockLogsQuerier(t)
		logsSvc.EXPECT().
			QueryLogs(mock.Anything, mock.MatchedBy(func(req *types.LogsQueryRequest) bool {
				if req.SearchScope == nil || req.SearchScope.Workflow == nil {
					return false
				}
				w := req.SearchScope.Workflow
				return w.Namespace == testNamespace &&
					w.WorkflowRunName == "run-1" &&
					w.TaskName == "task-a" &&
					req.StartTime == testStartTime &&
					req.EndTime == testEndTime
			})).
			Return(&types.LogsQueryResponse{}, nil)

		h := newTestMCPHandler(t, withLogsService(logsSvc))
		_, err := h.QueryWorkflowLogs(ctx, testNamespace, "run-1", "task-a",
			testStartTime, testEndTime, "", nil, 10, sortOrderDesc)
		require.NoError(t, err)
	})
}

func TestQueryResourceMetrics(t *testing.T) {
	ctx := context.Background()

	t.Run("resource metric type and scope forwarded", func(t *testing.T) {
		metricsSvc := mocks.NewMockMetricsQuerier(t)
		step := "5m"
		metricsSvc.EXPECT().
			QueryMetrics(mock.Anything, mock.MatchedBy(func(req *types.MetricsQueryRequest) bool {
				return req.Metric == types.MetricTypeResource &&
					req.SearchScope.Namespace == testNamespace &&
					req.SearchScope.Project == testProject &&
					req.SearchScope.Component == testComponent &&
					req.SearchScope.Environment == testEnvironment &&
					req.StartTime == testStartTime &&
					req.EndTime == testEndTime &&
					req.Step != nil && *req.Step == step
			})).
			Return(map[string]any{}, nil)

		h := newTestMCPHandler(t, withMetricsService(metricsSvc))
		_, err := h.QueryResourceMetrics(ctx, testNamespace, testProject, testComponent, testEnvironment,
			testStartTime, testEndTime, &step)
		require.NoError(t, err)
	})

	t.Run("nil step allowed", func(t *testing.T) {
		metricsSvc := mocks.NewMockMetricsQuerier(t)
		metricsSvc.EXPECT().
			QueryMetrics(mock.Anything, mock.MatchedBy(func(req *types.MetricsQueryRequest) bool {
				return req.Step == nil
			})).
			Return(nil, nil)

		h := newTestMCPHandler(t, withMetricsService(metricsSvc))
		_, err := h.QueryResourceMetrics(ctx, testNamespace, "", "", "", testStartTime, testEndTime, nil)
		require.NoError(t, err)
	})
}

func TestQueryHTTPMetrics(t *testing.T) {
	ctx := context.Background()

	metricsSvc := mocks.NewMockMetricsQuerier(t)
	metricsSvc.EXPECT().
		QueryMetrics(mock.Anything, mock.MatchedBy(func(req *types.MetricsQueryRequest) bool {
			return req.Metric == types.MetricTypeHTTP
		})).
		Return(nil, nil)

	h := newTestMCPHandler(t, withMetricsService(metricsSvc))
	_, err := h.QueryHTTPMetrics(ctx, testNamespace, testProject, testComponent, testEnvironment,
		testStartTime, testEndTime, nil)
	require.NoError(t, err)
}

func TestQueryTraces(t *testing.T) {
	ctx := context.Background()

	t.Run("parses RFC3339 and forwards request", func(t *testing.T) {
		tracesSvc := mocks.NewMockTracesQuerier(t)
		wantStart, _ := time.Parse(time.RFC3339, testStartTime)
		wantEnd, _ := time.Parse(time.RFC3339, testEndTime)
		tracesSvc.EXPECT().
			QueryTraces(mock.Anything, mock.MatchedBy(func(req *types.TracesQueryRequest) bool {
				return req.SearchScope.Namespace == testNamespace &&
					req.SearchScope.Project == testProject &&
					req.Limit == 25 &&
					req.SortOrder == sortOrderAsc &&
					req.StartTime.Equal(wantStart) &&
					req.EndTime.Equal(wantEnd)
			})).
			Return(&types.TracesQueryResponse{}, nil)

		h := newTestMCPHandler(t, withTracesService(tracesSvc))
		_, err := h.QueryTraces(ctx, testNamespace, testProject, testComponent, testEnvironment,
			testStartTime, testEndTime, 25, sortOrderAsc)
		require.NoError(t, err)
	})

	t.Run("invalid start_time", func(t *testing.T) {
		h := newTestMCPHandler(t)
		_, err := h.QueryTraces(ctx, testNamespace, testProject, testComponent, testEnvironment,
			"not-rfc3339", testEndTime, 10, sortOrderDesc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid start_time")
	})

	t.Run("invalid end_time", func(t *testing.T) {
		h := newTestMCPHandler(t)
		_, err := h.QueryTraces(ctx, testNamespace, testProject, testComponent, testEnvironment,
			testStartTime, "bad", 10, sortOrderDesc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid end_time")
	})
}

func TestQueryTraceSpans(t *testing.T) {
	ctx := context.Background()

	t.Run("forwards trace id and parsed window", func(t *testing.T) {
		tracesSvc := mocks.NewMockTracesQuerier(t)
		wantStart, _ := time.Parse(time.RFC3339, testStartTime)
		tracesSvc.EXPECT().
			QuerySpans(mock.Anything, testTraceID, mock.MatchedBy(func(req *types.TracesQueryRequest) bool {
				return req.StartTime.Equal(wantStart) && req.Limit == 200
			})).
			Return(&types.SpansQueryResponse{}, nil)

		h := newTestMCPHandler(t, withTracesService(tracesSvc))
		_, err := h.QueryTraceSpans(ctx, testTraceID, testNamespace, testProject, testComponent, testEnvironment,
			testStartTime, testEndTime, 200, sortOrderDesc)
		require.NoError(t, err)
	})

	t.Run("invalid start_time", func(t *testing.T) {
		h := newTestMCPHandler(t)
		_, err := h.QueryTraceSpans(ctx, testTraceID, testNamespace, "", "", "",
			"not-a-time", testEndTime, 10, sortOrderDesc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid start_time")
	})
}

func TestQueryAlerts(t *testing.T) {
	ctx := context.Background()

	t.Run("maps scope and times to gen request", func(t *testing.T) {
		alertsSvc := mocks.NewMockAlertIncidentService(t)
		alertsSvc.EXPECT().
			QueryAlerts(mock.Anything, mock.MatchedBy(func(req obsgen.AlertsQueryRequest) bool {
				if req.SearchScope.Namespace != testNamespace {
					return false
				}
				if req.SearchScope.Project == nil || *req.SearchScope.Project != testProject {
					return false
				}
				if req.Limit == nil || *req.Limit != 30 {
					return false
				}
				if req.SortOrder == nil || string(*req.SortOrder) != sortOrderAsc {
					return false
				}
				wantStart, _ := time.Parse(time.RFC3339, testStartTime)
				wantEnd, _ := time.Parse(time.RFC3339, testEndTime)
				return req.StartTime.Equal(wantStart) && req.EndTime.Equal(wantEnd)
			})).
			Return(&obsgen.AlertsQueryResponse{}, nil)

		h := newTestMCPHandler(t, withAlertIncidentService(alertsSvc))
		_, err := h.QueryAlerts(ctx, testNamespace, testProject, testComponent, testEnvironment,
			testStartTime, testEndTime, 30, sortOrderAsc)
		require.NoError(t, err)
	})

	t.Run("empty optional strings become nil pointers", func(t *testing.T) {
		alertsSvc := mocks.NewMockAlertIncidentService(t)
		alertsSvc.EXPECT().
			QueryAlerts(mock.Anything, mock.MatchedBy(func(req obsgen.AlertsQueryRequest) bool {
				return req.SearchScope.Project == nil &&
					req.SearchScope.Component == nil &&
					req.SearchScope.Environment == nil
			})).
			Return(&obsgen.AlertsQueryResponse{}, nil)

		h := newTestMCPHandler(t, withAlertIncidentService(alertsSvc))
		_, err := h.QueryAlerts(ctx, testNamespace, "", "", "",
			testStartTime, testEndTime, 10, sortOrderDesc)
		require.NoError(t, err)
	})

	t.Run("invalid end_time", func(t *testing.T) {
		h := newTestMCPHandler(t)
		_, err := h.QueryAlerts(ctx, testNamespace, testProject, testComponent, testEnvironment,
			testStartTime, "not-a-time", 10, sortOrderDesc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid end_time")
	})
}

func TestQueryIncidents(t *testing.T) {
	ctx := context.Background()

	t.Run("maps scope and sort order", func(t *testing.T) {
		incSvc := mocks.NewMockAlertIncidentService(t)
		incSvc.EXPECT().
			QueryIncidents(mock.Anything, mock.MatchedBy(func(req obsgen.IncidentsQueryRequest) bool {
				if req.SearchScope.Namespace != testNamespace {
					return false
				}
				if req.SortOrder == nil || string(*req.SortOrder) != sortOrderDesc {
					return false
				}
				return req.Limit != nil && *req.Limit == 15
			})).
			Return(&obsgen.IncidentsQueryResponse{}, nil)

		h := newTestMCPHandler(t, withAlertIncidentService(incSvc))
		_, err := h.QueryIncidents(ctx, testNamespace, testProject, testComponent, testEnvironment,
			testStartTime, testEndTime, 15, sortOrderDesc)
		require.NoError(t, err)
	})

	t.Run("invalid start_time", func(t *testing.T) {
		h := newTestMCPHandler(t)
		_, err := h.QueryIncidents(ctx, testNamespace, "", "", "",
			"bad", testEndTime, 10, sortOrderDesc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid start_time")
	})
}
