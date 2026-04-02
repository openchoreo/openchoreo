// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflow

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterworkflow/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func newCWFAuthzSvc(pdp *testutil.CapturingPDP, internal Service) Service {
	return &clusterWorkflowServiceWithAuthz{
		internal: internal,
		authz:    testutil.NewTestAuthzChecker(pdp),
	}
}

func TestCreateClusterWorkflow_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterWorkflow{ObjectMeta: metav1.ObjectMeta{Name: "my-cwf"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateClusterWorkflow", mock.Anything, resource).Return(resource, nil)
		svc := newCWFAuthzSvc(pdp, mockSvc)
		result, err := svc.CreateClusterWorkflow(testutil.AuthzContext(), resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterworkflow:create", "clusterWorkflow", "my-cwf", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCWFAuthzSvc(pdp, mockSvc)
		_, err := svc.CreateClusterWorkflow(testutil.AuthzContext(), resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestUpdateClusterWorkflow_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterWorkflow{ObjectMeta: metav1.ObjectMeta{Name: "my-cwf"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("UpdateClusterWorkflow", mock.Anything, resource).Return(resource, nil)
		svc := newCWFAuthzSvc(pdp, mockSvc)
		result, err := svc.UpdateClusterWorkflow(testutil.AuthzContext(), resource)
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterworkflow:update", "clusterWorkflow", "my-cwf", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCWFAuthzSvc(pdp, mockSvc)
		_, err := svc.UpdateClusterWorkflow(testutil.AuthzContext(), resource)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetClusterWorkflow_AuthzCheck(t *testing.T) {
	resource := &openchoreov1alpha1.ClusterWorkflow{ObjectMeta: metav1.ObjectMeta{Name: "my-cwf"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetClusterWorkflow", mock.Anything, "my-cwf").Return(resource, nil)
		svc := newCWFAuthzSvc(pdp, mockSvc)
		result, err := svc.GetClusterWorkflow(testutil.AuthzContext(), "my-cwf")
		require.NoError(t, err)
		require.Equal(t, resource, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterworkflow:view", "clusterWorkflow", "my-cwf", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCWFAuthzSvc(pdp, mockSvc)
		_, err := svc.GetClusterWorkflow(testutil.AuthzContext(), "my-cwf")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeleteClusterWorkflow_AuthzCheck(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("DeleteClusterWorkflow", mock.Anything, "my-cwf").Return(nil)
		svc := newCWFAuthzSvc(pdp, mockSvc)
		err := svc.DeleteClusterWorkflow(testutil.AuthzContext(), "my-cwf")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterworkflow:delete", "clusterWorkflow", "my-cwf", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCWFAuthzSvc(pdp, mockSvc)
		err := svc.DeleteClusterWorkflow(testutil.AuthzContext(), "my-cwf")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestGetClusterWorkflowSchema_AuthzCheck(t *testing.T) {
	schema := map[string]any{"type": "object"}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetClusterWorkflowSchema", mock.Anything, "my-cwf").Return(schema, nil)
		svc := newCWFAuthzSvc(pdp, mockSvc)
		result, err := svc.GetClusterWorkflowSchema(testutil.AuthzContext(), "my-cwf")
		require.NoError(t, err)
		require.Equal(t, schema, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterworkflow:view", "clusterWorkflow", "my-cwf", authzcore.ResourceHierarchy{})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := newCWFAuthzSvc(pdp, mockSvc)
		_, err := svc.GetClusterWorkflowSchema(testutil.AuthzContext(), "my-cwf")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestListClusterWorkflows_AuthzCheck(t *testing.T) {
	items := []openchoreov1alpha1.ClusterWorkflow{
		{ObjectMeta: metav1.ObjectMeta{Name: "cwf-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "cwf-2"}},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListClusterWorkflows", mock.Anything, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ClusterWorkflow]{Items: items}, nil)
		svc := newCWFAuthzSvc(pdp, mockSvc)
		result, err := svc.ListClusterWorkflows(testutil.AuthzContext(), services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "clusterworkflow:view", "clusterWorkflow", "cwf-1", authzcore.ResourceHierarchy{})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "clusterworkflow:view", "clusterWorkflow", "cwf-2", authzcore.ResourceHierarchy{})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListClusterWorkflows", mock.Anything, mock.Anything).Return(&services.ListResult[openchoreov1alpha1.ClusterWorkflow]{Items: items}, nil)
		svc := newCWFAuthzSvc(pdp, mockSvc)
		result, err := svc.ListClusterWorkflows(testutil.AuthzContext(), services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
