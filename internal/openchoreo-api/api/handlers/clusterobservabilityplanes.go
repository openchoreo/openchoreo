// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	clusterobservabilityplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterobservabilityplane"
)

// ListClusterObservabilityPlanes returns a paginated list of cluster-scoped observability planes.
func (h *Handler) ListClusterObservabilityPlanes(
	ctx context.Context,
	request gen.ListClusterObservabilityPlanesRequestObject,
) (gen.ListClusterObservabilityPlanesResponseObject, error) {
	h.logger.Debug("ListClusterObservabilityPlanes called")

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.services.ClusterObservabilityPlaneService.ListClusterObservabilityPlanes(ctx, opts)
	if err != nil {
		h.logger.Error("Failed to list cluster observability planes", "error", err)
		return gen.ListClusterObservabilityPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ClusterObservabilityPlane, gen.ClusterObservabilityPlane](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert cluster observability planes", "error", err)
		return gen.ListClusterObservabilityPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListClusterObservabilityPlanes200JSONResponse{
		Items:      items,
		Pagination: ToPaginationPtr(result),
	}, nil
}

// GetClusterObservabilityPlane returns details of a specific cluster-scoped observability plane.
func (h *Handler) GetClusterObservabilityPlane(
	ctx context.Context,
	request gen.GetClusterObservabilityPlaneRequestObject,
) (gen.GetClusterObservabilityPlaneResponseObject, error) {
	h.logger.Debug("GetClusterObservabilityPlane called", "clusterObservabilityPlaneName", request.ClusterObservabilityPlaneName)

	cop, err := h.services.ClusterObservabilityPlaneService.GetClusterObservabilityPlane(ctx, request.ClusterObservabilityPlaneName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetClusterObservabilityPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterobservabilityplanesvc.ErrClusterObservabilityPlaneNotFound) {
			return gen.GetClusterObservabilityPlane404JSONResponse{NotFoundJSONResponse: notFound("ClusterObservabilityPlane")}, nil
		}
		h.logger.Error("Failed to get cluster observability plane", "error", err)
		return gen.GetClusterObservabilityPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCOP, err := convert[openchoreov1alpha1.ClusterObservabilityPlane, gen.ClusterObservabilityPlane](*cop)
	if err != nil {
		h.logger.Error("Failed to convert cluster observability plane", "error", err)
		return gen.GetClusterObservabilityPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetClusterObservabilityPlane200JSONResponse(genCOP), nil
}

// CreateClusterObservabilityPlane creates a new cluster-scoped observability plane.
func (h *Handler) CreateClusterObservabilityPlane(
	ctx context.Context,
	request gen.CreateClusterObservabilityPlaneRequestObject,
) (gen.CreateClusterObservabilityPlaneResponseObject, error) {
	h.logger.Info("CreateClusterObservabilityPlane called")

	if request.Body == nil {
		return gen.CreateClusterObservabilityPlane400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	copCR, err := convert[gen.ClusterObservabilityPlane, openchoreov1alpha1.ClusterObservabilityPlane](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateClusterObservabilityPlane400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	copCR.Status = openchoreov1alpha1.ClusterObservabilityPlaneStatus{}

	created, err := h.services.ClusterObservabilityPlaneService.CreateClusterObservabilityPlane(ctx, &copCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateClusterObservabilityPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterobservabilityplanesvc.ErrClusterObservabilityPlaneAlreadyExists) {
			return gen.CreateClusterObservabilityPlane409JSONResponse{ConflictJSONResponse: conflict("ClusterObservabilityPlane already exists")}, nil
		}
		h.logger.Error("Failed to create cluster observability plane", "error", err)
		return gen.CreateClusterObservabilityPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCOP, err := convert[openchoreov1alpha1.ClusterObservabilityPlane, gen.ClusterObservabilityPlane](*created)
	if err != nil {
		h.logger.Error("Failed to convert created cluster observability plane", "error", err)
		return gen.CreateClusterObservabilityPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ClusterObservabilityPlane created successfully", "clusterObservabilityPlane", created.Name)
	return gen.CreateClusterObservabilityPlane201JSONResponse(genCOP), nil
}

// UpdateClusterObservabilityPlane replaces an existing cluster observability plane (full update).
func (h *Handler) UpdateClusterObservabilityPlane(
	ctx context.Context,
	request gen.UpdateClusterObservabilityPlaneRequestObject,
) (gen.UpdateClusterObservabilityPlaneResponseObject, error) {
	h.logger.Info("UpdateClusterObservabilityPlane called", "clusterObservabilityPlaneName", request.ClusterObservabilityPlaneName)

	if request.Body == nil {
		return gen.UpdateClusterObservabilityPlane400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	copCR, err := convert[gen.ClusterObservabilityPlane, openchoreov1alpha1.ClusterObservabilityPlane](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateClusterObservabilityPlane400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	copCR.Status = openchoreov1alpha1.ClusterObservabilityPlaneStatus{}

	// Ensure the name from the URL path is used
	copCR.Name = request.ClusterObservabilityPlaneName

	updated, err := h.services.ClusterObservabilityPlaneService.UpdateClusterObservabilityPlane(ctx, &copCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateClusterObservabilityPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterobservabilityplanesvc.ErrClusterObservabilityPlaneNotFound) {
			return gen.UpdateClusterObservabilityPlane404JSONResponse{NotFoundJSONResponse: notFound("ClusterObservabilityPlane")}, nil
		}
		h.logger.Error("Failed to update cluster observability plane", "error", err)
		return gen.UpdateClusterObservabilityPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCOP, err := convert[openchoreov1alpha1.ClusterObservabilityPlane, gen.ClusterObservabilityPlane](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated cluster observability plane", "error", err)
		return gen.UpdateClusterObservabilityPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ClusterObservabilityPlane updated successfully", "clusterObservabilityPlane", updated.Name)
	return gen.UpdateClusterObservabilityPlane200JSONResponse(genCOP), nil
}

// DeleteClusterObservabilityPlane deletes a cluster observability plane by name.
func (h *Handler) DeleteClusterObservabilityPlane(
	ctx context.Context,
	request gen.DeleteClusterObservabilityPlaneRequestObject,
) (gen.DeleteClusterObservabilityPlaneResponseObject, error) {
	h.logger.Info("DeleteClusterObservabilityPlane called", "clusterObservabilityPlaneName", request.ClusterObservabilityPlaneName)

	err := h.services.ClusterObservabilityPlaneService.DeleteClusterObservabilityPlane(ctx, request.ClusterObservabilityPlaneName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteClusterObservabilityPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterobservabilityplanesvc.ErrClusterObservabilityPlaneNotFound) {
			return gen.DeleteClusterObservabilityPlane404JSONResponse{NotFoundJSONResponse: notFound("ClusterObservabilityPlane")}, nil
		}
		h.logger.Error("Failed to delete cluster observability plane", "error", err)
		return gen.DeleteClusterObservabilityPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ClusterObservabilityPlane deleted successfully", "clusterObservabilityPlane", request.ClusterObservabilityPlaneName)
	return gen.DeleteClusterObservabilityPlane204Response{}, nil
}
