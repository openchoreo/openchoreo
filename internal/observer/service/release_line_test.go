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
		case strings.HasSuffix(r.URL.Path, "/components/comp/releaselines"):
			_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"main"}},{"metadata":{"name":"next"}}]}`))
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
	query := func(t *testing.T, scope types.ComponentSearchScope, applied bool) (*types.LogsQueryResponse, *fakeLogsAdapter, error) {
		t.Helper()
		adapter := &fakeLogsAdapter{componentResult: &observability.ComponentApplicationLogsResult{
			Logs:               []observability.LogEntry{{Log: "from next"}, {Log: "from main"}},
			TotalCount:         2,
			ReleaseLineApplied: applied,
		}}
		svc, err := NewLogsService(adapter, newReleaseLineResolver(t), &config.Config{}, discardLogger())
		require.NoError(t, err)
		resp, err := svc.QueryLogs(context.Background(), &types.LogsQueryRequest{
			SearchScope: &types.SearchScope{Component: &scope},
			StartTime:   "2026-03-07T10:00:00Z",
			EndTime:     "2026-03-07T11:00:00Z",
			Limit:       25,
		})
		return resp, adapter, err
	}
	everyLine := lineScope()
	everyLine.ReleaseLine = ""
	everyLine.AllReleaseLines = true

	t.Run("adapter filters by line", func(t *testing.T) {
		resp, adapter, err := query(t, lineScope(), true)
		require.NoError(t, err)
		assert.Equal(t, sampleReleaseLineUID, adapter.lastComponent.ReleaseLineID)
		assert.Len(t, resp.Logs, 2, "entries an adapter returned for the line are trusted as is")
	})

	t.Run("adapter without line support", func(t *testing.T) {
		_, _, err := query(t, lineScope(), false)
		assert.ErrorIs(t, err, ErrReleaseLineNotSupported)
	})

	t.Run("every line", func(t *testing.T) {
		_, adapter, err := query(t, everyLine, true)
		require.NoError(t, err)
		assert.True(t, adapter.lastComponent.AllReleaseLines)
		assert.Empty(t, adapter.lastComponent.ReleaseLineID)
	})

	t.Run("every line without line support", func(t *testing.T) {
		_, _, err := query(t, everyLine, false)
		assert.ErrorIs(t, err, ErrReleaseLineNotSupported, "the adapter returned only the main line of a component with lines")
	})

	t.Run("main line needs no line support", func(t *testing.T) {
		scope := lineScope()
		scope.ReleaseLine = ""
		resp, adapter, err := query(t, scope, false)
		require.NoError(t, err)
		assert.False(t, adapter.lastComponent.AllReleaseLines)
		assert.Empty(t, adapter.lastComponent.ReleaseLineID)
		assert.Len(t, resp.Logs, 2)
	})
}

func TestEventsService_ReleaseLine(t *testing.T) {
	for name, applied := range map[string]bool{"applied": true, "not supported": false} {
		t.Run(name, func(t *testing.T) {
			adapter := &fakeEventsAdapter{componentResult: &observability.ComponentEventsResult{
				Events:             []observability.EventEntry{{Message: "next deployment scaled"}},
				TotalCount:         1,
				ReleaseLineApplied: applied,
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
			assert.Equal(t, sampleReleaseLineUID, adapter.lastComponent.ReleaseLineID)
			if !applied {
				assert.ErrorIs(t, err, ErrReleaseLineNotSupported)
				return
			}
			require.NoError(t, err)
			assert.Len(t, resp.Events, 1)
		})
	}
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
		Namespace: "ns", Project: strp("proj"), Component: strPtr("comp"), Environment: strp("dev"), ReleaseLine: strp("next"),
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

	t.Run("a query without a line sends the component alone and needs no confirmation", func(t *testing.T) {
		var query url.Values
		req := costs()
		req.ReleaseLine = ""
		_, err := serve(t, false, &query).GetComponentCosts(context.Background(), req)
		require.NoError(t, err)
		assert.Empty(t, query.Get("releaseLineUid"))
		assert.Empty(t, query.Get("allReleaseLines"))
	})

	t.Run("a query for every line asks for every line", func(t *testing.T) {
		var query url.Values
		req := costs()
		req.ReleaseLine = ""
		req.AllReleaseLines = true
		_, err := serve(t, true, &query).GetComponentCosts(context.Background(), req)
		require.NoError(t, err)
		assert.Empty(t, query.Get("releaseLineUid"))
		assert.Equal(t, "true", query.Get("allReleaseLines"))
	})

	t.Run("an adapter that ignores every line is not supported", func(t *testing.T) {
		var query url.Values
		req := costs()
		req.ReleaseLine = ""
		req.AllReleaseLines = true
		_, err := serve(t, false, &query).GetComponentCosts(context.Background(), req)
		require.ErrorIs(t, err, ErrReleaseLineNotSupported)
	})
}

func TestResolveScopeReleaseLines(t *testing.T) {
	all := true
	for name, tc := range map[string]struct {
		scope gen.ComponentSearchScope
		want  ReleaseLineScope
	}{
		"no component":       {scope: gen.ComponentSearchScope{Namespace: "ns", AllReleaseLines: &all}},
		"component, no line": {scope: gen.ComponentSearchScope{Namespace: "ns", Component: strPtr("comp")}},
		"every line":         {scope: gen.ComponentSearchScope{Namespace: "ns", Component: strPtr("comp"), AllReleaseLines: &all}, want: ReleaseLineScope{All: true}},
		"main keyword":       {scope: gen.ComponentSearchScope{Namespace: "ns", Component: strPtr("comp"), ReleaseLine: strPtr("main")}},
		"one line":           {scope: gen.ComponentSearchScope{Namespace: "ns", Component: strPtr("comp"), ReleaseLine: strPtr("next")}, want: ReleaseLineScope{UID: sampleReleaseLineUID}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := resolveScopeReleaseLines(context.Background(), newReleaseLineResolver(t), &tc.scope)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMetricsAdapter_MainLineAndEveryLine(t *testing.T) {
	for name, tc := range map[string]struct {
		line    string
		all     bool
		wantAll bool
	}{
		"main line keyword": {line: releaseline.MainLine},
		"no line":           {line: ""},
		"every line":        {line: "", all: true, wantAll: true},
	} {
		t.Run(name, func(t *testing.T) {
			var captured metricsAdapterRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
				w.Header().Set(observability.ReleaseLineAppliedHeader, "true")
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()

			scope := lineScope()
			scope.ReleaseLine = tc.line
			scope.AllReleaseLines = tc.all
			adapter := NewMetricsAdapter(server.URL, 5*time.Second, newReleaseLineResolver(t), discardLogger())
			_, err := adapter.QueryMetrics(context.Background(), &types.MetricsQueryRequest{
				Metric: "resource", StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-01-01T01:00:00Z",
				SearchScope: scope,
			})
			require.NoError(t, err)
			assert.Nil(t, captured.SearchScope.ReleaseLineUID)
			assert.Equal(t, tc.wantAll, captured.SearchScope.AllReleaseLines)
			require.NotNil(t, captured.SearchScope.ComponentUID)
			assert.Equal(t, sampleComponentUID, *captured.SearchScope.ComponentUID)
		})
	}
}

func TestTracingAdapter_AllReleaseLines(t *testing.T) {
	for _, all := range []bool{true, false} {
		var captured struct {
			SearchScope map[string]any `json:"searchScope"`
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"traces":[],"total":0,"tookMs":1}`))
		}))
		adapter, err := NewTracingAdapter(TracingAdapterConfig{BaseURL: server.URL, Timeout: 5 * time.Second})
		require.NoError(t, err)
		_, err = adapter.GetTraces(context.Background(), observability.TracesQueryParams{
			Namespace: "ns", ComponentID: sampleComponentUID, AllReleaseLines: all,
		})
		server.Close()
		require.NoError(t, err)
		if all {
			assert.Equal(t, true, captured.SearchScope["allReleaseLines"])
		} else {
			assert.NotContains(t, captured.SearchScope, "allReleaseLines")
		}
	}
}

