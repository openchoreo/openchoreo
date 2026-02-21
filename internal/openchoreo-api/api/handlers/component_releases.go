// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	componentreleasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componentrelease"
)

// ListComponentReleases returns a paginated list of component releases within a namespace.
func (h *Handler) ListComponentReleases(
	ctx context.Context,
	request gen.ListComponentReleasesRequestObject,
) (gen.ListComponentReleasesResponseObject, error) {
	h.logger.Debug("ListComponentReleases called", "namespaceName", request.NamespaceName)

	componentName := ""
	if request.Params.Component != nil {
		componentName = *request.Params.Component
	}

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.services.ComponentReleaseService.ListComponentReleases(ctx, request.NamespaceName, componentName, opts)
	if err != nil {
		h.logger.Error("Failed to list component releases", "error", err)
		return gen.ListComponentReleases500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ComponentRelease, gen.ComponentRelease](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert component releases", "error", err)
		return gen.ListComponentReleases500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	resp := gen.ListComponentReleases200JSONResponse{
		Items: items,
	}
	if p := ToPaginationPtr(result); p != nil {
		resp.Pagination = *p
	}

	return resp, nil
}

// GetComponentRelease returns details of a specific component release.
func (h *Handler) GetComponentRelease(
	ctx context.Context,
	request gen.GetComponentReleaseRequestObject,
) (gen.GetComponentReleaseResponseObject, error) {
	h.logger.Debug("GetComponentRelease called", "namespaceName", request.NamespaceName, "componentReleaseName", request.ComponentReleaseName)

	cr, err := h.services.ComponentReleaseService.GetComponentRelease(ctx, request.NamespaceName, request.ComponentReleaseName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetComponentRelease403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, componentreleasesvc.ErrComponentReleaseNotFound) {
			return gen.GetComponentRelease404JSONResponse{NotFoundJSONResponse: notFound("ComponentRelease")}, nil
		}
		h.logger.Error("Failed to get component release", "error", err)
		return gen.GetComponentRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCR, err := convert[openchoreov1alpha1.ComponentRelease, gen.ComponentRelease](*cr)
	if err != nil {
		h.logger.Error("Failed to convert component release", "error", err)
		return gen.GetComponentRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetComponentRelease200JSONResponse(genCR), nil
}
