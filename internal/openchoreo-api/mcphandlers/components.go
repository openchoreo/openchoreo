// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	ocLabels "github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	componentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/component"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

func (h *MCPHandler) CreateComponent(
	ctx context.Context, namespaceName, projectName string, req *models.CreateComponentRequest,
) (any, error) {
	component := &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Name,
			Namespace:   namespaceName,
			Annotations: make(map[string]string),
		},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner: openchoreov1alpha1.ComponentOwner{
				ProjectName: projectName,
			},
		},
	}

	if req.DisplayName != "" {
		component.Annotations[controller.AnnotationKeyDisplayName] = req.DisplayName
	}
	if req.Description != "" {
		component.Annotations[controller.AnnotationKeyDescription] = req.Description
	}
	if req.ComponentType != nil {
		component.Spec.ComponentType = openchoreov1alpha1.ComponentTypeRef{
			Kind: openchoreov1alpha1.ComponentTypeRefKind(req.ComponentType.Kind),
			Name: req.ComponentType.Name,
		}
	}
	if req.AutoDeploy != nil {
		component.Spec.AutoDeploy = *req.AutoDeploy
	}
	if req.Parameters != nil {
		component.Spec.Parameters = req.Parameters
	}
	if req.WorkflowConfig != nil {
		component.Spec.Workflow = &openchoreov1alpha1.WorkflowRunConfig{
			Name:       req.WorkflowConfig.Name,
			Parameters: req.WorkflowConfig.Parameters,
		}
	}

	created, err := h.services.ComponentService.CreateComponent(ctx, namespaceName, component)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created", map[string]any{
		"componentType": created.Spec.ComponentType.Name,
	}), nil
}

