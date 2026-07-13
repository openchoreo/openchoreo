// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

// ---------------------------------------------------------------------------
// ProjectReleaseBinding
// ---------------------------------------------------------------------------

func (h *MCPHandler) ListProjectReleaseBindings(
	ctx context.Context, namespaceName, projectName string, opts tools.ListOpts,
) (any, error) {
	result, err := h.services.ProjectReleaseBindingService.ListProjectReleaseBindings(
		ctx, namespaceName, projectName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList(
		"project_release_bindings", result.Items, result.NextCursor, projectReleaseBindingSummary,
	), nil
}

func (h *MCPHandler) GetProjectReleaseBinding(
	ctx context.Context, namespaceName, bindingName string,
) (any, error) {
	rb, err := h.services.ProjectReleaseBindingService.GetProjectReleaseBinding(ctx, namespaceName, bindingName)
	if err != nil {
		return nil, err
	}
	return projectReleaseBindingDetail(rb), nil
}

func (h *MCPHandler) CreateProjectReleaseBinding(
	ctx context.Context, namespaceName string,
	req *gen.CreateProjectReleaseBindingJSONRequestBody,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	rb, err := convertSpec[gen.ProjectReleaseBinding, openchoreov1alpha1.ProjectReleaseBinding](*req)
	if err != nil {
		return nil, err
	}
	rb.Namespace = namespaceName

	created, err := h.services.ProjectReleaseBindingService.CreateProjectReleaseBinding(ctx, namespaceName, &rb)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) UpdateProjectReleaseBinding(
	ctx context.Context, namespaceName string,
	req *gen.UpdateProjectReleaseBindingJSONRequestBody,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	rb, err := convertSpec[gen.ProjectReleaseBinding, openchoreov1alpha1.ProjectReleaseBinding](*req)
	if err != nil {
		return nil, err
	}
	rb.Namespace = namespaceName

	updated, err := h.services.ProjectReleaseBindingService.UpdateProjectReleaseBinding(ctx, namespaceName, &rb)
	if err != nil {
		return nil, err
	}
	return mutationResult(updated, "updated"), nil
}

func (h *MCPHandler) DeleteProjectReleaseBinding(
	ctx context.Context, namespaceName, bindingName string,
) (any, error) {
	if err := h.services.ProjectReleaseBindingService.DeleteProjectReleaseBinding(ctx, namespaceName, bindingName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":      bindingName,
		"namespace": namespaceName,
		"action":    "deleted",
	}, nil
}
