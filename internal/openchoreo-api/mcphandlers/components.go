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

	return h.services.ComponentService.CreateComponent(ctx, namespaceName, component)
}

func (h *MCPHandler) ListComponents(ctx context.Context, namespaceName, projectName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.ComponentService.ListComponents(ctx, namespaceName, projectName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapList("components", result.Items, result.NextCursor), nil
}

func (h *MCPHandler) GetComponent(
	ctx context.Context, namespaceName, _, componentName string, _ []string,
) (any, error) {
	return h.services.ComponentService.GetComponent(ctx, namespaceName, componentName)
}

func (h *MCPHandler) GetComponentWorkloads(
	ctx context.Context, namespaceName, _, componentName string,
) (any, error) {
	result, err := h.services.WorkloadService.ListWorkloads(ctx, namespaceName, componentName, services.ListOptions{})
	if err != nil {
		return nil, err
	}
	return wrapList("workloads", result.Items, result.NextCursor), nil
}

func (h *MCPHandler) ListComponentReleases(
	ctx context.Context, namespaceName, _, componentName string, opts tools.ListOpts,
) (any, error) {
	result, err := h.services.ComponentReleaseService.ListComponentReleases(ctx, namespaceName, componentName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapList("releases", result.Items, result.NextCursor), nil
}

func (h *MCPHandler) CreateComponentRelease(
	ctx context.Context, namespaceName, _, componentName, releaseName string,
) (any, error) {
	return h.services.ComponentService.GenerateRelease(ctx, namespaceName, componentName, &componentsvc.GenerateReleaseRequest{
		ReleaseName: releaseName,
	})
}

func (h *MCPHandler) GetComponentRelease(
	ctx context.Context, namespaceName, _, _, releaseName string,
) (any, error) {
	return h.services.ComponentReleaseService.GetComponentRelease(ctx, namespaceName, releaseName)
}

func (h *MCPHandler) ListReleaseBindings(
	ctx context.Context, namespaceName, _, componentName string, _ []string, opts tools.ListOpts,
) (any, error) {
	result, err := h.services.ReleaseBindingService.ListReleaseBindings(ctx, namespaceName, componentName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapList("bindings", result.Items, result.NextCursor), nil
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

	return h.services.ReleaseBindingService.UpdateReleaseBinding(ctx, namespaceName, rb)
}

func (h *MCPHandler) DeployRelease(
	ctx context.Context, namespaceName, _, componentName string, req *models.DeployReleaseRequest,
) (any, error) {
	return h.services.ComponentService.DeployRelease(ctx, namespaceName, componentName, &componentsvc.DeployReleaseRequest{
		ReleaseName: req.ReleaseName,
	})
}

func (h *MCPHandler) PromoteComponent(
	ctx context.Context, namespaceName, _, componentName string, req *models.PromoteComponentRequest,
) (any, error) {
	return h.services.ComponentService.PromoteComponent(ctx, namespaceName, componentName, &componentsvc.PromoteComponentRequest{
		SourceEnvironment: req.SourceEnvironment,
		TargetEnvironment: req.TargetEnvironment,
	})
}

func (h *MCPHandler) CreateWorkload(
	ctx context.Context, namespaceName, _, componentName string, workloadSpec interface{},
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

	return h.services.WorkloadService.CreateWorkload(ctx, namespaceName, workload)
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
	return result.Items[0], nil
}

func (h *MCPHandler) PatchComponent(
	ctx context.Context, namespaceName, _, componentName string, req *models.PatchComponentRequest,
) (any, error) {
	component, err := h.services.ComponentService.GetComponent(ctx, namespaceName, componentName)
	if err != nil {
		return nil, err
	}

	if req.AutoDeploy != nil {
		component.Spec.AutoDeploy = *req.AutoDeploy
	}
	if req.Parameters != nil {
		component.Spec.Parameters = req.Parameters
	}

	return h.services.ComponentService.UpdateComponent(ctx, namespaceName, component)
}

func (h *MCPHandler) UpdateReleaseBindingState(
	ctx context.Context, namespaceName, _, _, bindingName string, req *models.UpdateBindingRequest,
) (any, error) {
	rb, err := h.services.ReleaseBindingService.GetReleaseBinding(ctx, namespaceName, bindingName)
	if err != nil {
		return nil, err
	}

	rb.Spec.State = openchoreov1alpha1.ReleaseState(req.ReleaseState)

	return h.services.ReleaseBindingService.UpdateReleaseBinding(ctx, namespaceName, rb)
}

func (h *MCPHandler) GetComponentReleaseSchema(
	ctx context.Context, namespaceName, _, componentName, _ string,
) (any, error) {
	return h.services.ComponentService.GetComponentSchema(ctx, namespaceName, componentName)
}

func (h *MCPHandler) ListComponentTypes(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.ComponentTypeService.ListComponentTypes(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapList("component_types", result.Items, result.NextCursor), nil
}

func (h *MCPHandler) GetComponentTypeSchema(ctx context.Context, namespaceName, ctName string) (any, error) {
	return h.services.ComponentTypeService.GetComponentTypeSchema(ctx, namespaceName, ctName)
}

func (h *MCPHandler) ListTraits(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.TraitService.ListTraits(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapList("traits", result.Items, result.NextCursor), nil
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

	return h.services.WorkflowRunService.CreateWorkflowRun(ctx, namespaceName, wfRun)
}

func (h *MCPHandler) ListWorkflowRuns(ctx context.Context, namespaceName, projectName, componentName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.WorkflowRunService.ListWorkflowRuns(ctx, namespaceName, projectName, componentName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapList("workflow_runs", result.Items, result.NextCursor), nil
}

func (h *MCPHandler) GetWorkflowRun(ctx context.Context, namespaceName, runName string) (any, error) {
	return h.services.WorkflowRunService.GetWorkflowRun(ctx, namespaceName, runName)
}

// ClusterComponentType operations

func (h *MCPHandler) ListClusterComponentTypes(ctx context.Context, opts tools.ListOpts) (any, error) {
	result, err := h.services.ClusterComponentTypeService.ListClusterComponentTypes(ctx, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapList("cluster_component_types", result.Items, result.NextCursor), nil
}

func (h *MCPHandler) GetClusterComponentType(ctx context.Context, cctName string) (any, error) {
	return h.services.ClusterComponentTypeService.GetClusterComponentType(ctx, cctName)
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
	return wrapList("cluster_traits", result.Items, result.NextCursor), nil
}

func (h *MCPHandler) GetClusterTrait(ctx context.Context, ctName string) (any, error) {
	return h.services.ClusterTraitService.GetClusterTrait(ctx, ctName)
}

func (h *MCPHandler) GetClusterTraitSchema(ctx context.Context, ctName string) (any, error) {
	return h.services.ClusterTraitService.GetClusterTraitSchema(ctx, ctName)
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
		var params map[string]interface{}
		if err := json.Unmarshal(parameters.Raw, &params); err == nil {
			params["commit"] = commit
			updatedRaw, err := json.Marshal(params)
			if err == nil {
				parameters = &runtime.RawExtension{Raw: updatedRaw}
			}
		}
	} else if commit != "" {
		raw, _ := json.Marshal(map[string]interface{}{"commit": commit})
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

	return h.services.WorkflowRunService.CreateWorkflowRun(ctx, namespaceName, workflowRun)
}
