// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/pkg/observability"
)

func rawParams() observability.RawTracesQueryParams {
	return observability.RawTracesQueryParams{
		ContentType: "application/json",
		Body:        []byte(`{"size":0,"query":{"term":{"status.code":"Error"}}}`),
	}
}

func newRawTestAdapter(t *testing.T, handler http.HandlerFunc, cfg TracingAdapterConfig) *TracingAdapter {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cfg.BaseURL = srv.URL
	adapter, err := NewTracingAdapter(cfg)
	require.NoError(t, err)
	return adapter
}

func TestQueryRawTraces_SendsTheBodyAndReturnsTheResponse(t *testing.T) {
	t.Parallel()

	const response = `{"took":7,"aggregations":{"n":{"value":12345678901234567890}}}`
	var gotPath, gotContentType, gotBody string
	adapter := newRawTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		_, _ = io.WriteString(w, response)
	}, TracingAdapterConfig{})

	result, err := adapter.QueryRawTraces(context.Background(), rawParams())
	require.NoError(t, err)

	assert.Equal(t, "/api/v1alpha1/traces/raw-query", gotPath)
	assert.Equal(t, "application/json", gotContentType)
	assert.Equal(t, string(rawParams().Body), gotBody, "the body must be forwarded byte for byte")
	assert.Equal(t, response, string(result.Body), "the response must come back unchanged")
}

func TestQueryRawTraces_SendsATextBody(t *testing.T) {
	t.Parallel()

	const kql = "AppDependencies | take 10"
	var gotContentType, gotBody string
	adapter := newRawTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		_, _ = io.WriteString(w, `{"tables":[]}`)
	}, TracingAdapterConfig{})

	_, err := adapter.QueryRawTraces(context.Background(),
		observability.RawTracesQueryParams{ContentType: "text/plain", Body: []byte(kql)})
	require.NoError(t, err)
	assert.Equal(t, "text/plain", gotContentType)
	assert.Equal(t, kql, gotBody)
}

func TestQueryRawTraces_MapsAdapterStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
		body   string
		check  func(t *testing.T, err error)
	}{
		{"400 keeps the adapter's reason", http.StatusBadRequest,
			`{"title":"badRequest","detail":"invalid OpenSearch query DSL: unknown query [termz]"}`,
			func(t *testing.T, err error) {
				var rejected *RawTracesRejectedError
				require.ErrorAs(t, err, &rejected)
				assert.ErrorIs(t, err, ErrRawTracesRejected)
				assert.Equal(t, "invalid OpenSearch query DSL: unknown query [termz]", rejected.Detail)
			}},
		{"400 without a JSON body", http.StatusBadRequest, `bad query`,
			func(t *testing.T, err error) {
				var rejected *RawTracesRejectedError
				require.ErrorAs(t, err, &rejected)
				assert.Equal(t, "bad query", rejected.Detail)
			}},
		{"501", http.StatusNotImplemented, `{"title":"notImplemented"}`,
			func(t *testing.T, err error) {
				assert.ErrorIs(t, err, ErrRawTracesNotSupported)
				var noRoute *rawTracesNoRouteError
				assert.False(t, errors.As(err, &noRoute), "501 is a supported answer, not a missing route")
			}},
		{"400 with the result too large code", http.StatusBadRequest,
			`{"title":"badRequest","errorCode":"RAW_QUERY_RESULT_TOO_LARGE","detail":"the result is too large"}`,
			func(t *testing.T, err error) {
				assert.ErrorIs(t, err, ErrRawTracesResultTooLarge)
				assert.NotErrorIs(t, err, ErrRawTracesRejected)
			}},
		{"404 from a module without the route", http.StatusNotFound, `404 page not found`,
			func(t *testing.T, err error) {
				assert.ErrorIs(t, err, ErrRawTracesNotSupported)
				var noRoute *rawTracesNoRouteError
				require.ErrorAs(t, err, &noRoute)
				assert.Equal(t, http.StatusNotFound, noRoute.status)
			}},
		{"405 from a module without the route", http.StatusMethodNotAllowed, `Method Not Allowed`,
			func(t *testing.T, err error) {
				assert.ErrorIs(t, err, ErrRawTracesNotSupported)
				var noRoute *rawTracesNoRouteError
				assert.ErrorAs(t, err, &noRoute)
			}},
		{"413", http.StatusRequestEntityTooLarge, `{"title":"payloadTooLarge"}`,
			func(t *testing.T, err error) { assert.ErrorIs(t, err, ErrRawTracesRequestTooLarge) }},
		{"415", http.StatusUnsupportedMediaType, `{"title":"unsupportedMediaType"}`,
			func(t *testing.T, err error) { assert.ErrorIs(t, err, ErrRawTracesUnsupportedMediaType) }},
		{"504", http.StatusGatewayTimeout, `{"title":"gatewayTimeout"}`,
			func(t *testing.T, err error) { assert.ErrorIs(t, err, ErrRawTracesTimeout) }},
		{"500", http.StatusInternalServerError, `{"title":"internalServerError"}`,
			func(t *testing.T, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "API returned status 500")
			}},
		{"200 that is not JSON", http.StatusOK, `not json`,
			func(t *testing.T, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "not JSON")
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			adapter := newRawTestAdapter(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}, TracingAdapterConfig{})

			_, err := adapter.QueryRawTraces(context.Background(), rawParams())
			tt.check(t, err)
		})
	}
}

