// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	ocLabels "github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	clustercomponenttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clustercomponenttype"
	componentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/component"
	componenttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componenttype"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

func (h *MCPHandler) CreateComponent(
	ctx context.Context, namespaceName, projectName string, req *gen.CreateComponentRequest,
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

	if req.DisplayName != nil && *req.DisplayName != "" {
		component.Annotations[controller.AnnotationKeyDisplayName] = *req.DisplayName
	}
	if req.Description != nil && *req.Description != "" {
		component.Annotations[controller.AnnotationKeyDescription] = *req.Description
	}
	if req.ComponentType != nil && *req.ComponentType != "" {
		kind, err := h.resolveComponentTypeKind(ctx, namespaceName, *req.ComponentType)
		if err != nil {
			return nil, err
		}
		component.Spec.ComponentType = openchoreov1alpha1.ComponentTypeRef{
			Kind: kind,
			Name: *req.ComponentType,
		}
	}
	if req.AutoDeploy != nil {
		component.Spec.AutoDeploy = *req.AutoDeploy
	}
	if req.Parameters != nil {
		paramsBytes, err := json.Marshal(*req.Parameters)
		if err != nil {
			return nil, err
		}
		component.Spec.Parameters = &runtime.RawExtension{Raw: paramsBytes}
	}
	if req.Workflow != nil {
		var workflowParams *runtime.RawExtension
		if req.Workflow.Parameters != nil {
			paramsBytes, err := json.Marshal(*req.Workflow.Parameters)
			if err != nil {
				return nil, err
			}
			workflowParams = &runtime.RawExtension{Raw: paramsBytes}
		}
		component.Spec.Workflow = &openchoreov1alpha1.ComponentWorkflowConfig{
			Name:       req.Workflow.Name,
			Parameters: workflowParams,
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
	ctx context.Context, namespaceName, componentName string,
) (any, error) {
	component, err := h.services.ComponentService.GetComponent(ctx, namespaceName, componentName)
	if err != nil {
		return nil, err
	}
	return componentDetail(component), nil
}

func (h *MCPHandler) ListWorkloads(
	ctx context.Context, namespaceName, componentName string,
) (any, error) {
	result, err := h.services.WorkloadService.ListWorkloads(ctx, namespaceName, componentName, services.ListOptions{})
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("workloads", result.Items, result.NextCursor, workloadSummary), nil
}

func (h *MCPHandler) GetWorkload(
	ctx context.Context, namespaceName, workloadName string,
) (any, error) {
	w, err := h.services.WorkloadService.GetWorkload(ctx, namespaceName, workloadName)
	if err != nil {
		return nil, err
	}
	return workloadDetail(w), nil
}

func (h *MCPHandler) ListComponentReleases(
	ctx context.Context, namespaceName, componentName string, opts tools.ListOpts,
) (any, error) {
	result, err := h.services.ComponentReleaseService.ListComponentReleases(ctx, namespaceName, componentName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("releases", result.Items, result.NextCursor, componentReleaseSummary), nil
}

func (h *MCPHandler) CreateComponentRelease(
	ctx context.Context, namespaceName, componentName, releaseName string,
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
	ctx context.Context, namespaceName, releaseName string,
) (any, error) {
	cr, err := h.services.ComponentReleaseService.GetComponentRelease(ctx, namespaceName, releaseName)
	if err != nil {
		return nil, err
	}
	return componentReleaseDetail(cr), nil
}

func (h *MCPHandler) ListReleaseBindings(
	ctx context.Context, namespaceName, componentName string, opts tools.ListOpts,
) (any, error) {
	result, err := h.services.ReleaseBindingService.ListReleaseBindings(ctx, namespaceName, componentName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("bindings", result.Items, result.NextCursor, releaseBindingSummary), nil
}

func (h *MCPHandler) GetReleaseBinding(
	ctx context.Context, namespaceName, bindingName string,
) (any, error) {
	rb, err := h.services.ReleaseBindingService.GetReleaseBinding(ctx, namespaceName, bindingName)
	if err != nil {
		return nil, err
	}
	return releaseBindingDetail(rb), nil
}

func (h *MCPHandler) CreateReleaseBinding(
	ctx context.Context, namespaceName string,
	req *gen.ReleaseBindingSpec,
) (any, error) {
	bindingName := fmt.Sprintf("%s-%s", req.Owner.ComponentName, req.Environment)

	rb := &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bindingName,
			Namespace: namespaceName,
		},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner: openchoreov1alpha1.ReleaseBindingOwner{
				ProjectName:   req.Owner.ProjectName,
				ComponentName: req.Owner.ComponentName,
			},
			Environment: req.Environment,
		},
	}
	if req.ReleaseName != nil && *req.ReleaseName != "" {
		rb.Spec.ReleaseName = *req.ReleaseName
	}
	if req.ComponentTypeEnvironmentConfigs != nil {
		overrideBytes, err := json.Marshal(*req.ComponentTypeEnvironmentConfigs)
		if err != nil {
			return nil, err
		}
		rb.Spec.ComponentTypeEnvironmentConfigs = &runtime.RawExtension{Raw: overrideBytes}
	}
	if req.TraitEnvironmentConfigs != nil {
		traitEnvironmentConfigs := make(map[string]runtime.RawExtension, len(*req.TraitEnvironmentConfigs))
		for k, v := range *req.TraitEnvironmentConfigs {
			overrideBytes, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			traitEnvironmentConfigs[k] = runtime.RawExtension{Raw: overrideBytes}
		}
		rb.Spec.TraitEnvironmentConfigs = traitEnvironmentConfigs
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

	created, err := h.services.ReleaseBindingService.CreateReleaseBinding(ctx, namespaceName, rb)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) UpdateReleaseBinding(
	ctx context.Context, namespaceName, bindingName string,
	req *gen.ReleaseBindingSpec,
) (any, error) {
	rb, err := h.services.ReleaseBindingService.GetReleaseBinding(ctx, namespaceName, bindingName)
	if err != nil {
		return nil, err
	}

	if req.ReleaseName != nil && *req.ReleaseName != "" {
		rb.Spec.ReleaseName = *req.ReleaseName
	}
	if req.Environment != "" && req.Environment != rb.Spec.Environment {
		return nil, fmt.Errorf("release binding environment is immutable")
	}
	if req.ComponentTypeEnvironmentConfigs != nil {
		overrideBytes, err := json.Marshal(*req.ComponentTypeEnvironmentConfigs)
		if err != nil {
			return nil, err
		}
		rb.Spec.ComponentTypeEnvironmentConfigs = &runtime.RawExtension{Raw: overrideBytes}
	}
	if req.TraitEnvironmentConfigs != nil {
		traitEnvironmentConfigs := make(map[string]runtime.RawExtension, len(*req.TraitEnvironmentConfigs))
		for k, v := range *req.TraitEnvironmentConfigs {
			overrideBytes, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			traitEnvironmentConfigs[k] = runtime.RawExtension{Raw: overrideBytes}
		}
		rb.Spec.TraitEnvironmentConfigs = traitEnvironmentConfigs
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

func (h *MCPHandler) CreateWorkload(
	ctx context.Context, namespaceName, componentName string, workloadSpec any,
) (any, error) {
	// Look up the component to get the project name
	component, err := h.services.ComponentService.GetComponent(ctx, namespaceName, componentName)
	if err != nil {
		return nil, fmt.Errorf("failed to validate component: %w", err)
	}

	specBytes, err := json.Marshal(workloadSpec)
	if err != nil {
		return nil, err
	}

	var spec openchoreov1alpha1.WorkloadSpec
	if err := json.Unmarshal(specBytes, &spec); err != nil {
		return nil, err
	}

	// Set the owner from the component
	spec.Owner = openchoreov1alpha1.WorkloadOwner{
		ProjectName:   component.Spec.Owner.ProjectName,
		ComponentName: componentName,
	}

	workload := &openchoreov1alpha1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName + "-workload",
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

func (h *MCPHandler) UpdateWorkload(
	ctx context.Context, namespaceName, workloadName string, workloadSpec any,
) (any, error) {
	// Get the existing workload to preserve metadata
	existing, err := h.services.WorkloadService.GetWorkload(ctx, namespaceName, workloadName)
	if err != nil {
		return nil, err
	}

	specBytes, err := json.Marshal(workloadSpec)
	if err != nil {
		return nil, err
	}

	var spec openchoreov1alpha1.WorkloadSpec
	if err := json.Unmarshal(specBytes, &spec); err != nil {
		return nil, err
	}

	// Preserve the owner from the existing workload since it is immutable
	// and the incoming spec from MCP tools typically does not include it.
	owner := existing.Spec.Owner
	existing.Spec = spec
	existing.Spec.Owner = owner

	updated, err := h.services.WorkloadService.UpdateWorkload(ctx, namespaceName, existing)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) GetWorkloadSchema(ctx context.Context) (any, error) {
	return h.services.WorkloadService.GetWorkloadSchema(ctx)
}

func (h *MCPHandler) GetComponentSchema(
	ctx context.Context, namespaceName, componentName string,
) (any, error) {
	return h.services.ComponentService.GetComponentSchema(ctx, namespaceName, componentName)
}

func (h *MCPHandler) PatchComponent(
	ctx context.Context, namespaceName, componentName string, req *gen.PatchComponentRequest,
) (any, error) {
	component, err := h.services.ComponentService.GetComponent(ctx, namespaceName, componentName)
	if err != nil {
		return nil, err
	}

	if req.AutoDeploy != nil {
		component.Spec.AutoDeploy = *req.AutoDeploy
	}
	if req.Parameters != nil {
		paramsBytes, err := json.Marshal(*req.Parameters)
		if err != nil {
			return nil, err
		}
		component.Spec.Parameters = &runtime.RawExtension{Raw: paramsBytes}
	}

	updated, err := h.services.ComponentService.UpdateComponent(ctx, namespaceName, component)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "patched"), nil
}

func (h *MCPHandler) UpdateReleaseBindingState(
	ctx context.Context, namespaceName, bindingName string, state *gen.ReleaseBindingSpecState,
) (any, error) {
	rb, err := h.services.ReleaseBindingService.GetReleaseBinding(ctx, namespaceName, bindingName)
	if err != nil {
		return nil, err
	}

	if state != nil {
		rb.Spec.State = openchoreov1alpha1.ReleaseState(*state)
	}

	updated, err := h.services.ReleaseBindingService.UpdateReleaseBinding(ctx, namespaceName, rb)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated", map[string]any{
		"state": string(updated.Spec.State),
	}), nil
}

func (h *MCPHandler) GetComponentReleaseSchema(
	ctx context.Context, namespaceName, componentName, releaseName string,
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

func (h *MCPHandler) GetComponentType(ctx context.Context, namespaceName, ctName string) (any, error) {
	ct, err := h.services.ComponentTypeService.GetComponentType(ctx, namespaceName, ctName)
	if err != nil {
		return nil, err
	}
	return componentTypeDetail(ct), nil
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

func (h *MCPHandler) GetTrait(ctx context.Context, namespaceName, traitName string) (any, error) {
	t, err := h.services.TraitService.GetTrait(ctx, namespaceName, traitName)
	if err != nil {
		return nil, err
	}
	return traitDetail(t), nil
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

// ClusterWorkflow operations

func (h *MCPHandler) ListClusterWorkflows(ctx context.Context, opts tools.ListOpts) (any, error) {
	result, err := h.services.ClusterWorkflowService.ListClusterWorkflows(ctx, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("cluster_workflows", result.Items, result.NextCursor, clusterWorkflowSummary), nil
}

func (h *MCPHandler) GetClusterWorkflow(ctx context.Context, cwfName string) (any, error) {
	cwf, err := h.services.ClusterWorkflowService.GetClusterWorkflow(ctx, cwfName)
	if err != nil {
		return nil, err
	}
	return clusterWorkflowDetail(cwf), nil
}

func (h *MCPHandler) GetClusterWorkflowSchema(ctx context.Context, cwfName string) (any, error) {
	return h.services.ClusterWorkflowService.GetClusterWorkflowSchema(ctx, cwfName)
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

func (h *MCPHandler) GetWorkflow(ctx context.Context, namespaceName, workflowName string) (any, error) {
	wf, err := h.services.WorkflowService.GetWorkflow(ctx, namespaceName, workflowName)
	if err != nil {
		return nil, err
	}
	return workflowDetail(wf), nil
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
				Kind:       component.Spec.Workflow.Kind,
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

// resolveComponentTypeKind resolves the kind of a component type reference by looking up
// both namespace-scoped ComponentType and cluster-scoped ClusterComponentType.
// The componentType string is in {workloadType}/{componentTypeName} format.
// Namespace-scoped ComponentType takes precedence; ClusterComponentType is the fallback.
func (h *MCPHandler) resolveComponentTypeKind(ctx context.Context, namespaceName, componentType string) (openchoreov1alpha1.ComponentTypeRefKind, error) {
	parts := strings.SplitN(componentType, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("invalid componentType format %q: expected {workloadType}/{name}", componentType)
	}
	typeName := parts[1]

	_, err := h.services.ComponentTypeService.GetComponentType(ctx, namespaceName, typeName)
	if err == nil {
		return openchoreov1alpha1.ComponentTypeRefKindComponentType, nil
	}
	if !errors.Is(err, componenttypesvc.ErrComponentTypeNotFound) {
		return "", fmt.Errorf("failed to look up ComponentType %q: %w", typeName, err)
	}

	_, cctErr := h.services.ClusterComponentTypeService.GetClusterComponentType(ctx, typeName)
	if cctErr == nil {
		return openchoreov1alpha1.ComponentTypeRefKindClusterComponentType, nil
	}
	if !errors.Is(cctErr, clustercomponenttypesvc.ErrClusterComponentTypeNotFound) {
		return "", fmt.Errorf("failed to look up ClusterComponentType %q: %w", typeName, cctErr)
	}

	return "", fmt.Errorf("component type %q not found: no ComponentType in namespace %q or ClusterComponentType with that name", typeName, namespaceName)
}
