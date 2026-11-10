// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointtype

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointtype/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const testNamespace = "ns-a"

func newRTAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &endpointTypeServiceWithAuthz{
		internal: internal,
		authz:    testutil.NewTestAuthzChecker(pdp),
	}
}

func TestCreateEndpointType_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.EndpointType{ObjectMeta: metav1.ObjectMeta{Name: "my-rt"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateEndpointType", mock.Anything, testNamespace, resource).Return(resource, nil)
		svc := newRTAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateEndpointType(testutil.AuthzContext(), testNamespace, resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "endpointtype:create", "endpointtype", "my-rt", authzcore.ResourceHierarchy{Namespace: testNamespace})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newRTAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateEndpointType(testutil.AuthzContext(), testNamespace, resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateEndpointType_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.EndpointType{ObjectMeta: metav1.ObjectMeta{Name: "my-rt"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("UpdateEndpointType", mock.Anything, testNamespace, resource).Return(resource, nil)
		svc := newRTAuthzSvc(pdp, mockSvc)
		result, err := svc.UpdateEndpointType(testutil.AuthzContext(), testNamespace, resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "endpointtype:update", "endpointtype", "my-rt", authzcore.ResourceHierarchy{Namespace: testNamespace})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newRTAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateEndpointType(testutil.AuthzContext(), testNamespace, resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetEndpointType_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.EndpointType{ObjectMeta: metav1.ObjectMeta{Name: "my-rt"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpointType", mock.Anything, testNamespace, "my-rt").Return(resource, nil)
		svc := newRTAuthzSvc(pdp, mockSvc)
		result, err := svc.GetEndpointType(testutil.AuthzContext(), testNamespace, "my-rt")
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "endpointtype:view", "endpointtype", "my-rt", authzcore.ResourceHierarchy{Namespace: testNamespace})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newRTAuthzSvc(pdp, mockSvc)
		_, err := svc.GetEndpointType(testutil.AuthzContext(), testNamespace, "my-rt")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeleteEndpointType_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("DeleteEndpointType", mock.Anything, testNamespace, "my-rt").Return(nil)
		svc := newRTAuthzSvc(pdp, mockSvc)
		err := svc.DeleteEndpointType(testutil.AuthzContext(), testNamespace, "my-rt")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "endpointtype:delete", "endpointtype", "my-rt", authzcore.ResourceHierarchy{Namespace: testNamespace})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newRTAuthzSvc(pdp, mockSvc)
		err := svc.DeleteEndpointType(testutil.AuthzContext(), testNamespace, "my-rt")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetEndpointTypeSchema_AuthzCheck(t *testing.T) {
	schema := map[string]any{"type": "object"}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetEndpointTypeSchema", mock.Anything, testNamespace, "my-rt").Return(schema, nil)
		svc := newRTAuthzSvc(pdp, mockSvc)
		result, err := svc.GetEndpointTypeSchema(testutil.AuthzContext(), testNamespace, "my-rt")
		require.NoError(t, err)
		require.Equal(t, schema, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "endpointtype:view", "endpointtype", "my-rt", authzcore.ResourceHierarchy{Namespace: testNamespace})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newRTAuthzSvc(pdp, mockSvc)
		_, err := svc.GetEndpointTypeSchema(testutil.AuthzContext(), testNamespace, "my-rt")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestListEndpointTypes_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.EndpointType{
		{ObjectMeta: metav1.ObjectMeta{Name: "rt-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "rt-2"}},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListEndpointTypes", mock.Anything, testNamespace, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.EndpointType]{Items: items}, nil)
		svc := newRTAuthzSvc(pdp, mockSvc)
		result, err := svc.ListEndpointTypes(testutil.AuthzContext(), testNamespace, services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "endpointtype:view", "endpointtype", "rt-1", authzcore.ResourceHierarchy{Namespace: testNamespace})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "endpointtype:view", "endpointtype", "rt-2", authzcore.ResourceHierarchy{Namespace: testNamespace})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListEndpointTypes", mock.Anything, testNamespace, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.EndpointType]{Items: items}, nil)
		svc := newRTAuthzSvc(pdp, mockSvc)
		result, err := svc.ListEndpointTypes(testutil.AuthzContext(), testNamespace, services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
