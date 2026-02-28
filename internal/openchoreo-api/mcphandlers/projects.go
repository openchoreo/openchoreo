// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

func (h *MCPHandler) ListProjects(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.ProjectService.ListProjects(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("projects", result.Items, result.NextCursor, projectSummary), nil
}

func (h *MCPHandler) CreateProject(ctx context.Context, namespaceName string, req *models.CreateProjectRequest) (any, error) {
	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Name,
			Namespace:   namespaceName,
			Annotations: make(map[string]string),
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: req.DeploymentPipeline,
		},
	}

	if req.DisplayName != "" {
		project.Annotations[controller.AnnotationKeyDisplayName] = req.DisplayName
	}
	if req.Description != "" {
		project.Annotations[controller.AnnotationKeyDescription] = req.Description
	}

	created, err := h.services.ProjectService.CreateProject(ctx, namespaceName, project)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}
