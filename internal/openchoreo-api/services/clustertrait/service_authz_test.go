// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustertrait

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clustertrait/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newCTAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &clusterTraitServiceWithAuthz{
		internal: internal,
		authz:    testutil.NewTestAuthzChecker(pdp),
	}
}

func TestCreateClusterTrait_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterTrait{ObjectMeta: metav1.ObjectMeta{Name: "my-ct"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateClusterTrait", mock.Anything, resource).Return(resource, nil)
		svc := newCTAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateClusterTrait(testutil.AuthzContext(), resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clustertrait:create", "clusterTrait", "my-ct", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCTAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateClusterTrait(testutil.AuthzContext(), resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateClusterTrait_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterTrait{ObjectMeta: metav1.ObjectMeta{Name: "my-ct"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("UpdateClusterTrait", mock.Anything, resource).Return(resource, nil)
		svc := newCTAuthzSvc(pdp, mockSvc)
		result, err := svc.UpdateClusterTrait(testutil.AuthzContext(), resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clustertrait:update", "clusterTrait", "my-ct", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCTAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateClusterTrait(testutil.AuthzContext(), resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetClusterTrait_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterTrait{ObjectMeta: metav1.ObjectMeta{Name: "my-ct"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetClusterTrait", mock.Anything, "my-ct").Return(resource, nil)
		svc := newCTAuthzSvc(pdp, mockSvc)
		result, err := svc.GetClusterTrait(testutil.AuthzContext(), "my-ct")
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clustertrait:view", "clusterTrait", "my-ct", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCTAuthzSvc(pdp, mockSvc)
		_, err := svc.GetClusterTrait(testutil.AuthzContext(), "my-ct")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeleteClusterTrait_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("DeleteClusterTrait", mock.Anything, "my-ct").Return(nil)
		svc := newCTAuthzSvc(pdp, mockSvc)
		err := svc.DeleteClusterTrait(testutil.AuthzContext(), "my-ct")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clustertrait:delete", "clusterTrait", "my-ct", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCTAuthzSvc(pdp, mockSvc)
		err := svc.DeleteClusterTrait(testutil.AuthzContext(), "my-ct")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetClusterTraitSchema_AuthzCheck(t *testing.T) {
	schema := map[string]any{"type": "object"}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetClusterTraitSchema", mock.Anything, "my-ct").Return(schema, nil)
		svc := newCTAuthzSvc(pdp, mockSvc)
		result, err := svc.GetClusterTraitSchema(testutil.AuthzContext(), "my-ct")
		require.NoError(t, err)
		require.Equal(t, schema, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clustertrait:view", "clusterTrait", "my-ct", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCTAuthzSvc(pdp, mockSvc)
		_, err := svc.GetClusterTraitSchema(testutil.AuthzContext(), "my-ct")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestListClusterTraits_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.ClusterTrait{
		{ObjectMeta: metav1.ObjectMeta{Name: "ct-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "ct-2"}},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListClusterTraits", mock.Anything, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ClusterTrait]{Items: items}, nil)
		svc := newCTAuthzSvc(pdp, mockSvc)
		result, err := svc.ListClusterTraits(testutil.AuthzContext(), services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clustertrait:view", "clusterTrait", "ct-1", authzcore.ResourceHierarchy{})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "clustertrait:view", "clusterTrait", "ct-2", authzcore.ResourceHierarchy{})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListClusterTraits", mock.Anything, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ClusterTrait]{Items: items}, nil)
		svc := newCTAuthzSvc(pdp, mockSvc)
		result, err := svc.ListClusterTraits(testutil.AuthzContext(), services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
