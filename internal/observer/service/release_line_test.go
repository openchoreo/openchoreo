// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/store/alertentry"
	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/internal/releaseline"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

const sampleReleaseLineUID = "9f8e7d6c-5b4a-4321-8fed-cba987654321"

// newReleaseLineResolver resolves every scope name, including the release line "next" of
// component "comp".
func newReleaseLineResolver(t *testing.T) *ResourceUIDResolver {
	t.Helper()
	tokenSrv := newAlwaysOKTokenServer(t)
	t.Cleanup(tokenSrv.Close)
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/components/comp/releaselines/next"):
			_, _ = w.Write([]byte(uidResponse(sampleReleaseLineUID)))
		case strings.Contains(r.URL.Path, "/releaselines/"):
			w.WriteHeader(http.StatusNotFound)
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
	t.Cleanup(apiSrv.Close)
	return newTestResolver(t, apiSrv, tokenSrv, &config.UIDResolverConfig{MaxAuthRetry: 1})
}

func lineScope() types.ComponentSearchScope {
	return types.ComponentSearchScope{
		Namespace: "ns", Project: "proj", Component: "comp", Environment: "dev", ReleaseLine: "next",
	}
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestLogsService_ReleaseLine(t *testing.T) {
	nextPod := releaseline.ObjectBaseName("comp", "next", "dev") + "-7d9c8b6f5-abcde"
	mainPod := releaseline.ObjectBaseName("comp", "main", "dev") + "-6c8d7b5f4-fghij"
	query := func(t *testing.T, applied bool) (*types.LogsQueryResponse, *fakeLogsAdapter) {
		t.Helper()
		adapter := &fakeLogsAdapter{componentResult: &observability.ComponentApplicationLogsResult{
			Logs: []observability.LogEntry{
				{Log: "from next", PodName: nextPod},
				{Log: "from main", PodName: mainPod},
			},
			TotalCount:         2,
			ReleaseLineApplied: applied,
		}}
		svc, err := NewLogsService(adapter, newReleaseLineResolver(t), &config.Config{}, discardLogger())
		require.NoError(t, err)
		scope := lineScope()
		resp, err := svc.QueryLogs(context.Background(), &types.LogsQueryRequest{
			SearchScope: &types.SearchScope{Component: &scope},
			StartTime:   "2026-03-07T10:00:00Z",
			EndTime:     "2026-03-07T11:00:00Z",
			Limit:       25,
		})
		require.NoError(t, err)
		return resp, adapter
	}

	t.Run("adapter filters by line", func(t *testing.T) {
		resp, adapter := query(t, true)
		assert.Equal(t, sampleReleaseLineUID, adapter.lastComponent.ReleaseLineID)
		assert.Len(t, resp.Logs, 2, "entries an adapter returned for the line are trusted as is")
	})

	t.Run("adapter without line support", func(t *testing.T) {
		resp, _ := query(t, false)
		require.Len(t, resp.Logs, 1)
		assert.Equal(t, "from next", resp.Logs[0].Log)
		assert.Equal(t, 1, resp.Total)
	})
}

func TestEventsService_ReleaseLineFallback(t *testing.T) {
	adapter := &fakeEventsAdapter{componentResult: &observability.ComponentEventsResult{
		Events: []observability.EventEntry{
			{Message: "next deployment scaled", ObjectName: releaseline.ObjectBaseName("comp", "next", "dev")},
			{Message: "main pod started", ObjectName: releaseline.ObjectBaseName("comp", "main", "dev") + "-6c8d7b5f4-fghij"},
		},
		TotalCount: 2,
	}}
	svc, err := NewEventsService(adapter, newReleaseLineResolver(t), &config.Config{}, discardLogger())
	require.NoError(t, err)
	scope := lineScope()
	resp, err := svc.QueryEvents(context.Background(), &types.EventsQueryRequest{
		SearchScope: &types.SearchScope{Component: &scope},
		StartTime:   "2026-03-07T10:00:00Z",
		EndTime:     "2026-03-07T11:00:00Z",
		Limit:       25,
	})
	require.NoError(t, err)
	assert.Equal(t, sampleReleaseLineUID, adapter.lastComponent.ReleaseLineID)
	require.Len(t, resp.Events, 1)
	assert.Equal(t, "next deployment scaled", resp.Events[0].Message)
}

func TestLogsAdapter_ReleaseLineHeader(t *testing.T) {
	for _, applied := range []bool{true, false} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				SearchScope componentSearchScope `json:"searchScope"`
			}
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, sampleReleaseLineUID, body.SearchScope.ReleaseLineUID)
			if applied {
				w.Header().Set(observability.ReleaseLineAppliedHeader, "true")
			}
			_, _ = w.Write([]byte(`{"logs":[],"total":0,"tookMs":1}`))
		}))
		adapter, err := NewLogsAdapter(LogsAdapterConfig{BaseURL: server.URL, Timeout: 5 * time.Second})
		require.NoError(t, err)
		result, err := adapter.GetComponentApplicationLogs(context.Background(), observability.ComponentApplicationLogsParams{
			Namespace: "ns", ComponentID: sampleComponentUID, ReleaseLineID: sampleReleaseLineUID,
		})
		server.Close()
		require.NoError(t, err)
		assert.Equal(t, applied, result.ReleaseLineApplied)
	}
}

