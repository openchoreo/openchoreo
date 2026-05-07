// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

func (h *MCPHandler) ListProjects(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.ProjectService.ListProjects(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("projects", result.Items, result.NextCursor, projectSummary), nil
}

func (h *MCPHandler) CreateProject(ctx context.Context, namespaceName string, req *gen.CreateProjectJSONRequestBody) (any, error) {
	annotations := map[string]string{}
	if req.Metadata.Annotations != nil {
		for key, value := range *req.Metadata.Annotations {
			annotations[key] = value
		}
	}

	deploymentPipelineRef := openchoreov1alpha1.DeploymentPipelineRef{
		Kind: openchoreov1alpha1.DeploymentPipelineRefKindDeploymentPipeline,
	}
	if req.Spec != nil && req.Spec.DeploymentPipelineRef != nil {
		deploymentPipelineRef.Name = req.Spec.DeploymentPipelineRef.Name
		if req.Spec.DeploymentPipelineRef.Kind != nil && *req.Spec.DeploymentPipelineRef.Kind != "" {
			deploymentPipelineRef.Kind = openchoreov1alpha1.DeploymentPipelineRefKind(*req.Spec.DeploymentPipelineRef.Kind)
		}
	}

	project := &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Metadata.Name,
			Namespace:   namespaceName,
			Annotations: annotations,
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: deploymentPipelineRef,
		},
	}

	if displayName, ok := project.Annotations[controller.AnnotationKeyDisplayName]; ok && displayName == "" {
		delete(project.Annotations, controller.AnnotationKeyDisplayName)
	}
	if description, ok := project.Annotations[controller.AnnotationKeyDescription]; ok && description == "" {
		delete(project.Annotations, controller.AnnotationKeyDescription)
	}

	created, err := h.services.ProjectService.CreateProject(ctx, namespaceName, project)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) DeleteProject(ctx context.Context, namespaceName, projectName string) (any, error) {
	if err := h.services.ProjectService.DeleteProject(ctx, namespaceName, projectName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":      projectName,
		"namespace": namespaceName,
		"action":    "deleted",
	}, nil
}
