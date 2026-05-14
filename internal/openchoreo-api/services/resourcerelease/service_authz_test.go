// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcerelease

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcerelease/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	authzNamespace = "ns-a"
	authzProject   = "proj-1"
)

func newAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &resourceReleaseServiceWithAuthz{
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

func newReleaseFixture(name string) *openchoreov1alpha1.ResourceRelease {
	return &openchoreov1alpha1.ResourceRelease{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: authzNamespace},
		Spec: openchoreov1alpha1.ResourceReleaseSpec{
			Owner: openchoreov1alpha1.ResourceReleaseOwner{
				ProjectName:  authzProject,
				ResourceName: "my-r",
			},
		},
	}
}

func TestCreateResourceRelease_AuthzCheck(t *testing.T) {
	rr := newReleaseFixture("my-rel")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateResourceRelease", mock.Anything, authzNamespace, rr).Return(rr, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateResourceRelease(testutil.AuthzContext(), authzNamespace, rr)
		require.NoError(t, err)
		require.Equal(t, rr, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "resourcerelease:create", "resourcerelease", "my-rel", projectHierarchy())
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateResourceRelease(testutil.AuthzContext(), authzNamespace, rr)
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("nil input", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateResourceRelease(testutil.AuthzContext(), authzNamespace, nil)
		require.ErrorIs(t, err, ErrResourceReleaseNil)
		require.Len(t, pdp.Captured, 0, "PDP should not be queried for nil input")
	})
}

func TestGetResourceRelease_AuthzCheck(t *testing.T) {
	rr := newReleaseFixture("my-rel")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResourceRelease", mock.Anything, authzNamespace, "my-rel").Return(rr, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.GetResourceRelease(testutil.AuthzContext(), authzNamespace, "my-rel")
		require.NoError(t, err)
		require.Equal(t, rr, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "resourcerelease:view", "resourcerelease", "my-rel", projectHierarchy())
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResourceRelease", mock.Anything, authzNamespace, "my-rel").Return(rr, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.GetResourceRelease(testutil.AuthzContext(), authzNamespace, "my-rel")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found bypasses authz", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResourceRelease", mock.Anything, authzNamespace, "missing").Return(nil, ErrResourceReleaseNotFound)
		svc := newAuthzSvc(pdp, mockSvc)
		_, err := svc.GetResourceRelease(testutil.AuthzContext(), authzNamespace, "missing")
		require.ErrorIs(t, err, ErrResourceReleaseNotFound)
		require.Len(t, pdp.Captured, 0)
	})
}

func TestDeleteResourceRelease_AuthzCheck(t *testing.T) {
	rr := newReleaseFixture("my-rel")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResourceRelease", mock.Anything, authzNamespace, "my-rel").Return(rr, nil)
		mockSvc.On("DeleteResourceRelease", mock.Anything, authzNamespace, "my-rel").Return(nil)
		svc := newAuthzSvc(pdp, mockSvc)
		err := svc.DeleteResourceRelease(testutil.AuthzContext(), authzNamespace, "my-rel")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "resourcerelease:delete", "resourcerelease", "my-rel", projectHierarchy())
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResourceRelease", mock.Anything, authzNamespace, "my-rel").Return(rr, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		err := svc.DeleteResourceRelease(testutil.AuthzContext(), authzNamespace, "my-rel")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("not found bypasses authz", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResourceRelease", mock.Anything, authzNamespace, "missing").Return(nil, ErrResourceReleaseNotFound)
		svc := newAuthzSvc(pdp, mockSvc)
		err := svc.DeleteResourceRelease(testutil.AuthzContext(), authzNamespace, "missing")
		require.ErrorIs(t, err, ErrResourceReleaseNotFound)
		require.Len(t, pdp.Captured, 0)
	})
}

func TestListResourceReleases_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.ResourceRelease{
		*newReleaseFixture("rel-1"),
		*newReleaseFixture("rel-2"),
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListResourceReleases", mock.Anything, authzNamespace, "", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ResourceRelease]{Items: items}, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.ListResourceReleases(testutil.AuthzContext(), authzNamespace, "", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "resourcerelease:view", "resourcerelease", "rel-1", projectHierarchy())
		testutil.RequireEvalRequest(t, pdp.Captured[1], "resourcerelease:view", "resourcerelease", "rel-2", projectHierarchy())
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListResourceReleases", mock.Anything, authzNamespace, "", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ResourceRelease]{Items: items}, nil)
		svc := newAuthzSvc(pdp, mockSvc)
		result, err := svc.ListResourceReleases(testutil.AuthzContext(), authzNamespace, "", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
