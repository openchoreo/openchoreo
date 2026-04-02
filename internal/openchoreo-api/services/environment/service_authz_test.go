// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/environment/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func TestEnvironmentAuthz_CreateEnvironment(t *testing.T) {
	env := &openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Name: "env-1", Namespace: "ns-1"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateEnvironment", mock.Anything, "ns-1", env).Return(env, nil)
		svc := &environmentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.CreateEnvironment(testutil.AuthzContext(), "ns-1", env)
		require.NoError(t, err)
		require.Equal(t, env, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "environment:create", "environment", "env-1", authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &environmentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.CreateEnvironment(testutil.AuthzContext(), "ns-1", env)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestEnvironmentAuthz_UpdateEnvironment(t *testing.T) {
	env := &openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Name: "env-1", Namespace: "ns-1"}}

	t.Run("nil input", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &environmentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.UpdateEnvironment(testutil.AuthzContext(), "ns-1", nil)
		require.ErrorIs(t, err, ErrEnvironmentNil)
		require.Empty(t, pdp.Captured, "authz should not be called on nil input")
	})

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("UpdateEnvironment", mock.Anything, "ns-1", env).Return(env, nil)
		svc := &environmentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.UpdateEnvironment(testutil.AuthzContext(), "ns-1", env)
		require.NoError(t, err)
		require.Equal(t, env, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "environment:update", "environment", "env-1", authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &environmentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.UpdateEnvironment(testutil.AuthzContext(), "ns-1", env)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestEnvironmentAuthz_GetEnvironment(t *testing.T) {
	env := &openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Name: "env-1", Namespace: "ns-1"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEnvironment", mock.Anything, "ns-1", "env-1").Return(env, nil)
		svc := &environmentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.GetEnvironment(testutil.AuthzContext(), "ns-1", "env-1")
		require.NoError(t, err)
		require.Equal(t, env, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "environment:view", "environment", "env-1", authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &environmentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetEnvironment(testutil.AuthzContext(), "ns-1", "env-1")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestEnvironmentAuthz_DeleteEnvironment(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("DeleteEnvironment", mock.Anything, "ns-1", "env-1").Return(nil)
		svc := &environmentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		err := svc.DeleteEnvironment(testutil.AuthzContext(), "ns-1", "env-1")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "environment:delete", "environment", "env-1", authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &environmentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		err := svc.DeleteEnvironment(testutil.AuthzContext(), "ns-1", "env-1")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestEnvironmentAuthz_ListEnvironments(t *testing.T) {
	items := []openchoreov1alpha1.Environment{
		{ObjectMeta: metav1.ObjectMeta{Name: "env-1", Namespace: "ns-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "env-2", Namespace: "ns-1"}},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListEnvironments", mock.Anything, "ns-1", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.Environment]{Items: items}, nil)
		svc := &environmentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.ListEnvironments(testutil.AuthzContext(), "ns-1", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "environment:view", "environment", "env-1", authzcore.ResourceHierarchy{Namespace: "ns-1"})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "environment:view", "environment", "env-2", authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListEnvironments", mock.Anything, "ns-1", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.Environment]{Items: items}, nil)
		svc := &environmentServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.ListEnvironments(testutil.AuthzContext(), "ns-1", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
