// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componenttype

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componenttype/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newComponentTypeAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &componentTypeServiceWithAuthz{
		internal: internal,
		authz:    testutil.NewTestAuthzChecker(pdp),
	}
}

func testComponentType(ns, name string) *openchoreov1alpha1.ComponentType {
	return &openchoreov1alpha1.ComponentType{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}
}

func TestCreateComponentType_AuthzCheck(t *testing.T) {
	ct := testComponentType("ns-1", "my-ct")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateComponentType", mock.Anything, "ns-1", ct).Return(ct, nil)
		svc := newComponentTypeAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateComponentType(testutil.AuthzContext(), "ns-1", ct)
		require.NoError(t, err)
		require.Equal(t, ct, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "componenttype:create", "componenttype", "my-ct",
			authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newComponentTypeAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateComponentType(testutil.AuthzContext(), "ns-1", ct)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateComponentType_AuthzCheck(t *testing.T) {
	ct := testComponentType("ns-1", "my-ct")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("UpdateComponentType", mock.Anything, "ns-1", ct).Return(ct, nil)
		svc := newComponentTypeAuthzSvc(pdp, mockSvc)
		result, err := svc.UpdateComponentType(testutil.AuthzContext(), "ns-1", ct)
		require.NoError(t, err)
		require.Equal(t, ct, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "componenttype:update", "componenttype", "my-ct",
			authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newComponentTypeAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateComponentType(testutil.AuthzContext(), "ns-1", ct)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetComponentType_AuthzCheck(t *testing.T) {
	ct := testComponentType("ns-1", "my-ct")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetComponentType", mock.Anything, "ns-1", "my-ct").Return(ct, nil)
		svc := newComponentTypeAuthzSvc(pdp, mockSvc)
		result, err := svc.GetComponentType(testutil.AuthzContext(), "ns-1", "my-ct")
		require.NoError(t, err)
		require.Equal(t, ct, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "componenttype:view", "componenttype", "my-ct",
			authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newComponentTypeAuthzSvc(pdp, mockSvc)
		_, err := svc.GetComponentType(testutil.AuthzContext(), "ns-1", "my-ct")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeleteComponentType_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("DeleteComponentType", mock.Anything, "ns-1", "my-ct").Return(nil)
		svc := newComponentTypeAuthzSvc(pdp, mockSvc)
		err := svc.DeleteComponentType(testutil.AuthzContext(), "ns-1", "my-ct")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "componenttype:delete", "componenttype", "my-ct",
			authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newComponentTypeAuthzSvc(pdp, mockSvc)
		err := svc.DeleteComponentType(testutil.AuthzContext(), "ns-1", "my-ct")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestListComponentTypes_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.ComponentType{
		{ObjectMeta: metav1.ObjectMeta{Name: "ct-1", Namespace: "ns-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "ct-2", Namespace: "ns-1"}},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListComponentTypes", mock.Anything, "ns-1", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ComponentType]{Items: items}, nil)
		svc := newComponentTypeAuthzSvc(pdp, mockSvc)
		result, err := svc.ListComponentTypes(testutil.AuthzContext(), "ns-1", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "componenttype:view", "componenttype", "ct-1",
			authzcore.ResourceHierarchy{Namespace: "ns-1"})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "componenttype:view", "componenttype", "ct-2",
			authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListComponentTypes", mock.Anything, "ns-1", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ComponentType]{Items: items}, nil)
		svc := newComponentTypeAuthzSvc(pdp, mockSvc)
		result, err := svc.ListComponentTypes(testutil.AuthzContext(), "ns-1", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}

func TestGetComponentTypeSchema_AuthzCheck(t *testing.T) {
	schema := map[string]any{"type": "object"}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetComponentTypeSchema", mock.Anything, "ns-1", "my-ct").Return(schema, nil)
		svc := newComponentTypeAuthzSvc(pdp, mockSvc)
		result, err := svc.GetComponentTypeSchema(testutil.AuthzContext(), "ns-1", "my-ct")
		require.NoError(t, err)
		require.Equal(t, schema, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "componenttype:view", "componenttype", "my-ct",
			authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newComponentTypeAuthzSvc(pdp, mockSvc)
		_, err := svc.GetComponentTypeSchema(testutil.AuthzContext(), "ns-1", "my-ct")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}
