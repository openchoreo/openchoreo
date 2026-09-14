// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/service"
	"github.com/openchoreo/openchoreo/internal/observer/service/mocks"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

// platformLogsResponse is the canned records answer the mock returns.
func platformLogsResponse() *types.PlatformLogsResponse {
	return &types.PlatformLogsResponse{
		Logs:   []types.PlatformLog{{Timestamp: testStartTime, Log: "reconcile failed", Level: logLevelError}},
		Total:  1,
		TookMs: 7,
	}
}

func filterValuesResponse(filter, value string) *types.PlatformLogFilterValuesResponse {
	return &types.PlatformLogFilterValuesResponse{
		Filter:      filter,
		Values:      []types.PlatformLogFilterValue{{Value: value, Count: 3}},
		TotalValues: 1,
		TookMs:      2,
	}
}

// queryPlatformLogs calls the handler with the defaults most cases want, so each
// test states only what it is about.
func queryPlatformLogs(t *testing.T, h *MCPHandler, includeSources []string, maxSources int) (any, error) {
	t.Helper()
	return h.QueryPlatformLogs(context.Background(),
		nil, nil, nil, nil,
		"", testStartTime, testEndTime, "", nil,
		0, "", includeSources, maxSources,
	)
}

func TestQueryPlatformLogs(t *testing.T) {
	t.Run("forwards coordinates and parsed labels to the platform logs service", func(t *testing.T) {
		svc := mocks.NewMockPlatformLogsQuerier(t)
		svc.EXPECT().
			QueryPlatformLogs(mock.Anything, mock.MatchedBy(func(req *types.PlatformLogsQueryRequest) bool {
				return len(req.ClusterInstances) == 1 && req.ClusterInstances[0] == testClusterInstance &&
					len(req.Namespaces) == 1 && req.Namespaces[0] == testK8sNamespace &&
					len(req.PodNames) == 1 && req.PodNames[0] == testPodName &&
					len(req.ContainerNames) == 1 && req.ContainerNames[0] == testContainerName &&
					req.Labels["openchoreo.dev/plane"] == "controlplane" &&
					req.StartTime == testStartTime && req.EndTime == testEndTime &&
					req.SearchPhrase == "reconcile" &&
					len(req.LogLevels) == 1 && req.LogLevels[0] == logLevelError &&
					req.Limit == 50 && req.SortOrder == sortOrderAsc
			})).
			Return(platformLogsResponse(), nil).Once()

		h := newTestMCPHandler(t, withPlatformLogsService(svc))
		result, err := h.QueryPlatformLogs(context.Background(),
			[]string{testClusterInstance}, []string{testK8sNamespace},
			[]string{testPodName}, []string{testContainerName},
			testLabelSelector, testStartTime, testEndTime, "reconcile", []string{logLevelError},
			50, sortOrderAsc, nil, 0,
		)
		require.NoError(t, err)

		res, ok := result.(*PlatformLogsResult)
		require.True(t, ok, "expected *PlatformLogsResult, got %T", result)
		assert.Len(t, res.Logs, 1)
		assert.Equal(t, 1, res.Total)
		assert.Equal(t, 7, res.TookMs)
		assert.Nil(t, res.Sources, "sources must be absent when include_sources is empty")
		assert.Empty(t, res.SourcesError)
	})

	t.Run("include_sources aggregates one coordinate per named field", func(t *testing.T) {
		svc := mocks.NewMockPlatformLogsQuerier(t)
		svc.EXPECT().QueryPlatformLogs(mock.Anything, mock.Anything).
			Return(platformLogsResponse(), nil).Once()
		svc.EXPECT().
			QueryPlatformLogFilterValues(mock.Anything, mock.MatchedBy(func(req *types.PlatformLogFilterValuesRequest) bool {
				return req.Filter == "podName"
			})).
			Return(filterValuesResponse("podName", testPodName), nil).Once()
		svc.EXPECT().
			QueryPlatformLogFilterValues(mock.Anything, mock.MatchedBy(func(req *types.PlatformLogFilterValuesRequest) bool {
				return req.Filter == "namespace"
			})).
			Return(filterValuesResponse("namespace", testK8sNamespace), nil).Once()

		h := newTestMCPHandler(t, withPlatformLogsService(svc))
		result, err := queryPlatformLogs(t, h, []string{"pod_name", "kubernetes_namespace"}, 0)
		require.NoError(t, err)

		res := result.(*PlatformLogsResult)
		require.Len(t, res.Sources, 2)
		// Keyed by the include_sources values the caller passed, not the API filter names.
		require.Contains(t, res.Sources, "pod_name")
		require.Contains(t, res.Sources, "kubernetes_namespace")
		assert.Equal(t, testPodName, res.Sources["pod_name"][0].Value)
		assert.Equal(t, int64(3), res.Sources["pod_name"][0].Count)
		assert.Equal(t, testK8sNamespace, res.Sources["kubernetes_namespace"][0].Value)
		assert.Empty(t, res.SourcesError)
	})

	t.Run("the record query is passed through to the aggregation unchanged", func(t *testing.T) {
		svc := mocks.NewMockPlatformLogsQuerier(t)
		svc.EXPECT().QueryPlatformLogs(mock.Anything, mock.Anything).
			Return(platformLogsResponse(), nil).Once()
		svc.EXPECT().
			QueryPlatformLogFilterValues(mock.Anything, mock.MatchedBy(func(req *types.PlatformLogFilterValuesRequest) bool {
				// The named filter's own selections stay in the query: the adapter
				// drops them, so counting podName under a selected pod still offers
				// the other pods.
				return len(req.Query.PodNames) == 1 && req.Query.PodNames[0] == testPodName &&
					req.Query.Labels["openchoreo.dev/plane"] == "controlplane" &&
					req.Query.StartTime == testStartTime
			})).
			Return(filterValuesResponse("podName", testPodName), nil).Once()

		h := newTestMCPHandler(t, withPlatformLogsService(svc))
		_, err := h.QueryPlatformLogs(context.Background(),
			nil, nil, []string{testPodName}, nil,
			testLabelSelector, testStartTime, testEndTime, "", nil,
			0, "", []string{"pod_name"}, 0,
		)
		require.NoError(t, err)
	})

	t.Run("max_sources defaults to defaultMaxSources and is otherwise honored", func(t *testing.T) {
		svc := mocks.NewMockPlatformLogsQuerier(t)
		svc.EXPECT().QueryPlatformLogs(mock.Anything, mock.Anything).
			Return(platformLogsResponse(), nil).Twice()
		svc.EXPECT().
			QueryPlatformLogFilterValues(mock.Anything, mock.MatchedBy(func(req *types.PlatformLogFilterValuesRequest) bool {
				return req.MaxValues == defaultMaxSources
			})).
			Return(filterValuesResponse("podName", testPodName), nil).Once()
		svc.EXPECT().
			QueryPlatformLogFilterValues(mock.Anything, mock.MatchedBy(func(req *types.PlatformLogFilterValuesRequest) bool {
				return req.MaxValues == 7
			})).
			Return(filterValuesResponse("podName", testPodName), nil).Once()

		h := newTestMCPHandler(t, withPlatformLogsService(svc))
		_, err := queryPlatformLogs(t, h, []string{"pod_name"}, 0)
		require.NoError(t, err)
		_, err = queryPlatformLogs(t, h, []string{"pod_name"}, 7)
		require.NoError(t, err)
	})

	t.Run("an aggregation failure returns the records with sourcesError", func(t *testing.T) {
		svc := mocks.NewMockPlatformLogsQuerier(t)
		svc.EXPECT().QueryPlatformLogs(mock.Anything, mock.Anything).
			Return(platformLogsResponse(), nil).Once()
		svc.EXPECT().QueryPlatformLogFilterValues(mock.Anything, mock.Anything).
			Return(nil, errors.New("aggregation blew up")).Once()

		h := newTestMCPHandler(t, withPlatformLogsService(svc))
		result, err := queryPlatformLogs(t, h, []string{"pod_name"}, 0)
		require.NoError(t, err, "a breakdown failure must not throw away the logs")

		res := result.(*PlatformLogsResult)
		assert.Len(t, res.Logs, 1)
		assert.Nil(t, res.Sources)
		assert.Contains(t, res.SourcesError, "aggregation blew up")
	})

	t.Run("an adapter that cannot aggregate still serves the records", func(t *testing.T) {
		svc := mocks.NewMockPlatformLogsQuerier(t)
		svc.EXPECT().QueryPlatformLogs(mock.Anything, mock.Anything).
			Return(platformLogsResponse(), nil).Once()
		svc.EXPECT().QueryPlatformLogFilterValues(mock.Anything, mock.Anything).
			Return(nil, service.ErrPlatformLogFilterValuesNotSupported).Once()

		h := newTestMCPHandler(t, withPlatformLogsService(svc))
		result, err := queryPlatformLogs(t, h, []string{"pod_name"}, 0)
		require.NoError(t, err)

		res := result.(*PlatformLogsResult)
		assert.Len(t, res.Logs, 1)
		assert.Contains(t, res.SourcesError, service.ErrPlatformLogFilterValuesNotSupported.Error())
	})

	t.Run("a records failure fails the call", func(t *testing.T) {
		svc := mocks.NewMockPlatformLogsQuerier(t)
		svc.EXPECT().QueryPlatformLogs(mock.Anything, mock.Anything).
			Return(nil, service.ErrPlatformLogsRetrieval).Once()

		h := newTestMCPHandler(t, withPlatformLogsService(svc))
		_, err := queryPlatformLogs(t, h, nil, 0)
		require.ErrorIs(t, err, service.ErrPlatformLogsRetrieval)
	})

	t.Run("caller mistakes are rejected before any service call", func(t *testing.T) {
		tests := []struct {
			name           string
			labels         string
			includeSources []string
			maxSources     int
			startTime      string
			endTime        string
			wantErr        string
		}{
			{
				name:           "unknown include_sources value",
				includeSources: []string{"plane"},
				wantErr:        "is not a coordinate",
			},
			{
				name:           "duplicate include_sources value",
				includeSources: []string{"pod_name", "pod_name"},
				wantErr:        "duplicate include_sources value",
			},
			{
				name:           "negative max_sources",
				includeSources: []string{"pod_name"},
				maxSources:     -1,
				wantErr:        "maxValues must be a positive integer",
			},
			{
				name:           "max_sources above the API cap",
				includeSources: []string{"pod_name"},
				maxSources:     1001,
				wantErr:        "maxValues cannot exceed 1000",
			},
			{
				name:    "set-based label selector",
				labels:  "openchoreo.dev/plane!=controlplane",
				wantErr: "only equality selectors",
			},
			{
				name:      "time range beyond the 30 day cap",
				startTime: "2025-01-01T00:00:00Z",
				endTime:   "2025-03-01T00:00:00Z",
				wantErr:   "cannot exceed 30 days",
			},
			{
				name:      "end before start",
				startTime: testEndTime,
				endTime:   testStartTime,
				wantErr:   "endTime",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// A strict mock with no expectations: any service call fails the test.
				svc := mocks.NewMockPlatformLogsQuerier(t)
				h := newTestMCPHandler(t, withPlatformLogsService(svc))

				start, end := testStartTime, testEndTime
				if tt.startTime != "" {
					start, end = tt.startTime, tt.endTime
				}
				_, err := h.QueryPlatformLogs(context.Background(),
					nil, nil, nil, nil,
					tt.labels, start, end, "", nil,
					0, "", tt.includeSources, tt.maxSources,
				)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			})
		}
	})
}

func TestResolvePlatformLogSourceFields(t *testing.T) {
	t.Run("maps every documented value onto its API filter name", func(t *testing.T) {
		fields, err := resolvePlatformLogSourceFields(platformLogSourceFieldNames)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			"cluster_instance":     "clusterInstance",
			"kubernetes_namespace": "namespace",
			"pod_name":             "podName",
			"container_name":       "containerName",
		}, fields)
	})

	t.Run("nothing requested maps to nothing", func(t *testing.T) {
		fields, err := resolvePlatformLogSourceFields(nil)
		require.NoError(t, err)
		assert.Empty(t, fields)
	})

	t.Run("the error names the valid values", func(t *testing.T) {
		_, err := resolvePlatformLogSourceFields([]string{"podName"})
		require.Error(t, err)
		for _, name := range platformLogSourceFieldNames {
			assert.Contains(t, err.Error(), name)
		}
	})
}
