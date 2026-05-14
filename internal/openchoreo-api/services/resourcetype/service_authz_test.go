// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcetype

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcetype/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const testNamespace = "ns-a"

func newRTAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &resourceTypeServiceWithAuthz{
		internal: internal,
		authz:    testutil.NewTestAuthzChecker(pdp),
	}
}

func TestCreateResourceType_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ResourceType{ObjectMeta: metav1.ObjectMeta{Name: "my-rt"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateResourceType", mock.Anything, testNamespace, resource).Return(resource, nil)
		svc := newRTAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateResourceType(testutil.AuthzContext(), testNamespace, resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "resourcetype:create", "resourcetype", "my-rt", authzcore.ResourceHierarchy{Namespace: testNamespace})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newRTAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateResourceType(testutil.AuthzContext(), testNamespace, resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateResourceType_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ResourceType{ObjectMeta: metav1.ObjectMeta{Name: "my-rt"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("UpdateResourceType", mock.Anything, testNamespace, resource).Return(resource, nil)
		svc := newRTAuthzSvc(pdp, mockSvc)
		result, err := svc.UpdateResourceType(testutil.AuthzContext(), testNamespace, resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "resourcetype:update", "resourcetype", "my-rt", authzcore.ResourceHierarchy{Namespace: testNamespace})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newRTAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateResourceType(testutil.AuthzContext(), testNamespace, resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetResourceType_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ResourceType{ObjectMeta: metav1.ObjectMeta{Name: "my-rt"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResourceType", mock.Anything, testNamespace, "my-rt").Return(resource, nil)
		svc := newRTAuthzSvc(pdp, mockSvc)
		result, err := svc.GetResourceType(testutil.AuthzContext(), testNamespace, "my-rt")
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "resourcetype:view", "resourcetype", "my-rt", authzcore.ResourceHierarchy{Namespace: testNamespace})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newRTAuthzSvc(pdp, mockSvc)
		_, err := svc.GetResourceType(testutil.AuthzContext(), testNamespace, "my-rt")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeleteResourceType_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("DeleteResourceType", mock.Anything, testNamespace, "my-rt").Return(nil)
		svc := newRTAuthzSvc(pdp, mockSvc)
		err := svc.DeleteResourceType(testutil.AuthzContext(), testNamespace, "my-rt")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "resourcetype:delete", "resourcetype", "my-rt", authzcore.ResourceHierarchy{Namespace: testNamespace})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newRTAuthzSvc(pdp, mockSvc)
		err := svc.DeleteResourceType(testutil.AuthzContext(), testNamespace, "my-rt")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetResourceTypeSchema_AuthzCheck(t *testing.T) {
	schema := map[string]any{"type": "object"}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetResourceTypeSchema", mock.Anything, testNamespace, "my-rt").Return(schema, nil)
		svc := newRTAuthzSvc(pdp, mockSvc)
		result, err := svc.GetResourceTypeSchema(testutil.AuthzContext(), testNamespace, "my-rt")
		require.NoError(t, err)
		require.Equal(t, schema, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "resourcetype:view", "resourcetype", "my-rt", authzcore.ResourceHierarchy{Namespace: testNamespace})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newRTAuthzSvc(pdp, mockSvc)
		_, err := svc.GetResourceTypeSchema(testutil.AuthzContext(), testNamespace, "my-rt")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestListResourceTypes_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.ResourceType{
		{ObjectMeta: metav1.ObjectMeta{Name: "rt-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "rt-2"}},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListResourceTypes", mock.Anything, testNamespace, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ResourceType]{Items: items}, nil)
		svc := newRTAuthzSvc(pdp, mockSvc)
		result, err := svc.ListResourceTypes(testutil.AuthzContext(), testNamespace, services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "resourcetype:view", "resourcetype", "rt-1", authzcore.ResourceHierarchy{Namespace: testNamespace})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "resourcetype:view", "resourcetype", "rt-2", authzcore.ResourceHierarchy{Namespace: testNamespace})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListResourceTypes", mock.Anything, testNamespace, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ResourceType]{Items: items}, nil)
		svc := newRTAuthzSvc(pdp, mockSvc)
		result, err := svc.ListResourceTypes(testutil.AuthzContext(), testNamespace, services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
