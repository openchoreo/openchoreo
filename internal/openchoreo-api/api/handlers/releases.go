// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	releasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/release"
)

// ListReleases returns a paginated list of releases within a namespace.
func (h *Handler) ListReleases(
	ctx context.Context,
	request gen.ListReleasesRequestObject,
) (gen.ListReleasesResponseObject, error) {
	h.logger.Debug("ListReleases called", "namespaceName", request.NamespaceName)

	componentName := ""
	if request.Params.Component != nil {
		componentName = *request.Params.Component
	}

	environmentName := ""
	if request.Params.Environment != nil {
		environmentName = *request.Params.Environment
	}

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.releaseService.ListReleases(ctx, request.NamespaceName, componentName, environmentName, opts)
	if err != nil {
		h.logger.Error("Failed to list releases", "error", err)
		return gen.ListReleases500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.Release, gen.Release](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert releases", "error", err)
		return gen.ListReleases500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	resp := gen.ListReleases200JSONResponse{
		Items: items,
	}
	if p := ToPaginationPtr(result); p != nil {
		resp.Pagination = p
	}

	return resp, nil
}

// GetRelease returns details of a specific release.
func (h *Handler) GetRelease(
	ctx context.Context,
	request gen.GetReleaseRequestObject,
) (gen.GetReleaseResponseObject, error) {
	h.logger.Debug("GetRelease called", "namespaceName", request.NamespaceName, "releaseName", request.ReleaseName)

	r, err := h.releaseService.GetRelease(ctx, request.NamespaceName, request.ReleaseName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetRelease403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, releasesvc.ErrReleaseNotFound) {
			return gen.GetRelease404JSONResponse{NotFoundJSONResponse: notFound("Release")}, nil
		}
		h.logger.Error("Failed to get release", "error", err)
		return gen.GetRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRelease, err := convert[openchoreov1alpha1.Release, gen.Release](*r)
	if err != nil {
		h.logger.Error("Failed to convert release", "error", err)
		return gen.GetRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetRelease200JSONResponse(genRelease), nil
}
