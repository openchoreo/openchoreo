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

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.ComponentReleaseService.ListComponentReleases(ctx, request.NamespaceName, componentName, opts)
	if err != nil {
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.ListComponentReleases400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
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
	resp.Pagination = ToPagination(result)

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

// CreateComponentRelease creates a new component release within a namespace.
func (h *Handler) CreateComponentRelease(
	ctx context.Context,
	request gen.CreateComponentReleaseRequestObject,
) (gen.CreateComponentReleaseResponseObject, error) {
	h.logger.Info("CreateComponentRelease called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateComponentRelease400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	crCR, err := convert[gen.ComponentRelease, openchoreov1alpha1.ComponentRelease](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateComponentRelease400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	created, err := h.services.ComponentReleaseService.CreateComponentRelease(ctx, request.NamespaceName, &crCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateComponentRelease403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, componentreleasesvc.ErrComponentReleaseAlreadyExists) {
			return gen.CreateComponentRelease409JSONResponse{ConflictJSONResponse: conflict("ComponentRelease already exists")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.CreateComponentRelease400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to create component release", "error", err)
		return gen.CreateComponentRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCR, err := convert[openchoreov1alpha1.ComponentRelease, gen.ComponentRelease](*created)
	if err != nil {
		h.logger.Error("Failed to convert created component release", "error", err)
		return gen.CreateComponentRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ComponentRelease created successfully", "namespaceName", request.NamespaceName, "componentRelease", created.Name)
	return gen.CreateComponentRelease201JSONResponse(genCR), nil
}

// DeleteComponentRelease deletes a component release by name.
func (h *Handler) DeleteComponentRelease(
	ctx context.Context,
	request gen.DeleteComponentReleaseRequestObject,
) (gen.DeleteComponentReleaseResponseObject, error) {
	h.logger.Info("DeleteComponentRelease called", "namespaceName", request.NamespaceName, "componentReleaseName", request.ComponentReleaseName)

	err := h.services.ComponentReleaseService.DeleteComponentRelease(ctx, request.NamespaceName, request.ComponentReleaseName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteComponentRelease403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, componentreleasesvc.ErrComponentReleaseNotFound) {
			return gen.DeleteComponentRelease404JSONResponse{NotFoundJSONResponse: notFound("ComponentRelease")}, nil
		}
		h.logger.Error("Failed to delete component release", "error", err)
		return gen.DeleteComponentRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ComponentRelease deleted successfully", "namespaceName", request.NamespaceName, "componentRelease", request.ComponentReleaseName)
	return gen.DeleteComponentRelease204Response{}, nil
}
