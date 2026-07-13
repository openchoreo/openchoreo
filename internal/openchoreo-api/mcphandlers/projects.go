// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

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

// CreateProject creates a Project and, unless skipBindings is set, one
// ProjectReleaseBinding per environment in the project's deployment pipeline.
//
// Bindings are best-effort: the project is never rolled back and a per-binding
// failure does not fail the call. Failures are reported in the result so the
// caller can retry them with create_project_release_binding.
func (h *MCPHandler) CreateProject(
	ctx context.Context, namespaceName string, req *gen.CreateProjectJSONRequestBody, skipBindings bool,
) (any, error) {
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

	if req.Spec != nil && req.Spec.Type != nil {
		project.Spec.Type = openchoreov1alpha1.ProjectTypeRef{
			Name: req.Spec.Type.Name,
		}
		if req.Spec.Type.Kind != nil {
			project.Spec.Type.Kind = openchoreov1alpha1.ProjectTypeRefKind(*req.Spec.Type.Kind)
		}
	}

	if req.Spec != nil && req.Spec.Parameters != nil {
		paramsBytes, err := json.Marshal(*req.Spec.Parameters)
		if err != nil {
			return nil, fmt.Errorf("marshal parameters: %w", err)
		}
		project.Spec.Parameters = &runtime.RawExtension{Raw: paramsBytes}
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

	result := mutationResult(created, "created")
	if skipBindings {
		result["releaseBindingsNote"] = "Skipped by skip_bindings. The project is not bound to any " +
			"environment until a ProjectReleaseBinding is created (create_project_release_binding)."
		return result, nil
	}
	maps.Copy(result, h.createDefaultProjectBindings(ctx, namespaceName, created))
	return result, nil
}

// createDefaultProjectBindings creates one unpinned ProjectReleaseBinding per
// environment in the project's deployment pipeline and returns the result fields
// describing what happened. The release pin is deliberately left empty: the first
// ProjectRelease name embeds a controller-computed hash and is not knowable here,
// so the Project controller seeds it once the release lands.
func (h *MCPHandler) createDefaultProjectBindings(
	ctx context.Context, namespaceName string, project *openchoreov1alpha1.Project,
) map[string]any {
	pipelineName := project.Spec.DeploymentPipelineRef.Name

	pipeline, err := h.services.DeploymentPipelineService.GetDeploymentPipeline(ctx, namespaceName, pipelineName)
	if err != nil {
		return map[string]any{
			"releaseBindingsNote": fmt.Sprintf(
				"No ProjectReleaseBindings were created: failed to resolve deployment pipeline %q: %v. "+
					"Create them manually with create_project_release_binding.", pipelineName, err),
		}
	}

	environments := expandPipelineEnvironments(pipeline)
	if len(environments) == 0 {
		return map[string]any{
			"releaseBindingsNote": fmt.Sprintf(
				"No ProjectReleaseBindings were created: deployment pipeline %q defines no environments.",
				pipelineName),
		}
	}

	createdBindings := make([]string, 0, len(environments))
	var failures []map[string]any
	for _, env := range environments {
		name := fmt.Sprintf("%s-%s", project.Name, env)
		binding := &openchoreov1alpha1.ProjectReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespaceName,
			},
			Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ProjectReleaseBindingOwner{ProjectName: project.Name},
				Environment: env,
			},
		}
		if _, bErr := h.services.ProjectReleaseBindingService.CreateProjectReleaseBinding(
			ctx, namespaceName, binding,
		); bErr != nil {
			failures = append(failures, map[string]any{
				"name":        name,
				"environment": env,
				"error":       bErr.Error(),
			})
			continue
		}
		createdBindings = append(createdBindings, name)
	}

	out := map[string]any{"releaseBindings": createdBindings}
	if len(failures) > 0 {
		out["releaseBindingFailures"] = failures
		out["releaseBindingsNote"] = "Some ProjectReleaseBindings could not be created. The project was still " +
			"created; retry the failed environments with create_project_release_binding."
	}
	return out
}

// expandPipelineEnvironments returns the distinct environments referenced by the
// pipeline's promotion paths, in promotion order. Mirrors the expansion occ uses
// when scaffolding a project (internal/occ/cmd/utils.ExpandEnvironments), which
// is typed against the generated API model rather than the CRD.
func expandPipelineEnvironments(pipeline *openchoreov1alpha1.DeploymentPipeline) []string {
	if pipeline == nil {
		return nil
	}
	seen := make(map[string]bool)
	var envs []string
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			envs = append(envs, name)
		}
	}
	for _, path := range pipeline.Spec.PromotionPaths {
		add(path.SourceEnvironmentRef.Name)
		for _, target := range path.TargetEnvironmentRefs {
			add(target.Name)
		}
	}
	return envs
}

func (h *MCPHandler) UpdateProject(
	ctx context.Context,
	namespaceName, projectName string, req *gen.PatchProjectRequest,
) (any, error) {
	if req == nil {
		req = &gen.PatchProjectRequest{}
	}

	project, err := h.services.ProjectService.GetProject(ctx, namespaceName, projectName)
	if err != nil {
		return nil, fmt.Errorf("UpdateProject: GetProject namespace=%s project=%s: %w", namespaceName, projectName, err)
	}

	updatedProject := project.DeepCopy()
	if updatedProject.Annotations == nil {
		updatedProject.Annotations = map[string]string{}
	}
	if req.DisplayName != nil && *req.DisplayName != "" {
		updatedProject.Annotations[controller.AnnotationKeyDisplayName] = *req.DisplayName
	}
	if req.Description != nil && *req.Description != "" {
		updatedProject.Annotations[controller.AnnotationKeyDescription] = *req.Description
	}

	deploymentPipeline := ""
	if req.DeploymentPipeline != nil && *req.DeploymentPipeline != "" {
		deploymentPipeline = *req.DeploymentPipeline
		updatedProject.Spec.DeploymentPipelineRef = openchoreov1alpha1.DeploymentPipelineRef{
			Kind: openchoreov1alpha1.DeploymentPipelineRefKindDeploymentPipeline,
			Name: deploymentPipeline,
		}
	}

	updated, err := h.services.ProjectService.UpdateProject(ctx, namespaceName, updatedProject)
	if err != nil {
		return nil, fmt.Errorf(
			"UpdateProject: UpdateProject namespace=%s project=%s deploymentPipeline=%s: %w",
			namespaceName, projectName, deploymentPipeline, err,
		)
	}
	return mutationResult(updated, "updated", map[string]any{
		"deploymentPipelineRef": updated.Spec.DeploymentPipelineRef.Name,
	}), nil
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
