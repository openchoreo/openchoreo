// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	dataplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/dataplane"
)

// ListDataPlanes returns a paginated list of data planes within a namespace.
func (h *Handler) ListDataPlanes(
	ctx context.Context,
	request gen.ListDataPlanesRequestObject,
) (gen.ListDataPlanesResponseObject, error) {
	h.logger.Debug("ListDataPlanes called", "namespaceName", request.NamespaceName)

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.dataPlaneService.ListDataPlanes(ctx, request.NamespaceName, opts)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListDataPlanes403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to list data planes", "error", err)
		return gen.ListDataPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.DataPlane, gen.DataPlane](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert data planes", "error", err)
		return gen.ListDataPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListDataPlanes200JSONResponse{
		Items:      items,
		Pagination: ToPaginationPtr(result),
	}, nil
}

// CreateDataPlane creates a new data plane within a namespace.
func (h *Handler) CreateDataPlane(
	ctx context.Context,
	request gen.CreateDataPlaneRequestObject,
) (gen.CreateDataPlaneResponseObject, error) {
	h.logger.Info("CreateDataPlane called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateDataPlane400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	dpCR, err := convert[gen.DataPlane, openchoreov1alpha1.DataPlane](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateDataPlane400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	dpCR.Status = openchoreov1alpha1.DataPlaneStatus{}

	created, err := h.dataPlaneService.CreateDataPlane(ctx, request.NamespaceName, &dpCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateDataPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, dataplanesvc.ErrDataPlaneAlreadyExists) {
			return gen.CreateDataPlane409JSONResponse{ConflictJSONResponse: conflict("DataPlane already exists")}, nil
		}
		h.logger.Error("Failed to create data plane", "error", err)
		return gen.CreateDataPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genDP, err := convert[openchoreov1alpha1.DataPlane, gen.DataPlane](*created)
	if err != nil {
		h.logger.Error("Failed to convert created data plane", "error", err)
		return gen.CreateDataPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("DataPlane created successfully", "namespaceName", request.NamespaceName, "dataPlane", created.Name)
	return gen.CreateDataPlane201JSONResponse(genDP), nil
}

// GetDataPlane returns details of a specific data plane.
func (h *Handler) GetDataPlane(
	ctx context.Context,
	request gen.GetDataPlaneRequestObject,
) (gen.GetDataPlaneResponseObject, error) {
	h.logger.Debug("GetDataPlane called", "namespaceName", request.NamespaceName, "dpName", request.DpName)

	dataPlane, err := h.dataPlaneService.GetDataPlane(ctx, request.NamespaceName, request.DpName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetDataPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, dataplanesvc.ErrDataPlaneNotFound) {
			return gen.GetDataPlane404JSONResponse{NotFoundJSONResponse: notFound("DataPlane")}, nil
		}
		h.logger.Error("Failed to get data plane", "error", err)
		return gen.GetDataPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genDP, err := convert[openchoreov1alpha1.DataPlane, gen.DataPlane](*dataPlane)
	if err != nil {
		h.logger.Error("Failed to convert data plane", "error", err)
		return gen.GetDataPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetDataPlane200JSONResponse(genDP), nil
}
