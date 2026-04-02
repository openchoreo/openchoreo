// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package namespace

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/namespace/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func TestNamespaceAuthz_CreateNamespace(t *testing.T) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-1"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateNamespace", mock.Anything, ns).Return(ns, nil)
		svc := &namespaceServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.CreateNamespace(testutil.AuthzContext(), ns)
		require.NoError(t, err)
		require.Equal(t, ns, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "namespace:create", "namespace", "ns-1", authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &namespaceServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.CreateNamespace(testutil.AuthzContext(), ns)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestNamespaceAuthz_UpdateNamespace(t *testing.T) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-1"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("UpdateNamespace", mock.Anything, ns).Return(ns, nil)
		svc := &namespaceServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.UpdateNamespace(testutil.AuthzContext(), ns)
		require.NoError(t, err)
		require.Equal(t, ns, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "namespace:update", "namespace", "ns-1", authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &namespaceServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.UpdateNamespace(testutil.AuthzContext(), ns)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestNamespaceAuthz_GetNamespace(t *testing.T) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-1"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetNamespace", mock.Anything, "ns-1").Return(ns, nil)
		svc := &namespaceServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.GetNamespace(testutil.AuthzContext(), "ns-1")
		require.NoError(t, err)
		require.Equal(t, ns, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "namespace:view", "namespace", "ns-1", authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &namespaceServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetNamespace(testutil.AuthzContext(), "ns-1")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestNamespaceAuthz_DeleteNamespace(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("DeleteNamespace", mock.Anything, "ns-1").Return(nil)
		svc := &namespaceServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		err := svc.DeleteNamespace(testutil.AuthzContext(), "ns-1")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "namespace:delete", "namespace", "ns-1", authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &namespaceServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		err := svc.DeleteNamespace(testutil.AuthzContext(), "ns-1")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestNamespaceAuthz_ListNamespaces(t *testing.T) {
	items := []corev1.Namespace{
		{ObjectMeta: metav1.ObjectMeta{Name: "ns-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "ns-2"}},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListNamespaces", mock.Anything, mock.Anything).Return(&services.ListResult[corev1.Namespace]{Items: items}, nil)
		svc := &namespaceServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.ListNamespaces(testutil.AuthzContext(), services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "namespace:view", "namespace", "ns-1", authzcore.ResourceHierarchy{Namespace: "ns-1"})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "namespace:view", "namespace", "ns-2", authzcore.ResourceHierarchy{Namespace: "ns-2"})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListNamespaces", mock.Anything, mock.Anything).Return(&services.ListResult[corev1.Namespace]{Items: items}, nil)
		svc := &namespaceServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.ListNamespaces(testutil.AuthzContext(), services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
