// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// convertSpec converts a gen spec type to a CRD spec type using JSON round-trip.
func convertSpec[S any, D any](src S) (D, error) {
	var dst D
	data, err := json.Marshal(src)
	if err != nil {
		return dst, fmt.Errorf("marshal spec: %w", err)
	}
	if err := json.Unmarshal(data, &dst); err != nil {
		return dst, fmt.Errorf("unmarshal spec: %w", err)
	}
	return dst, nil
}

// cleanAnnotations removes empty-value annotation entries that were set as defaults.
func cleanAnnotations(annotations map[string]string) {
	for _, key := range []string{controller.AnnotationKeyDisplayName, controller.AnnotationKeyDescription} {
		if v, ok := annotations[key]; ok && v == "" {
			delete(annotations, key)
		}
	}
}

// ---------------------------------------------------------------------------
// ComponentType (namespace-scoped) write operations
// ---------------------------------------------------------------------------

func (h *MCPHandler) CreateComponentType(ctx context.Context, namespaceName string, req *gen.CreateComponentTypeJSONRequestBody) (any, error) {
	annotations := map[string]string{}
	if req.Metadata.Annotations != nil {
		maps.Copy(annotations, *req.Metadata.Annotations)
	}
	cleanAnnotations(annotations)

	ct := &openchoreov1alpha1.ComponentType{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Metadata.Name,
			Namespace:   namespaceName,
			Annotations: annotations,
		},
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.ComponentTypeSpec, openchoreov1alpha1.ComponentTypeSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		ct.Spec = spec
	}

	created, err := h.services.ComponentTypeService.CreateComponentType(ctx, namespaceName, ct)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) UpdateComponentType(ctx context.Context, namespaceName string, req *gen.UpdateComponentTypeJSONRequestBody) (any, error) {
	existing, err := h.services.ComponentTypeService.GetComponentType(ctx, namespaceName, req.Metadata.Name)
	if err != nil {
		return nil, err
	}

	if req.Metadata.Annotations != nil {
		if existing.Annotations == nil {
			existing.Annotations = map[string]string{}
		}
		maps.Copy(existing.Annotations, *req.Metadata.Annotations)
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.ComponentTypeSpec, openchoreov1alpha1.ComponentTypeSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		existing.Spec = spec
	}

	updated, err := h.services.ComponentTypeService.UpdateComponentType(ctx, namespaceName, existing)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteComponentType(ctx context.Context, namespaceName, ctName string) (any, error) {
	if err := h.services.ComponentTypeService.DeleteComponentType(ctx, namespaceName, ctName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":      ctName,
		"namespace": namespaceName,
		"action":    "deleted",
	}, nil
}

// ---------------------------------------------------------------------------
// Trait (namespace-scoped) write operations
// ---------------------------------------------------------------------------

func (h *MCPHandler) CreateTrait(ctx context.Context, namespaceName string, req *gen.CreateTraitJSONRequestBody) (any, error) {
	annotations := map[string]string{}
	if req.Metadata.Annotations != nil {
		maps.Copy(annotations, *req.Metadata.Annotations)
	}
	cleanAnnotations(annotations)

	t := &openchoreov1alpha1.Trait{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Metadata.Name,
			Namespace:   namespaceName,
			Annotations: annotations,
		},
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.TraitSpec, openchoreov1alpha1.TraitSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		t.Spec = spec
	}

	created, err := h.services.TraitService.CreateTrait(ctx, namespaceName, t)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) UpdateTrait(ctx context.Context, namespaceName string, req *gen.UpdateTraitJSONRequestBody) (any, error) {
	existing, err := h.services.TraitService.GetTrait(ctx, namespaceName, req.Metadata.Name)
	if err != nil {
		return nil, err
	}

	if req.Metadata.Annotations != nil {
		if existing.Annotations == nil {
			existing.Annotations = map[string]string{}
		}
		maps.Copy(existing.Annotations, *req.Metadata.Annotations)
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.TraitSpec, openchoreov1alpha1.TraitSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		existing.Spec = spec
	}

	updated, err := h.services.TraitService.UpdateTrait(ctx, namespaceName, existing)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteTrait(ctx context.Context, namespaceName, traitName string) (any, error) {
	if err := h.services.TraitService.DeleteTrait(ctx, namespaceName, traitName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":      traitName,
		"namespace": namespaceName,
		"action":    "deleted",
	}, nil
}

// ---------------------------------------------------------------------------
// Workflow (namespace-scoped) write operations
// ---------------------------------------------------------------------------

func (h *MCPHandler) CreateWorkflow(ctx context.Context, namespaceName string, req *gen.CreateWorkflowJSONRequestBody) (any, error) {
	annotations := map[string]string{}
	if req.Metadata.Annotations != nil {
		maps.Copy(annotations, *req.Metadata.Annotations)
	}
	cleanAnnotations(annotations)

	wf := &openchoreov1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Metadata.Name,
			Namespace:   namespaceName,
			Annotations: annotations,
		},
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.WorkflowSpec, openchoreov1alpha1.WorkflowSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		wf.Spec = spec
	}

	created, err := h.services.WorkflowService.CreateWorkflow(ctx, namespaceName, wf)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) UpdateWorkflow(ctx context.Context, namespaceName string, req *gen.UpdateWorkflowJSONRequestBody) (any, error) {
	existing, err := h.services.WorkflowService.GetWorkflow(ctx, namespaceName, req.Metadata.Name)
	if err != nil {
		return nil, err
	}

	if req.Metadata.Annotations != nil {
		if existing.Annotations == nil {
			existing.Annotations = map[string]string{}
		}
		maps.Copy(existing.Annotations, *req.Metadata.Annotations)
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.WorkflowSpec, openchoreov1alpha1.WorkflowSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		existing.Spec = spec
	}

	updated, err := h.services.WorkflowService.UpdateWorkflow(ctx, namespaceName, existing)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteWorkflow(ctx context.Context, namespaceName, workflowName string) (any, error) {
	if err := h.services.WorkflowService.DeleteWorkflow(ctx, namespaceName, workflowName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":      workflowName,
		"namespace": namespaceName,
		"action":    "deleted",
	}, nil
}

// ---------------------------------------------------------------------------
// ClusterComponentType write operations
// ---------------------------------------------------------------------------

func (h *MCPHandler) CreateClusterComponentType(ctx context.Context, req *gen.CreateClusterComponentTypeJSONRequestBody) (any, error) {
	annotations := map[string]string{}
	if req.Metadata.Annotations != nil {
		maps.Copy(annotations, *req.Metadata.Annotations)
	}
	cleanAnnotations(annotations)

	cct := &openchoreov1alpha1.ClusterComponentType{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Metadata.Name,
			Annotations: annotations,
		},
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.ClusterComponentTypeSpec, openchoreov1alpha1.ClusterComponentTypeSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		cct.Spec = spec
	}

	created, err := h.services.ClusterComponentTypeService.CreateClusterComponentType(ctx, cct)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) UpdateClusterComponentType(ctx context.Context, req *gen.UpdateClusterComponentTypeJSONRequestBody) (any, error) {
	existing, err := h.services.ClusterComponentTypeService.GetClusterComponentType(ctx, req.Metadata.Name)
	if err != nil {
		return nil, err
	}

	if req.Metadata.Annotations != nil {
		if existing.Annotations == nil {
			existing.Annotations = map[string]string{}
		}
		maps.Copy(existing.Annotations, *req.Metadata.Annotations)
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.ClusterComponentTypeSpec, openchoreov1alpha1.ClusterComponentTypeSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		existing.Spec = spec
	}

	updated, err := h.services.ClusterComponentTypeService.UpdateClusterComponentType(ctx, existing)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteClusterComponentType(ctx context.Context, cctName string) (any, error) {
	if err := h.services.ClusterComponentTypeService.DeleteClusterComponentType(ctx, cctName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":   cctName,
		"action": "deleted",
	}, nil
}

// ---------------------------------------------------------------------------
// ClusterTrait write operations
// ---------------------------------------------------------------------------

func (h *MCPHandler) CreateClusterTrait(ctx context.Context, req *gen.CreateClusterTraitJSONRequestBody) (any, error) {
	annotations := map[string]string{}
	if req.Metadata.Annotations != nil {
		maps.Copy(annotations, *req.Metadata.Annotations)
	}
	cleanAnnotations(annotations)

	ct := &openchoreov1alpha1.ClusterTrait{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Metadata.Name,
			Annotations: annotations,
		},
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.ClusterTraitSpec, openchoreov1alpha1.ClusterTraitSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		ct.Spec = spec
	}

	created, err := h.services.ClusterTraitService.CreateClusterTrait(ctx, ct)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) UpdateClusterTrait(ctx context.Context, req *gen.UpdateClusterTraitJSONRequestBody) (any, error) {
	existing, err := h.services.ClusterTraitService.GetClusterTrait(ctx, req.Metadata.Name)
	if err != nil {
		return nil, err
	}

	if req.Metadata.Annotations != nil {
		if existing.Annotations == nil {
			existing.Annotations = map[string]string{}
		}
		maps.Copy(existing.Annotations, *req.Metadata.Annotations)
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.ClusterTraitSpec, openchoreov1alpha1.ClusterTraitSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		existing.Spec = spec
	}

	updated, err := h.services.ClusterTraitService.UpdateClusterTrait(ctx, existing)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteClusterTrait(ctx context.Context, clusterTraitName string) (any, error) {
	if err := h.services.ClusterTraitService.DeleteClusterTrait(ctx, clusterTraitName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":   clusterTraitName,
		"action": "deleted",
	}, nil
}

// ---------------------------------------------------------------------------
// ClusterWorkflow write operations
// ---------------------------------------------------------------------------

func (h *MCPHandler) CreateClusterWorkflow(ctx context.Context, req *gen.CreateClusterWorkflowJSONRequestBody) (any, error) {
	annotations := map[string]string{}
	if req.Metadata.Annotations != nil {
		maps.Copy(annotations, *req.Metadata.Annotations)
	}
	cleanAnnotations(annotations)

	cwf := &openchoreov1alpha1.ClusterWorkflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Metadata.Name,
			Annotations: annotations,
		},
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.ClusterWorkflowSpec, openchoreov1alpha1.ClusterWorkflowSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		cwf.Spec = spec
	}

	created, err := h.services.ClusterWorkflowService.CreateClusterWorkflow(ctx, cwf)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) UpdateClusterWorkflow(ctx context.Context, req *gen.UpdateClusterWorkflowJSONRequestBody) (any, error) {
	existing, err := h.services.ClusterWorkflowService.GetClusterWorkflow(ctx, req.Metadata.Name)
	if err != nil {
		return nil, err
	}

	if req.Metadata.Annotations != nil {
		if existing.Annotations == nil {
			existing.Annotations = map[string]string{}
		}
		maps.Copy(existing.Annotations, *req.Metadata.Annotations)
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.ClusterWorkflowSpec, openchoreov1alpha1.ClusterWorkflowSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		existing.Spec = spec
	}

	updated, err := h.services.ClusterWorkflowService.UpdateClusterWorkflow(ctx, existing)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteClusterWorkflow(ctx context.Context, clusterWorkflowName string) (any, error) {
	if err := h.services.ClusterWorkflowService.DeleteClusterWorkflow(ctx, clusterWorkflowName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":   clusterWorkflowName,
		"action": "deleted",
	}, nil
}
