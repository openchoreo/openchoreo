// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	coremocks "github.com/openchoreo/openchoreo/internal/authz/core/mocks"
	observerAuthz "github.com/openchoreo/openchoreo/internal/observer/authz"
	"github.com/openchoreo/openchoreo/internal/observer/service/mocks"
	"github.com/openchoreo/openchoreo/internal/observer/types"
)

func platformLogsRequest() *types.PlatformLogsQueryRequest {
	return &types.PlatformLogsQueryRequest{
		Namespaces: []string{"openchoreo-control-plane"},
		StartTime:  "2026-08-14T16:30:00Z",
		EndTime:    "2026-08-14T17:30:00Z",
	}
}

func TestPlatformLogsAuthz_NilPDP(t *testing.T) {
	inner := mocks.NewMockPlatformLogsQuerier(t)
	expected := &types.PlatformLogsResponse{}
	inner.EXPECT().QueryPlatformLogs(mock.Anything, mock.Anything).Return(expected, nil)

	svc := NewPlatformLogsServiceWithAuthz(inner, nil, testLogger())

	resp, err := svc.QueryPlatformLogs(context.Background(), platformLogsRequest())
	require.NoError(t, err)
	assert.Equal(t, expected, resp)
}

func TestPlatformLogsAuthz_Allowed(t *testing.T) {
	inner := mocks.NewMockPlatformLogsQuerier(t)
	expected := &types.PlatformLogsResponse{}
	inner.EXPECT().QueryPlatformLogs(mock.Anything, mock.Anything).Return(expected, nil)

	svc := NewPlatformLogsServiceWithAuthz(inner, mockPDPAllow(t), testLogger())

	resp, err := svc.QueryPlatformLogs(authedCtx(), platformLogsRequest())
	require.NoError(t, err)
	assert.Equal(t, expected, resp)
}

func TestPlatformLogsAuthz_Denied(t *testing.T) {
	inner := mocks.NewMockPlatformLogsQuerier(t)

	svc := NewPlatformLogsServiceWithAuthz(inner, mockPDPDeny(t), testLogger())

	_, err := svc.QueryPlatformLogs(authedCtx(), platformLogsRequest())
	require.ErrorIs(t, err, observerAuthz.ErrAuthzForbidden)
	inner.AssertNotCalled(t, "QueryPlatformLogs", mock.Anything, mock.Anything)
}

func TestPlatformLogsAuthz_NoSubjectContext(t *testing.T) {
	inner := mocks.NewMockPlatformLogsQuerier(t)
	pdp := coremocks.NewMockPDP(t)

	svc := NewPlatformLogsServiceWithAuthz(inner, pdp, testLogger())

	_, err := svc.QueryPlatformLogs(context.Background(), platformLogsRequest())
	require.ErrorIs(t, err, observerAuthz.ErrAuthzUnauthorized)
	inner.AssertNotCalled(t, "QueryPlatformLogs", mock.Anything, mock.Anything)
}

// TestPlatformLogsAuthz_EvaluatesAtClusterScope pins the property the permission
// depends on: the request must carry an empty hierarchy, which casbin maps to "*" and
// only a cluster-scoped binding satisfies. A hierarchy derived from the query's
// Kubernetes namespaces would hand out access on a name collision with an OpenChoreo
// namespace.
func TestPlatformLogsAuthz_EvaluatesAtClusterScope(t *testing.T) {
	inner := mocks.NewMockPlatformLogsQuerier(t)
	inner.EXPECT().QueryPlatformLogs(mock.Anything, mock.Anything).
		Return(&types.PlatformLogsResponse{}, nil)

	var captured authzcore.EvaluateRequest
	pdp := coremocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Run(func(_ context.Context, req *authzcore.EvaluateRequest) {
			captured = *req
		}).
		Return(&authzcore.Decision{Decision: true}, nil).Once()

	svc := NewPlatformLogsServiceWithAuthz(inner, pdp, testLogger())
	_, err := svc.QueryPlatformLogs(authedCtx(), platformLogsRequest())
	require.NoError(t, err)

	assert.Equal(t, string(observerAuthz.ActionViewPlatformLogs), captured.Action)
	assert.Equal(t, authzcore.ResourceHierarchy{}, captured.Resource.Hierarchy,
		"platform logs must evaluate at cluster scope")
}
