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

func clusterLogsRequest() *types.ClusterLogsQueryRequest {
	return &types.ClusterLogsQueryRequest{
		Namespaces: []string{"openchoreo-control-plane"},
		StartTime:  "2026-08-14T16:30:00Z",
		EndTime:    "2026-08-14T17:30:00Z",
	}
}

func TestClusterLogsAuthz_NilPDP(t *testing.T) {
	inner := mocks.NewMockClusterLogsQuerier(t)
	expected := &types.ClusterLogsResponse{}
	inner.EXPECT().QueryClusterLogs(mock.Anything, mock.Anything).Return(expected, nil)

	svc := NewClusterLogsServiceWithAuthz(inner, nil, testLogger())

	resp, err := svc.QueryClusterLogs(context.Background(), clusterLogsRequest())
	require.NoError(t, err)
	assert.Equal(t, expected, resp)
}

func TestClusterLogsAuthz_Allowed(t *testing.T) {
	inner := mocks.NewMockClusterLogsQuerier(t)
	expected := &types.ClusterLogsResponse{}
	inner.EXPECT().QueryClusterLogs(mock.Anything, mock.Anything).Return(expected, nil)

	svc := NewClusterLogsServiceWithAuthz(inner, mockPDPAllow(t), testLogger())

	resp, err := svc.QueryClusterLogs(authedCtx(), clusterLogsRequest())
	require.NoError(t, err)
	assert.Equal(t, expected, resp)
}

func TestClusterLogsAuthz_Denied(t *testing.T) {
	inner := mocks.NewMockClusterLogsQuerier(t)

	svc := NewClusterLogsServiceWithAuthz(inner, mockPDPDeny(t), testLogger())

	_, err := svc.QueryClusterLogs(authedCtx(), clusterLogsRequest())
	require.ErrorIs(t, err, observerAuthz.ErrAuthzForbidden)
	inner.AssertNotCalled(t, "QueryClusterLogs", mock.Anything, mock.Anything)
}

func TestClusterLogsAuthz_NoSubjectContext(t *testing.T) {
	inner := mocks.NewMockClusterLogsQuerier(t)
	pdp := coremocks.NewMockPDP(t)

	svc := NewClusterLogsServiceWithAuthz(inner, pdp, testLogger())

	_, err := svc.QueryClusterLogs(context.Background(), clusterLogsRequest())
	require.ErrorIs(t, err, observerAuthz.ErrAuthzUnauthorized)
	inner.AssertNotCalled(t, "QueryClusterLogs", mock.Anything, mock.Anything)
}

// TestClusterLogsAuthz_EvaluatesAtClusterScope pins the property the permission
// depends on: the request must carry an empty hierarchy, which casbin maps to "*" and
// only a cluster-scoped binding satisfies. A hierarchy derived from the query's
// Kubernetes namespaces would hand out access on a name collision with an OpenChoreo
// namespace.
func TestClusterLogsAuthz_EvaluatesAtClusterScope(t *testing.T) {
	inner := mocks.NewMockClusterLogsQuerier(t)
	inner.EXPECT().QueryClusterLogs(mock.Anything, mock.Anything).
		Return(&types.ClusterLogsResponse{}, nil)

	var captured authzcore.EvaluateRequest
	pdp := coremocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Run(func(_ context.Context, req *authzcore.EvaluateRequest) {
			captured = *req
		}).
		Return(&authzcore.Decision{Decision: true}, nil).Once()

	svc := NewClusterLogsServiceWithAuthz(inner, pdp, testLogger())
	_, err := svc.QueryClusterLogs(authedCtx(), clusterLogsRequest())
	require.NoError(t, err)

	assert.Equal(t, string(observerAuthz.ActionViewClusterLogs), captured.Action)
	assert.Equal(t, authzcore.ResourceHierarchy{}, captured.Resource.Hierarchy,
		"cluster logs must evaluate at cluster scope")
}
