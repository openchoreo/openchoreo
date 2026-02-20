// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

import (
	"context"
	"fmt"
	"log/slog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// deploymentPipelineService handles deployment pipeline business logic without authorization checks.
// Other services within this layer should use this directly to avoid double authz.
type deploymentPipelineService struct {
	k8sClient client.Client
	logger    *slog.Logger
}

var _ Service = (*deploymentPipelineService)(nil)

// NewService creates a new deployment pipeline service without authorization.
func NewService(k8sClient client.Client, logger *slog.Logger) Service {
	return &deploymentPipelineService{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

func (s *deploymentPipelineService) CreateDeploymentPipeline(ctx context.Context, namespaceName string, dp *openchoreov1alpha1.DeploymentPipeline) (*openchoreov1alpha1.DeploymentPipeline, error) {
	if dp == nil {
		return nil, fmt.Errorf("deployment pipeline cannot be nil")
	}

	s.logger.Debug("Creating deployment pipeline", "namespace", namespaceName, "deploymentPipeline", dp.Name)

	exists, err := s.deploymentPipelineExists(ctx, namespaceName, dp.Name)
	if err != nil {
		s.logger.Error("Failed to check deployment pipeline existence", "error", err)
		return nil, fmt.Errorf("failed to check deployment pipeline existence: %w", err)
	}
	if exists {
		s.logger.Warn("Deployment pipeline already exists", "namespace", namespaceName, "deploymentPipeline", dp.Name)
		return nil, ErrDeploymentPipelineAlreadyExists
	}

	// Set defaults
	dp.TypeMeta = metav1.TypeMeta{
		Kind:       "DeploymentPipeline",
		APIVersion: "openchoreo.dev/v1alpha1",
	}
	dp.Namespace = namespaceName
	if dp.Labels == nil {
		dp.Labels = make(map[string]string)
	}
	dp.Labels[labels.LabelKeyNamespaceName] = namespaceName
	dp.Labels[labels.LabelKeyName] = dp.Name

	if err := s.k8sClient.Create(ctx, dp); err != nil {
		if apierrors.IsAlreadyExists(err) {
			s.logger.Warn("Deployment pipeline already exists", "namespace", namespaceName, "deploymentPipeline", dp.Name)
			return nil, ErrDeploymentPipelineAlreadyExists
		}
		s.logger.Error("Failed to create deployment pipeline CR", "error", err)
		return nil, fmt.Errorf("failed to create deployment pipeline: %w", err)
	}

	s.logger.Debug("Deployment pipeline created successfully", "namespace", namespaceName, "deploymentPipeline", dp.Name)
	return dp, nil
}

func (s *deploymentPipelineService) UpdateDeploymentPipeline(ctx context.Context, namespaceName string, dp *openchoreov1alpha1.DeploymentPipeline) (*openchoreov1alpha1.DeploymentPipeline, error) {
	if dp == nil {
		return nil, fmt.Errorf("deployment pipeline cannot be nil")
	}

	s.logger.Debug("Updating deployment pipeline", "namespace", namespaceName, "deploymentPipeline", dp.Name)

	existing := &openchoreov1alpha1.DeploymentPipeline{}
	if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: dp.Name, Namespace: namespaceName}, existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Deployment pipeline not found", "namespace", namespaceName, "deploymentPipeline", dp.Name)
			return nil, ErrDeploymentPipelineNotFound
		}
		s.logger.Error("Failed to get deployment pipeline", "error", err)
		return nil, fmt.Errorf("failed to get deployment pipeline: %w", err)
	}

	// Apply incoming spec directly from the request body, preserving server-managed fields
	dp.ResourceVersion = existing.ResourceVersion
	dp.Namespace = namespaceName

	if err := s.k8sClient.Update(ctx, dp); err != nil {
		s.logger.Error("Failed to update deployment pipeline CR", "error", err)
		return nil, fmt.Errorf("failed to update deployment pipeline: %w", err)
	}

	s.logger.Debug("Deployment pipeline updated successfully", "namespace", namespaceName, "deploymentPipeline", dp.Name)
	return dp, nil
}

