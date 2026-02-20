// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	buildplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/buildplane"
)

// ListBuildPlanes returns a paginated list of build planes within a namespace.
func (h *Handler) ListBuildPlanes(
	ctx context.Context,
	request gen.ListBuildPlanesRequestObject,
) (gen.ListBuildPlanesResponseObject, error) {
	h.logger.Debug("ListBuildPlanes called", "namespaceName", request.NamespaceName)

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.buildPlaneService.ListBuildPlanes(ctx, request.NamespaceName, opts)
	if err != nil {
		h.logger.Error("Failed to list build planes", "error", err)
		return gen.ListBuildPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.BuildPlane, gen.BuildPlane](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert build planes", "error", err)
		return gen.ListBuildPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListBuildPlanes200JSONResponse{
		Items:      items,
		Pagination: ToPaginationPtr(result),
	}, nil
}

// GetBuildPlane returns details of a specific build plane.
func (h *Handler) GetBuildPlane(
	ctx context.Context,
	request gen.GetBuildPlaneRequestObject,
) (gen.GetBuildPlaneResponseObject, error) {
	h.logger.Debug("GetBuildPlane called", "namespaceName", request.NamespaceName, "buildPlaneName", request.BuildPlaneName)

	buildPlane, err := h.buildPlaneService.GetBuildPlane(ctx, request.NamespaceName, request.BuildPlaneName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetBuildPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, buildplanesvc.ErrBuildPlaneNotFound) {
			return gen.GetBuildPlane404JSONResponse{NotFoundJSONResponse: notFound("BuildPlane")}, nil
		}
		h.logger.Error("Failed to get build plane", "error", err)
		return gen.GetBuildPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genBuildPlane, err := convert[openchoreov1alpha1.BuildPlane, gen.BuildPlane](*buildPlane)
	if err != nil {
		h.logger.Error("Failed to convert build plane", "error", err)
		return gen.GetBuildPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetBuildPlane200JSONResponse(genBuildPlane), nil
}
