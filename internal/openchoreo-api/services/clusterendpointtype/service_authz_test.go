// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterendpointtype

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterendpointtype/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newCRTAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &clusterEndpointTypeServiceWithAuthz{
		internal: internal,
		authz:    testutil.NewTestAuthzChecker(pdp),
	}
}

func TestCreateClusterEndpointType_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterEndpointType{ObjectMeta: metav1.ObjectMeta{Name: "my-crt"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateClusterEndpointType", mock.Anything, resource).Return(resource, nil)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateClusterEndpointType(testutil.AuthzContext(), resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterendpointtype:create", "clusterendpointtype", "my-crt", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateClusterEndpointType(testutil.AuthzContext(), resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateClusterEndpointType_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterEndpointType{ObjectMeta: metav1.ObjectMeta{Name: "my-crt"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("UpdateClusterEndpointType", mock.Anything, resource).Return(resource, nil)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		result, err := svc.UpdateClusterEndpointType(testutil.AuthzContext(), resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterendpointtype:update", "clusterendpointtype", "my-crt", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateClusterEndpointType(testutil.AuthzContext(), resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetClusterEndpointType_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterEndpointType{ObjectMeta: metav1.ObjectMeta{Name: "my-crt"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetClusterEndpointType", mock.Anything, "my-crt").Return(resource, nil)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		result, err := svc.GetClusterEndpointType(testutil.AuthzContext(), "my-crt")
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterendpointtype:view", "clusterendpointtype", "my-crt", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		_, err := svc.GetClusterEndpointType(testutil.AuthzContext(), "my-crt")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeleteClusterEndpointType_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("DeleteClusterEndpointType", mock.Anything, "my-crt").Return(nil)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		err := svc.DeleteClusterEndpointType(testutil.AuthzContext(), "my-crt")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterendpointtype:delete", "clusterendpointtype", "my-crt", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		err := svc.DeleteClusterEndpointType(testutil.AuthzContext(), "my-crt")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetClusterEndpointTypeSchema_AuthzCheck(t *testing.T) {
	schema := map[string]any{"type": "object"}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetClusterEndpointTypeSchema", mock.Anything, "my-crt").Return(schema, nil)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		result, err := svc.GetClusterEndpointTypeSchema(testutil.AuthzContext(), "my-crt")
		require.NoError(t, err)
		require.Equal(t, schema, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterendpointtype:view", "clusterendpointtype", "my-crt", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		_, err := svc.GetClusterEndpointTypeSchema(testutil.AuthzContext(), "my-crt")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestListClusterEndpointTypes_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.ClusterEndpointType{
		{ObjectMeta: metav1.ObjectMeta{Name: "crt-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "crt-2"}},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListClusterEndpointTypes", mock.Anything, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ClusterEndpointType]{Items: items}, nil)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		result, err := svc.ListClusterEndpointTypes(testutil.AuthzContext(), services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterendpointtype:view", "clusterendpointtype", "crt-1", authzcore.ResourceHierarchy{})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "clusterendpointtype:view", "clusterendpointtype", "crt-2", authzcore.ResourceHierarchy{})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListClusterEndpointTypes", mock.Anything, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ClusterEndpointType]{Items: items}, nil)
		svc := newCRTAuthzSvc(pdp, mockSvc)
		result, err := svc.ListClusterEndpointTypes(testutil.AuthzContext(), services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
