// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointrelease

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointrelease/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	authzNamespace = "ns-a"
	authzProject   = "proj-1"
)

func newAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &endpointReleaseServiceWithAuthz{
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

func newReleaseFixture(name string) *openchoreov1alpha1.EndpointRelease {
	return &openchoreov1alpha1.EndpointRelease{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: authzNamespace},
		Spec: openchoreov1alpha1.EndpointReleaseSpec{
			Owner: openchoreov1alpha1.EndpointReleaseOwner{
				ProjectName:  authzProject,
				EndpointName: "my-r",
			},
		},
	}
}

func TestGetEndpointRelease_AuthzCheck(t *testing.T) {
	rr := newReleaseFixture("my-rel")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpointRelease", mock.Anything, authzNamespace, "my-rel").Return(rr, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.GetEndpointRelease(testutil.AuthzContext(), authzNamespace, "my-rel")
		require.NoError(t, err)
		require.Equal(t, rr, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "endpointrelease:view", "endpointrelease", "my-rel", projectHierarchy())
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpointRelease", mock.Anything, authzNamespace, "my-rel").Return(rr, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.GetEndpointRelease(testutil.AuthzContext(), authzNamespace, "my-rel")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found bypasses authz", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpointRelease", mock.Anything, authzNamespace, "missing").Return(nil, ErrEndpointReleaseNotFound)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.GetEndpointRelease(testutil.AuthzContext(), authzNamespace, "missing")
		require.ErrorIs(t, err, ErrEndpointReleaseNotFound)
		require.Len(t, pdp.Captured, 0)
	})
}

func TestDeleteEndpointRelease_AuthzCheck(t *testing.T) {
	rr := newReleaseFixture("my-rel")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpointRelease", mock.Anything, authzNamespace, "my-rel").Return(rr, nil)
		mockSvc.On("DeleteEndpointRelease", mock.Anything, authzNamespace, "my-rel").Return(nil)
		svc := newAuthzSvc(pdp, mockSvc)
		err := svc.DeleteEndpointRelease(testutil.AuthzContext(), authzNamespace, "my-rel")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "endpointrelease:delete", "endpointrelease", "my-rel", projectHierarchy())
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpointRelease", mock.Anything, authzNamespace, "my-rel").Return(rr, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		err := svc.DeleteEndpointRelease(testutil.AuthzContext(), authzNamespace, "my-rel")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found bypasses authz", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpointRelease", mock.Anything, authzNamespace, "missing").Return(nil, ErrEndpointReleaseNotFound)
		svc := newAuthzSvc(pdp, mockSvc)
		err := svc.DeleteEndpointRelease(testutil.AuthzContext(), authzNamespace, "missing")
		require.ErrorIs(t, err, ErrEndpointReleaseNotFound)
		require.Len(t, pdp.Captured, 0)
	})
}

func TestListEndpointReleases_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.EndpointRelease{
		*newReleaseFixture("rel-1"),
		*newReleaseFixture("rel-2"),
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListEndpointReleases", mock.Anything, authzNamespace, "", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.EndpointRelease]{Items: items}, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.ListEndpointReleases(testutil.AuthzContext(), authzNamespace, "", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "endpointrelease:view", "endpointrelease", "rel-1", projectHierarchy())
		testutil.RequireEvalRequest(t, pdp.Captured[1], "endpointrelease:view", "endpointrelease", "rel-2", projectHierarchy())
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListEndpointReleases", mock.Anything, authzNamespace, "", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.EndpointRelease]{Items: items}, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.ListEndpointReleases(testutil.AuthzContext(), authzNamespace, "", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
