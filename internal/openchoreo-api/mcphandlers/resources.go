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
// Resource
// ---------------------------------------------------------------------------

func (h *MCPHandler) ListResources(
	ctx context.Context, namespaceName, projectName string, opts tools.ListOpts,
) (any, error) {
	result, err := h.services.ResourceService.ListResources(ctx, namespaceName, projectName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("resources", result.Items, result.NextCursor, resourceSummary), nil
}

func (h *MCPHandler) GetResource(ctx context.Context, namespaceName, resourceName string) (any, error) {
	r, err := h.services.ResourceService.GetResource(ctx, namespaceName, resourceName)
	if err != nil {
		return nil, err
	}
	return resourceDetail(r), nil
}

func (h *MCPHandler) CreateResource(
	ctx context.Context, namespaceName, projectName string,
	req *gen.CreateResourceJSONRequestBody,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	if req.Spec == nil {
		return nil, errors.New("spec is required")
	}

	annotations := map[string]string{}
	if req.Metadata.Annotations != nil {
		maps.Copy(annotations, *req.Metadata.Annotations)
	}
	cleanAnnotations(annotations)

	r := &openchoreov1alpha1.Resource{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Metadata.Name,
			Namespace:   namespaceName,
			Annotations: annotations,
		},
		Spec: openchoreov1alpha1.ResourceSpec{
			Owner: openchoreov1alpha1.ResourceOwner{
				ProjectName: projectName,
			},
			Type: openchoreov1alpha1.ResourceTypeRef{
				Name: req.Spec.Type.Name,
			},
		},
	}
	if req.Spec.Type.Kind != nil {
		r.Spec.Type.Kind = openchoreov1alpha1.ResourceTypeRefKind(*req.Spec.Type.Kind)
	}
	if req.Spec.Owner.ProjectName != "" && req.Spec.Owner.ProjectName != projectName {
		return nil, fmt.Errorf(
			"spec.owner.projectName (%s) must match projectName (%s)",
			req.Spec.Owner.ProjectName, projectName,
		)
	}
	if req.Spec.Parameters != nil {
		paramsBytes, err := json.Marshal(*req.Spec.Parameters)
		if err != nil {
			return nil, fmt.Errorf("marshal parameters: %w", err)
		}
		r.Spec.Parameters = &runtime.RawExtension{Raw: paramsBytes}
	}

	created, err := h.services.ResourceService.CreateResource(ctx, namespaceName, r)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created", map[string]any{
		"type": map[string]any{
			"kind": string(created.Spec.Type.Kind),
			"name": created.Spec.Type.Name,
		},
	}), nil
}

func (h *MCPHandler) UpdateResource(
	ctx context.Context, namespaceName string, req *gen.UpdateResourceJSONRequestBody,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}

	existing, err := h.services.ResourceService.GetResource(ctx, namespaceName, req.Metadata.Name)
	if err != nil {
		return nil, err
	}

	if req.Metadata.Annotations != nil {
		existing.Annotations = mergeAnnotations(existing.Annotations, *req.Metadata.Annotations)
	}
	if req.Spec != nil && req.Spec.Parameters != nil {
		paramsBytes, err := json.Marshal(*req.Spec.Parameters)
		if err != nil {
			return nil, fmt.Errorf("marshal parameters: %w", err)
		}
		existing.Spec.Parameters = &runtime.RawExtension{Raw: paramsBytes}
	}

	updated, err := h.services.ResourceService.UpdateResource(ctx, namespaceName, existing)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteResource(ctx context.Context, namespaceName, resourceName string) (any, error) {
	if err := h.services.ResourceService.DeleteResource(ctx, namespaceName, resourceName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":      resourceName,
		"namespace": namespaceName,
		"action":    "deleted",
	}, nil
}

// ---------------------------------------------------------------------------
// ResourceRelease
// ---------------------------------------------------------------------------

func (h *MCPHandler) ListResourceReleases(
	ctx context.Context, namespaceName, resourceName string, opts tools.ListOpts,
) (any, error) {
	result, err := h.services.ResourceReleaseService.ListResourceReleases(
		ctx, namespaceName, resourceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("resource_releases", result.Items, result.NextCursor, resourceReleaseSummary), nil
}

func (h *MCPHandler) GetResourceRelease(
	ctx context.Context, namespaceName, releaseName string,
) (any, error) {
	rr, err := h.services.ResourceReleaseService.GetResourceRelease(ctx, namespaceName, releaseName)
	if err != nil {
		return nil, err
	}
	return resourceReleaseDetail(rr), nil
}

func (h *MCPHandler) CreateResourceRelease(
	ctx context.Context, namespaceName string,
	req *gen.CreateResourceReleaseJSONRequestBody,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	rr, err := convertSpec[gen.ResourceRelease, openchoreov1alpha1.ResourceRelease](*req)
	if err != nil {
		return nil, err
	}
	rr.Namespace = namespaceName

	created, err := h.services.ResourceReleaseService.CreateResourceRelease(ctx, namespaceName, &rr)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) DeleteResourceRelease(
	ctx context.Context, namespaceName, resourceReleaseName string,
) (any, error) {
	if err := h.services.ResourceReleaseService.DeleteResourceRelease(ctx, namespaceName, resourceReleaseName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":      resourceReleaseName,
		"namespace": namespaceName,
		"action":    "deleted",
	}, nil
}

// ---------------------------------------------------------------------------
// ResourceReleaseBinding
// ---------------------------------------------------------------------------

func (h *MCPHandler) ListResourceReleaseBindings(
	ctx context.Context, namespaceName, resourceName string, opts tools.ListOpts,
) (any, error) {
	result, err := h.services.ResourceReleaseBindingService.ListResourceReleaseBindings(
		ctx, namespaceName, resourceName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList(
		"resource_release_bindings", result.Items, result.NextCursor, resourceReleaseBindingSummary,
	), nil
}

func (h *MCPHandler) GetResourceReleaseBinding(
	ctx context.Context, namespaceName, bindingName string,
) (any, error) {
	rb, err := h.services.ResourceReleaseBindingService.GetResourceReleaseBinding(ctx, namespaceName, bindingName)
	if err != nil {
		return nil, err
	}
	return resourceReleaseBindingDetail(rb), nil
}

func (h *MCPHandler) CreateResourceReleaseBinding(
	ctx context.Context, namespaceName string,
	req *gen.CreateResourceReleaseBindingJSONRequestBody,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	rb, err := convertSpec[gen.ResourceReleaseBinding, openchoreov1alpha1.ResourceReleaseBinding](*req)
	if err != nil {
		return nil, err
	}
	rb.Namespace = namespaceName

	created, err := h.services.ResourceReleaseBindingService.CreateResourceReleaseBinding(ctx, namespaceName, &rb)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) UpdateResourceReleaseBinding(
	ctx context.Context, namespaceName string,
	req *gen.UpdateResourceReleaseBindingJSONRequestBody,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	rb, err := convertSpec[gen.ResourceReleaseBinding, openchoreov1alpha1.ResourceReleaseBinding](*req)
	if err != nil {
		return nil, err
	}
	rb.Namespace = namespaceName

	updated, err := h.services.ResourceReleaseBindingService.UpdateResourceReleaseBinding(ctx, namespaceName, &rb)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteResourceReleaseBinding(
	ctx context.Context, namespaceName, bindingName string,
) (any, error) {
	if err := h.services.ResourceReleaseBindingService.DeleteResourceReleaseBinding(ctx, namespaceName, bindingName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":      bindingName,
		"namespace": namespaceName,
		"action":    "deleted",
	}, nil
}
