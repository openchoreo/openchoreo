// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

import (
	"context"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// Service defines the deployment pipeline service interface.
type Service interface {
	CreateDeploymentPipeline(ctx context.Context, namespaceName string, dp *openchoreov1alpha1.DeploymentPipeline) (*openchoreov1alpha1.DeploymentPipeline, error)
	UpdateDeploymentPipeline(ctx context.Context, namespaceName string, dp *openchoreov1alpha1.DeploymentPipeline) (*openchoreov1alpha1.DeploymentPipeline, error)
	ListDeploymentPipelines(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.DeploymentPipeline], error)
	GetDeploymentPipeline(ctx context.Context, namespaceName, deploymentPipelineName string) (*openchoreov1alpha1.DeploymentPipeline, error)
	DeleteDeploymentPipeline(ctx context.Context, namespaceName, deploymentPipelineName string) error
}
