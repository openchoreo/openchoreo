// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustercomponenttype

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clustercomponenttype/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newCCTAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &clusterComponentTypeServiceWithAuthz{
		internal: internal,
		authz:    testutil.NewTestAuthzChecker(pdp),
	}
}

func TestCreateClusterComponentType_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterComponentType{ObjectMeta: metav1.ObjectMeta{Name: "my-cct"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateClusterComponentType", mock.Anything, resource).Return(resource, nil)
		svc := newCCTAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateClusterComponentType(testutil.AuthzContext(), resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clustercomponenttype:create", "clusterComponentType", "my-cct", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCCTAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateClusterComponentType(testutil.AuthzContext(), resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateClusterComponentType_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterComponentType{ObjectMeta: metav1.ObjectMeta{Name: "my-cct"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("UpdateClusterComponentType", mock.Anything, resource).Return(resource, nil)
		svc := newCCTAuthzSvc(pdp, mockSvc)
		result, err := svc.UpdateClusterComponentType(testutil.AuthzContext(), resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clustercomponenttype:update", "clusterComponentType", "my-cct", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCCTAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateClusterComponentType(testutil.AuthzContext(), resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetClusterComponentType_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterComponentType{ObjectMeta: metav1.ObjectMeta{Name: "my-cct"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetClusterComponentType", mock.Anything, "my-cct").Return(resource, nil)
		svc := newCCTAuthzSvc(pdp, mockSvc)
		result, err := svc.GetClusterComponentType(testutil.AuthzContext(), "my-cct")
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clustercomponenttype:view", "clusterComponentType", "my-cct", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCCTAuthzSvc(pdp, mockSvc)
		_, err := svc.GetClusterComponentType(testutil.AuthzContext(), "my-cct")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeleteClusterComponentType_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("DeleteClusterComponentType", mock.Anything, "my-cct").Return(nil)
		svc := newCCTAuthzSvc(pdp, mockSvc)
		err := svc.DeleteClusterComponentType(testutil.AuthzContext(), "my-cct")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clustercomponenttype:delete", "clusterComponentType", "my-cct", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCCTAuthzSvc(pdp, mockSvc)
		err := svc.DeleteClusterComponentType(testutil.AuthzContext(), "my-cct")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetClusterComponentTypeSchema_AuthzCheck(t *testing.T) {
	schema := map[string]any{"type": "object"}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetClusterComponentTypeSchema", mock.Anything, "my-cct").Return(schema, nil)
		svc := newCCTAuthzSvc(pdp, mockSvc)
		result, err := svc.GetClusterComponentTypeSchema(testutil.AuthzContext(), "my-cct")
		require.NoError(t, err)
		require.Equal(t, schema, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clustercomponenttype:view", "clusterComponentType", "my-cct", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCCTAuthzSvc(pdp, mockSvc)
		_, err := svc.GetClusterComponentTypeSchema(testutil.AuthzContext(), "my-cct")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestListClusterComponentTypes_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.ClusterComponentType{
		{ObjectMeta: metav1.ObjectMeta{Name: "cct-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "cct-2"}},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListClusterComponentTypes", mock.Anything, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ClusterComponentType]{Items: items}, nil)
		svc := newCCTAuthzSvc(pdp, mockSvc)
		result, err := svc.ListClusterComponentTypes(testutil.AuthzContext(), services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clustercomponenttype:view", "clusterComponentType", "cct-1", authzcore.ResourceHierarchy{})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "clustercomponenttype:view", "clusterComponentType", "cct-2", authzcore.ResourceHierarchy{})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListClusterComponentTypes", mock.Anything, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ClusterComponentType]{Items: items}, nil)
		svc := newCCTAuthzSvc(pdp, mockSvc)
		result, err := svc.ListClusterComponentTypes(testutil.AuthzContext(), services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
