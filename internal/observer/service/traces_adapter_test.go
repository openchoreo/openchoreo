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

// fakeTracingAdapter implements observability.TracingAdapter for tests.
type fakeTracingAdapter struct {
	tracesResult     *observability.TracesQueryResult
	tracesErr        error
	spansResult      *observability.SpansResult
	spansErr         error
	spanDetailResult *observability.SpanDetail
	spanDetailErr    error

	tracesCalled      bool
	spansCalled       bool
	spanDetailsCalled bool
	lastTraceID       string
	lastSpanID        string
	lastParams        observability.TracesQueryParams
}

func (f *fakeTracingAdapter) GetTraces(_ context.Context,
	params observability.TracesQueryParams,
) (*observability.TracesQueryResult, error) {
	f.tracesCalled = true
	f.lastParams = params
	return f.tracesResult, f.tracesErr
}

func (f *fakeTracingAdapter) GetSpans(_ context.Context, traceID string,
	params observability.TracesQueryParams,
) (*observability.SpansResult, error) {
	f.spansCalled = true
	f.lastTraceID = traceID
	f.lastParams = params
	return f.spansResult, f.spansErr
}

func (f *fakeTracingAdapter) GetSpanDetails(_ context.Context, traceID, spanID string,
) (*observability.SpanDetail, error) {
	f.spanDetailsCalled = true
	f.lastTraceID = traceID
	f.lastSpanID = spanID
	return f.spanDetailResult, f.spanDetailErr
}

func newTracesServiceForTest(t *testing.T, adapter observability.TracingAdapter) *TracesService {
	t.Helper()
	svc, err := NewTracesService(
		adapter,
		nil, // resolver — not used when scope filters are empty
		&config.Config{},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	require.NoError(t, err)
	return svc
}

func TestNewTracesService(t *testing.T) {
	t.Parallel()

	t.Run("rejects nil adapter", func(t *testing.T) {
		t.Parallel()
		_, err := NewTracesService(nil, nil, &config.Config{},
			slog.New(slog.NewTextHandler(io.Discard, nil)))
		require.Error(t, err)
	})

	t.Run("accepts non-nil adapter", func(t *testing.T) {
		t.Parallel()
		svc, err := NewTracesService(&fakeTracingAdapter{}, nil, &config.Config{},
			slog.New(slog.NewTextHandler(io.Discard, nil)))
		require.NoError(t, err)
		require.NotNil(t, svc)
	})
}

func TestTracesService_QueryTraces_NilRequest(t *testing.T) {
	t.Parallel()
	svc := newTracesServiceForTest(t, &fakeTracingAdapter{})
	_, err := svc.QueryTraces(context.Background(), nil)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrTracesInvalidRequest)
}

func TestTracesService_QueryTraces_Success(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)
	adapter := &fakeTracingAdapter{
		tracesResult: &observability.TracesQueryResult{
			Traces: []observability.Trace{
				{
					TraceID:      "trace-1",
					SpanCount:    3,
					StartTime:    now,
					EndTime:      now.Add(time.Second),
					DurationNs:   1000000000,
					RootSpanID:   "span-root",
					RootSpanName: "http.request",
					RootSpanKind: "SERVER",
					TraceName:    "http.request",
				},
			},
			TotalCount: 1,
			Took:       9,
		},
	}
	svc := newTracesServiceForTest(t, adapter)

	resp, err := svc.QueryTraces(context.Background(), &types.TracesQueryRequest{
		SearchScope: types.ComponentSearchScope{Namespace: "ns"},
		StartTime:   now,
		EndTime:     now.Add(time.Hour),
		Limit:       50,
		SortOrder:   "desc",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, adapter.tracesCalled)
	assert.Equal(t, "ns", adapter.lastParams.Namespace)
	assert.Equal(t, 50, adapter.lastParams.Limit)
	require.Len(t, resp.Traces, 1)
	assert.Equal(t, "trace-1", resp.Traces[0].TraceID)
	assert.Equal(t, 1, resp.Total)
	assert.Equal(t, 9, resp.TookMs)
}

