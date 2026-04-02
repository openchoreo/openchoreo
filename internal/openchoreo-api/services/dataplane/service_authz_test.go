// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/dataplane/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func TestDataPlaneAuthz_CreateDataPlane(t *testing.T) {
	dp := &openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Name: "dp-1", Namespace: "ns-1"}}

	t.Run("nil input", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &dataPlaneServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.CreateDataPlane(testutil.AuthzContext(), "ns-1", nil)
		require.ErrorIs(t, err, ErrDataPlaneNil)
		require.Empty(t, pdp.Captured, "authz should not be called on nil input")
	})

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateDataPlane", mock.Anything, "ns-1", dp).Return(dp, nil)
		svc := &dataPlaneServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.CreateDataPlane(testutil.AuthzContext(), "ns-1", dp)
		require.NoError(t, err)
		require.Equal(t, dp, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "dataplane:create", "dataPlane", "dp-1", authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &dataPlaneServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.CreateDataPlane(testutil.AuthzContext(), "ns-1", dp)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDataPlaneAuthz_UpdateDataPlane(t *testing.T) {
	dp := &openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Name: "dp-1", Namespace: "ns-1"}}

	t.Run("nil input", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &dataPlaneServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.UpdateDataPlane(testutil.AuthzContext(), "ns-1", nil)
		require.ErrorIs(t, err, ErrDataPlaneNil)
		require.Empty(t, pdp.Captured, "authz should not be called on nil input")
	})

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("UpdateDataPlane", mock.Anything, "ns-1", dp).Return(dp, nil)
		svc := &dataPlaneServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.UpdateDataPlane(testutil.AuthzContext(), "ns-1", dp)
		require.NoError(t, err)
		require.Equal(t, dp, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "dataplane:update", "dataPlane", "dp-1", authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &dataPlaneServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.UpdateDataPlane(testutil.AuthzContext(), "ns-1", dp)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDataPlaneAuthz_GetDataPlane(t *testing.T) {
	dp := &openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Name: "dp-1", Namespace: "ns-1"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetDataPlane", mock.Anything, "ns-1", "dp-1").Return(dp, nil)
		svc := &dataPlaneServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.GetDataPlane(testutil.AuthzContext(), "ns-1", "dp-1")
		require.NoError(t, err)
		require.Equal(t, dp, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "dataplane:view", "dataPlane", "dp-1", authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &dataPlaneServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetDataPlane(testutil.AuthzContext(), "ns-1", "dp-1")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDataPlaneAuthz_DeleteDataPlane(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("DeleteDataPlane", mock.Anything, "ns-1", "dp-1").Return(nil)
		svc := &dataPlaneServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		err := svc.DeleteDataPlane(testutil.AuthzContext(), "ns-1", "dp-1")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "dataplane:delete", "dataPlane", "dp-1", authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &dataPlaneServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		err := svc.DeleteDataPlane(testutil.AuthzContext(), "ns-1", "dp-1")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDataPlaneAuthz_ListDataPlanes(t *testing.T) {
	items := []openchoreov1alpha1.DataPlane{
		{ObjectMeta: metav1.ObjectMeta{Name: "dp-1", Namespace: "ns-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "dp-2", Namespace: "ns-1"}},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListDataPlanes", mock.Anything, "ns-1", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.DataPlane]{Items: items}, nil)
		svc := &dataPlaneServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.ListDataPlanes(testutil.AuthzContext(), "ns-1", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "dataplane:view", "dataPlane", "dp-1", authzcore.ResourceHierarchy{Namespace: "ns-1"})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "dataplane:view", "dataPlane", "dp-2", authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListDataPlanes", mock.Anything, "ns-1", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.DataPlane]{Items: items}, nil)
		svc := &dataPlaneServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.ListDataPlanes(testutil.AuthzContext(), "ns-1", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
