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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/config"
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
