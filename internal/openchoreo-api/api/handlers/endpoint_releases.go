// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	endpointreleasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointrelease"
)

// ListEndpointReleases returns a paginated list of endpoint releases within a namespace.
func (h *Handler) ListEndpointReleases(
	ctx context.Context,
	request gen.ListEndpointReleasesRequestObject,
) (gen.ListEndpointReleasesResponseObject, error) {
	h.logger.Debug("ListEndpointReleases called", "namespaceName", request.NamespaceName)

	endpointName := ""
	if request.Params.Endpoint != nil {
		endpointName = *request.Params.Endpoint
	}

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.EndpointReleaseService.ListEndpointReleases(ctx, request.NamespaceName, endpointName, opts)
	if err != nil {
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.ListEndpointReleases400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to list endpoint releases", "error", err)
		return gen.ListEndpointReleases500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.EndpointRelease, gen.EndpointRelease](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert endpoint releases", "error", err)
		return gen.ListEndpointReleases500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	resp := gen.ListEndpointReleases200JSONResponse{
		Items: items,
	}
	resp.Pagination = ToPagination(result)

	return resp, nil
}

// GetEndpointRelease returns details of a specific endpoint release.
func (h *Handler) GetEndpointRelease(
	ctx context.Context,
	request gen.GetEndpointReleaseRequestObject,
) (gen.GetEndpointReleaseResponseObject, error) {
	h.logger.Debug("GetEndpointRelease called", "namespaceName", request.NamespaceName, "endpointReleaseName", request.EndpointReleaseName)

	rr, err := h.services.EndpointReleaseService.GetEndpointRelease(ctx, request.NamespaceName, request.EndpointReleaseName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetEndpointRelease403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, endpointreleasesvc.ErrEndpointReleaseNotFound) {
			return gen.GetEndpointRelease404JSONResponse{NotFoundJSONResponse: notFound("EndpointRelease")}, nil
		}
		h.logger.Error("Failed to get endpoint release", "error", err)
		return gen.GetEndpointRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRR, err := convert[openchoreov1alpha1.EndpointRelease, gen.EndpointRelease](*rr)
	if err != nil {
		h.logger.Error("Failed to convert endpoint release", "error", err)
		return gen.GetEndpointRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetEndpointRelease200JSONResponse(genRR), nil
}

// DeleteEndpointRelease deletes a endpoint release by name.
func (h *Handler) DeleteEndpointRelease(
	ctx context.Context,
	request gen.DeleteEndpointReleaseRequestObject,
) (gen.DeleteEndpointReleaseResponseObject, error) {
	h.logger.Info("DeleteEndpointRelease called", "namespaceName", request.NamespaceName, "endpointReleaseName", request.EndpointReleaseName)

	err := h.services.EndpointReleaseService.DeleteEndpointRelease(ctx, request.NamespaceName, request.EndpointReleaseName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteEndpointRelease403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, endpointreleasesvc.ErrEndpointReleaseNotFound) {
			return gen.DeleteEndpointRelease404JSONResponse{NotFoundJSONResponse: notFound("EndpointRelease")}, nil
		}
		h.logger.Error("Failed to delete endpoint release", "error", err)
		return gen.DeleteEndpointRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("EndpointRelease deleted successfully", "namespaceName", request.NamespaceName, "endpointRelease", request.EndpointReleaseName)
	return gen.DeleteEndpointRelease204Response{}, nil
}
