// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componentrelease/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func testCR() *openchoreov1alpha1.ComponentRelease {
	return &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cr", Namespace: "ns-1"},
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{ProjectName: "my-proj", ComponentName: "my-comp"},
			ComponentType: openchoreov1alpha1.ComponentReleaseComponentType{
				Kind: openchoreov1alpha1.ComponentTypeRefKindComponentType,
				Name: "deployment/default",
			},
			Workload: openchoreov1alpha1.WorkloadTemplateSpec{Container: openchoreov1alpha1.Container{Image: "test:latest"}},
		},
	}
}

var crHierarchy = authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-proj", Component: "my-comp"}

// --- ListComponentReleases ---

func TestListComponentReleases_AuthzCheck(t *testing.T) {
	crs := []openchoreov1alpha1.ComponentRelease{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "cr-1", Namespace: "ns-1"},
			Spec: openchoreov1alpha1.ComponentReleaseSpec{
				Owner: openchoreov1alpha1.ComponentReleaseOwner{ProjectName: "my-proj", ComponentName: "my-comp"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "cr-2", Namespace: "ns-1"},
			Spec: openchoreov1alpha1.ComponentReleaseSpec{
				Owner: openchoreov1alpha1.ComponentReleaseOwner{ProjectName: "my-proj", ComponentName: "my-comp"},
			},
		},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListComponentReleases", mock.Anything, "ns-1", "my-comp", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ComponentRelease]{Items: crs}, nil)
		svc := &componentReleaseServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.ListComponentReleases(testutil.AuthzContext(), "ns-1", "my-comp", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "componentrelease:view", "componentrelease", "cr-1",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-proj", Component: "my-comp"})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "componentrelease:view", "componentrelease", "cr-2",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-proj", Component: "my-comp"})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListComponentReleases", mock.Anything, "ns-1", "my-comp", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ComponentRelease]{Items: crs}, nil)
		svc := &componentReleaseServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.ListComponentReleases(testutil.AuthzContext(), "ns-1", "my-comp", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}

// --- GetComponentRelease ---

func TestGetComponentRelease_AuthzCheck(t *testing.T) {
	fetched := testCR()

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetComponentRelease", mock.Anything, "ns-1", "my-cr").Return(fetched, nil)
		svc := &componentReleaseServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.GetComponentRelease(testutil.AuthzContext(), "ns-1", "my-cr")
		require.NoError(t, err)
		require.Equal(t, fetched, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "componentrelease:view", "componentrelease", "my-cr", crHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetComponentRelease", mock.Anything, "ns-1", "my-cr").Return(fetched, nil)
		svc := &componentReleaseServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetComponentRelease(testutil.AuthzContext(), "ns-1", "my-cr")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("fetch error", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		fetchErr := errors.New("not found")
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetComponentRelease", mock.Anything, "ns-1", "my-cr").Return(nil, fetchErr)
		svc := &componentReleaseServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetComponentRelease(testutil.AuthzContext(), "ns-1", "my-cr")
		require.ErrorIs(t, err, fetchErr)
		require.Empty(t, pdp.Captured, "authz should not be called when fetch fails")
	})
}

// --- P1 (with nil guard): CreateComponentRelease ---

func TestCreateComponentRelease_AuthzCheck(t *testing.T) {
	cr := testCR()

	t.Run("nil guard", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &componentReleaseServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.CreateComponentRelease(testutil.AuthzContext(), "ns-1", nil)
		require.ErrorIs(t, err, ErrComponentReleaseNil)
		require.Empty(t, pdp.Captured)
	})

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateComponentRelease", mock.Anything, "ns-1", cr).Return(cr, nil)
		svc := &componentReleaseServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.CreateComponentRelease(testutil.AuthzContext(), "ns-1", cr)
		require.NoError(t, err)
		require.Equal(t, cr, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "componentrelease:create", "componentrelease", "my-cr", crHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &componentReleaseServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.CreateComponentRelease(testutil.AuthzContext(), "ns-1", cr)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

// --- DeleteComponentRelease ---

func TestDeleteComponentRelease_AuthzCheck(t *testing.T) {
	fetched := testCR()

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetComponentRelease", mock.Anything, "ns-1", "my-cr").Return(fetched, nil)
		mockSvc.On("DeleteComponentRelease", mock.Anything, "ns-1", "my-cr").Return(nil)
		svc := &componentReleaseServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		err := svc.DeleteComponentRelease(testutil.AuthzContext(), "ns-1", "my-cr")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "componentrelease:delete", "componentrelease", "my-cr", crHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetComponentRelease", mock.Anything, "ns-1", "my-cr").Return(fetched, nil)
		svc := &componentReleaseServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		err := svc.DeleteComponentRelease(testutil.AuthzContext(), "ns-1", "my-cr")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("fetch error", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		fetchErr := errors.New("not found")
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetComponentRelease", mock.Anything, "ns-1", "my-cr").Return(nil, fetchErr)
		svc := &componentReleaseServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		err := svc.DeleteComponentRelease(testutil.AuthzContext(), "ns-1", "my-cr")
		require.ErrorIs(t, err, fetchErr)
		require.Empty(t, pdp.Captured, "authz should not be called when fetch fails")
	})
}
