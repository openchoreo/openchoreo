// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"errors"
	"maps"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// ---------------------------------------------------------------------------
// ProjectType (namespace-scoped)
// ---------------------------------------------------------------------------

func (h *MCPHandler) ListProjectTypes(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.ProjectTypeService.ListProjectTypes(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("project_types", result.Items, result.NextCursor, projectTypeSummary), nil
}

func (h *MCPHandler) GetProjectType(ctx context.Context, namespaceName, ptName string) (any, error) {
	pt, err := h.services.ProjectTypeService.GetProjectType(ctx, namespaceName, ptName)
	if err != nil {
		return nil, err
	}
	return projectTypeDetail(pt), nil
}

func (h *MCPHandler) GetProjectTypeSchema(ctx context.Context, namespaceName, ptName string) (any, error) {
	return h.services.ProjectTypeService.GetProjectTypeSchema(ctx, namespaceName, ptName)
}

func (h *MCPHandler) CreateProjectType(
	ctx context.Context, namespaceName string, req *gen.CreateProjectTypeJSONRequestBody,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	annotations := map[string]string{}
	if req.Metadata.Annotations != nil {
		maps.Copy(annotations, *req.Metadata.Annotations)
	}
	cleanAnnotations(annotations)

	pt := &openchoreov1alpha1.ProjectType{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Metadata.Name,
			Namespace:   namespaceName,
			Annotations: annotations,
		},
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.ProjectTypeSpec, openchoreov1alpha1.ProjectTypeSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		pt.Spec = spec
	}

	created, err := h.services.ProjectTypeService.CreateProjectType(ctx, namespaceName, pt)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) UpdateProjectType(
	ctx context.Context, namespaceName string, req *gen.UpdateProjectTypeJSONRequestBody,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	existing, err := h.services.ProjectTypeService.GetProjectType(ctx, namespaceName, req.Metadata.Name)
	if err != nil {
		return nil, err
	}

	if req.Metadata.Annotations != nil {
		existing.Annotations = mergeAnnotations(existing.Annotations, *req.Metadata.Annotations)
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.ProjectTypeSpec, openchoreov1alpha1.ProjectTypeSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		existing.Spec = spec
	}

	updated, err := h.services.ProjectTypeService.UpdateProjectType(ctx, namespaceName, existing)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteProjectType(ctx context.Context, namespaceName, ptName string) (any, error) {
	if err := h.services.ProjectTypeService.DeleteProjectType(ctx, namespaceName, ptName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":      ptName,
		"namespace": namespaceName,
		"action":    "deleted",
	}, nil
}

// ---------------------------------------------------------------------------
// ClusterProjectType (cluster-scoped)
// ---------------------------------------------------------------------------

func (h *MCPHandler) ListClusterProjectTypes(ctx context.Context, opts tools.ListOpts) (any, error) {
	result, err := h.services.ClusterProjectTypeService.ListClusterProjectTypes(ctx, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList(
		"cluster_project_types", result.Items, result.NextCursor, clusterProjectTypeSummary,
	), nil
}

func (h *MCPHandler) GetClusterProjectType(ctx context.Context, cptName string) (any, error) {
	cpt, err := h.services.ClusterProjectTypeService.GetClusterProjectType(ctx, cptName)
	if err != nil {
		return nil, err
	}
	return clusterProjectTypeDetail(cpt), nil
}

func (h *MCPHandler) GetClusterProjectTypeSchema(ctx context.Context, cptName string) (any, error) {
	return h.services.ClusterProjectTypeService.GetClusterProjectTypeSchema(ctx, cptName)
}

func (h *MCPHandler) CreateClusterProjectType(
	ctx context.Context, req *gen.CreateClusterProjectTypeJSONRequestBody,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	annotations := map[string]string{}
	if req.Metadata.Annotations != nil {
		maps.Copy(annotations, *req.Metadata.Annotations)
	}
	cleanAnnotations(annotations)

	cpt := &openchoreov1alpha1.ClusterProjectType{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Metadata.Name,
			Annotations: annotations,
		},
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.ProjectTypeSpec, openchoreov1alpha1.ClusterProjectTypeSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		cpt.Spec = spec
	}

	created, err := h.services.ClusterProjectTypeService.CreateClusterProjectType(ctx, cpt)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) UpdateClusterProjectType(
	ctx context.Context, req *gen.UpdateClusterProjectTypeJSONRequestBody,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	existing, err := h.services.ClusterProjectTypeService.GetClusterProjectType(ctx, req.Metadata.Name)
	if err != nil {
		return nil, err
	}

	if req.Metadata.Annotations != nil {
		existing.Annotations = mergeAnnotations(existing.Annotations, *req.Metadata.Annotations)
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.ProjectTypeSpec, openchoreov1alpha1.ClusterProjectTypeSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		existing.Spec = spec
	}

	updated, err := h.services.ClusterProjectTypeService.UpdateClusterProjectType(ctx, existing)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteClusterProjectType(ctx context.Context, cptName string) (any, error) {
	if err := h.services.ClusterProjectTypeService.DeleteClusterProjectType(ctx, cptName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":   cptName,
		"action": "deleted",
	}, nil
}
