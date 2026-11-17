// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	coremocks "github.com/openchoreo/openchoreo/internal/authz/core/mocks"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/querylimit"
	"github.com/openchoreo/openchoreo/internal/observer/service/mocks"
	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

func rawRequest() *types.RawTracesQueryRequest {
	return &types.RawTracesQueryRequest{
		ContentType: "application/json",
		Body:        []byte(`{"query":{"sql":"SELECT trace_id FROM \"default\" LIMIT 10"}}`),
	}
}

func rawTracesService(t *testing.T, adapter observability.TracingAdapter) *TracesService {
	t.Helper()
	svc, err := NewTracesService(adapter, nil, &config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	return svc
}

func TestTracesService_QueryRawTraces(t *testing.T) {
	t.Parallel()

	t.Run("forwards the request and returns the response", func(t *testing.T) {
		t.Parallel()
		adapter := &fakeTracingAdapter{rawResult: &observability.RawTracesQueryResult{
			Body: json.RawMessage(`{"hits":[]}`),
		}}
		svc := rawTracesService(t, adapter)

		resp, err := svc.QueryRawTraces(context.Background(), rawRequest())
		require.NoError(t, err)
		assert.Equal(t, `{"hits":[]}`, string(resp.Body))
		require.NotNil(t, adapter.lastRawParams)
		assert.Equal(t, "application/json", adapter.lastRawParams.ContentType)
		assert.Equal(t, string(rawRequest().Body), string(adapter.lastRawParams.Body))
	})

	t.Run("rejects an empty body before calling the adapter", func(t *testing.T) {
		t.Parallel()
		adapter := &fakeTracingAdapter{}
		svc := rawTracesService(t, adapter)

		_, err := svc.QueryRawTraces(context.Background(), &types.RawTracesQueryRequest{ContentType: "text/plain"})
		require.ErrorIs(t, err, ErrRawTracesInvalidRequest)
		assert.Nil(t, adapter.lastRawParams)
	})

	t.Run("passes raw query errors through unchanged", func(t *testing.T) {
		t.Parallel()
		for _, want := range []error{
			ErrRawTracesNotSupported, ErrRawTracesTimeout, ErrRawTracesResultTooLarge,
			ErrRawTracesRequestTooLarge, ErrRawTracesUnsupportedMediaType,
			&RawTracesRejectedError{Detail: "bad SQL"},
		} {
			svc := rawTracesService(t, &fakeTracingAdapter{rawErr: want})
			_, err := svc.QueryRawTraces(context.Background(), rawRequest())
			assert.Equal(t, want, err)
		}
	})

	t.Run("passes a canceled request through, not as a retrieval failure", func(t *testing.T) {
		t.Parallel()
		svc := rawTracesService(t, &fakeTracingAdapter{rawErr: fmt.Errorf("failed to execute request: %w", context.Canceled)})
		_, err := svc.QueryRawTraces(context.Background(), rawRequest())
		assert.ErrorIs(t, err, context.Canceled)
		assert.NotErrorIs(t, err, ErrTracesRetrieval)
	})

	t.Run("warns about an adapter without the raw query route", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		svc, err := NewTracesService(&fakeTracingAdapter{rawErr: &rawTracesNoRouteError{status: http.StatusNotFound}},
			nil, &config.Config{}, slog.New(slog.NewTextHandler(&logs, nil)))
		require.NoError(t, err)

		_, err = svc.QueryRawTraces(context.Background(), rawRequest())
		assert.ErrorIs(t, err, ErrRawTracesNotSupported)
		assert.Contains(t, logs.String(), "level=WARN")
		assert.Contains(t, logs.String(), "no raw query route")
	})

	t.Run("wraps other errors as a retrieval failure", func(t *testing.T) {
		t.Parallel()
		svc := rawTracesService(t, &fakeTracingAdapter{rawErr: errors.New("connection refused")})
		_, err := svc.QueryRawTraces(context.Background(), rawRequest())
		assert.ErrorIs(t, err, ErrTracesRetrieval)
	})
}

func TestTracesAuthz_QueryRawTraces(t *testing.T) {
	t.Parallel()

	t.Run("denied without the permission", func(t *testing.T) {
		t.Parallel()
		inner := mocks.NewMockTracesQuerier(t)
		svc := NewTracesServiceWithAuthz(inner, mockPDPDeny(t), testLogger())

		_, err := svc.QueryRawTraces(authedCtx(), rawRequest())
		require.ErrorIs(t, err, observerAuthz.ErrAuthzForbidden)
		inner.AssertNotCalled(t, "QueryRawTraces", mock.Anything, mock.Anything)
	})

	// A raw query is not limited to a namespace or project, so the check must use an
	// empty hierarchy, which casbin maps to "*" and only a cluster-scoped binding satisfies.
	t.Run("evaluates traces:rawquery at cluster scope", func(t *testing.T) {
		t.Parallel()
		inner := mocks.NewMockTracesQuerier(t)
		inner.EXPECT().QueryRawTraces(mock.Anything, mock.Anything).
			Return(&types.RawTracesQueryResponse{Body: json.RawMessage(`{}`)}, nil)

		var captured authzcore.EvaluateRequest
		pdp := coremocks.NewMockPDP(t)
		pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
			Run(func(_ context.Context, req *authzcore.EvaluateRequest) { captured = *req }).
			Return(&authzcore.Decision{Decision: true}, nil).Once()

		svc := NewTracesServiceWithAuthz(inner, pdp, testLogger())
		_, err := svc.QueryRawTraces(authedCtx(), rawRequest())
		require.NoError(t, err)

		assert.Equal(t, string(observerAuthz.ActionRawQueryTraces), captured.Action)
		assert.Equal(t, authzcore.ResourceHierarchy{}, captured.Resource.Hierarchy,
			"raw trace queries must evaluate at cluster scope")
	})
}

