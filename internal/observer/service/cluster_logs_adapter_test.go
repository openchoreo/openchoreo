// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/pkg/observability"
)

func newTestClusterLogsAdapter(t *testing.T, baseURL string) *LogsAdapter {
	t.Helper()
	adapter, err := NewLogsAdapter(LogsAdapterConfig{BaseURL: baseURL, Timeout: 30 * time.Second})
	require.NoError(t, err)
	return adapter
}

// clusterLogsServer stands in for a logs module. It records the request the
// adapter sent and replies with the given status and body.
func clusterLogsServer(t *testing.T, status int, body any, capturedBody *map[string]any,
	capturedMethod, capturedPath *string,
) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capturedMethod != nil {
			*capturedMethod = r.Method
		}
		if capturedPath != nil {
			*capturedPath = r.URL.Path
		}
		if capturedBody != nil {
			require.NoError(t, json.NewDecoder(r.Body).Decode(capturedBody))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != nil {
			require.NoError(t, json.NewEncoder(w).Encode(body))
		}
	}))
}

// TestLogsAdapter_GetClusterLogs_RequestContract pins what the observer puts on
// the wire: the POST path from the adapter contract, and every filter mapped onto
// the body fields the spec declares.
func TestLogsAdapter_GetClusterLogs_RequestContract(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	var gotMethod, gotPath string
	server := clusterLogsServer(t, http.StatusOK,
		map[string]any{"logs": []any{}, "total": 0, "tookMs": 1},
		&gotBody, &gotMethod, &gotPath)
	defer server.Close()

	start := time.Date(2026, 8, 14, 16, 30, 0, 0, time.UTC)
	end := time.Date(2026, 8, 14, 17, 30, 0, 0, time.UTC)

	_, err := newTestClusterLogsAdapter(t, server.URL).GetClusterLogs(context.Background(),
		observability.ClusterLogsParams{
			ClusterInstances: []string{"cluster1"},
			Namespaces:       []string{"openchoreo-control-plane", "cert-manager"},
			PodNames:         []string{"controller-manager-7f58b689b5-pwsb5"},
			ContainerNames:   []string{"manager"},
			Labels:           map[string]string{"openchoreo.dev/plane": "controlplane"},
			LogLevels:        []string{"ERROR", "WARN"},
			SearchPhrase:     "reconcile",
			StartTime:        start,
			EndTime:          end,
			Limit:            25,
			SortOrder:        "asc",
		})
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/api/v1alpha1/cluster-logs/query", gotPath)

	assert.Equal(t, []any{"cluster1"}, gotBody["clusterInstance"])
	assert.Equal(t, []any{"openchoreo-control-plane", "cert-manager"}, gotBody["namespace"])
	assert.Equal(t, []any{"controller-manager-7f58b689b5-pwsb5"}, gotBody["podName"])
	assert.Equal(t, []any{"manager"}, gotBody["containerName"])
	assert.Equal(t, map[string]any{"openchoreo.dev/plane": "controlplane"}, gotBody["labels"])
	assert.Equal(t, []any{"ERROR", "WARN"}, gotBody["logLevels"])
	assert.Equal(t, "reconcile", gotBody["searchPhrase"])
	assert.EqualValues(t, 25, gotBody["limit"])
	assert.Equal(t, "asc", gotBody["sortOrder"])
	assert.Equal(t, start.Format(time.RFC3339), gotBody["startTime"])
	assert.Equal(t, end.Format(time.RFC3339), gotBody["endTime"])
}

// TestLogsAdapter_GetClusterLogs_OmitsEmptyFilters pins that an absent filter is
// not sent as an empty value - the contract treats a missing field as "no filter".
func TestLogsAdapter_GetClusterLogs_OmitsEmptyFilters(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	server := clusterLogsServer(t, http.StatusOK,
		map[string]any{"logs": []any{}, "total": 0, "tookMs": 0}, &gotBody, nil, nil)
	defer server.Close()

	_, err := newTestClusterLogsAdapter(t, server.URL).GetClusterLogs(context.Background(),
		observability.ClusterLogsParams{
			StartTime: time.Date(2026, 8, 14, 16, 30, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 8, 14, 17, 30, 0, 0, time.UTC),
		})
	require.NoError(t, err)

	for _, key := range []string{
		"clusterInstance", "namespace", "podName", "containerName",
		"labels", "logLevels", "searchPhrase", "limit", "sortOrder",
	} {
		assert.NotContains(t, gotBody, key, "%s should be omitted when unset", key)
	}
	assert.Contains(t, gotBody, "startTime")
	assert.Contains(t, gotBody, "endTime")
}

