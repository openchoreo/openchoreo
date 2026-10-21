// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	coremocks "github.com/openchoreo/openchoreo/internal/authz/core/mocks"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/config"
	"github.com/openchoreo/openchoreo/internal/observer/service/mocks"
	"github.com/openchoreo/openchoreo/internal/observer/types"
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
