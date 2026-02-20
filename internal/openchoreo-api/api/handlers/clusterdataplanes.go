// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	clusterdataplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterdataplane"
)

// ListClusterDataPlanes returns a paginated list of cluster-scoped data planes.
func (h *Handler) ListClusterDataPlanes(
	ctx context.Context,
	request gen.ListClusterDataPlanesRequestObject,
) (gen.ListClusterDataPlanesResponseObject, error) {
	h.logger.Debug("ListClusterDataPlanes called")

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.clusterDataPlaneService.ListClusterDataPlanes(ctx, opts)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListClusterDataPlanes403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to list cluster data planes", "error", err)
		return gen.ListClusterDataPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ClusterDataPlane, gen.ClusterDataPlane](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert cluster data planes", "error", err)
		return gen.ListClusterDataPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListClusterDataPlanes200JSONResponse{
		Items:      items,
		Pagination: ToPaginationPtr(result),
	}, nil
}

// CreateClusterDataPlane creates a new cluster-scoped data plane.
func (h *Handler) CreateClusterDataPlane(
	ctx context.Context,
	request gen.CreateClusterDataPlaneRequestObject,
) (gen.CreateClusterDataPlaneResponseObject, error) {
	h.logger.Info("CreateClusterDataPlane called")

	if request.Body == nil {
		return gen.CreateClusterDataPlane400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	cdpCR, err := convert[gen.ClusterDataPlane, openchoreov1alpha1.ClusterDataPlane](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateClusterDataPlane400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	cdpCR.Status = openchoreov1alpha1.ClusterDataPlaneStatus{}

	created, err := h.clusterDataPlaneService.CreateClusterDataPlane(ctx, &cdpCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateClusterDataPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterdataplanesvc.ErrClusterDataPlaneAlreadyExists) {
			return gen.CreateClusterDataPlane409JSONResponse{ConflictJSONResponse: conflict("ClusterDataPlane already exists")}, nil
		}
		h.logger.Error("Failed to create cluster data plane", "error", err)
		return gen.CreateClusterDataPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCDP, err := convert[openchoreov1alpha1.ClusterDataPlane, gen.ClusterDataPlane](*created)
	if err != nil {
		h.logger.Error("Failed to convert created cluster data plane", "error", err)
		return gen.CreateClusterDataPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ClusterDataPlane created successfully", "clusterDataPlane", created.Name)
	return gen.CreateClusterDataPlane201JSONResponse(genCDP), nil
}

// GetClusterDataPlane returns details of a specific cluster-scoped data plane.
func (h *Handler) GetClusterDataPlane(
	ctx context.Context,
	request gen.GetClusterDataPlaneRequestObject,
) (gen.GetClusterDataPlaneResponseObject, error) {
	h.logger.Debug("GetClusterDataPlane called", "clusterDataPlane", request.CdpName)

	cdp, err := h.clusterDataPlaneService.GetClusterDataPlane(ctx, request.CdpName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetClusterDataPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterdataplanesvc.ErrClusterDataPlaneNotFound) {
			return gen.GetClusterDataPlane404JSONResponse{NotFoundJSONResponse: notFound("ClusterDataPlane")}, nil
		}
		h.logger.Error("Failed to get cluster data plane", "error", err, "clusterDataPlane", request.CdpName)
		return gen.GetClusterDataPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCDP, err := convert[openchoreov1alpha1.ClusterDataPlane, gen.ClusterDataPlane](*cdp)
	if err != nil {
		h.logger.Error("Failed to convert cluster data plane", "error", err)
		return gen.GetClusterDataPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetClusterDataPlane200JSONResponse(genCDP), nil
}