func (h *MCPHandler) ListComponents(ctx context.Context, namespaceName, projectName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.ComponentService.ListComponents(ctx, namespaceName, projectName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("components", result.Items, result.NextCursor, componentSummary), nil
}

func (h *MCPHandler) GetComponent(
	ctx context.Context, namespaceName, _, componentName string, _ []string,
) (any, error) {
	component, err := h.services.ComponentService.GetComponent(ctx, namespaceName, componentName)
	if err != nil {
		return nil, err
	}
	return componentDetail(component), nil
}

func (h *MCPHandler) GetComponentWorkloads(
	ctx context.Context, namespaceName, _, componentName string,
) (any, error) {
	result, err := h.services.WorkloadService.ListWorkloads(ctx, namespaceName, componentName, services.ListOptions{})
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("workloads", result.Items, result.NextCursor, workloadSummary), nil
}

func (h *MCPHandler) GetComponentWorkload(
	ctx context.Context, namespaceName, _, _, workloadName string,
) (any, error) {
	w, err := h.services.WorkloadService.GetWorkload(ctx, namespaceName, workloadName)
	if err != nil {
		return nil, err
	}
	return workloadDetail(w), nil
}

func (h *MCPHandler) ListComponentReleases(
	ctx context.Context, namespaceName, _, componentName string, opts tools.ListOpts,
) (any, error) {
	result, err := h.services.ComponentReleaseService.ListComponentReleases(ctx, namespaceName, componentName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("releases", result.Items, result.NextCursor, componentReleaseSummary), nil
}

func (h *MCPHandler) CreateComponentRelease(
	ctx context.Context, namespaceName, _, componentName, releaseName string,
) (any, error) {
	cr, err := h.services.ComponentService.GenerateRelease(ctx, namespaceName, componentName, &componentsvc.GenerateReleaseRequest{
		ReleaseName: releaseName,
	})
	if err != nil {
		return nil, err
	}
	return mutationResult(cr, "created"), nil
}

func (h *MCPHandler) GetComponentRelease(
	ctx context.Context, namespaceName, _, _, releaseName string,
) (any, error) {
	cr, err := h.services.ComponentReleaseService.GetComponentRelease(ctx, namespaceName, releaseName)
	if err != nil {
		return nil, err
	}
	return componentReleaseDetail(cr), nil
}

func (h *MCPHandler) ListReleaseBindings(
	ctx context.Context, namespaceName, _, componentName string, _ []string, opts tools.ListOpts,
) (any, error) {
	result, err := h.services.ReleaseBindingService.ListReleaseBindings(ctx, namespaceName, componentName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("bindings", result.Items, result.NextCursor, releaseBindingSummary), nil
}

func (h *MCPHandler) GetReleaseBinding(
	ctx context.Context, namespaceName, _, _, bindingName string,
) (any, error) {
	rb, err := h.services.ReleaseBindingService.GetReleaseBinding(ctx, namespaceName, bindingName)
	if err != nil {
		return nil, err
	}
	return releaseBindingDetail(rb), nil
}

func (h *MCPHandler) PatchReleaseBinding(
	ctx context.Context, namespaceName, _, _, bindingName string,
	req *models.PatchReleaseBindingRequest,
) (any, error) {
	rb, err := h.services.ReleaseBindingService.GetReleaseBinding(ctx, namespaceName, bindingName)
	if err != nil {
		return nil, err
	}

	if req.ReleaseName != "" {
		rb.Spec.ReleaseName = req.ReleaseName
	}
	if req.Environment != "" {
		rb.Spec.Environment = req.Environment
	}
	if req.ComponentTypeEnvOverrides != nil {
		overrideBytes, err := json.Marshal(req.ComponentTypeEnvOverrides)
		if err != nil {
			return nil, err
		}
		rb.Spec.ComponentTypeEnvOverrides = &runtime.RawExtension{Raw: overrideBytes}
	}
	if req.TraitOverrides != nil {
		traitOverrides := make(map[string]runtime.RawExtension, len(req.TraitOverrides))
		for k, v := range req.TraitOverrides {
			overrideBytes, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			traitOverrides[k] = runtime.RawExtension{Raw: overrideBytes}
		}
		rb.Spec.TraitOverrides = traitOverrides
	}
	if req.WorkloadOverrides != nil {
		overrideBytes, err := json.Marshal(req.WorkloadOverrides)
		if err != nil {
			return nil, err
		}
		var wo openchoreov1alpha1.WorkloadOverrideTemplateSpec
		if err := json.Unmarshal(overrideBytes, &wo); err != nil {
			return nil, err
		}
		rb.Spec.WorkloadOverrides = &wo
	}

	updated, err := h.services.ReleaseBindingService.UpdateReleaseBinding(ctx, namespaceName, rb)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "patched"), nil
}

func (h *MCPHandler) DeployRelease(
	ctx context.Context, namespaceName, _, componentName string, req *models.DeployReleaseRequest,
) (any, error) {
	rb, err := h.services.ComponentService.DeployRelease(ctx, namespaceName, componentName, &componentsvc.DeployReleaseRequest{
		ReleaseName: req.ReleaseName,
	})
	if err != nil {
		return nil, err
	}
	return mutationResult(rb, "deployed", map[string]any{
		"environment": rb.Spec.Environment,
		"releaseName": rb.Spec.ReleaseName,
	}), nil
}

func (h *MCPHandler) PromoteComponent(
	ctx context.Context, namespaceName, _, componentName string, req *models.PromoteComponentRequest,
) (any, error) {
	rb, err := h.services.ComponentService.PromoteComponent(ctx, namespaceName, componentName, &componentsvc.PromoteComponentRequest{
		SourceEnvironment: req.SourceEnvironment,
		TargetEnvironment: req.TargetEnvironment,
	})
	if err != nil {
		return nil, err
	}
	return mutationResult(rb, "promoted", map[string]any{
		"environment": rb.Spec.Environment,
	}), nil
}

func (h *MCPHandler) CreateWorkload(
	ctx context.Context, namespaceName, _, componentName string, workloadSpec any,
) (any, error) {
	specBytes, err := json.Marshal(workloadSpec)
	if err != nil {
		return nil, err
	}

	var spec openchoreov1alpha1.WorkloadSpec
	if err := json.Unmarshal(specBytes, &spec); err != nil {
		return nil, err
	}

	workload := &openchoreov1alpha1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespaceName,
		},
		Spec: spec,
	}

	created, err := h.services.WorkloadService.CreateWorkload(ctx, namespaceName, workload)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) GetComponentSchema(
	ctx context.Context, namespaceName, _, componentName string,
) (any, error) {
	return h.services.ComponentService.GetComponentSchema(ctx, namespaceName, componentName)
}

func (h *MCPHandler) GetEnvironmentRelease(
	ctx context.Context, namespaceName, _, componentName, environmentName string,
) (any, error) {
	result, err := h.services.ReleaseService.ListReleases(ctx, namespaceName, componentName, environmentName, services.ListOptions{})
	if err != nil {
		return nil, err
	}
	if len(result.Items) == 0 {
		return nil, nil
	}
	return releaseDetail(&result.Items[0]), nil
}

