// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterobservabilityplane

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterobservabilityplane/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newCOPAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &clusterObservabilityPlaneServiceWithAuthz{
		internal: internal,
		authz:    testutil.NewTestAuthzChecker(pdp),
	}
}

func TestCreateClusterObservabilityPlane_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterObservabilityPlane{ObjectMeta: metav1.ObjectMeta{Name: "my-cop"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateClusterObservabilityPlane", mock.Anything, resource).Return(resource, nil)
		svc := newCOPAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateClusterObservabilityPlane(testutil.AuthzContext(), resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterobservabilityplane:create", "clusterobservabilityplane", "my-cop", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCOPAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateClusterObservabilityPlane(testutil.AuthzContext(), resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateClusterObservabilityPlane_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterObservabilityPlane{ObjectMeta: metav1.ObjectMeta{Name: "my-cop"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("UpdateClusterObservabilityPlane", mock.Anything, resource).Return(resource, nil)
		svc := newCOPAuthzSvc(pdp, mockSvc)
		result, err := svc.UpdateClusterObservabilityPlane(testutil.AuthzContext(), resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterobservabilityplane:update", "clusterobservabilityplane", "my-cop", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCOPAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateClusterObservabilityPlane(testutil.AuthzContext(), resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetClusterObservabilityPlane_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterObservabilityPlane{ObjectMeta: metav1.ObjectMeta{Name: "my-cop"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetClusterObservabilityPlane", mock.Anything, "my-cop").Return(resource, nil)
		svc := newCOPAuthzSvc(pdp, mockSvc)
		result, err := svc.GetClusterObservabilityPlane(testutil.AuthzContext(), "my-cop")
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterobservabilityplane:view", "clusterobservabilityplane", "my-cop", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCOPAuthzSvc(pdp, mockSvc)
		_, err := svc.GetClusterObservabilityPlane(testutil.AuthzContext(), "my-cop")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeleteClusterObservabilityPlane_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("DeleteClusterObservabilityPlane", mock.Anything, "my-cop").Return(nil)
		svc := newCOPAuthzSvc(pdp, mockSvc)
		err := svc.DeleteClusterObservabilityPlane(testutil.AuthzContext(), "my-cop")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterobservabilityplane:delete", "clusterobservabilityplane", "my-cop", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCOPAuthzSvc(pdp, mockSvc)
		err := svc.DeleteClusterObservabilityPlane(testutil.AuthzContext(), "my-cop")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestListClusterObservabilityPlanes_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.ClusterObservabilityPlane{
		{ObjectMeta: metav1.ObjectMeta{Name: "cop-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "cop-2"}},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListClusterObservabilityPlanes", mock.Anything, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ClusterObservabilityPlane]{Items: items}, nil)
		svc := newCOPAuthzSvc(pdp, mockSvc)
		result, err := svc.ListClusterObservabilityPlanes(testutil.AuthzContext(), services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterobservabilityplane:view", "clusterobservabilityplane", "cop-1", authzcore.ResourceHierarchy{})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "clusterobservabilityplane:view", "clusterobservabilityplane", "cop-2", authzcore.ResourceHierarchy{})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListClusterObservabilityPlanes", mock.Anything, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ClusterObservabilityPlane]{Items: items}, nil)
		svc := newCOPAuthzSvc(pdp, mockSvc)
		result, err := svc.ListClusterObservabilityPlanes(testutil.AuthzContext(), services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