// blockingRawAdapter holds each raw query until release is closed.
type blockingRawAdapter struct {
	fakeTracingAdapter
	entered chan struct{}
	release chan struct{}
}

func (b *blockingRawAdapter) QueryRawTraces(ctx context.Context,
	_ observability.RawTracesQueryParams,
) (*observability.RawTracesQueryResult, error) {
	b.entered <- struct{}{}
	select {
	case <-b.release:
		return &observability.RawTracesQueryResult{Body: json.RawMessage(`{}`)}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func limitedTracesService(t *testing.T, adapter observability.TracingAdapter, maxConcurrent, perMinute, burst int) *TracesService {
	t.Helper()
	cfg := &config.Config{}
	cfg.Adapters.TracingRawQueryMaxConcurrent = maxConcurrent
	cfg.Adapters.TracingRawQueryRequestsPerMinute = perMinute
	cfg.Adapters.TracingRawQueryBurst = burst
	svc, err := NewTracesService(adapter, nil, cfg, testLogger())
	require.NoError(t, err)
	return svc
}

func subjectCtx(id string) context.Context {
	return auth.SetSubjectContext(context.Background(), &auth.SubjectContext{ID: id, Issuer: "idp", Type: "user"})
}

func requireRawLimited(t *testing.T, err error, reason querylimit.Reason) {
	t.Helper()
	require.ErrorIs(t, err, ErrRawTracesLimited)
	var limited *querylimit.LimitedError
	require.ErrorAs(t, err, &limited)
	assert.Equal(t, reason, limited.Reason)
}

func TestTracesService_QueryRawTraces_Limits(t *testing.T) {
	t.Parallel()

	t.Run("refuses a query while the maximum are running", func(t *testing.T) {
		t.Parallel()
		adapter := &blockingRawAdapter{entered: make(chan struct{}, 1), release: make(chan struct{})}
		svc := limitedTracesService(t, adapter, 1, 0, 0)

		done := make(chan error, 1)
		go func() {
			_, err := svc.QueryRawTraces(subjectCtx("alice"), rawRequest())
			done <- err
		}()
		<-adapter.entered

		_, err := svc.QueryRawTraces(subjectCtx("bob"), rawRequest())
		requireRawLimited(t, err, querylimit.ReasonConcurrency)

		close(adapter.release)
		require.NoError(t, <-done)

		// The finished query gave its slot back.
		_, err = svc.QueryRawTraces(subjectCtx("bob"), rawRequest())
		require.NoError(t, err)
	})

	t.Run("gives the slot back when the adapter fails", func(t *testing.T) {
		t.Parallel()
		adapter := &fakeTracingAdapter{rawErr: ErrRawTracesTimeout}
		svc := limitedTracesService(t, adapter, 1, 0, 0)

		for range 3 {
			_, err := svc.QueryRawTraces(subjectCtx("alice"), rawRequest())
			require.ErrorIs(t, err, ErrRawTracesTimeout)
		}
	})

	t.Run("gives the slot back when the caller cancels", func(t *testing.T) {
		t.Parallel()
		adapter := &blockingRawAdapter{entered: make(chan struct{}, 1), release: make(chan struct{})}
		svc := limitedTracesService(t, adapter, 1, 0, 0)

		ctx, cancel := context.WithCancel(subjectCtx("alice"))
		done := make(chan error, 1)
		go func() {
			_, err := svc.QueryRawTraces(ctx, rawRequest())
			done <- err
		}()
		<-adapter.entered
		cancel()
		require.ErrorIs(t, <-done, context.Canceled)

		close(adapter.release)
		go func() { <-adapter.entered }()
		_, err := svc.QueryRawTraces(subjectCtx("alice"), rawRequest())
		require.NoError(t, err)
	})

	t.Run("limits each caller's rate separately", func(t *testing.T) {
		t.Parallel()
		adapter := &fakeTracingAdapter{rawResult: &observability.RawTracesQueryResult{Body: json.RawMessage(`{}`)}}
		svc := limitedTracesService(t, adapter, 0, 1, 2)

		for range 2 {
			_, err := svc.QueryRawTraces(subjectCtx("alice"), rawRequest())
			require.NoError(t, err)
		}
		adapter.lastRawParams = nil
		_, err := svc.QueryRawTraces(subjectCtx("alice"), rawRequest())
		requireRawLimited(t, err, querylimit.ReasonRate)
		assert.Nil(t, adapter.lastRawParams, "a refused query must not reach the adapter")

		_, err = svc.QueryRawTraces(subjectCtx("bob"), rawRequest())
		require.NoError(t, err)
	})

	t.Run("an empty request uses up no limit", func(t *testing.T) {
		t.Parallel()
		adapter := &fakeTracingAdapter{rawResult: &observability.RawTracesQueryResult{Body: json.RawMessage(`{}`)}}
		svc := limitedTracesService(t, adapter, 0, 1, 1)

		_, err := svc.QueryRawTraces(subjectCtx("alice"), &types.RawTracesQueryRequest{ContentType: "text/plain"})
		require.ErrorIs(t, err, ErrRawTracesInvalidRequest)
		_, err = svc.QueryRawTraces(subjectCtx("alice"), rawRequest())
		require.NoError(t, err)
	})
}
