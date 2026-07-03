// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentreleasebinding

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componentreleasebinding/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func testRB() *openchoreov1alpha1.ComponentReleaseBinding {
	return &openchoreov1alpha1.ComponentReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "my-rb", Namespace: "ns-1"},
		Spec: openchoreov1alpha1.ComponentReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ComponentReleaseBindingOwner{ProjectName: "my-proj", ComponentName: "my-comp"},
			Environment: "dev",
		},
	}
}

var rbHierarchy = authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-proj", Component: "my-comp"}

// --- CreateComponentReleaseBinding ---

func TestCreateComponentReleaseBinding_AuthzCheck(t *testing.T) {
	rb := testRB()

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateComponentReleaseBinding", mock.Anything, "ns-1", rb).Return(rb, nil)
		svc := &componentReleaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.CreateComponentReleaseBinding(testutil.AuthzContext(), "ns-1", rb)
		require.NoError(t, err)
		require.Equal(t, rb, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "componentreleasebinding:create", "componentreleasebinding", "my-rb", rbHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &componentReleaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.CreateComponentReleaseBinding(testutil.AuthzContext(), "ns-1", rb)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

// --- UpdateComponentReleaseBinding ---

func TestUpdateComponentReleaseBinding_AuthzCheck(t *testing.T) {
	rb := testRB()

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetComponentReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(rb, nil)
		mockSvc.On("UpdateComponentReleaseBinding", mock.Anything, "ns-1", rb).Return(rb, nil)
		svc := &componentReleaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.UpdateComponentReleaseBinding(testutil.AuthzContext(), "ns-1", rb)
		require.NoError(t, err)
		require.Equal(t, rb, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "componentreleasebinding:update", "componentreleasebinding", "my-rb", rbHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetComponentReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(rb, nil)
		svc := &componentReleaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.UpdateComponentReleaseBinding(testutil.AuthzContext(), "ns-1", rb)
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("fetch error", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		fetchErr := errors.New("not found")
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetComponentReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(nil, fetchErr)
		svc := &componentReleaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.UpdateComponentReleaseBinding(testutil.AuthzContext(), "ns-1", rb)
		require.ErrorIs(t, err, fetchErr)
		require.Empty(t, pdp.Captured, "authz should not be called when fetch fails")
	})
}

// --- ListComponentComponentReleaseBindings ---

func TestListComponentComponentReleaseBindings_AuthzCheck(t *testing.T) {
	rbs := []openchoreov1alpha1.ComponentReleaseBinding{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "rb-1", Namespace: "ns-1"},
			Spec: openchoreov1alpha1.ComponentReleaseBindingSpec{
				Owner: openchoreov1alpha1.ComponentReleaseBindingOwner{ProjectName: "my-proj", ComponentName: "my-comp"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "rb-2", Namespace: "ns-1"},
			Spec: openchoreov1alpha1.ComponentReleaseBindingSpec{
				Owner: openchoreov1alpha1.ComponentReleaseBindingOwner{ProjectName: "my-proj", ComponentName: "my-comp"},
			},
		},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListComponentComponentReleaseBindings", mock.Anything, "ns-1", "my-comp", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ComponentReleaseBinding]{Items: rbs}, nil)
		svc := &componentReleaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.ListComponentComponentReleaseBindings(testutil.AuthzContext(), "ns-1", "my-comp", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "componentreleasebinding:view", "componentreleasebinding", "rb-1",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-proj", Component: "my-comp"})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "componentreleasebinding:view", "componentreleasebinding", "rb-2",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-proj", Component: "my-comp"})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListComponentComponentReleaseBindings", mock.Anything, "ns-1", "my-comp", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ComponentReleaseBinding]{Items: rbs}, nil)
		svc := &componentReleaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.ListComponentComponentReleaseBindings(testutil.AuthzContext(), "ns-1", "my-comp", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}

// --- GetComponentReleaseBinding ---

func TestGetComponentReleaseBinding_AuthzCheck(t *testing.T) {
	fetched := testRB()

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetComponentReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(fetched, nil)
		svc := &componentReleaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.GetComponentReleaseBinding(testutil.AuthzContext(), "ns-1", "my-rb")
		require.NoError(t, err)
		require.Equal(t, fetched, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "componentreleasebinding:view", "componentreleasebinding", "my-rb", rbHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetComponentReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(fetched, nil)
		svc := &componentReleaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetComponentReleaseBinding(testutil.AuthzContext(), "ns-1", "my-rb")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("fetch error", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		fetchErr := errors.New("not found")
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetComponentReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(nil, fetchErr)
		svc := &componentReleaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetComponentReleaseBinding(testutil.AuthzContext(), "ns-1", "my-rb")
		require.ErrorIs(t, err, fetchErr)
		require.Empty(t, pdp.Captured, "authz should not be called when fetch fails")
	})
}

// --- DeleteComponentReleaseBinding ---

func TestDeleteComponentReleaseBinding_AuthzCheck(t *testing.T) {
	fetched := testRB()

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetComponentReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(fetched, nil)
		mockSvc.On("DeleteComponentReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(nil)
		svc := &componentReleaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		err := svc.DeleteComponentReleaseBinding(testutil.AuthzContext(), "ns-1", "my-rb")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "componentreleasebinding:delete", "componentreleasebinding", "my-rb", rbHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetComponentReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(fetched, nil)
		svc := &componentReleaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		err := svc.DeleteComponentReleaseBinding(testutil.AuthzContext(), "ns-1", "my-rb")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("fetch error", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		fetchErr := errors.New("not found")
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetComponentReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(nil, fetchErr)
		svc := &componentReleaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		err := svc.DeleteComponentReleaseBinding(testutil.AuthzContext(), "ns-1", "my-rb")
		require.ErrorIs(t, err, fetchErr)
		require.Empty(t, pdp.Captured, "authz should not be called when fetch fails")
	})
}
