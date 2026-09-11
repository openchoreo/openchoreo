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

func TestAuditLogsAuthz_NilPDP(t *testing.T) {
	inner := mocks.NewMockAuditLogsQuerier(t)
	expected := &types.AuditLogsResponse{}
	inner.EXPECT().QueryAuditLogs(mock.Anything, mock.Anything).Return(expected, nil)

	svc := NewAuditLogsServiceWithAuthz(inner, nil, testLogger())

	resp, err := svc.QueryAuditLogs(context.Background(), auditLogsRequest())
	require.NoError(t, err)
	assert.Equal(t, expected, resp)
}

func TestAuditLogsAuthz_Allowed(t *testing.T) {
	inner := mocks.NewMockAuditLogsQuerier(t)
	expected := &types.AuditLogsResponse{}
	inner.EXPECT().QueryAuditLogs(mock.Anything, mock.Anything).Return(expected, nil)

	svc := NewAuditLogsServiceWithAuthz(inner, mockPDPAllow(t), testLogger())

	resp, err := svc.QueryAuditLogs(authedCtx(), auditLogsRequest())
	require.NoError(t, err)
	assert.Equal(t, expected, resp)
}

func TestAuditLogsAuthz_Denied(t *testing.T) {
	inner := mocks.NewMockAuditLogsQuerier(t)

	svc := NewAuditLogsServiceWithAuthz(inner, mockPDPDeny(t), testLogger())

	_, err := svc.QueryAuditLogs(authedCtx(), auditLogsRequest())
	require.ErrorIs(t, err, observerAuthz.ErrAuthzForbidden)
	inner.AssertNotCalled(t, "QueryAuditLogs", mock.Anything, mock.Anything)
}

func TestAuditLogsAuthz_NoSubjectContext(t *testing.T) {
	inner := mocks.NewMockAuditLogsQuerier(t)
	pdp := coremocks.NewMockPDP(t)

	svc := NewAuditLogsServiceWithAuthz(inner, pdp, testLogger())

	_, err := svc.QueryAuditLogs(context.Background(), auditLogsRequest())
	require.ErrorIs(t, err, observerAuthz.ErrAuthzUnauthorized)
	inner.AssertNotCalled(t, "QueryAuditLogs", mock.Anything, mock.Anything)
}

// TestAuditLogsAuthz_FilterValuesCarriesTheSameCheck: the filter values read
// enumerates actors, resource names and source addresses, so gating only the
// record read would leave this as a way around it.
func TestAuditLogsAuthz_FilterValuesCarriesTheSameCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		inner := mocks.NewMockAuditLogsQuerier(t)
		expected := &types.AuditLogFilterValuesResponse{}
		inner.EXPECT().QueryAuditLogFilterValues(mock.Anything, mock.Anything).Return(expected, nil)

		svc := NewAuditLogsServiceWithAuthz(inner, mockPDPAllow(t), testLogger())

		resp, err := svc.QueryAuditLogFilterValues(authedCtx(), auditLogFilterValuesRequest())
		require.NoError(t, err)
		assert.Equal(t, expected, resp)
	})

	t.Run("denied", func(t *testing.T) {
		inner := mocks.NewMockAuditLogsQuerier(t)

		svc := NewAuditLogsServiceWithAuthz(inner, mockPDPDeny(t), testLogger())

		_, err := svc.QueryAuditLogFilterValues(authedCtx(), auditLogFilterValuesRequest())
		require.ErrorIs(t, err, observerAuthz.ErrAuthzForbidden)
		inner.AssertNotCalled(t, "QueryAuditLogFilterValues", mock.Anything, mock.Anything)
	})
}

// TestAuditLogsAuthz_EvaluatesAtClusterScope pins what the decorator asks the
// PDP: the cluster-scoped action, and an empty hierarchy that only a
// cluster-scoped binding can satisfy.
func TestAuditLogsAuthz_EvaluatesAtClusterScope(t *testing.T) {
	inner := mocks.NewMockAuditLogsQuerier(t)
	inner.EXPECT().QueryAuditLogs(mock.Anything, mock.Anything).
		Return(&types.AuditLogsResponse{}, nil)

	var got *authzcore.EvaluateRequest
	pdp := coremocks.NewMockPDP(t)
	pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
		Run(func(_ context.Context, req *authzcore.EvaluateRequest) { got = req }).
		Return(&authzcore.Decision{Decision: true}, nil).Once()

	svc := NewAuditLogsServiceWithAuthz(inner, pdp, testLogger())
	_, err := svc.QueryAuditLogs(authedCtx(), auditLogsRequest())
	require.NoError(t, err)

	assert.Equal(t, string(observerAuthz.ActionViewAuditLogs), got.Action)
	assert.Equal(t, authzcore.ResourceHierarchy{}, got.Resource.Hierarchy)
}

// TestAuditLogsAuthz_TenancyFilterDoesNotWidenScope is the property this commit
// exists for. A namespace-scoped binding must not satisfy an audit query just
// because the query filters to the namespace it holds: working in a project does
// not confer the right to read its audit trail.
//
// Enforced by the decorator passing an empty hierarchy regardless of the
// request, which resourceHierarchyToPath maps to "*". The assertion is that the
// namespace in the body reaches the PDP nowhere.
func TestAuditLogsAuthz_TenancyFilterDoesNotWidenScope(t *testing.T) {
	tests := []struct {
		name string
		call func(svc AuditLogsQuerier) error
	}{
		{
			name: "record query",
			call: func(svc AuditLogsQuerier) error {
				req := auditLogsRequest()
				req.Resource.Namespaces = []string{"payments"}
				_, err := svc.QueryAuditLogs(authedCtx(), req)
				return err
			},
		},
		{
			name: "filter values query",
			call: func(svc AuditLogsQuerier) error {
				req := auditLogFilterValuesRequest()
				req.Query.Resource.Namespaces = []string{"payments"}
				_, err := svc.QueryAuditLogFilterValues(authedCtx(), req)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := mocks.NewMockAuditLogsQuerier(t)

			var got *authzcore.EvaluateRequest
			pdp := coremocks.NewMockPDP(t)
			pdp.EXPECT().Evaluate(mock.Anything, mock.Anything).
				Run(func(_ context.Context, req *authzcore.EvaluateRequest) { got = req }).
				Return(&authzcore.Decision{Decision: false}, nil).Once()

			svc := NewAuditLogsServiceWithAuthz(inner, pdp, testLogger())

			err := tt.call(svc)
			require.ErrorIs(t, err, observerAuthz.ErrAuthzForbidden)

			// The filtered namespace is a filter, never a scope: it must not
			// appear anywhere in what the PDP evaluated.
			assert.Equal(t, authzcore.ResourceHierarchy{}, got.Resource.Hierarchy)
			assert.NotEqual(t, "payments", got.Resource.Hierarchy.Namespace)
		})
	}
}
