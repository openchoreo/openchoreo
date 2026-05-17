// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"maps"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// ---------------------------------------------------------------------------
// ResourceType (namespace-scoped)
// ---------------------------------------------------------------------------

func (h *MCPHandler) ListResourceTypes(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.ResourceTypeService.ListResourceTypes(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("resource_types", result.Items, result.NextCursor, resourceTypeSummary), nil
}

func (h *MCPHandler) GetResourceType(ctx context.Context, namespaceName, rtName string) (any, error) {
	rt, err := h.services.ResourceTypeService.GetResourceType(ctx, namespaceName, rtName)
	if err != nil {
		return nil, err
	}
	return resourceTypeDetail(rt), nil
}

func (h *MCPHandler) GetResourceTypeSchema(ctx context.Context, namespaceName, rtName string) (any, error) {
	return h.services.ResourceTypeService.GetResourceTypeSchema(ctx, namespaceName, rtName)
}

func (h *MCPHandler) CreateResourceType(
	ctx context.Context, namespaceName string, req *gen.CreateResourceTypeJSONRequestBody,
) (any, error) {
	annotations := map[string]string{}
	if req.Metadata.Annotations != nil {
		maps.Copy(annotations, *req.Metadata.Annotations)
	}
	cleanAnnotations(annotations)

	rt := &openchoreov1alpha1.ResourceType{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Metadata.Name,
			Namespace:   namespaceName,
			Annotations: annotations,
		},
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.ResourceTypeSpec, openchoreov1alpha1.ResourceTypeSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		rt.Spec = spec
	}

	created, err := h.services.ResourceTypeService.CreateResourceType(ctx, namespaceName, rt)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) UpdateResourceType(
	ctx context.Context, namespaceName string, req *gen.UpdateResourceTypeJSONRequestBody,
) (any, error) {
	existing, err := h.services.ResourceTypeService.GetResourceType(ctx, namespaceName, req.Metadata.Name)
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
		spec, err := convertSpec[gen.ResourceTypeSpec, openchoreov1alpha1.ResourceTypeSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		existing.Spec = spec
	}

	updated, err := h.services.ResourceTypeService.UpdateResourceType(ctx, namespaceName, existing)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteResourceType(ctx context.Context, namespaceName, rtName string) (any, error) {
	if err := h.services.ResourceTypeService.DeleteResourceType(ctx, namespaceName, rtName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":      rtName,
		"namespace": namespaceName,
		"action":    "deleted",
	}, nil
}

// ---------------------------------------------------------------------------
// ClusterResourceType (cluster-scoped)
// ---------------------------------------------------------------------------

func (h *MCPHandler) ListClusterResourceTypes(ctx context.Context, opts tools.ListOpts) (any, error) {
	result, err := h.services.ClusterResourceTypeService.ListClusterResourceTypes(ctx, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("cluster_resource_types", result.Items, result.NextCursor, clusterResourceTypeSummary), nil
}

func (h *MCPHandler) GetClusterResourceType(ctx context.Context, crtName string) (any, error) {
	crt, err := h.services.ClusterResourceTypeService.GetClusterResourceType(ctx, crtName)
	if err != nil {
		return nil, err
	}
	return clusterResourceTypeDetail(crt), nil
}

func (h *MCPHandler) GetClusterResourceTypeSchema(ctx context.Context, crtName string) (any, error) {
	return h.services.ClusterResourceTypeService.GetClusterResourceTypeSchema(ctx, crtName)
}

func (h *MCPHandler) CreateClusterResourceType(
	ctx context.Context, req *gen.CreateClusterResourceTypeJSONRequestBody,
) (any, error) {
	annotations := map[string]string{}
	if req.Metadata.Annotations != nil {
		maps.Copy(annotations, *req.Metadata.Annotations)
	}
	cleanAnnotations(annotations)

	crt := &openchoreov1alpha1.ClusterResourceType{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Metadata.Name,
			Annotations: annotations,
		},
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.ResourceTypeSpec, openchoreov1alpha1.ClusterResourceTypeSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		crt.Spec = spec
	}

	created, err := h.services.ClusterResourceTypeService.CreateClusterResourceType(ctx, crt)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) UpdateClusterResourceType(
	ctx context.Context, req *gen.UpdateClusterResourceTypeJSONRequestBody,
) (any, error) {
	existing, err := h.services.ClusterResourceTypeService.GetClusterResourceType(ctx, req.Metadata.Name)
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
		spec, err := convertSpec[gen.ResourceTypeSpec, openchoreov1alpha1.ClusterResourceTypeSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		existing.Spec = spec
	}

	updated, err := h.services.ClusterResourceTypeService.UpdateClusterResourceType(ctx, existing)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteClusterResourceType(ctx context.Context, crtName string) (any, error) {
	if err := h.services.ClusterResourceTypeService.DeleteClusterResourceType(ctx, crtName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":   crtName,
		"action": "deleted",
	}, nil
}
