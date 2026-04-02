// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityplane

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/observabilityplane/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newObservabilityPlaneAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &observabilityPlaneServiceWithAuthz{
		internal: internal,
		authz:    testutil.NewTestAuthzChecker(pdp),
	}
}

func testObservabilityPlane(ns, name string) *openchoreov1alpha1.ObservabilityPlane {
	return &openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}
}

func TestCreateObservabilityPlane_AuthzCheck(t *testing.T) {
	op := testObservabilityPlane("ns-1", "my-op")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateObservabilityPlane", mock.Anything, "ns-1", op).Return(op, nil)
		svc := newObservabilityPlaneAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateObservabilityPlane(testutil.AuthzContext(), "ns-1", op)
		require.NoError(t, err)
		require.Equal(t, op, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "observabilityplane:create", "observabilityplane", "my-op",
			authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newObservabilityPlaneAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateObservabilityPlane(testutil.AuthzContext(), "ns-1", op)
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("nil input", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newObservabilityPlaneAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateObservabilityPlane(testutil.AuthzContext(), "ns-1", nil)
		require.Error(t, err)
		require.Empty(t, pdp.Captured)
	})
}

func TestUpdateObservabilityPlane_AuthzCheck(t *testing.T) {
	op := testObservabilityPlane("ns-1", "my-op")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("UpdateObservabilityPlane", mock.Anything, "ns-1", op).Return(op, nil)
		svc := newObservabilityPlaneAuthzSvc(pdp, mockSvc)
		result, err := svc.UpdateObservabilityPlane(testutil.AuthzContext(), "ns-1", op)
		require.NoError(t, err)
		require.Equal(t, op, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "observabilityplane:update", "observabilityplane", "my-op",
			authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newObservabilityPlaneAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateObservabilityPlane(testutil.AuthzContext(), "ns-1", op)
		require.ErrorIs(t, err, services.ErrForbidden)
	})

	t.Run("nil input", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newObservabilityPlaneAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateObservabilityPlane(testutil.AuthzContext(), "ns-1", nil)
		require.Error(t, err)
		require.Empty(t, pdp.Captured)
	})
}

func TestGetObservabilityPlane_AuthzCheck(t *testing.T) {
	op := testObservabilityPlane("ns-1", "my-op")

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetObservabilityPlane", mock.Anything, "ns-1", "my-op").Return(op, nil)
		svc := newObservabilityPlaneAuthzSvc(pdp, mockSvc)
		result, err := svc.GetObservabilityPlane(testutil.AuthzContext(), "ns-1", "my-op")
		require.NoError(t, err)
		require.Equal(t, op, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "observabilityplane:view", "observabilityplane", "my-op",
			authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newObservabilityPlaneAuthzSvc(pdp, mockSvc)
		_, err := svc.GetObservabilityPlane(testutil.AuthzContext(), "ns-1", "my-op")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeleteObservabilityPlane_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("DeleteObservabilityPlane", mock.Anything, "ns-1", "my-op").Return(nil)
		svc := newObservabilityPlaneAuthzSvc(pdp, mockSvc)
		err := svc.DeleteObservabilityPlane(testutil.AuthzContext(), "ns-1", "my-op")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "observabilityplane:delete", "observabilityplane", "my-op",
			authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newObservabilityPlaneAuthzSvc(pdp, mockSvc)
		err := svc.DeleteObservabilityPlane(testutil.AuthzContext(), "ns-1", "my-op")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestListObservabilityPlanes_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.ObservabilityPlane{
		{ObjectMeta: metav1.ObjectMeta{Name: "op-1", Namespace: "ns-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "op-2", Namespace: "ns-1"}},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListObservabilityPlanes", mock.Anything, "ns-1", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ObservabilityPlane]{Items: items}, nil)
		svc := newObservabilityPlaneAuthzSvc(pdp, mockSvc)
		result, err := svc.ListObservabilityPlanes(testutil.AuthzContext(), "ns-1", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "observabilityplane:view", "observabilityplane", "op-1",
			authzcore.ResourceHierarchy{Namespace: "ns-1"})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "observabilityplane:view", "observabilityplane", "op-2",
			authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListObservabilityPlanes", mock.Anything, "ns-1", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ObservabilityPlane]{Items: items}, nil)
		svc := newObservabilityPlaneAuthzSvc(pdp, mockSvc)
		result, err := svc.ListObservabilityPlanes(testutil.AuthzContext(), "ns-1", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
