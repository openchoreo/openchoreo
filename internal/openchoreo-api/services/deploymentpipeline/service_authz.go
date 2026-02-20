// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

import (
	"context"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	actionCreateDeploymentPipeline = "deploymentpipeline:create"
	actionUpdateDeploymentPipeline = "deploymentpipeline:update"
	actionViewDeploymentPipeline   = "deploymentpipeline:view"
	actionDeleteDeploymentPipeline = "deploymentpipeline:delete"

	resourceTypeDeploymentPipeline = "deploymentPipeline"
)

// deploymentPipelineServiceWithAuthz wraps a Service and adds authorization checks.
// Handlers should use this. Other services should use the unwrapped Service directly.
type deploymentPipelineServiceWithAuthz struct {
	internal Service
	authz    *services.AuthzChecker
}

var _ Service = (*deploymentPipelineServiceWithAuthz)(nil)

// NewServiceWithAuthz creates a deployment pipeline service with authorization checks.
func NewServiceWithAuthz(k8sClient client.Client, authzPDP authz.PDP, logger *slog.Logger) Service {
	return &deploymentPipelineServiceWithAuthz{
		internal: NewService(k8sClient, logger),
		authz:    services.NewAuthzChecker(authzPDP, logger),
	}
}

func (s *deploymentPipelineServiceWithAuthz) CreateDeploymentPipeline(ctx context.Context, namespaceName string, dp *openchoreov1alpha1.DeploymentPipeline) (*openchoreov1alpha1.DeploymentPipeline, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionCreateDeploymentPipeline,
		ResourceType: resourceTypeDeploymentPipeline,
		ResourceID:   dp.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.CreateDeploymentPipeline(ctx, namespaceName, dp)
}

func (s *deploymentPipelineServiceWithAuthz) UpdateDeploymentPipeline(ctx context.Context, namespaceName string, dp *openchoreov1alpha1.DeploymentPipeline) (*openchoreov1alpha1.DeploymentPipeline, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionUpdateDeploymentPipeline,
		ResourceType: resourceTypeDeploymentPipeline,
		ResourceID:   dp.Name,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.UpdateDeploymentPipeline(ctx, namespaceName, dp)
}

func (s *deploymentPipelineServiceWithAuthz) ListDeploymentPipelines(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.DeploymentPipeline], error) {
	return services.FilteredList(ctx, opts, s.authz,
		func(ctx context.Context, pageOpts services.ListOptions) (*services.ListResult[openchoreov1alpha1.DeploymentPipeline], error) {
			return s.internal.ListDeploymentPipelines(ctx, namespaceName, pageOpts)
		},
		func(dp openchoreov1alpha1.DeploymentPipeline) services.CheckRequest {
			return services.CheckRequest{
				Action:       actionViewDeploymentPipeline,
				ResourceType: resourceTypeDeploymentPipeline,
				ResourceID:   dp.Name,
				Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
			}
		},
	)
}

func (s *deploymentPipelineServiceWithAuthz) GetDeploymentPipeline(ctx context.Context, namespaceName, deploymentPipelineName string) (*openchoreov1alpha1.DeploymentPipeline, error) {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionViewDeploymentPipeline,
		ResourceType: resourceTypeDeploymentPipeline,
		ResourceID:   deploymentPipelineName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return nil, err
	}
	return s.internal.GetDeploymentPipeline(ctx, namespaceName, deploymentPipelineName)
}

func (s *deploymentPipelineServiceWithAuthz) DeleteDeploymentPipeline(ctx context.Context, namespaceName, deploymentPipelineName string) error {
	if err := s.authz.Check(ctx, services.CheckRequest{
		Action:       actionDeleteDeploymentPipeline,
		ResourceType: resourceTypeDeploymentPipeline,
		ResourceID:   deploymentPipelineName,
		Hierarchy:    authz.ResourceHierarchy{Namespace: namespaceName},
	}); err != nil {
		return err
	}
	return s.internal.DeleteDeploymentPipeline(ctx, namespaceName, deploymentPipelineName)
}