func (h *MCPHandler) PatchComponent(
	ctx context.Context, namespaceName, _, componentName string, req *models.PatchComponentRequest,
) (any, error) {
	component, err := h.services.ComponentService.GetComponent(ctx, namespaceName, componentName)
	if err != nil {
		return nil, err
	}

	if req.DisplayName != "" {
		if component.Annotations == nil {
			component.Annotations = make(map[string]string)
		}
		component.Annotations[controller.AnnotationKeyDisplayName] = req.DisplayName
	}
	if req.Description != "" {
		if component.Annotations == nil {
			component.Annotations = make(map[string]string)
		}
		component.Annotations[controller.AnnotationKeyDescription] = req.Description
	}
	if req.AutoDeploy != nil {
		component.Spec.AutoDeploy = *req.AutoDeploy
	}
	if req.Parameters != nil {
		component.Spec.Parameters = req.Parameters
	}
	if req.WorkflowConfig != nil {
		component.Spec.Workflow = &openchoreov1alpha1.WorkflowRunConfig{
			Name:       req.WorkflowConfig.Name,
			Parameters: req.WorkflowConfig.Parameters,
		}
	}

	updated, err := h.services.ComponentService.UpdateComponent(ctx, namespaceName, component)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "patched"), nil
}

func (h *MCPHandler) UpdateReleaseBindingState(
	ctx context.Context, namespaceName, _, _, bindingName string, req *models.UpdateBindingRequest,
) (any, error) {
	rb, err := h.services.ReleaseBindingService.GetReleaseBinding(ctx, namespaceName, bindingName)
	if err != nil {
		return nil, err
	}

	rb.Spec.State = openchoreov1alpha1.ReleaseState(req.ReleaseState)

	updated, err := h.services.ReleaseBindingService.UpdateReleaseBinding(ctx, namespaceName, rb)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated", map[string]any{
		"state": string(updated.Spec.State),
	}), nil
}

func (h *MCPHandler) GetComponentReleaseSchema(
	ctx context.Context, namespaceName, _, componentName, releaseName string,
) (any, error) {
	return h.services.ComponentService.GetComponentReleaseSchema(ctx, namespaceName, releaseName, componentName)
}

func (h *MCPHandler) ListComponentTypes(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.ComponentTypeService.ListComponentTypes(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("component_types", result.Items, result.NextCursor, componentTypeSummary), nil
}

func (h *MCPHandler) GetComponentTypeSchema(ctx context.Context, namespaceName, ctName string) (any, error) {
	return h.services.ComponentTypeService.GetComponentTypeSchema(ctx, namespaceName, ctName)
}

func (h *MCPHandler) ListTraits(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.TraitService.ListTraits(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("traits", result.Items, result.NextCursor, traitSummary), nil
}

func (h *MCPHandler) GetTraitSchema(ctx context.Context, namespaceName, traitName string) (any, error) {
	return h.services.TraitService.GetTraitSchema(ctx, namespaceName, traitName)
}

func (h *MCPHandler) CreateWorkflowRun(ctx context.Context, namespaceName, workflowName string, parameters map[string]any) (any, error) {
	wfRun := &openchoreov1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: workflowName + "-run-",
			Namespace:    namespaceName,
		},
		Spec: openchoreov1alpha1.WorkflowRunSpec{
			Workflow: openchoreov1alpha1.WorkflowRunConfig{
				Name: workflowName,
			},
		},
	}

	if parameters != nil {
		rawParams, err := json.Marshal(parameters)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal workflow parameters: %w", err)
		}
		wfRun.Spec.Workflow.Parameters = &runtime.RawExtension{Raw: rawParams}
	}

	created, err := h.services.WorkflowRunService.CreateWorkflowRun(ctx, namespaceName, wfRun)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created", map[string]any{
		"workflowName": workflowName,
	}), nil
}

