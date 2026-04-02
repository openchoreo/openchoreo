// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/deploymentpipeline/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

func TestDeploymentPipelineAuthz_CreateDeploymentPipeline(t *testing.T) {
	dp := &openchoreov1alpha1.DeploymentPipeline{ObjectMeta: metav1.ObjectMeta{Name: "dp-1", Namespace: "ns-1"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("CreateDeploymentPipeline", mock.Anything, "ns-1", dp).Return(dp, nil)
		svc := &deploymentPipelineServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.CreateDeploymentPipeline(testutil.AuthzContext(), "ns-1", dp)
		require.NoError(t, err)
		require.Equal(t, dp, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "deploymentpipeline:create", "deploymentPipeline", "dp-1", authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &deploymentPipelineServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.CreateDeploymentPipeline(testutil.AuthzContext(), "ns-1", dp)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeploymentPipelineAuthz_UpdateDeploymentPipeline(t *testing.T) {
	dp := &openchoreov1alpha1.DeploymentPipeline{ObjectMeta: metav1.ObjectMeta{Name: "dp-1", Namespace: "ns-1"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("UpdateDeploymentPipeline", mock.Anything, "ns-1", dp).Return(dp, nil)
		svc := &deploymentPipelineServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.UpdateDeploymentPipeline(testutil.AuthzContext(), "ns-1", dp)
		require.NoError(t, err)
		require.Equal(t, dp, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "deploymentpipeline:update", "deploymentPipeline", "dp-1", authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &deploymentPipelineServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.UpdateDeploymentPipeline(testutil.AuthzContext(), "ns-1", dp)
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeploymentPipelineAuthz_GetDeploymentPipeline(t *testing.T) {
	dp := &openchoreov1alpha1.DeploymentPipeline{ObjectMeta: metav1.ObjectMeta{Name: "dp-1", Namespace: "ns-1"}}

	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("GetDeploymentPipeline", mock.Anything, "ns-1", "dp-1").Return(dp, nil)
		svc := &deploymentPipelineServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.GetDeploymentPipeline(testutil.AuthzContext(), "ns-1", "dp-1")
		require.NoError(t, err)
		require.Equal(t, dp, result)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "deploymentpipeline:view", "deploymentPipeline", "dp-1", authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &deploymentPipelineServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		_, err := svc.GetDeploymentPipeline(testutil.AuthzContext(), "ns-1", "dp-1")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeploymentPipelineAuthz_DeleteDeploymentPipeline(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("DeleteDeploymentPipeline", mock.Anything, "ns-1", "dp-1").Return(nil)
		svc := &deploymentPipelineServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		err := svc.DeleteDeploymentPipeline(testutil.AuthzContext(), "ns-1", "dp-1")
		require.NoError(t, err)
		require.Len(t, pdp.Captured, 1)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "deploymentpipeline:delete", "deploymentPipeline", "dp-1", authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("denied", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		svc := &deploymentPipelineServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		err := svc.DeleteDeploymentPipeline(testutil.AuthzContext(), "ns-1", "dp-1")
		require.ErrorIs(t, err, services.ErrForbidden)
	})
}

func TestDeploymentPipelineAuthz_ListDeploymentPipelines(t *testing.T) {
	items := []openchoreov1alpha1.DeploymentPipeline{
		{ObjectMeta: metav1.ObjectMeta{Name: "dp-1", Namespace: "ns-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "dp-2", Namespace: "ns-1"}},
	}

	t.Run("all allowed — per-item check request fields", func(t *testing.T) {
		pdp := testutil.AllowPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListDeploymentPipelines", mock.Anything, "ns-1", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.DeploymentPipeline]{Items: items}, nil)
		svc := &deploymentPipelineServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.ListDeploymentPipelines(testutil.AuthzContext(), "ns-1", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		require.Len(t, pdp.Captured, 2)
		testutil.RequireEvalRequest(t, pdp.Captured[0], "deploymentpipeline:view", "deploymentPipeline", "dp-1", authzcore.ResourceHierarchy{Namespace: "ns-1"})
		testutil.RequireEvalRequest(t, pdp.Captured[1], "deploymentpipeline:view", "deploymentPipeline", "dp-2", authzcore.ResourceHierarchy{Namespace: "ns-1"})
	})

	t.Run("all denied — empty result", func(t *testing.T) {
		pdp := testutil.DenyPDP()
		mockSvc := mocks.NewMockService(t)
		mockSvc.On("ListDeploymentPipelines", mock.Anything, "ns-1", mock.Anything).Return(&services.ListResult[openchoreov1alpha1.DeploymentPipeline]{Items: items}, nil)
		svc := &deploymentPipelineServiceWithAuthz{
			internal: mockSvc,
			authz:    testutil.NewTestAuthzChecker(pdp),
		}
		result, err := svc.ListDeploymentPipelines(testutil.AuthzContext(), "ns-1", services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, result.Items)
	})
}
