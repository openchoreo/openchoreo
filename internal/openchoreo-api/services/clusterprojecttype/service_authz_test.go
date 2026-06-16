// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterprojecttype

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterprojecttype/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newCPTAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &clusterProjectTypeServiceWithAuthz{
		internal: internal,
		authz:    testutil.NewTestAuthzChecker(pdp),
	}
}

func TestCreateClusterProjectType_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterProjectType{ObjectMeta: metav1.ObjectMeta{Name: "my-cpt"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateClusterProjectType", mock.Anything, resource).Return(resource, nil)
		svc := newCPTAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateClusterProjectType(testutil.AuthzContext(), resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterprojecttype:create", "clusterprojecttype", "my-cpt", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCPTAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateClusterProjectType(testutil.AuthzContext(), resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateClusterProjectType_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterProjectType{ObjectMeta: metav1.ObjectMeta{Name: "my-cpt"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("UpdateClusterProjectType", mock.Anything, resource).Return(resource, nil)
		svc := newCPTAuthzSvc(pdp, mockSvc)
		result, err := svc.UpdateClusterProjectType(testutil.AuthzContext(), resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterprojecttype:update", "clusterprojecttype", "my-cpt", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCPTAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateClusterProjectType(testutil.AuthzContext(), resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetClusterProjectType_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterProjectType{ObjectMeta: metav1.ObjectMeta{Name: "my-cpt"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetClusterProjectType", mock.Anything, "my-cpt").Return(resource, nil)
		svc := newCPTAuthzSvc(pdp, mockSvc)
		result, err := svc.GetClusterProjectType(testutil.AuthzContext(), "my-cpt")
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterprojecttype:view", "clusterprojecttype", "my-cpt", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCPTAuthzSvc(pdp, mockSvc)
		_, err := svc.GetClusterProjectType(testutil.AuthzContext(), "my-cpt")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeleteClusterProjectType_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("DeleteClusterProjectType", mock.Anything, "my-cpt").Return(nil)
		svc := newCPTAuthzSvc(pdp, mockSvc)
		err := svc.DeleteClusterProjectType(testutil.AuthzContext(), "my-cpt")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterprojecttype:delete", "clusterprojecttype", "my-cpt", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCPTAuthzSvc(pdp, mockSvc)
		err := svc.DeleteClusterProjectType(testutil.AuthzContext(), "my-cpt")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetClusterProjectTypeSchema_AuthzCheck(t *testing.T) {
	schema := map[string]any{"type": "object"}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetClusterProjectTypeSchema", mock.Anything, "my-cpt").Return(schema, nil)
		svc := newCPTAuthzSvc(pdp, mockSvc)
		result, err := svc.GetClusterProjectTypeSchema(testutil.AuthzContext(), "my-cpt")
		require.NoError(t, err)
		require.Equal(t, schema, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterprojecttype:view", "clusterprojecttype", "my-cpt", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCPTAuthzSvc(pdp, mockSvc)
		_, err := svc.GetClusterProjectTypeSchema(testutil.AuthzContext(), "my-cpt")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestListClusterProjectTypes_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.ClusterProjectType{
		{ObjectMeta: metav1.ObjectMeta{Name: "cpt-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "cpt-2"}},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListClusterProjectTypes", mock.Anything, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ClusterProjectType]{Items: items}, nil)
		svc := newCPTAuthzSvc(pdp, mockSvc)
		result, err := svc.ListClusterProjectTypes(testutil.AuthzContext(), services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterprojecttype:view", "clusterprojecttype", "cpt-1", authzcore.ResourceHierarchy{})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "clusterprojecttype:view", "clusterprojecttype", "cpt-2", authzcore.ResourceHierarchy{})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListClusterProjectTypes", mock.Anything, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ClusterProjectType]{Items: items}, nil)
		svc := newCPTAuthzSvc(pdp, mockSvc)
		result, err := svc.ListClusterProjectTypes(testutil.AuthzContext(), services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