func TestTracesService_QueryTraces_AdapterError(t *testing.T) {
	t.Parallel()
	adapter := &fakeTracingAdapter{tracesErr: errors.New("upstream boom")}
	svc := newTracesServiceForTest(t, adapter)

	_, err := svc.QueryTraces(context.Background(), &types.TracesQueryRequest{
		SearchScope: types.ComponentSearchScope{Namespace: "ns"},
		StartTime:   time.Now(),
		EndTime:     time.Now().Add(time.Hour),
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrTracesRetrieval)
}

func TestTracesService_QueryTraces_ResolveScopeError(t *testing.T) {
	t.Parallel()
	// resolver is nil but scope has Project, which should trigger the resolver-not-initialized error.
	svc := newTracesServiceForTest(t, &fakeTracingAdapter{})

	_, err := svc.QueryTraces(context.Background(), &types.TracesQueryRequest{
		SearchScope: types.ComponentSearchScope{
			Namespace: "ns",
			Project:   "proj",
		},
		StartTime: time.Now(),
		EndTime:   time.Now().Add(time.Hour),
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrTracesResolveSearchScope)
}

func TestTracesService_QueryTraces_WithResolver_Success(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)

	tokenSrv := newAlwaysOKTokenServer(t)
	defer tokenSrv.Close()

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

	adapter := &fakeTracingAdapter{
		tracesResult: &observability.TracesQueryResult{
			Traces:     []observability.Trace{{TraceID: "t-1", RootSpanName: "op"}},
			TotalCount: 1,
			Took:       3,
		},
	}
	svc, err := NewTracesService(adapter, resolver, &config.Config{},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	resp, err := svc.QueryTraces(context.Background(), &types.TracesQueryRequest{
		SearchScope: types.ComponentSearchScope{
			Namespace:   "ns",
			Project:     "proj",
			Component:   "comp",
			Environment: "env",
		},
		StartTime: now,
		EndTime:   now.Add(time.Hour),
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, sampleProjectUID, adapter.lastParams.ProjectID)
	assert.Equal(t, sampleComponentUID, adapter.lastParams.ComponentID)
	assert.Equal(t, sampleEnvironmentUID, adapter.lastParams.EnvironmentID)
}

func TestTracesService_QueryTraces_ComponentWithoutProject(t *testing.T) {
	t.Parallel()
	// resolver non-nil but scope has component without project → validation error.
	tokenSrv := newAlwaysOKTokenServer(t)
	defer tokenSrv.Close()
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer apiSrv.Close()
	resolver := newTestResolver(t, apiSrv, tokenSrv, &config.UIDResolverConfig{MaxAuthRetry: 1})

	svc, err := NewTracesService(&fakeTracingAdapter{}, resolver, &config.Config{},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	_, err = svc.QueryTraces(context.Background(), &types.TracesQueryRequest{
		SearchScope: types.ComponentSearchScope{
			Namespace: "ns",
			Component: "comp", // no Project
		},
		StartTime: time.Now(),
		EndTime:   time.Now().Add(time.Hour),
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrTracesResolveSearchScope)
}

func TestTracesService_QuerySpans_NilRequest(t *testing.T) {
	t.Parallel()
	svc := newTracesServiceForTest(t, &fakeTracingAdapter{})
	_, err := svc.QuerySpans(context.Background(), "trace-1", nil)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrTracesInvalidRequest)
}

func TestTracesService_QuerySpans_EmptyTraceID(t *testing.T) {
	t.Parallel()
	svc := newTracesServiceForTest(t, &fakeTracingAdapter{})
	_, err := svc.QuerySpans(context.Background(), "", &types.TracesQueryRequest{
		SearchScope: types.ComponentSearchScope{Namespace: "ns"},
		StartTime:   time.Now(),
		EndTime:     time.Now().Add(time.Hour),
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrTracesInvalidRequest)
}

func TestTracesService_QuerySpans_Success(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)
	adapter := &fakeTracingAdapter{
		spansResult: &observability.SpansResult{
			Spans: []observability.TraceSpan{
				{
					SpanID:    "span-1",
					Name:      "http.request",
					SpanKind:  "SERVER",
					StartTime: now,
					EndTime:   now.Add(100 * time.Millisecond),
				},
				{
					SpanID:       "span-2",
					Name:         "db.query",
					SpanKind:     "CLIENT",
					ParentSpanID: "span-1",
					StartTime:    now.Add(20 * time.Millisecond),
					EndTime:      now.Add(80 * time.Millisecond),
				},
			},
			TotalCount: 2,
			Took:       6,
		},
	}
	svc := newTracesServiceForTest(t, adapter)

	resp, err := svc.QuerySpans(context.Background(), "trace-xyz", &types.TracesQueryRequest{
		SearchScope: types.ComponentSearchScope{Namespace: "ns"},
		StartTime:   now,
		EndTime:     now.Add(time.Hour),
		Limit:       20,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, adapter.spansCalled)
	assert.Equal(t, "trace-xyz", adapter.lastTraceID)
	require.Len(t, resp.Spans, 2)
	assert.Equal(t, "span-1", resp.Spans[0].SpanID)
	assert.Equal(t, "SERVER", resp.Spans[0].SpanKind)
	assert.Equal(t, "span-2", resp.Spans[1].SpanID)
	assert.Equal(t, "span-1", resp.Spans[1].ParentSpanID)
	assert.Equal(t, 2, resp.Total)
	assert.Equal(t, 6, resp.TookMs)
}

func TestTracesService_QuerySpans_AdapterError(t *testing.T) {
	t.Parallel()
	adapter := &fakeTracingAdapter{spansErr: errors.New("upstream boom")}
	svc := newTracesServiceForTest(t, adapter)

	_, err := svc.QuerySpans(context.Background(), "trace-1", &types.TracesQueryRequest{
		SearchScope: types.ComponentSearchScope{Namespace: "ns"},
		StartTime:   time.Now(),
		EndTime:     time.Now().Add(time.Hour),
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrTracesRetrieval)
}

func TestTracesService_GetSpanDetails_EmptyTraceID(t *testing.T) {
	t.Parallel()
	svc := newTracesServiceForTest(t, &fakeTracingAdapter{})
	_, err := svc.GetSpanDetails(context.Background(), "", "span-1")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrTracesInvalidRequest)
}

func TestTracesService_GetSpanDetails_EmptySpanID(t *testing.T) {
	t.Parallel()
	svc := newTracesServiceForTest(t, &fakeTracingAdapter{})
	_, err := svc.GetSpanDetails(context.Background(), "trace-1", "")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrTracesInvalidRequest)
}

func TestTracesService_GetSpanDetails_Success(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)
	adapter := &fakeTracingAdapter{
		spanDetailResult: &observability.SpanDetail{
			SpanID:       "span-1",
			SpanName:     "http.request",
			SpanKind:     "SERVER",
			ParentSpanID: "",
			StartTime:    now,
			EndTime:      now.Add(time.Second),
			DurationNs:   1000000000,
			Status:       &observability.SpanStatus{Code: "error", Message: "failed to initialize connection to database"},
			Attributes:   map[string]interface{}{"http.method": "GET"},
		},
	}
	svc := newTracesServiceForTest(t, adapter)

	resp, err := svc.GetSpanDetails(context.Background(), "trace-1", "span-1")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, adapter.spanDetailsCalled)
	assert.Equal(t, "trace-1", adapter.lastTraceID)
	assert.Equal(t, "span-1", adapter.lastSpanID)
	assert.Equal(t, "span-1", resp.SpanID)
	assert.Equal(t, "http.request", resp.SpanName)
	assert.Equal(t, "SERVER", resp.SpanKind)
	assert.Equal(t, int64(1000000000), resp.DurationNs)
	assert.Equal(t, "GET", resp.Attributes["http.method"])
	require.NotNil(t, resp.Status)
	assert.Equal(t, "error", resp.Status.Code)
	assert.Equal(t, "failed to initialize connection to database", resp.Status.Message)
}

func TestTracesService_GetSpanDetails_NotFoundPassthrough(t *testing.T) {
	t.Parallel()
	adapter := &fakeTracingAdapter{spanDetailErr: ErrSpanNotFound}
	svc := newTracesServiceForTest(t, adapter)

	_, err := svc.GetSpanDetails(context.Background(), "trace-1", "missing")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSpanNotFound)
}

func TestTracesService_GetSpanDetails_OtherError(t *testing.T) {
	t.Parallel()
	adapter := &fakeTracingAdapter{spanDetailErr: errors.New("upstream boom")}
	svc := newTracesServiceForTest(t, adapter)

	_, err := svc.GetSpanDetails(context.Background(), "trace-1", "span-1")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrTracesRetrieval)
}

func TestTracesService_ConvertAdapterSpansToResponse(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)
	svc := newTracesServiceForTest(t, &fakeTracingAdapter{})
	result := &observability.SpansResult{
		Spans: []observability.TraceSpan{
			{
				SpanID:    "span-1",
				Name:      "operation",
				SpanKind:  "SERVER",
				StartTime: now,
				EndTime:   now.Add(time.Second),
			},
		},
		TotalCount: 1,
		Took:       4,
	}
	resp := svc.convertAdapterSpansToResponse(result)
	require.NotNil(t, resp)
	require.Len(t, resp.Spans, 1)
	assert.Equal(t, "span-1", resp.Spans[0].SpanID)
	assert.Equal(t, 1, resp.Total)
	assert.Equal(t, 4, resp.TookMs)
}
