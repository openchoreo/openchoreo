// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func testRB() *openchoreov1alpha1.ReleaseBinding {
	return &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "my-rb", Namespace: "ns-1"},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ReleaseBindingOwner{ProjectName: "my-proj", ComponentName: "my-comp"},
			Environment: "dev",
		},
	}
}

var rbHierarchy = authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-proj", Component: "my-comp"}

// --- CreateReleaseBinding ---

func TestCreateReleaseBinding_AuthzCheck(t *testing.T) {
	rb := testRB()

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateReleaseBinding", mock.Anything, "ns-1", rb).Return(rb, nil)
		svc := &releaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.CreateReleaseBinding(testutil.AuthzContext(), "ns-1", rb)
		require.NoError(t, err)
		require.Equal(t, rb, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "releasebinding:create", "releasebinding", "my-rb", rbHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &releaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.CreateReleaseBinding(testutil.AuthzContext(), "ns-1", rb)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

// --- UpdateReleaseBinding ---

func TestUpdateReleaseBinding_AuthzCheck(t *testing.T) {
	rb := testRB()

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(rb, nil)
		mockSvc.On("UpdateReleaseBinding", mock.Anything, "ns-1", rb).Return(rb, nil)
		svc := &releaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.UpdateReleaseBinding(testutil.AuthzContext(), "ns-1", rb)
		require.NoError(t, err)
		require.Equal(t, rb, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "releasebinding:update", "releasebinding", "my-rb", rbHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(rb, nil)
		svc := &releaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.UpdateReleaseBinding(testutil.AuthzContext(), "ns-1", rb)
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("fetch error", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		fetchErr := errors.New("not found")
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(nil, fetchErr)
		svc := &releaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.UpdateReleaseBinding(testutil.AuthzContext(), "ns-1", rb)
		require.ErrorIs(t, err, fetchErr)
		require.Empty(t, pdp.Captured, "authz should not be called when fetch fails")
	})
}

// --- ListReleaseBindings ---

func TestListReleaseBindings_AuthzCheck(t *testing.T) {
	rbs := []openchoreov1alpha1.ReleaseBinding{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "rb-1", Namespace: "ns-1"},
			Spec: openchoreov1alpha1.ReleaseBindingSpec{
				Owner: openchoreov1alpha1.ReleaseBindingOwner{ProjectName: "my-proj", ComponentName: "my-comp"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "rb-2", Namespace: "ns-1"},
			Spec: openchoreov1alpha1.ReleaseBindingSpec{
				Owner: openchoreov1alpha1.ReleaseBindingOwner{ProjectName: "my-proj", ComponentName: "my-comp"},
			},
		},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListReleaseBindings", mock.Anything, "ns-1", "my-comp", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ReleaseBinding]{Items: rbs}, nil)
		svc := &releaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.ListReleaseBindings(testutil.AuthzContext(), "ns-1", "my-comp", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "releasebinding:view", "releasebinding", "rb-1",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-proj", Component: "my-comp"})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "releasebinding:view", "releasebinding", "rb-2",
			authzcore.ResourceHierarchy{Namespace: "ns-1", Project: "my-proj", Component: "my-comp"})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListReleaseBindings", mock.Anything, "ns-1", "my-comp", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ReleaseBinding]{Items: rbs}, nil)
		svc := &releaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.ListReleaseBindings(testutil.AuthzContext(), "ns-1", "my-comp", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}

// --- GetReleaseBinding ---

func TestGetReleaseBinding_AuthzCheck(t *testing.T) {
	fetched := testRB()

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(fetched, nil)
		svc := &releaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.GetReleaseBinding(testutil.AuthzContext(), "ns-1", "my-rb")
		require.NoError(t, err)
		require.Equal(t, fetched, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "releasebinding:view", "releasebinding", "my-rb", rbHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(fetched, nil)
		svc := &releaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetReleaseBinding(testutil.AuthzContext(), "ns-1", "my-rb")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("fetch error", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		fetchErr := errors.New("not found")
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(nil, fetchErr)
		svc := &releaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetReleaseBinding(testutil.AuthzContext(), "ns-1", "my-rb")
		require.ErrorIs(t, err, fetchErr)
		require.Empty(t, pdp.Captured, "authz should not be called when fetch fails")
	})
}

// --- DeleteReleaseBinding ---

func TestDeleteReleaseBinding_AuthzCheck(t *testing.T) {
	fetched := testRB()

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(fetched, nil)
		mockSvc.On("DeleteReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(nil)
		svc := &releaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		err := svc.DeleteReleaseBinding(testutil.AuthzContext(), "ns-1", "my-rb")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "releasebinding:delete", "releasebinding", "my-rb", rbHierarchy)
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(fetched, nil)
		svc := &releaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		err := svc.DeleteReleaseBinding(testutil.AuthzContext(), "ns-1", "my-rb")
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("fetch error", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		fetchErr := errors.New("not found")
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetReleaseBinding", mock.Anything, "ns-1", "my-rb").Return(nil, fetchErr)
		svc := &releaseBindingServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		err := svc.DeleteReleaseBinding(testutil.AuthzContext(), "ns-1", "my-rb")
		require.ErrorIs(t, err, fetchErr)
		require.Empty(t, pdp.Captured, "authz should not be called when fetch fails")
	})
}
