// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	clusterbuildplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterbuildplane"
)

// ListClusterBuildPlanes returns a paginated list of cluster-scoped build planes.
func (h *Handler) ListClusterBuildPlanes(
	ctx context.Context,
	request gen.ListClusterBuildPlanesRequestObject,
) (gen.ListClusterBuildPlanesResponseObject, error) {
	h.logger.Debug("ListClusterBuildPlanes called")

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.clusterBuildPlaneService.ListClusterBuildPlanes(ctx, opts)
	if err != nil {
		h.logger.Error("Failed to list cluster build planes", "error", err)
		return gen.ListClusterBuildPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ClusterBuildPlane, gen.ClusterBuildPlane](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert cluster build planes", "error", err)
		return gen.ListClusterBuildPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListClusterBuildPlanes200JSONResponse{
		Items:      items,
		Pagination: ToPaginationPtr(result),
	}, nil
}

// GetClusterBuildPlane returns details of a specific cluster-scoped build plane.
func (h *Handler) GetClusterBuildPlane(
	ctx context.Context,
	request gen.GetClusterBuildPlaneRequestObject,
) (gen.GetClusterBuildPlaneResponseObject, error) {
	h.logger.Debug("GetClusterBuildPlane called", "clusterBuildPlaneName", request.ClusterBuildPlaneName)

	cbp, err := h.clusterBuildPlaneService.GetClusterBuildPlane(ctx, request.ClusterBuildPlaneName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetClusterBuildPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterbuildplanesvc.ErrClusterBuildPlaneNotFound) {
			return gen.GetClusterBuildPlane404JSONResponse{NotFoundJSONResponse: notFound("ClusterBuildPlane")}, nil
		}
		h.logger.Error("Failed to get cluster build plane", "error", err)
		return gen.GetClusterBuildPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCBP, err := convert[openchoreov1alpha1.ClusterBuildPlane, gen.ClusterBuildPlane](*cbp)
	if err != nil {
		h.logger.Error("Failed to convert cluster build plane", "error", err)
		return gen.GetClusterBuildPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetClusterBuildPlane200JSONResponse(genCBP), nil
}

// CreateClusterBuildPlane creates a new cluster-scoped build plane.
func (h *Handler) CreateClusterBuildPlane(
	ctx context.Context,
	request gen.CreateClusterBuildPlaneRequestObject,
) (gen.CreateClusterBuildPlaneResponseObject, error) {
	h.logger.Info("CreateClusterBuildPlane called")

	if request.Body == nil {
		return gen.CreateClusterBuildPlane400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	cbpCR, err := convert[gen.ClusterBuildPlane, openchoreov1alpha1.ClusterBuildPlane](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateClusterBuildPlane400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	cbpCR.Status = openchoreov1alpha1.ClusterBuildPlaneStatus{}

	created, err := h.clusterBuildPlaneService.CreateClusterBuildPlane(ctx, &cbpCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateClusterBuildPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterbuildplanesvc.ErrClusterBuildPlaneAlreadyExists) {
			return gen.CreateClusterBuildPlane409JSONResponse{ConflictJSONResponse: conflict("ClusterBuildPlane already exists")}, nil
		}
		h.logger.Error("Failed to create cluster build plane", "error", err)
		return gen.CreateClusterBuildPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCBP, err := convert[openchoreov1alpha1.ClusterBuildPlane, gen.ClusterBuildPlane](*created)
	if err != nil {
		h.logger.Error("Failed to convert created cluster build plane", "error", err)
		return gen.CreateClusterBuildPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ClusterBuildPlane created successfully", "clusterBuildPlane", created.Name)
	return gen.CreateClusterBuildPlane201JSONResponse(genCBP), nil
}

// UpdateClusterBuildPlane replaces an existing cluster-scoped build plane (full update).
func (h *Handler) UpdateClusterBuildPlane(
	ctx context.Context,
	request gen.UpdateClusterBuildPlaneRequestObject,
) (gen.UpdateClusterBuildPlaneResponseObject, error) {
	h.logger.Info("UpdateClusterBuildPlane called", "clusterBuildPlaneName", request.ClusterBuildPlaneName)

	if request.Body == nil {
		return gen.UpdateClusterBuildPlane400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	cbpCR, err := convert[gen.ClusterBuildPlane, openchoreov1alpha1.ClusterBuildPlane](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateClusterBuildPlane400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	cbpCR.Status = openchoreov1alpha1.ClusterBuildPlaneStatus{}

	// Ensure the name from the URL path is used
	cbpCR.Name = request.ClusterBuildPlaneName

	updated, err := h.clusterBuildPlaneService.UpdateClusterBuildPlane(ctx, &cbpCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateClusterBuildPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterbuildplanesvc.ErrClusterBuildPlaneNotFound) {
			return gen.UpdateClusterBuildPlane404JSONResponse{NotFoundJSONResponse: notFound("ClusterBuildPlane")}, nil
		}
		h.logger.Error("Failed to update cluster build plane", "error", err)
		return gen.UpdateClusterBuildPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCBP, err := convert[openchoreov1alpha1.ClusterBuildPlane, gen.ClusterBuildPlane](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated cluster build plane", "error", err)
		return gen.UpdateClusterBuildPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ClusterBuildPlane updated successfully", "clusterBuildPlane", updated.Name)
	return gen.UpdateClusterBuildPlane200JSONResponse(genCBP), nil
}

// DeleteClusterBuildPlane deletes a cluster-scoped build plane by name.
func (h *Handler) DeleteClusterBuildPlane(
	ctx context.Context,
	request gen.DeleteClusterBuildPlaneRequestObject,
) (gen.DeleteClusterBuildPlaneResponseObject, error) {
	h.logger.Info("DeleteClusterBuildPlane called", "clusterBuildPlaneName", request.ClusterBuildPlaneName)

	err := h.clusterBuildPlaneService.DeleteClusterBuildPlane(ctx, request.ClusterBuildPlaneName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteClusterBuildPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterbuildplanesvc.ErrClusterBuildPlaneNotFound) {
			return gen.DeleteClusterBuildPlane404JSONResponse{NotFoundJSONResponse: notFound("ClusterBuildPlane")}, nil
		}
		h.logger.Error("Failed to delete cluster build plane", "error", err)
		return gen.DeleteClusterBuildPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ClusterBuildPlane deleted successfully", "clusterBuildPlane", request.ClusterBuildPlaneName)
	return gen.DeleteClusterBuildPlane204Response{}, nil
}
