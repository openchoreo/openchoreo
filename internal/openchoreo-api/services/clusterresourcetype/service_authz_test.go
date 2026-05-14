// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterresourcetype

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterresourcetype/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newCRTAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &clusterResourceTypeServiceWithAuthz{
		internal: internal,
		authz:    testutil.NewTestAuthzChecker(pdp),
	}
}

func TestCreateClusterResourceType_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterResourceType{ObjectMeta: metav1.ObjectMeta{Name: "my-crt"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateClusterResourceType", mock.Anything, resource).Return(resource, nil)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateClusterResourceType(testutil.AuthzContext(), resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterresourcetype:create", "clusterresourcetype", "my-crt", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateClusterResourceType(testutil.AuthzContext(), resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateClusterResourceType_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterResourceType{ObjectMeta: metav1.ObjectMeta{Name: "my-crt"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("UpdateClusterResourceType", mock.Anything, resource).Return(resource, nil)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		result, err := svc.UpdateClusterResourceType(testutil.AuthzContext(), resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterresourcetype:update", "clusterresourcetype", "my-crt", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateClusterResourceType(testutil.AuthzContext(), resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetClusterResourceType_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterResourceType{ObjectMeta: metav1.ObjectMeta{Name: "my-crt"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetClusterResourceType", mock.Anything, "my-crt").Return(resource, nil)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		result, err := svc.GetClusterResourceType(testutil.AuthzContext(), "my-crt")
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterresourcetype:view", "clusterresourcetype", "my-crt", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		_, err := svc.GetClusterResourceType(testutil.AuthzContext(), "my-crt")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeleteClusterResourceType_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("DeleteClusterResourceType", mock.Anything, "my-crt").Return(nil)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		err := svc.DeleteClusterResourceType(testutil.AuthzContext(), "my-crt")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterresourcetype:delete", "clusterresourcetype", "my-crt", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		err := svc.DeleteClusterResourceType(testutil.AuthzContext(), "my-crt")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetClusterResourceTypeSchema_AuthzCheck(t *testing.T) {
	schema := map[string]any{"type": "object"}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetClusterResourceTypeSchema", mock.Anything, "my-crt").Return(schema, nil)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		result, err := svc.GetClusterResourceTypeSchema(testutil.AuthzContext(), "my-crt")
		require.NoError(t, err)
		require.Equal(t, schema, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterresourcetype:view", "clusterresourcetype", "my-crt", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		_, err := svc.GetClusterResourceTypeSchema(testutil.AuthzContext(), "my-crt")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestListClusterResourceTypes_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.ClusterResourceType{
		{ObjectMeta: metav1.ObjectMeta{Name: "crt-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "crt-2"}},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListClusterResourceTypes", mock.Anything, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ClusterResourceType]{Items: items}, nil)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		result, err := svc.ListClusterResourceTypes(testutil.AuthzContext(), services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterresourcetype:view", "clusterresourcetype", "crt-1", authzcore.ResourceHierarchy{})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "clusterresourcetype:view", "clusterresourcetype", "crt-2", authzcore.ResourceHierarchy{})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListClusterResourceTypes", mock.Anything, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ClusterResourceType]{Items: items}, nil)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		result, err := svc.ListClusterResourceTypes(testutil.AuthzContext(), services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