func TestAlertService_NotificationDetailsMainLine(t *testing.T) {
	t.Parallel()

	tokenSrv := newAlwaysOKTokenServer(t)
	t.Cleanup(tokenSrv.Close)
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces/ns/components/lined/releaselines":
			assert.Equal(t, "1", r.URL.Query().Get("limit"))
			_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"main"}},{"metadata":{"name":"next"}}]}`))
		case "/api/v1/namespaces/ns/components/plain/releaselines":
			_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"main"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(apiSrv.Close)

	svc := newTestAlertService()
	svc.resolver = newTestResolver(t, apiSrv, tokenSrv, &config.UIDResolverConfig{MaxAuthRetry: 1})

	tests := []struct {
		name        string
		component   string
		releaseLine string
		want        string
	}{
		{name: "main line of a component with lines", component: "lined", want: releaseline.MainLine},
		{name: "component without lines", component: "plain", want: ""},
		{name: "release line", component: "lined", releaseLine: "next", want: "next"},
		{name: "lookup failure", component: "missing", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			details := &types.AlertDetails{Namespace: "ns", Component: tt.component, ReleaseLine: tt.releaseLine}
			got := svc.notificationDetails(context.Background(), details)

			assert.Equal(t, tt.want, got.ReleaseLine)
			assert.Equal(t, tt.releaseLine, details.ReleaseLine, "the stored alert keeps its release line")
		})
	}
}
