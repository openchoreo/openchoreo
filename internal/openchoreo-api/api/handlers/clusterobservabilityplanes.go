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

	result, err := h.clusterObservabilityPlaneService.ListClusterObservabilityPlanes(ctx, opts)
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

	cop, err := h.clusterObservabilityPlaneService.GetClusterObservabilityPlane(ctx, request.ClusterObservabilityPlaneName)
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