func (s *deploymentPipelineService) ListDeploymentPipelines(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[openchoreov1alpha1.DeploymentPipeline], error) {
	s.logger.Debug("Listing deployment pipelines", "namespace", namespaceName, "limit", opts.Limit, "cursor", opts.Cursor)

	listOpts := []client.ListOption{
		client.InNamespace(namespaceName),
	}
	if opts.Limit > 0 {
		listOpts = append(listOpts, client.Limit(int64(opts.Limit)))
	}
	if opts.Cursor != "" {
		listOpts = append(listOpts, client.Continue(opts.Cursor))
	}

	var dpList openchoreov1alpha1.DeploymentPipelineList
	if err := s.k8sClient.List(ctx, &dpList, listOpts...); err != nil {
		s.logger.Error("Failed to list deployment pipelines", "error", err)
		return nil, fmt.Errorf("failed to list deployment pipelines: %w", err)
	}

	result := &services.ListResult[openchoreov1alpha1.DeploymentPipeline]{
		Items:      dpList.Items,
		NextCursor: dpList.Continue,
	}
	if dpList.RemainingItemCount != nil {
		remaining := *dpList.RemainingItemCount
		result.RemainingCount = &remaining
	}

	s.logger.Debug("Listed deployment pipelines", "namespace", namespaceName, "count", len(dpList.Items))
	return result, nil
}

func (s *deploymentPipelineService) GetDeploymentPipeline(ctx context.Context, namespaceName, deploymentPipelineName string) (*openchoreov1alpha1.DeploymentPipeline, error) {
	s.logger.Debug("Getting deployment pipeline", "namespace", namespaceName, "deploymentPipeline", deploymentPipelineName)

	dp := &openchoreov1alpha1.DeploymentPipeline{}
	key := client.ObjectKey{
		Name:      deploymentPipelineName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, dp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Deployment pipeline not found", "namespace", namespaceName, "deploymentPipeline", deploymentPipelineName)
			return nil, ErrDeploymentPipelineNotFound
		}
		s.logger.Error("Failed to get deployment pipeline", "error", err)
		return nil, fmt.Errorf("failed to get deployment pipeline: %w", err)
	}

	return dp, nil
}

func (s *deploymentPipelineService) DeleteDeploymentPipeline(ctx context.Context, namespaceName, deploymentPipelineName string) error {
	s.logger.Debug("Deleting deployment pipeline", "namespace", namespaceName, "deploymentPipeline", deploymentPipelineName)

	dp := &openchoreov1alpha1.DeploymentPipeline{}
	key := client.ObjectKey{
		Name:      deploymentPipelineName,
		Namespace: namespaceName,
	}

	if err := s.k8sClient.Get(ctx, key, dp); err != nil {
		if client.IgnoreNotFound(err) == nil {
			s.logger.Warn("Deployment pipeline not found", "namespace", namespaceName, "deploymentPipeline", deploymentPipelineName)
			return ErrDeploymentPipelineNotFound
		}
		s.logger.Error("Failed to get deployment pipeline", "error", err)
		return fmt.Errorf("failed to get deployment pipeline: %w", err)
	}

	if err := s.k8sClient.Delete(ctx, dp); err != nil {
		s.logger.Error("Failed to delete deployment pipeline CR", "error", err)
		return fmt.Errorf("failed to delete deployment pipeline: %w", err)
	}

	s.logger.Debug("Deployment pipeline deleted successfully", "namespace", namespaceName, "deploymentPipeline", deploymentPipelineName)
	return nil
}

func (s *deploymentPipelineService) deploymentPipelineExists(ctx context.Context, namespaceName, deploymentPipelineName string) (bool, error) {
	dp := &openchoreov1alpha1.DeploymentPipeline{}
	key := client.ObjectKey{
		Name:      deploymentPipelineName,
		Namespace: namespaceName,
	}

	err := s.k8sClient.Get(ctx, key, dp)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return false, nil
		}
		return false, fmt.Errorf("checking existence of deployment pipeline %s/%s: %w", namespaceName, deploymentPipelineName, err)
	}
	return true, nil
}