func TestQueryRawTraces_RejectsAnOversizedResponse(t *testing.T) {
	t.Parallel()

	adapter := newRawTestAdapter(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"hits":"`+strings.Repeat("x", 100<<10)+`"}`)
	}, TracingAdapterConfig{RawQueryMaxResponseBytes: 1024})

	_, err := adapter.QueryRawTraces(context.Background(), rawParams())
	assert.ErrorIs(t, err, ErrRawTracesResultTooLarge)
}

func TestQueryRawTraces_KeepsTheMeaningOfAnOversizedErrorAnswer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status int
		want   error
	}{
		{http.StatusRequestEntityTooLarge, ErrRawTracesRequestTooLarge},
		{http.StatusUnsupportedMediaType, ErrRawTracesUnsupportedMediaType},
		{http.StatusNotImplemented, ErrRawTracesNotSupported},
		{http.StatusGatewayTimeout, ErrRawTracesTimeout},
		{http.StatusBadRequest, ErrRawTracesRejected},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			t.Parallel()
			adapter := newRawTestAdapter(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, `{"detail":"`+strings.Repeat("x", 4<<10)+`"}`)
			}, TracingAdapterConfig{RawQueryMaxResponseBytes: 1024})

			_, err := adapter.QueryRawTraces(context.Background(), rawParams())
			assert.ErrorIs(t, err, tt.want)
			assert.NotErrorIs(t, err, ErrRawTracesResultTooLarge,
				"the maximum response size applies to results only")
		})
	}
}

func TestQueryRawTraces_AcceptsTheLargestResponseLimit(t *testing.T) {
	t.Parallel()

	adapter := newRawTestAdapter(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"took":1}`)
	}, TracingAdapterConfig{RawQueryMaxResponseBytes: math.MaxInt64})

	result, err := adapter.QueryRawTraces(context.Background(), rawParams())
	require.NoError(t, err)
	assert.JSONEq(t, `{"took":1}`, string(result.Body))
}

func TestQueryRawTraces_TimesOutPastTheMargin(t *testing.T) {
	t.Parallel()

	// The handler blocks until the client gives up, so the deadline is what ends the call.
	// The server only notices the client leaving once the body has been read.
	adapter := newRawTestAdapter(t, func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	}, TracingAdapterConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := adapter.QueryRawTraces(ctx, rawParams())
	assert.ErrorIs(t, err, ErrRawTracesTimeout)
}

func TestQueryRawTraces_KeepsACanceledRequestApart(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	adapter := newRawTestAdapter(t, func(http.ResponseWriter, *http.Request) { <-release }, TracingAdapterConfig{})
	t.Cleanup(func() { close(release) })

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	_, err := adapter.QueryRawTraces(ctx, rawParams())
	assert.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, ErrRawTracesTimeout)
}

func TestAdapterError(t *testing.T) {
	t.Parallel()

	code, detail := adapterError([]byte(`{"errorCode":"X","detail":"` + strings.Repeat("a", 2000) + `"}`))
	assert.Equal(t, "X", code)
	assert.LessOrEqual(t, len(detail), maxRejectionDetailLength+len("…"))
	assert.True(t, errors.Is(&RawTracesRejectedError{Detail: detail}, ErrRawTracesRejected))
}

func TestTruncate_KeepsRunesWhole(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "abc", truncate("abc", 3))
	assert.Equal(t, "ab…", truncate("abcd", 2))
	// "é" is two bytes; a cut through it backs off to the rune before.
	got := truncate("aé", 2)
	assert.Equal(t, "a…", got)
	assert.True(t, utf8.ValidString(got))
}
