// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterdataplane

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterdataplane/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newCDPAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &clusterDataPlaneServiceWithAuthz{
		internal: internal,
		authz:    testutil.NewTestAuthzChecker(pdp),
	}
}

func TestCreateClusterDataPlane_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterDataPlane{ObjectMeta: metav1.ObjectMeta{Name: "my-cdp"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateClusterDataPlane", mock.Anything, resource).Return(resource, nil)
		svc := newCDPAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateClusterDataPlane(testutil.AuthzContext(), resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterdataplane:create", "clusterDataPlane", "my-cdp", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCDPAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateClusterDataPlane(testutil.AuthzContext(), resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateClusterDataPlane_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterDataPlane{ObjectMeta: metav1.ObjectMeta{Name: "my-cdp"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("UpdateClusterDataPlane", mock.Anything, resource).Return(resource, nil)
		svc := newCDPAuthzSvc(pdp, mockSvc)
		result, err := svc.UpdateClusterDataPlane(testutil.AuthzContext(), resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterdataplane:update", "clusterDataPlane", "my-cdp", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCDPAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateClusterDataPlane(testutil.AuthzContext(), resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetClusterDataPlane_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterDataPlane{ObjectMeta: metav1.ObjectMeta{Name: "my-cdp"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetClusterDataPlane", mock.Anything, "my-cdp").Return(resource, nil)
		svc := newCDPAuthzSvc(pdp, mockSvc)
		result, err := svc.GetClusterDataPlane(testutil.AuthzContext(), "my-cdp")
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterdataplane:view", "clusterDataPlane", "my-cdp", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCDPAuthzSvc(pdp, mockSvc)
		_, err := svc.GetClusterDataPlane(testutil.AuthzContext(), "my-cdp")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeleteClusterDataPlane_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("DeleteClusterDataPlane", mock.Anything, "my-cdp").Return(nil)
		svc := newCDPAuthzSvc(pdp, mockSvc)
		err := svc.DeleteClusterDataPlane(testutil.AuthzContext(), "my-cdp")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterdataplane:delete", "clusterDataPlane", "my-cdp", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCDPAuthzSvc(pdp, mockSvc)
		err := svc.DeleteClusterDataPlane(testutil.AuthzContext(), "my-cdp")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestListClusterDataPlanes_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.ClusterDataPlane{
		{ObjectMeta: metav1.ObjectMeta{Name: "cdp-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "cdp-2"}},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListClusterDataPlanes", mock.Anything, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ClusterDataPlane]{Items: items}, nil)
		svc := newCDPAuthzSvc(pdp, mockSvc)
		result, err := svc.ListClusterDataPlanes(testutil.AuthzContext(), services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterdataplane:view", "clusterDataPlane", "cdp-1", authzcore.ResourceHierarchy{})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "clusterdataplane:view", "clusterDataPlane", "cdp-2", authzcore.ResourceHierarchy{})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListClusterDataPlanes", mock.Anything, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ClusterDataPlane]{Items: items}, nil)
		svc := newCDPAuthzSvc(pdp, mockSvc)
		result, err := svc.ListClusterDataPlanes(testutil.AuthzContext(), services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
