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
// ProjectRelease
// ---------------------------------------------------------------------------

func (h *MCPHandler) ListProjectReleases(
	ctx context.Context, namespaceName, projectName string, opts tools.ListOpts,
) (any, error) {
	result, err := h.services.ProjectReleaseService.ListProjectReleases(
		ctx, namespaceName, projectName, toServiceListOptions(opts))
	if err != nil {
		return nil, err
	}
	return wrapTransformedList("project_releases", result.Items, result.NextCursor, projectReleaseSummary), nil
}

func (h *MCPHandler) GetProjectRelease(
	ctx context.Context, namespaceName, releaseName string,
) (any, error) {
	pr, err := h.services.ProjectReleaseService.GetProjectRelease(ctx, namespaceName, releaseName)
	if err != nil {
		return nil, err
	}
	return projectReleaseDetail(pr), nil
}

func (h *MCPHandler) CreateProjectRelease(
	ctx context.Context, namespaceName string,
	req *gen.CreateProjectReleaseJSONRequestBody,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	pr, err := convertSpec[gen.ProjectRelease, openchoreov1alpha1.ProjectRelease](*req)
	if err != nil {
		return nil, err
	}
	pr.Namespace = namespaceName

	created, err := h.services.ProjectReleaseService.CreateProjectRelease(ctx, namespaceName, &pr)
	if err != nil {
		return nil, err
	}
	return mutationResult(created, "created"), nil
}

func (h *MCPHandler) DeleteProjectRelease(
	ctx context.Context, namespaceName, projectReleaseName string,
) (any, error) {
	if err := h.services.ProjectReleaseService.DeleteProjectRelease(ctx, namespaceName, projectReleaseName); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":      projectReleaseName,
		"namespace": namespaceName,
		"action":    "deleted",
	}, nil
}