func TestMetricsAdapter_ReleaseLine(t *testing.T) {
	for name, tc := range map[string]struct {
		applied bool
		wantErr error
	}{
		"applied":       {applied: true},
		"not supported": {wantErr: ErrReleaseLineNotSupported},
	} {
		t.Run(name, func(t *testing.T) {
			var captured metricsAdapterRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
				if tc.applied {
					w.Header().Set(observability.ReleaseLineAppliedHeader, "true")
				}
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()

			adapter := NewMetricsAdapter(server.URL, 5*time.Second, newReleaseLineResolver(t), discardLogger())
			_, err := adapter.QueryMetrics(context.Background(), &types.MetricsQueryRequest{
				Metric: "resource", StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-01-01T01:00:00Z",
				SearchScope: lineScope(),
			})
			require.NotNil(t, captured.SearchScope.ReleaseLineUID)
			assert.Equal(t, sampleReleaseLineUID, *captured.SearchScope.ReleaseLineUID)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestTracesService_ReleaseLine(t *testing.T) {
	for name, tc := range map[string]struct {
		applied bool
		wantErr error
	}{
		"applied":       {applied: true},
		"not supported": {wantErr: ErrReleaseLineNotSupported},
	} {
		t.Run(name, func(t *testing.T) {
			var captured struct {
				SearchScope struct {
					ReleaseLine string `json:"releaseLine"`
				} `json:"searchScope"`
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
				w.Header().Set("Content-Type", "application/json")
				if tc.applied {
					w.Header().Set(observability.ReleaseLineAppliedHeader, "true")
				}
				_, _ = w.Write([]byte(`{"traces":[],"total":0,"tookMs":1}`))
			}))
			defer server.Close()

			adapter, err := NewTracingAdapter(TracingAdapterConfig{BaseURL: server.URL, Timeout: 5 * time.Second})
			require.NoError(t, err)
			svc, err := NewTracesService(adapter, newReleaseLineResolver(t), &config.Config{}, discardLogger())
			require.NoError(t, err)

			_, err = svc.QueryTraces(context.Background(), &types.TracesQueryRequest{
				StartTime:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				EndTime:     time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC),
				Limit:       10,
				SearchScope: lineScope(),
			})
			assert.Equal(t, sampleReleaseLineUID, captured.SearchScope.ReleaseLine)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestDecodeAdapterSyncResponse_ReleaseLineApplied(t *testing.T) {
	for _, applied := range []bool{true, false} {
		resp := &http.Response{Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"status":"synced"}`))}
		if applied {
			resp.Header.Set(observability.ReleaseLineAppliedHeader, "true")
		}
		result, err := decodeAdapterSyncResponse(resp)
		require.NoError(t, err)
		if applied {
			require.NotNil(t, result.ReleaseLineApplied)
			assert.True(t, *result.ReleaseLineApplied)
		} else {
			assert.Nil(t, result.ReleaseLineApplied)
		}
	}
}

func TestAlertService_QueryReleaseLine(t *testing.T) {
	strp := func(s string) *string { return &s }
	scope := gen.ComponentSearchScope{
		Namespace: "ns", Project: strp("proj"), Component: strp("comp"), Environment: strp("dev"), ReleaseLine: strp("next"),
	}
	window := func() (time.Time, time.Time) {
		return time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC), time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC)
	}

	t.Run("alerts are filtered by the line's UID and carry the line", func(t *testing.T) {
		store := &fakeAlertEntryStore{entries: []alertentry.AlertEntry{{
			ID: "a-1", Timestamp: "2026-03-07T10:20:30Z", ReleaseLineName: "next", ReleaseLineID: sampleReleaseLineUID,
		}}, total: 1}
		svc := &AlertService{alertEntryStore: store, resolver: newReleaseLineResolver(t), logger: discardLogger()}
		start, end := window()
		resp, err := svc.QueryAlerts(context.Background(), gen.AlertsQueryRequest{StartTime: start, EndTime: end, SearchScope: scope})
		require.NoError(t, err)
		assert.Equal(t, sampleReleaseLineUID, store.lastQueryParams.ReleaseLineID)
		labels := (*resp.Alerts)[0].Metadata.Labels
		require.NotNil(t, labels.ReleaseLineName)
		assert.Equal(t, "next", *labels.ReleaseLineName)
		assert.Equal(t, sampleReleaseLineUID, labels.ReleaseLineUid.String())
	})

	t.Run("incidents are filtered by the line's UID", func(t *testing.T) {
		store := &fakeIncidentEntryStore{}
		svc := &AlertService{incidentEntryStore: store, resolver: newReleaseLineResolver(t), logger: discardLogger()}
		start, end := window()
		_, err := svc.QueryIncidents(context.Background(), gen.IncidentsQueryRequest{StartTime: start, EndTime: end, SearchScope: scope})
		require.NoError(t, err)
		assert.Equal(t, sampleReleaseLineUID, store.lastQueryParams.ReleaseLineID)
	})

	t.Run("an unknown line is a scope error", func(t *testing.T) {
		svc := &AlertService{alertEntryStore: &fakeAlertEntryStore{}, resolver: newReleaseLineResolver(t), logger: discardLogger()}
		start, end := window()
		unknown := scope
		unknown.ReleaseLine = strp("v9")
		_, err := svc.QueryAlerts(context.Background(), gen.AlertsQueryRequest{StartTime: start, EndTime: end, SearchScope: unknown})
		require.ErrorIs(t, err, ErrScopeNotFound)
	})
}

func TestFinOpsAdapter_ReleaseLine(t *testing.T) {
	serve := func(t *testing.T, applied bool, captured *url.Values) *FinOpsAdapter {
		t.Helper()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*captured = r.URL.Query()
			if applied {
				w.Header().Set(observability.ReleaseLineAppliedHeader, "true")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[]}`))
		}))
		t.Cleanup(server.Close)
		adapter, err := NewFinOpsAdapter(server.URL, 30*time.Second, newReleaseLineResolver(t), discardLogger())
		require.NoError(t, err)
		return adapter
	}
	costs := func() *types.CostQueryRequest {
		return &types.CostQueryRequest{
			Namespace: "ns", Environment: "dev", Project: "proj", Component: "comp", ReleaseLine: "next",
			StartTime: "2026-05-23T10:00:01Z", EndTime: "2026-05-24T10:00:01Z",
		}
	}

	t.Run("costs forward the line's UID", func(t *testing.T) {
		var query url.Values
		_, err := serve(t, true, &query).GetComponentCosts(context.Background(), costs())
		require.NoError(t, err)
		assert.Equal(t, sampleReleaseLineUID, query.Get("releaseLineUid"))
	})

	t.Run("recommendations forward the line's UID and name", func(t *testing.T) {
		var query url.Values
		_, err := serve(t, true, &query).GetRecommendations(context.Background(), &types.RecommendationQueryRequest{
			Namespace: "ns", Environment: "dev", Project: "proj", Component: "comp", ReleaseLine: "next",
			StartTime: "2026-05-23T10:00:01Z", EndTime: "2026-05-24T10:00:01Z",
		})
		require.NoError(t, err)
		assert.Equal(t, sampleReleaseLineUID, query.Get("releaseLineUid"))
		assert.Equal(t, "next", query.Get("releaseLine"))
	})

	t.Run("an adapter that ignores the line is not supported", func(t *testing.T) {
		var query url.Values
		_, err := serve(t, false, &query).GetComponentCosts(context.Background(), costs())
		require.ErrorIs(t, err, ErrReleaseLineNotSupported)
	})

	t.Run("a query without a line needs no confirmation", func(t *testing.T) {
		var query url.Values
		req := costs()
		req.ReleaseLine = ""
		_, err := serve(t, false, &query).GetComponentCosts(context.Background(), req)
		require.NoError(t, err)
		assert.Empty(t, query.Get("releaseLineUid"))
	})
}
