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
