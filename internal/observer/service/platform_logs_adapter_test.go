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

func newTestPlatformLogsAdapter(t *testing.T, baseURL string) *LogsAdapter {
	t.Helper()
	adapter, err := NewLogsAdapter(LogsAdapterConfig{BaseURL: baseURL, Timeout: 30 * time.Second})
	require.NoError(t, err)
	return adapter
}

// platformLogsServer stands in for a logs module. It records the request the
// adapter sent and replies with the given status and body.
func platformLogsServer(t *testing.T, status int, body any, capturedBody *map[string]any,
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

// TestLogsAdapter_GetPlatformLogs_RequestContract pins what the observer puts on
// the wire: the POST path from the adapter contract, and every filter mapped onto
// the body fields the spec declares.
func TestLogsAdapter_GetPlatformLogs_RequestContract(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	var gotMethod, gotPath string
	server := platformLogsServer(t, http.StatusOK,
		map[string]any{"logs": []any{}, "total": 0, "tookMs": 1},
		&gotBody, &gotMethod, &gotPath)
	defer server.Close()

	start := time.Date(2026, 8, 14, 16, 30, 0, 0, time.UTC)
	end := time.Date(2026, 8, 14, 17, 30, 0, 0, time.UTC)

	_, err := newTestPlatformLogsAdapter(t, server.URL).GetPlatformLogs(context.Background(),
		observability.PlatformLogsParams{
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
	assert.Equal(t, "/api/v1alpha1/platform-logs/query", gotPath)

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

// TestLogsAdapter_GetPlatformLogs_OmitsEmptyFilters pins that an absent filter is
// not sent as an empty value - the contract treats a missing field as "no filter".
func TestLogsAdapter_GetPlatformLogs_OmitsEmptyFilters(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	server := platformLogsServer(t, http.StatusOK,
		map[string]any{"logs": []any{}, "total": 0, "tookMs": 0}, &gotBody, nil, nil)
	defer server.Close()

	_, err := newTestPlatformLogsAdapter(t, server.URL).GetPlatformLogs(context.Background(),
		observability.PlatformLogsParams{
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

// TestLogsAdapter_GetPlatformLogs_MapsResponse pins the record mapping, including
// the pod metadata and labels the contract returns alongside the coordinates.
func TestLogsAdapter_GetPlatformLogs_MapsResponse(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 8, 14, 16, 31, 0, 0, time.UTC)
	server := platformLogsServer(t, http.StatusOK, map[string]any{
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

	result, err := newTestPlatformLogsAdapter(t, server.URL).GetPlatformLogs(context.Background(),
		observability.PlatformLogsParams{
			StartTime: ts.Add(-time.Hour),
			EndTime:   ts.Add(time.Hour),
		})
	require.NoError(t, err)
	require.Len(t, result.Logs, 1)

	assert.Equal(t, 1, result.TotalCount)
	assert.Equal(t, 4, result.Took)
	assert.Equal(t, observability.PlatformLogEntry{
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

// TestLogsAdapter_GetPlatformLogs_OmittedRecordFields pins the deref helpers: a
// record missing optional fields maps to zero values, not a panic.
//
// timestamp and log are required by the contract, so a record without them is a
// module bug rather than a legitimate state. The adapter passes through whatever
// arrived - it does not police the contract on the module's behalf, since a
// visibly wrong value is easier to notice and fix than a silently dropped record.
func TestLogsAdapter_GetPlatformLogs_OmittedRecordFields(t *testing.T) {
	t.Parallel()

	server := platformLogsServer(t, http.StatusOK, map[string]any{
		"logs":   []map[string]any{{"log": "bare record"}},
		"total":  1,
		"tookMs": 0,
	}, nil, nil, nil)
	defer server.Close()

	result, err := newTestPlatformLogsAdapter(t, server.URL).GetPlatformLogs(context.Background(),
		observability.PlatformLogsParams{
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

// TestLogsAdapter_GetPlatformLogs_NotImplemented pins the 501 mapping. A module
// that has not adopted the endpoint is a deployment fact, not a failure, so it
// surfaces as ErrPlatformLogsNotSupported for the handler to turn into a 501.
func TestLogsAdapter_GetPlatformLogs_NotImplemented(t *testing.T) {
	t.Parallel()

	server := platformLogsServer(t, http.StatusNotImplemented, map[string]any{
		"title":     "notImplemented",
		"errorCode": "",
		"message":   "platform logs are not supported by this adapter",
	}, nil, nil, nil)
	defer server.Close()

	_, err := newTestPlatformLogsAdapter(t, server.URL).GetPlatformLogs(context.Background(),
		observability.PlatformLogsParams{
			StartTime: time.Date(2026, 8, 14, 16, 30, 0, 0, time.UTC),
			EndTime:   time.Date(2026, 8, 14, 17, 30, 0, 0, time.UTC),
		})
	require.ErrorIs(t, err, ErrPlatformLogsNotSupported)
}

// TestLogsAdapter_GetPlatformLogs_UpstreamErrors pins that any other non-200 is a
// failure carrying the status, distinct from the 501 case above.
func TestLogsAdapter_GetPlatformLogs_UpstreamErrors(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusBadRequest, http.StatusInternalServerError, http.StatusBadGateway} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			server := platformLogsServer(t, status,
				map[string]any{"title": "error", "message": "upstream said no"}, nil, nil, nil)
			defer server.Close()

			_, err := newTestPlatformLogsAdapter(t, server.URL).GetPlatformLogs(context.Background(),
				observability.PlatformLogsParams{
					StartTime: time.Date(2026, 8, 14, 16, 30, 0, 0, time.UTC),
					EndTime:   time.Date(2026, 8, 14, 17, 30, 0, 0, time.UTC),
				})
			require.Error(t, err)
			assert.NotErrorIs(t, err, ErrPlatformLogsNotSupported)
			assert.Contains(t, err.Error(), strconv.Itoa(status))
			assert.Contains(t, err.Error(), "upstream said no")
		})
	}
}

// --- filter values ---

func filterValuesParams() observability.PlatformLogFilterValuesParams {
	return observability.PlatformLogFilterValuesParams{
		Filter:      "podName",
		ValueSearch: "controller",
		MaxValues:   50,
		Query: observability.PlatformLogsParams{
			Namespaces:   []string{"openchoreo-control-plane"},
			PodNames:     []string{"already-selected"},
			Labels:       map[string]string{"openchoreo.dev/plane": "controlplane"},
			LogLevels:    []string{"ERROR"},
			SearchPhrase: "reconcile",
			StartTime:    time.Date(2026, 8, 14, 16, 30, 0, 0, time.UTC),
			EndTime:      time.Date(2026, 8, 14, 17, 30, 0, 0, time.UTC),
		},
	}
}

func okFilterValuesBody() map[string]any {
	return map[string]any{
		"filter": "podName", "values": []any{},
		"totalValues": 0, "totalRelation": "eq", "tookMs": 1,
	}
}

// Pins what the observer puts on the wire: the POST path from the adapter contract, and
// the record query nested under `query` rather than flattened alongside `filter`.
func TestLogsAdapter_GetPlatformLogFilterValues_RequestContract(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	var gotMethod, gotPath string
	server := platformLogsServer(t, http.StatusOK, okFilterValuesBody(), &gotBody, &gotMethod, &gotPath)
	defer server.Close()

	_, err := newTestPlatformLogsAdapter(t, server.URL).
		GetPlatformLogFilterValues(context.Background(), filterValuesParams())
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/api/v1alpha1/platform-logs/filter-values", gotPath)
	assert.Equal(t, "podName", gotBody["filter"])
	assert.Equal(t, "controller", gotBody["valueSearch"])
	assert.EqualValues(t, 50, gotBody["maxValues"])

	query, ok := gotBody["query"].(map[string]any)
	require.True(t, ok, "the record query must be nested under query, got %v", gotBody)
	assert.Equal(t, []any{"openchoreo-control-plane"}, query["namespace"])
	assert.Equal(t, map[string]any{"openchoreo.dev/plane": "controlplane"}, query["labels"])
	assert.Equal(t, []any{"ERROR"}, query["logLevels"])
	assert.Equal(t, "reconcile", query["searchPhrase"])
	assert.Equal(t, "2026-08-14T16:30:00Z", query["startTime"])

	// Paging and ordering describe a page of records, of which this returns none.
	assert.NotContains(t, query, "limit")
	assert.NotContains(t, query, "sortOrder")
}

// The named filter's own selections are sent untouched: excluding them is the adapter's
// job, and a doctored query would leave it unable to tell "not selected" from "excluded
// for this call".
func TestLogsAdapter_GetPlatformLogFilterValues_SendsNamedFilterSelections(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	server := platformLogsServer(t, http.StatusOK, okFilterValuesBody(), &gotBody, nil, nil)
	defer server.Close()

	_, err := newTestPlatformLogsAdapter(t, server.URL).
		GetPlatformLogFilterValues(context.Background(), filterValuesParams())
	require.NoError(t, err)

	query := gotBody["query"].(map[string]any)
	assert.Equal(t, []any{"already-selected"}, query["podName"])
}

func TestLogsAdapter_GetPlatformLogFilterValues_OmitsUnsetOptions(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	server := platformLogsServer(t, http.StatusOK, okFilterValuesBody(), &gotBody, nil, nil)
	defer server.Close()

	params := filterValuesParams()
	params.ValueSearch = ""
	params.MaxValues = 0

	_, err := newTestPlatformLogsAdapter(t, server.URL).
		GetPlatformLogFilterValues(context.Background(), params)
	require.NoError(t, err)

	assert.NotContains(t, gotBody, "valueSearch")
	assert.NotContains(t, gotBody, "maxValues")
}

func TestLogsAdapter_GetPlatformLogFilterValues_MapsResponse(t *testing.T) {
	t.Parallel()

	server := platformLogsServer(t, http.StatusOK, map[string]any{
		"filter": "podName",
		"values": []map[string]any{
			{"value": "controller-manager-abc", "count": 412},
			{"value": "controller-manager-xyz", "count": 88},
		},
		"totalValues": 940, "totalRelation": "gte", "tookMs": 12,
	}, nil, nil, nil)
	defer server.Close()

	result, err := newTestPlatformLogsAdapter(t, server.URL).
		GetPlatformLogFilterValues(context.Background(), filterValuesParams())
	require.NoError(t, err)

	assert.Equal(t, "podName", result.Filter)
	assert.Equal(t, []observability.PlatformLogFilterValue{
		{Value: "controller-manager-abc", Count: 412},
		{Value: "controller-manager-xyz", Count: 88},
	}, result.Values)
	// A truncated list says so, rather than ending silently.
	assert.EqualValues(t, 940, result.TotalValues)
	assert.Equal(t, "gte", result.TotalRelation)
	assert.Equal(t, 12, result.Took)
}

// An adapter may serve platform logs and still not be able to aggregate their fields,
// so this 501 is its own condition rather than the platform logs one.
func TestLogsAdapter_GetPlatformLogFilterValues_NotImplemented(t *testing.T) {
	t.Parallel()

	server := platformLogsServer(t, http.StatusNotImplemented, map[string]any{
		"title":   "notImplemented",
		"message": "platform log filter values are not supported by this adapter",
	}, nil, nil, nil)
	defer server.Close()

	_, err := newTestPlatformLogsAdapter(t, server.URL).
		GetPlatformLogFilterValues(context.Background(), filterValuesParams())

	require.ErrorIs(t, err, ErrPlatformLogFilterValuesNotSupported)
	assert.NotErrorIs(t, err, ErrPlatformLogsNotSupported)
}

func TestLogsAdapter_GetPlatformLogFilterValues_UpstreamError(t *testing.T) {
	t.Parallel()

	server := platformLogsServer(t, http.StatusInternalServerError,
		map[string]any{"title": "internalServerError"}, nil, nil, nil)
	defer server.Close()

	_, err := newTestPlatformLogsAdapter(t, server.URL).
		GetPlatformLogFilterValues(context.Background(), filterValuesParams())

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrPlatformLogFilterValuesNotSupported)
	assert.Contains(t, err.Error(), "500")
}
