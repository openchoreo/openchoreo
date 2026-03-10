// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	clusterworkflowplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterworkflowplane"
)

// ListClusterWorkflowPlanes returns a paginated list of cluster-scoped workflow planes.
func (h *Handler) ListClusterWorkflowPlanes(
	ctx context.Context,
	request gen.ListClusterWorkflowPlanesRequestObject,
) (gen.ListClusterWorkflowPlanesResponseObject, error) {
	h.logger.Debug("ListClusterWorkflowPlanes called")

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.ClusterWorkflowPlaneService.ListClusterWorkflowPlanes(ctx, opts)
	if err != nil {
		h.logger.Error("Failed to list cluster workflow planes", "error", err)
		return gen.ListClusterWorkflowPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ClusterWorkflowPlane, gen.ClusterWorkflowPlane](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert cluster workflow planes", "error", err)
		return gen.ListClusterWorkflowPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListClusterWorkflowPlanes200JSONResponse{
		Items:      items,
		Pagination: ToPagination(result),
	}, nil
}

// GetClusterWorkflowPlane returns details of a specific cluster-scoped workflow plane.
func (h *Handler) GetClusterWorkflowPlane(
	ctx context.Context,
	request gen.GetClusterWorkflowPlaneRequestObject,
) (gen.GetClusterWorkflowPlaneResponseObject, error) {
	h.logger.Debug("GetClusterWorkflowPlane called", "clusterWorkflowPlaneName", request.ClusterWorkflowPlaneName)

	cwp, err := h.services.ClusterWorkflowPlaneService.GetClusterWorkflowPlane(ctx, request.ClusterWorkflowPlaneName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetClusterWorkflowPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterworkflowplanesvc.ErrClusterWorkflowPlaneNotFound) {
			return gen.GetClusterWorkflowPlane404JSONResponse{NotFoundJSONResponse: notFound("ClusterWorkflowPlane")}, nil
		}
		h.logger.Error("Failed to get cluster workflow plane", "error", err)
		return gen.GetClusterWorkflowPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCWP, err := convert[openchoreov1alpha1.ClusterWorkflowPlane, gen.ClusterWorkflowPlane](*cwp)
	if err != nil {
		h.logger.Error("Failed to convert cluster workflow plane", "error", err)
		return gen.GetClusterWorkflowPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetClusterWorkflowPlane200JSONResponse(genCWP), nil
}

// CreateClusterWorkflowPlane creates a new cluster-scoped workflow plane.
func (h *Handler) CreateClusterWorkflowPlane(
	ctx context.Context,
	request gen.CreateClusterWorkflowPlaneRequestObject,
) (gen.CreateClusterWorkflowPlaneResponseObject, error) {
	h.logger.Info("CreateClusterWorkflowPlane called")

	if request.Body == nil {
		return gen.CreateClusterWorkflowPlane400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	cbpCR, err := convert[gen.ClusterWorkflowPlane, openchoreov1alpha1.ClusterWorkflowPlane](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateClusterWorkflowPlane400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	created, err := h.services.ClusterWorkflowPlaneService.CreateClusterWorkflowPlane(ctx, &cbpCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateClusterWorkflowPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterworkflowplanesvc.ErrClusterWorkflowPlaneAlreadyExists) {
			return gen.CreateClusterWorkflowPlane409JSONResponse{ConflictJSONResponse: conflict("ClusterWorkflowPlane already exists")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.CreateClusterWorkflowPlane400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to create cluster workflow plane", "error", err)
		return gen.CreateClusterWorkflowPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCBP, err := convert[openchoreov1alpha1.ClusterWorkflowPlane, gen.ClusterWorkflowPlane](*created)
	if err != nil {
		h.logger.Error("Failed to convert created cluster workflow plane", "error", err)
		return gen.CreateClusterWorkflowPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ClusterWorkflowPlane created successfully", "clusterWorkflowPlane", created.Name)
	return gen.CreateClusterWorkflowPlane201JSONResponse(genCBP), nil
}

// UpdateClusterWorkflowPlane replaces an existing cluster-scoped workflow plane (full update).
func (h *Handler) UpdateClusterWorkflowPlane(
	ctx context.Context,
	request gen.UpdateClusterWorkflowPlaneRequestObject,
) (gen.UpdateClusterWorkflowPlaneResponseObject, error) {
	h.logger.Info("UpdateClusterWorkflowPlane called", "clusterWorkflowPlaneName", request.ClusterWorkflowPlaneName)

	if request.Body == nil {
		return gen.UpdateClusterWorkflowPlane400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	cbpCR, err := convert[gen.ClusterWorkflowPlane, openchoreov1alpha1.ClusterWorkflowPlane](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateClusterWorkflowPlane400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	// Ensure the name from the URL path is used
	cbpCR.Name = request.ClusterWorkflowPlaneName

	updated, err := h.services.ClusterWorkflowPlaneService.UpdateClusterWorkflowPlane(ctx, &cbpCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateClusterWorkflowPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterworkflowplanesvc.ErrClusterWorkflowPlaneNotFound) {
			return gen.UpdateClusterWorkflowPlane404JSONResponse{NotFoundJSONResponse: notFound("ClusterWorkflowPlane")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.UpdateClusterWorkflowPlane400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to update cluster workflow plane", "error", err)
		return gen.UpdateClusterWorkflowPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCBP, err := convert[openchoreov1alpha1.ClusterWorkflowPlane, gen.ClusterWorkflowPlane](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated cluster workflow plane", "error", err)
		return gen.UpdateClusterWorkflowPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ClusterWorkflowPlane updated successfully", "clusterWorkflowPlane", updated.Name)
	return gen.UpdateClusterWorkflowPlane200JSONResponse(genCBP), nil
}

// DeleteClusterWorkflowPlane deletes a cluster-scoped workflow plane by name.
func (h *Handler) DeleteClusterWorkflowPlane(
	ctx context.Context,
	request gen.DeleteClusterWorkflowPlaneRequestObject,
) (gen.DeleteClusterWorkflowPlaneResponseObject, error) {
	h.logger.Info("DeleteClusterWorkflowPlane called", "clusterWorkflowPlaneName", request.ClusterWorkflowPlaneName)

	err := h.services.ClusterWorkflowPlaneService.DeleteClusterWorkflowPlane(ctx, request.ClusterWorkflowPlaneName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteClusterWorkflowPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterworkflowplanesvc.ErrClusterWorkflowPlaneNotFound) {
			return gen.DeleteClusterWorkflowPlane404JSONResponse{NotFoundJSONResponse: notFound("ClusterWorkflowPlane")}, nil
		}
		h.logger.Error("Failed to delete cluster workflow plane", "error", err)
		return gen.DeleteClusterWorkflowPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ClusterWorkflowPlane deleted successfully", "clusterWorkflowPlane", request.ClusterWorkflowPlaneName)
	return gen.DeleteClusterWorkflowPlane204Response{}, nil
}
