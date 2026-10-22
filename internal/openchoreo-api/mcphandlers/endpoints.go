// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// ---------------------------------------------------------------------------
// Endpoint
// ---------------------------------------------------------------------------

func (h *MCPHandler) ListEndpoints(
	ctx context.Context, namespaceName, projectName string, opts tools.ListOpts,
) (any, error) {
	result, err := h.services.EndpointService.ListEndpoints(ctx, namespaceName, projectName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("endpoints", result.Items, result.NextCursor, endpointSummary), nil
}

func (h *MCPHandler) GetEndpoint(ctx context.Context, namespaceName, endpointName string) (any, error) {
	ep, err := h.services.EndpointService.GetEndpoint(ctx, namespaceName, endpointName)
	if err != nil {
		return nil, err
	}
	return endpointDetail(ep), nil
}

func (h *MCPHandler) CreateEndpoint(
	ctx context.Context, namespaceName, projectName string,
	req *gen.CreateEndpointJSONRequestBody,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	if req.Spec == nil {
		return nil, errors.New("spec is required")
	}
	if req.Spec.Owner.ProjectName != "" && req.Spec.Owner.ProjectName != projectName {
		return nil, fmt.Errorf(
			"spec.owner.projectName (%s) must match projectName (%s)",
			req.Spec.Owner.ProjectName, projectName,
		)
	}

	spec, err := convertSpec[gen.EndpointSpec, openchoreov1alpha1.EndpointSpec](*req.Spec)
	if err != nil {
		return nil, err
	}
	spec.Owner.ProjectName = projectName

	annotations := map[string]string{}
	if req.Metadata.Annotations != nil {
		maps.Copy(annotations, *req.Metadata.Annotations)
	}
	cleanAnnotations(annotations)

	ep := &openchoreov1alpha1.Endpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Metadata.Name,
			Namespace:   namespaceName,
			Annotations: annotations,
		},
		Spec: spec,
	}

	created, err := h.services.EndpointService.CreateEndpoint(ctx, namespaceName, ep)
	if err != nil {
		return nil, err
	}
	setAuditResource(ctx, created)
	return mutationResult(created, "created", map[string]any{
		"type": map[string]any{
			"kind": string(created.Spec.Type.Kind),
			"name": created.Spec.Type.Name,
		},
	}), nil
}

// UpdateEndpoint applies the provided fields onto the stored Endpoint, so the
// immutable spec.owner and spec.type, and anything the caller did not send,
// are preserved.
func (h *MCPHandler) UpdateEndpoint(
	ctx context.Context, namespaceName string, req *gen.UpdateEndpointJSONRequestBody,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}

	existing, err := h.services.EndpointService.GetEndpoint(ctx, namespaceName, req.Metadata.Name)
	if err != nil {
		return nil, err
	}

	if req.Metadata.Annotations != nil {
		existing.Annotations = mergeAnnotations(existing.Annotations, *req.Metadata.Annotations)
	}
	if req.Spec != nil {
		if req.Spec.Visibility != nil {
			visibility := make([]openchoreov1alpha1.EndpointVisibility, 0, len(req.Spec.Visibility))
			for _, v := range req.Spec.Visibility {
				visibility = append(visibility, openchoreov1alpha1.EndpointVisibility(v))
			}
			existing.Spec.Visibility = visibility
		}
		if req.Spec.BackendRefs != nil {
			backends, err := convertSpec[[]gen.EndpointBackendRef, []openchoreov1alpha1.EndpointBackendRef](
				req.Spec.BackendRefs)
			if err != nil {
				return nil, err
			}
			existing.Spec.BackendRefs = backends
		}
		if req.Spec.AutoDeploy != nil {
			existing.Spec.AutoDeploy = *req.Spec.AutoDeploy
		}
		if req.Spec.Parameters != nil {
			paramsBytes, err := json.Marshal(*req.Spec.Parameters)
			if err != nil {
				return nil, fmt.Errorf("marshal parameters: %w", err)
			}
			existing.Spec.Parameters = &runtime.RawExtension{Raw: paramsBytes}
		}
	}

	updated, err := h.services.EndpointService.UpdateEndpoint(ctx, namespaceName, existing)
	if err != nil {
		return nil, err
	}
	setAuditResource(ctx, updated)
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteEndpoint(ctx context.Context, namespaceName, endpointName string) (any, error) {
	if err := h.services.EndpointService.DeleteEndpoint(ctx, namespaceName, endpointName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":      endpointName,
		"namespace": namespaceName,
		"action":    "deleted",
	}, nil
}

// ---------------------------------------------------------------------------
// EndpointType (namespace-scoped)
// ---------------------------------------------------------------------------

