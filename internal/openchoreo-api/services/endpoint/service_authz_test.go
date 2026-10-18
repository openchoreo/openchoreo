// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpoint/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	authzNamespace = "ns-a"
	authzProject   = "proj-1"
)

func newAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &endpointServiceWithAuthz{
		internal: internal,
		authz:    testutil.NewTestAuthzChecker(pdp),
	}
}

func projectHierarchy() authzcore.ResourceHierarchy {
	return authzcore.ResourceHierarchy{
		Namespace: authzNamespace,
		Project:   authzProject,
	}
}

func newEndpointFixture(name string) *openchoreov1alpha1.Endpoint {
	return &openchoreov1alpha1.Endpoint{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: authzNamespace},
		Spec: openchoreov1alpha1.EndpointSpec{
			Owner: openchoreov1alpha1.EndpointOwner{ProjectName: authzProject},
			Type: openchoreov1alpha1.EndpointTypeRef{
				Kind: openchoreov1alpha1.EndpointTypeRefKindEndpointType,
				Name: "postgres",
			},
		},
	}
}

func TestCreateEndpoint_AuthzCheck(t *testing.T) {
	endpoint := newEndpointFixture("my-r")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateEndpoint", mock.Anything, authzNamespace, endpoint).Return(endpoint, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateEndpoint(testutil.AuthzContext(), authzNamespace, endpoint)
		require.NoError(t, err)
		require.Equal(t, endpoint, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "endpoint:create", "endpoint", "my-r", projectHierarchy())
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateEndpoint(testutil.AuthzContext(), authzNamespace, endpoint)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateEndpoint_AuthzCheck(t *testing.T) {
	endpoint := newEndpointFixture("my-r")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpoint", mock.Anything, authzNamespace, "my-r").Return(endpoint, nil)
		mockSvc.On("UpdateEndpoint", mock.Anything, authzNamespace, endpoint).Return(endpoint, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.UpdateEndpoint(testutil.AuthzContext(), authzNamespace, endpoint)
		require.NoError(t, err)
		require.Equal(t, endpoint, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "endpoint:update", "endpoint", "my-r", projectHierarchy())
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpoint", mock.Anything, authzNamespace, "my-r").Return(endpoint, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateEndpoint(testutil.AuthzContext(), authzNamespace, endpoint)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetEndpoint_AuthzCheck(t *testing.T) {
	endpoint := newEndpointFixture("my-r")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpoint", mock.Anything, authzNamespace, "my-r").Return(endpoint, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.GetEndpoint(testutil.AuthzContext(), authzNamespace, "my-r")
		require.NoError(t, err)
		require.Equal(t, endpoint, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "endpoint:view", "endpoint", "my-r", projectHierarchy())
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpoint", mock.Anything, authzNamespace, "my-r").Return(endpoint, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.GetEndpoint(testutil.AuthzContext(), authzNamespace, "my-r")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found bypasses authz", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpoint", mock.Anything, authzNamespace, "missing").Return(nil, ErrEndpointNotFound)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.GetEndpoint(testutil.AuthzContext(), authzNamespace, "missing")
		require.ErrorIs(t, err, ErrEndpointNotFound)
		require.Len(t, pdp.Captured, 0, "PDP should not be queried when endpoint is not found")
	})
}

func TestDeleteEndpoint_AuthzCheck(t *testing.T) {
	endpoint := newEndpointFixture("my-r")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpoint", mock.Anything, authzNamespace, "my-r").Return(endpoint, nil)
		mockSvc.On("DeleteEndpoint", mock.Anything, authzNamespace, "my-r").Return(nil)
		svc := newAuthzSvc(pdp, mockSvc)
		err := svc.DeleteEndpoint(testutil.AuthzContext(), authzNamespace, "my-r")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "endpoint:delete", "endpoint", "my-r", projectHierarchy())
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpoint", mock.Anything, authzNamespace, "my-r").Return(endpoint, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		err := svc.DeleteEndpoint(testutil.AuthzContext(), authzNamespace, "my-r")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found bypasses authz", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpoint", mock.Anything, authzNamespace, "missing").Return(nil, ErrEndpointNotFound)
		svc := newAuthzSvc(pdp, mockSvc)
		err := svc.DeleteEndpoint(testutil.AuthzContext(), authzNamespace, "missing")
		require.ErrorIs(t, err, ErrEndpointNotFound)
		require.Len(t, pdp.Captured, 0)
	})
}

func TestListEndpoints_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.Endpoint{
		*newEndpointFixture("r-1"),
		*newEndpointFixture("r-2"),
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListEndpoints", mock.Anything, authzNamespace, "", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.Endpoint]{Items: items}, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.ListEndpoints(testutil.AuthzContext(), authzNamespace, "", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "endpoint:view", "endpoint", "r-1", projectHierarchy())
		testutil.RequireEvalRequest(t, pdp.Captured[1], "endpoint:view", "endpoint", "r-2", projectHierarchy())
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListEndpoints", mock.Anything, authzNamespace, "", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.Endpoint]{Items: items}, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.ListEndpoints(testutil.AuthzContext(), authzNamespace, "", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}

func TestUpdateEndpoint_AuthzUsesStoredProject(t *testing.T) {
	stored := newEndpointFixture("my-r")
	claimed := newEndpointFixture("my-r")
	claimed.Spec.Owner.ProjectName = "project-the-caller-holds"

	pdp := testutil.AllowPDP()
	mockSvc := mocks.NewMockService(t)
	mockSvc.On("GetEndpoint", mock.Anything, authzNamespace, "my-r").Return(stored, nil)
	mockSvc.On("UpdateEndpoint", mock.Anything, authzNamespace, claimed).Return(claimed, nil)
	svc := newAuthzSvc(pdp, mockSvc)

	_, err := svc.UpdateEndpoint(testutil.AuthzContext(), authzNamespace, claimed)
	require.NoError(t, err)
	require.Len(t, pdp.Captured, 1)
	require.Equal(t, authzProject, pdp.Captured[0].Resource.Hierarchy.Project,
		"authz must check the stored project, not the one claimed in the request body")
}