// TestLogsAdapter_GetClusterLogs_MapsResponse pins the record mapping, including
// the pod metadata and labels the contract returns alongside the coordinates.
func TestLogsAdapter_GetClusterLogs_MapsResponse(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 8, 14, 16, 31, 0, 0, time.UTC)
	server := clusterLogsServer(t, http.StatusOK, map[string]any{
		"logs": []map[string]any{{
			"timestamp":       ts.Format(time.RFC3339),
			"log":             "reconcile failed",
			"level":           "ERROR",
			"clusterInstance": "cluster1",
			"namespaceName":   "openchoreo-control-plane",
			"podName":         "controller-manager-7f58b689b5-pwsb5",
			"containerName":   "manager",
			"podIp":           "10.42.0.17",
			"nodeName":        "k3d-openchoreo-server-0",
			"containerImage":  "ghcr.io/openchoreo/controller:latest",
			"labels":          map[string]string{"openchoreo.dev/plane": "controlplane"},
		}},
		"total":  1,
		"tookMs": 4,
	}, nil, nil, nil)
	defer server.Close()

	result, err := newTestClusterLogsAdapter(t, server.URL).GetClusterLogs(context.Background(),
		observability.ClusterLogsParams{
			StartTime: ts.Add(-time.Hour),
			EndTime:   ts.Add(time.Hour),
		})
	require.NoError(t, err)
	require.Len(t, result.Logs, 1)

	assert.Equal(t, 1, result.TotalCount)
	assert.Equal(t, 4, result.Took)
	assert.Equal(t, observability.ClusterLogEntry{
		Timestamp:       ts,
		Log:             "reconcile failed",
		LogLevel:        "ERROR",
		ClusterInstance: "cluster1",
		NamespaceName:   "openchoreo-control-plane",
		PodName:         "controller-manager-7f58b689b5-pwsb5",
		ContainerName:   "manager",
		PodIP:           "10.42.0.17",
		NodeName:        "k3d-openchoreo-server-0",
		ContainerImage:  "ghcr.io/openchoreo/controller:latest",
		Labels:          map[string]string{"openchoreo.dev/plane": "controlplane"},
	}, result.Logs[0])
}

// TestLogsAdapter_GetClusterLogs_OmittedRecordFields pins the deref helpers: a
// record with only the required fields maps to zero values, not a panic.
func TestLogsAdapter_GetClusterLogs_OmittedRecordFields(t *testing.T) {
	t.Parallel()

	server := clusterLogsServer(t, http.StatusOK, map[string]any{
		"logs":   []map[string]any{{"log": "bare record"}},
		"total":  1,
		"tookMs": 0,
	}, nil, nil, nil)
	defer server.Close()

	result, err := newTestClusterLogsAdapter(t, server.URL).GetClusterLogs(context.Background(),
		observability.ClusterLogsParams{
			StartTime: time.Date(2026, 8, 14, 16, 30, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 8, 14, 17, 30, 0, 0, time.UTC),
		})
	require.NoError(t, err)
	require.Len(t, result.Logs, 1)

	got := result.Logs[0]
	assert.Equal(t, "bare record", got.Log)
	assert.True(t, got.Timestamp.IsZero())
	assert.Empty(t, got.PodIP)
	assert.Empty(t, got.NodeName)
	assert.Empty(t, got.ContainerImage)
	assert.Nil(t, got.Labels)
}

// TestLogsAdapter_GetClusterLogs_NotImplemented pins the 501 mapping. A module
// that has not adopted the endpoint is a deployment fact, not a failure, so it
// surfaces as ErrClusterLogsNotSupported for the handler to turn into a 501.
func TestLogsAdapter_GetClusterLogs_NotImplemented(t *testing.T) {
	t.Parallel()

	server := clusterLogsServer(t, http.StatusNotImplemented, map[string]any{
		"title":     "notImplemented",
		"errorCode": "",
		"message":   "cluster logs are not supported by this adapter",
	}, nil, nil, nil)
	defer server.Close()

	_, err := newTestClusterLogsAdapter(t, server.URL).GetClusterLogs(context.Background(),
		observability.ClusterLogsParams{
			StartTime: time.Date(2026, 8, 14, 16, 30, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 8, 14, 17, 30, 0, 0, time.UTC),
		})
	require.ErrorIs(t, err, ErrClusterLogsNotSupported)
}

// TestLogsAdapter_GetClusterLogs_UpstreamErrors pins that any other non-200 is a
// failure carrying the status, distinct from the 501 case above.
func TestLogsAdapter_GetClusterLogs_UpstreamErrors(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusBadRequest, http.StatusInternalServerError, http.StatusBadGateway} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			server := clusterLogsServer(t, status,
				map[string]any{"title": "error", "message": "upstream said no"}, nil, nil, nil)
			defer server.Close()

			_, err := newTestClusterLogsAdapter(t, server.URL).GetClusterLogs(context.Background(),
				observability.ClusterLogsParams{
					StartTime: time.Date(2026, 8, 14, 16, 30, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 8, 14, 17, 30, 0, 0, time.UTC),
				})
			require.Error(t, err)
			assert.NotErrorIs(t, err, ErrClusterLogsNotSupported)
			assert.Contains(t, err.Error(), strconv.Itoa(status))
			assert.Contains(t, err.Error(), "upstream said no")
		})
	}
}
