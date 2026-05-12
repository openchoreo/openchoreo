// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resource/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	authzNamespace = "ns-a"
	authzProject   = "proj-1"
)

func newAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &resourceServiceWithAuthz{
		internal: internal,
		authz:    testutil.NewTestAuthzChecker(pdp),
	}
}

func projectHierarchy(resourceName string) authzcore.ResourceHierarchy {
	return authzcore.ResourceHierarchy{
		Namespace: authzNamespace,
		Project:   authzProject,
		Resource:  resourceName,
	}
}

func newResourceFixture(name string) *openchoreov1alpha1.Resource {
	return &openchoreov1alpha1.Resource{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: authzNamespace},
		Spec: openchoreov1alpha1.ResourceSpec{
			Owner: openchoreov1alpha1.ResourceOwner{ProjectName: authzProject},
		},
	}
}

func TestCreateResource_AuthzCheck(t *testing.T) {
	resource := newResourceFixture("my-r")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateResource", mock.Anything, authzNamespace, resource).Return(resource, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateResource(testutil.AuthzContext(), authzNamespace, resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "resource:create", "resource", "my-r", projectHierarchy("my-r"))
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateResource(testutil.AuthzContext(), authzNamespace, resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateResource_AuthzCheck(t *testing.T) {
	resource := newResourceFixture("my-r")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("UpdateResource", mock.Anything, authzNamespace, resource).Return(resource, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.UpdateResource(testutil.AuthzContext(), authzNamespace, resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "resource:update", "resource", "my-r", projectHierarchy("my-r"))
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateResource(testutil.AuthzContext(), authzNamespace, resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetResource_AuthzCheck(t *testing.T) {
	resource := newResourceFixture("my-r")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResource", mock.Anything, authzNamespace, "my-r").Return(resource, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.GetResource(testutil.AuthzContext(), authzNamespace, "my-r")
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "resource:view", "resource", "my-r", projectHierarchy("my-r"))
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResource", mock.Anything, authzNamespace, "my-r").Return(resource, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.GetResource(testutil.AuthzContext(), authzNamespace, "my-r")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found bypasses authz", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResource", mock.Anything, authzNamespace, "missing").Return(nil, ErrResourceNotFound)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.GetResource(testutil.AuthzContext(), authzNamespace, "missing")
		require.ErrorIs(t, err, ErrResourceNotFound)
		require.Len(t, pdp.Captured, 0, "PDP should not be queried when resource is not found")
	})
}

func TestDeleteResource_AuthzCheck(t *testing.T) {
	resource := newResourceFixture("my-r")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResource", mock.Anything, authzNamespace, "my-r").Return(resource, nil)
		mockSvc.On("DeleteResource", mock.Anything, authzNamespace, "my-r").Return(nil)
		svc := newAuthzSvc(pdp, mockSvc)
		err := svc.DeleteResource(testutil.AuthzContext(), authzNamespace, "my-r")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "resource:delete", "resource", "my-r", projectHierarchy("my-r"))
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResource", mock.Anything, authzNamespace, "my-r").Return(resource, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		err := svc.DeleteResource(testutil.AuthzContext(), authzNamespace, "my-r")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found bypasses authz", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResource", mock.Anything, authzNamespace, "missing").Return(nil, ErrResourceNotFound)
		svc := newAuthzSvc(pdp, mockSvc)
		err := svc.DeleteResource(testutil.AuthzContext(), authzNamespace, "missing")
		require.ErrorIs(t, err, ErrResourceNotFound)
		require.Len(t, pdp.Captured, 0)
	})
}

func TestListResources_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.Resource{
		*newResourceFixture("r-1"),
		*newResourceFixture("r-2"),
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListResources", mock.Anything, authzNamespace, "", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.Resource]{Items: items}, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.ListResources(testutil.AuthzContext(), authzNamespace, "", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "resource:view", "resource", "r-1", projectHierarchy("r-1"))
		testutil.RequireEvalRequest(t, pdp.Captured[1], "resource:view", "resource", "r-2", projectHierarchy("r-2"))
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListResources", mock.Anything, authzNamespace, "", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.Resource]{Items: items}, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.ListResources(testutil.AuthzContext(), authzNamespace, "", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
