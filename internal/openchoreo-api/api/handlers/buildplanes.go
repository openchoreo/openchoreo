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

// UpdateBuildPlane replaces an existing build plane.
func (h *Handler) UpdateBuildPlane(
	ctx context.Context,
	request gen.UpdateBuildPlaneRequestObject,
) (gen.UpdateBuildPlaneResponseObject, error) {
	h.logger.Info("UpdateBuildPlane called", "namespaceName", request.NamespaceName, "buildPlaneName", request.BuildPlaneName)

	if request.Body == nil {
		return gen.UpdateBuildPlane400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	bpCR, err := convert[gen.BuildPlane, openchoreov1alpha1.BuildPlane](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateBuildPlane400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	bpCR.Status = openchoreov1alpha1.BuildPlaneStatus{}

	// Ensure the name from the URL path is used
	bpCR.Name = request.BuildPlaneName

	updated, err := h.buildPlaneService.UpdateBuildPlane(ctx, request.NamespaceName, &bpCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateBuildPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, buildplanesvc.ErrBuildPlaneNotFound) {
			return gen.UpdateBuildPlane404JSONResponse{NotFoundJSONResponse: notFound("BuildPlane")}, nil
		}
		h.logger.Error("Failed to update build plane", "error", err)
		return gen.UpdateBuildPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genBP, err := convert[openchoreov1alpha1.BuildPlane, gen.BuildPlane](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated build plane", "error", err)
		return gen.UpdateBuildPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("BuildPlane updated successfully", "namespaceName", request.NamespaceName, "buildPlane", updated.Name)
	return gen.UpdateBuildPlane200JSONResponse(genBP), nil
}

// DeleteBuildPlane deletes a build plane by name.
func (h *Handler) DeleteBuildPlane(
	ctx context.Context,
	request gen.DeleteBuildPlaneRequestObject,
) (gen.DeleteBuildPlaneResponseObject, error) {
	h.logger.Info("DeleteBuildPlane called", "namespaceName", request.NamespaceName, "buildPlaneName", request.BuildPlaneName)

	err := h.buildPlaneService.DeleteBuildPlane(ctx, request.NamespaceName, request.BuildPlaneName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteBuildPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, buildplanesvc.ErrBuildPlaneNotFound) {
			return gen.DeleteBuildPlane404JSONResponse{NotFoundJSONResponse: notFound("BuildPlane")}, nil
		}
		h.logger.Error("Failed to delete build plane", "error", err)
		return gen.DeleteBuildPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("BuildPlane deleted successfully", "namespaceName", request.NamespaceName, "buildPlane", request.BuildPlaneName)
	return gen.DeleteBuildPlane204Response{}, nil
}