func (h *MCPHandler) ListWorkflowRuns(ctx context.Context, namespaceName, projectName, componentName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.WorkflowRunService.ListWorkflowRuns(ctx, namespaceName, projectName, componentName, "", toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("workflow_runs", result.Items, result.NextCursor, workflowRunSummary), nil
}

func (h *MCPHandler) GetWorkflowRun(ctx context.Context, namespaceName, runName string) (any, error) {
	wr, err := h.services.WorkflowRunService.GetWorkflowRun(ctx, namespaceName, runName)
	if err != nil {
		return nil, err
	}
	return workflowRunDetail(wr), nil
}

// ClusterComponentType operations

func (h *MCPHandler) ListClusterComponentTypes(ctx context.Context, opts tools.ListOpts) (any, error) {
	result, err := h.services.ClusterComponentTypeService.ListClusterComponentTypes(ctx, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("cluster_component_types", result.Items, result.NextCursor, clusterComponentTypeSummary), nil
}

func (h *MCPHandler) GetClusterComponentType(ctx context.Context, cctName string) (any, error) {
	cct, err := h.services.ClusterComponentTypeService.GetClusterComponentType(ctx, cctName)
	if err != nil {
		return nil, err
	}
	return clusterComponentTypeDetail(cct), nil
}

func (h *MCPHandler) GetClusterComponentTypeSchema(ctx context.Context, cctName string) (any, error) {
	return h.services.ClusterComponentTypeService.GetClusterComponentTypeSchema(ctx, cctName)
}

// ClusterTrait operations

func (h *MCPHandler) ListClusterTraits(ctx context.Context, opts tools.ListOpts) (any, error) {
	result, err := h.services.ClusterTraitService.ListClusterTraits(ctx, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("cluster_traits", result.Items, result.NextCursor, clusterTraitSummary), nil
}

func (h *MCPHandler) GetClusterTrait(ctx context.Context, ctName string) (any, error) {
	ct, err := h.services.ClusterTraitService.GetClusterTrait(ctx, ctName)
	if err != nil {
		return nil, err
	}
	return clusterTraitDetail(ct), nil
}

func (h *MCPHandler) GetClusterTraitSchema(ctx context.Context, ctName string) (any, error) {
	return h.services.ClusterTraitService.GetClusterTraitSchema(ctx, ctName)
}

// Workflow operations

func (h *MCPHandler) ListWorkflows(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.WorkflowService.ListWorkflows(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("workflows", result.Items, result.NextCursor, workflowSummary), nil
}

func (h *MCPHandler) GetWorkflowSchema(ctx context.Context, namespaceName, workflowName string) (any, error) {
	return h.services.WorkflowService.GetWorkflowSchema(ctx, namespaceName, workflowName)
}

func (h *MCPHandler) TriggerWorkflowRun(
	ctx context.Context, namespaceName, projectName, componentName, commit string,
) (any, error) {
	// Get the component to read its workflow configuration
	component, err := h.services.ComponentService.GetComponent(ctx, namespaceName, componentName)
	if err != nil {
		return nil, err
	}

	if component.Spec.Workflow == nil || component.Spec.Workflow.Name == "" {
		return nil, fmt.Errorf("component %s does not have a workflow configured", componentName)
	}

	// Start with the component's existing parameters
	parameters := component.Spec.Workflow.Parameters

	// If a commit is provided, inject it into the parameters
	if commit != "" && parameters != nil {
		var params map[string]any
		if err := json.Unmarshal(parameters.Raw, &params); err == nil {
			params["commit"] = commit
			updatedRaw, err := json.Marshal(params)
			if err == nil {
				parameters = &runtime.RawExtension{Raw: updatedRaw}
			}
		}
	} else if commit != "" {
		raw, _ := json.Marshal(map[string]any{"commit": commit})
		parameters = &runtime.RawExtension{Raw: raw}
	}

	// Generate a unique name for the workflow run
	workflowRun := &openchoreov1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: componentName + "-workflow-",
			Namespace:    namespaceName,
			Labels: map[string]string{
				ocLabels.LabelKeyProjectName:   projectName,
				ocLabels.LabelKeyComponentName: componentName,
			},
		},
		Spec: openchoreov1alpha1.WorkflowRunSpec{
			Workflow: openchoreov1alpha1.WorkflowRunConfig{
				Name:       component.Spec.Workflow.Name,
				Parameters: parameters,
			},
		},
	}

	created, err := h.services.WorkflowRunService.CreateWorkflowRun(ctx, namespaceName, workflowRun)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "triggered", map[string]any{
		"workflowName": component.Spec.Workflow.Name,
	}), nil
}
