// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	resourcereleasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcerelease"
)

// ListResourceReleases returns a paginated list of resource releases within a namespace.
func (h *Handler) ListResourceReleases(
	ctx context.Context,
	request gen.ListResourceReleasesRequestObject,
) (gen.ListResourceReleasesResponseObject, error) {
	h.logger.Debug("ListResourceReleases called", "namespaceName", request.NamespaceName)

	resourceName := ""
	if request.Params.Resource != nil {
		resourceName = *request.Params.Resource
	}

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.ResourceReleaseService.ListResourceReleases(ctx, request.NamespaceName, resourceName, opts)
	if err != nil {
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.ListResourceReleases400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to list resource releases", "error", err)
		return gen.ListResourceReleases500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ResourceRelease, gen.ResourceRelease](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert resource releases", "error", err)
		return gen.ListResourceReleases500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	resp := gen.ListResourceReleases200JSONResponse{
		Items: items,
	}
	resp.Pagination = ToPagination(result)

	return resp, nil
}

// GetResourceRelease returns details of a specific resource release.
func (h *Handler) GetResourceRelease(
	ctx context.Context,
	request gen.GetResourceReleaseRequestObject,
) (gen.GetResourceReleaseResponseObject, error) {
	h.logger.Debug("GetResourceRelease called", "namespaceName", request.NamespaceName, "resourceReleaseName", request.ResourceReleaseName)

	rr, err := h.services.ResourceReleaseService.GetResourceRelease(ctx, request.NamespaceName, request.ResourceReleaseName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetResourceRelease403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, resourcereleasesvc.ErrResourceReleaseNotFound) {
			return gen.GetResourceRelease404JSONResponse{NotFoundJSONResponse: notFound("ResourceRelease")}, nil
		}
		h.logger.Error("Failed to get resource release", "error", err)
		return gen.GetResourceRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRR, err := convert[openchoreov1alpha1.ResourceRelease, gen.ResourceRelease](*rr)
	if err != nil {
		h.logger.Error("Failed to convert resource release", "error", err)
		return gen.GetResourceRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetResourceRelease200JSONResponse(genRR), nil
}

// CreateResourceRelease creates a new resource release within a namespace.
func (h *Handler) CreateResourceRelease(
	ctx context.Context,
	request gen.CreateResourceReleaseRequestObject,
) (gen.CreateResourceReleaseResponseObject, error) {
	h.logger.Info("CreateResourceRelease called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateResourceRelease400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	rrCR, err := convert[gen.ResourceRelease, openchoreov1alpha1.ResourceRelease](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateResourceRelease400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	created, err := h.services.ResourceReleaseService.CreateResourceRelease(ctx, request.NamespaceName, &rrCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateResourceRelease403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, resourcereleasesvc.ErrResourceReleaseAlreadyExists) {
			return gen.CreateResourceRelease409JSONResponse{ConflictJSONResponse: conflict("ResourceRelease already exists")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.CreateResourceRelease400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to create resource release", "error", err)
		return gen.CreateResourceRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRR, err := convert[openchoreov1alpha1.ResourceRelease, gen.ResourceRelease](*created)
	if err != nil {
		h.logger.Error("Failed to convert created resource release", "error", err)
		return gen.CreateResourceRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ResourceRelease created successfully", "namespaceName", request.NamespaceName, "resourceRelease", created.Name)
	return gen.CreateResourceRelease201JSONResponse(genRR), nil
}

// DeleteResourceRelease deletes a resource release by name.
func (h *Handler) DeleteResourceRelease(
	ctx context.Context,
	request gen.DeleteResourceReleaseRequestObject,
) (gen.DeleteResourceReleaseResponseObject, error) {
	h.logger.Info("DeleteResourceRelease called", "namespaceName", request.NamespaceName, "resourceReleaseName", request.ResourceReleaseName)

	err := h.services.ResourceReleaseService.DeleteResourceRelease(ctx, request.NamespaceName, request.ResourceReleaseName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteResourceRelease403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, resourcereleasesvc.ErrResourceReleaseNotFound) {
			return gen.DeleteResourceRelease404JSONResponse{NotFoundJSONResponse: notFound("ResourceRelease")}, nil
		}
		h.logger.Error("Failed to delete resource release", "error", err)
		return gen.DeleteResourceRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ResourceRelease deleted successfully", "namespaceName", request.NamespaceName, "resourceRelease", request.ResourceReleaseName)
	return gen.DeleteResourceRelease204Response{}, nil
}