func (h *MCPHandler) ListEndpointTypes(ctx context.Context, namespaceName string, opts tools.ListOpts) (any, error) {
	result, err := h.services.EndpointTypeService.ListEndpointTypes(ctx, namespaceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("endpoint_types", result.Items, result.NextCursor, endpointTypeSummary), nil
}

func (h *MCPHandler) GetEndpointType(ctx context.Context, namespaceName, etName string) (any, error) {
	et, err := h.services.EndpointTypeService.GetEndpointType(ctx, namespaceName, etName)
	if err != nil {
		return nil, err
	}
	return endpointTypeDetail(et), nil
}

func (h *MCPHandler) GetEndpointTypeSchema(ctx context.Context, namespaceName, etName string) (any, error) {
	return h.services.EndpointTypeService.GetEndpointTypeSchema(ctx, namespaceName, etName)
}

func (h *MCPHandler) CreateEndpointType(
	ctx context.Context, namespaceName string, req *gen.CreateEndpointTypeJSONRequestBody,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	annotations := map[string]string{}
	if req.Metadata.Annotations != nil {
		maps.Copy(annotations, *req.Metadata.Annotations)
	}
	cleanAnnotations(annotations)

	et := &openchoreov1alpha1.EndpointType{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Metadata.Name,
			Namespace:   namespaceName,
			Annotations: annotations,
		},
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.EndpointTypeSpec, openchoreov1alpha1.EndpointTypeSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		et.Spec = spec
	}

	created, err := h.services.EndpointTypeService.CreateEndpointType(ctx, namespaceName, et)
	if err != nil {
		return nil, err
	}
	setAuditResource(ctx, created)
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) UpdateEndpointType(
	ctx context.Context, namespaceName string, req *gen.UpdateEndpointTypeJSONRequestBody,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	existing, err := h.services.EndpointTypeService.GetEndpointType(ctx, namespaceName, req.Metadata.Name)
	if err != nil {
		return nil, err
	}

	if req.Metadata.Annotations != nil {
		existing.Annotations = mergeAnnotations(existing.Annotations, *req.Metadata.Annotations)
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.EndpointTypeSpec, openchoreov1alpha1.EndpointTypeSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		existing.Spec = spec
	}

	updated, err := h.services.EndpointTypeService.UpdateEndpointType(ctx, namespaceName, existing)
	if err != nil {
		return nil, err
	}
	setAuditResource(ctx, updated)
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteEndpointType(ctx context.Context, namespaceName, etName string) (any, error) {
	if err := h.services.EndpointTypeService.DeleteEndpointType(ctx, namespaceName, etName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":      etName,
		"namespace": namespaceName,
		"action":    "deleted",
	}, nil
}

// ---------------------------------------------------------------------------
// ClusterEndpointType (cluster-scoped)
// ---------------------------------------------------------------------------

func (h *MCPHandler) ListClusterEndpointTypes(ctx context.Context, opts tools.ListOpts) (any, error) {
	result, err := h.services.ClusterEndpointTypeService.ListClusterEndpointTypes(ctx, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("cluster_endpoint_types", result.Items, result.NextCursor, clusterEndpointTypeSummary), nil
}

func (h *MCPHandler) GetClusterEndpointType(ctx context.Context, cetName string) (any, error) {
	cet, err := h.services.ClusterEndpointTypeService.GetClusterEndpointType(ctx, cetName)
	if err != nil {
		return nil, err
	}
	return clusterEndpointTypeDetail(cet), nil
}

func (h *MCPHandler) GetClusterEndpointTypeSchema(ctx context.Context, cetName string) (any, error) {
	return h.services.ClusterEndpointTypeService.GetClusterEndpointTypeSchema(ctx, cetName)
}

func (h *MCPHandler) CreateClusterEndpointType(
	ctx context.Context, req *gen.CreateClusterEndpointTypeJSONRequestBody,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	annotations := map[string]string{}
	if req.Metadata.Annotations != nil {
		maps.Copy(annotations, *req.Metadata.Annotations)
	}
	cleanAnnotations(annotations)

	cet := &openchoreov1alpha1.ClusterEndpointType{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Metadata.Name,
			Annotations: annotations,
		},
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.EndpointTypeSpec, openchoreov1alpha1.ClusterEndpointTypeSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		cet.Spec = spec
	}

	created, err := h.services.ClusterEndpointTypeService.CreateClusterEndpointType(ctx, cet)
	if err != nil {
		return nil, err
	}
	setAuditResource(ctx, created)
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) UpdateClusterEndpointType(
	ctx context.Context, req *gen.UpdateClusterEndpointTypeJSONRequestBody,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	existing, err := h.services.ClusterEndpointTypeService.GetClusterEndpointType(ctx, req.Metadata.Name)
	if err != nil {
		return nil, err
	}

	if req.Metadata.Annotations != nil {
		existing.Annotations = mergeAnnotations(existing.Annotations, *req.Metadata.Annotations)
	}
	if req.Spec != nil {
		spec, err := convertSpec[gen.EndpointTypeSpec, openchoreov1alpha1.ClusterEndpointTypeSpec](*req.Spec)
		if err != nil {
			return nil, err
		}
		existing.Spec = spec
	}

	updated, err := h.services.ClusterEndpointTypeService.UpdateClusterEndpointType(ctx, existing)
	if err != nil {
		return nil, err
	}
	setAuditResource(ctx, updated)
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteClusterEndpointType(ctx context.Context, cetName string) (any, error) {
	if err := h.services.ClusterEndpointTypeService.DeleteClusterEndpointType(ctx, cetName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":   cetName,
		"action": "deleted",
	}, nil
}

// ---------------------------------------------------------------------------
// EndpointRelease
// ---------------------------------------------------------------------------

func (h *MCPHandler) ListEndpointReleases(
	ctx context.Context, namespaceName, endpointName string, opts tools.ListOpts,
) (any, error) {
	result, err := h.services.EndpointReleaseService.ListEndpointReleases(
		ctx, namespaceName, endpointName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("endpoint_releases", result.Items, result.NextCursor, endpointReleaseSummary), nil
}

func (h *MCPHandler) GetEndpointRelease(ctx context.Context, namespaceName, releaseName string) (any, error) {
	er, err := h.services.EndpointReleaseService.GetEndpointRelease(ctx, namespaceName, releaseName)
	if err != nil {
		return nil, err
	}
	return endpointReleaseDetail(er), nil
}

func (h *MCPHandler) DeleteEndpointRelease(
	ctx context.Context, namespaceName, endpointReleaseName string,
) (any, error) {
	if err := h.services.EndpointReleaseService.DeleteEndpointRelease(ctx, namespaceName, endpointReleaseName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":      endpointReleaseName,
		"namespace": namespaceName,
		"action":    "deleted",
	}, nil
}

// ---------------------------------------------------------------------------
// EndpointReleaseBinding
// ---------------------------------------------------------------------------

func (h *MCPHandler) ListEndpointReleaseBindings(
	ctx context.Context, namespaceName, endpointName string, opts tools.ListOpts,
) (any, error) {
	result, err := h.services.EndpointReleaseBindingService.ListEndpointReleaseBindings(
		ctx, namespaceName, endpointName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList(
		"endpoint_release_bindings", result.Items, result.NextCursor, endpointReleaseBindingSummary,
	), nil
}

func (h *MCPHandler) GetEndpointReleaseBinding(
	ctx context.Context, namespaceName, bindingName string,
) (any, error) {
	b, err := h.services.EndpointReleaseBindingService.GetEndpointReleaseBinding(ctx, namespaceName, bindingName)
	if err != nil {
		return nil, err
	}
	return endpointReleaseBindingDetail(b), nil
}

func (h *MCPHandler) CreateEndpointReleaseBinding(
	ctx context.Context, namespaceName string,
	req *gen.CreateEndpointReleaseBindingJSONRequestBody,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	b, err := convertSpec[gen.EndpointReleaseBinding, openchoreov1alpha1.EndpointReleaseBinding](*req)
	if err != nil {
		return nil, err
	}
	b.Namespace = namespaceName

	created, err := h.services.EndpointReleaseBindingService.CreateEndpointReleaseBinding(ctx, namespaceName, &b)
	if err != nil {
		return nil, err
	}
	setAuditResource(ctx, created)
	return mutationResult(created, "created"), nil
}

// UpdateEndpointReleaseBinding applies the provided mutable fields onto the
// stored binding so the immutable spec.owner and spec.environment are kept,
// rather than being submitted empty and rejected (mirrors the resource binding).
func (h *MCPHandler) UpdateEndpointReleaseBinding(
	ctx context.Context, namespaceName string,
	req *gen.UpdateEndpointReleaseBindingJSONRequestBody,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	existing, err := h.services.EndpointReleaseBindingService.GetEndpointReleaseBinding(
		ctx, namespaceName, req.Metadata.Name)
	if err != nil {
		return nil, err
	}

	if req.Spec != nil {
		if req.Spec.EndpointRelease != nil {
			existing.Spec.EndpointRelease = *req.Spec.EndpointRelease
		}
		if req.Spec.EndpointTypeEnvironmentConfigs != nil {
			raw, mErr := json.Marshal(*req.Spec.EndpointTypeEnvironmentConfigs)
			if mErr != nil {
				return nil, fmt.Errorf("marshal endpointTypeEnvironmentConfigs: %w", mErr)
			}
			existing.Spec.EndpointTypeEnvironmentConfigs = &runtime.RawExtension{Raw: raw}
		}
	}

	updated, err := h.services.EndpointReleaseBindingService.UpdateEndpointReleaseBinding(ctx, namespaceName, existing)
	if err != nil {
		return nil, err
	}
	setAuditResource(ctx, updated)
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteEndpointReleaseBinding(
	ctx context.Context, namespaceName, bindingName string,
) (any, error) {
	if err := h.services.EndpointReleaseBindingService.DeleteEndpointReleaseBinding(ctx, namespaceName, bindingName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":      bindingName,
		"namespace": namespaceName,
		"action":    "deleted",
	}, nil
}
