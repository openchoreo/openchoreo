// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	observabilityplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/observabilityplane"
)

// ListObservabilityPlanes returns a paginated list of observability planes within a namespace.
func (h *Handler) ListObservabilityPlanes(
	ctx context.Context,
	request gen.ListObservabilityPlanesRequestObject,
) (gen.ListObservabilityPlanesResponseObject, error) {
	h.logger.Debug("ListObservabilityPlanes called", "namespaceName", request.NamespaceName)

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.services.ObservabilityPlaneService.ListObservabilityPlanes(ctx, request.NamespaceName, opts)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListObservabilityPlanes403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to list observability planes", "error", err)
		return gen.ListObservabilityPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ObservabilityPlane, gen.ObservabilityPlane](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert observability planes", "error", err)
		return gen.ListObservabilityPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListObservabilityPlanes200JSONResponse{
		Items:      items,
		Pagination: ToPaginationPtr(result),
	}, nil
}

// GetObservabilityPlane returns details of a specific observability plane.
func (h *Handler) GetObservabilityPlane(
	ctx context.Context,
	request gen.GetObservabilityPlaneRequestObject,
) (gen.GetObservabilityPlaneResponseObject, error) {
	h.logger.Debug("GetObservabilityPlane called", "namespaceName", request.NamespaceName, "observabilityPlaneName", request.ObservabilityPlaneName)

	op, err := h.services.ObservabilityPlaneService.GetObservabilityPlane(ctx, request.NamespaceName, request.ObservabilityPlaneName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetObservabilityPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, observabilityplanesvc.ErrObservabilityPlaneNotFound) {
			return gen.GetObservabilityPlane404JSONResponse{NotFoundJSONResponse: notFound("ObservabilityPlane")}, nil
		}
		h.logger.Error("Failed to get observability plane", "error", err)
		return gen.GetObservabilityPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genOP, err := convert[openchoreov1alpha1.ObservabilityPlane, gen.ObservabilityPlane](*op)
	if err != nil {
		h.logger.Error("Failed to convert observability plane", "error", err)
		return gen.GetObservabilityPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetObservabilityPlane200JSONResponse(genOP), nil
}

// CreateObservabilityPlane creates a new observability plane within a namespace.
func (h *Handler) CreateObservabilityPlane(
	ctx context.Context,
	request gen.CreateObservabilityPlaneRequestObject,
) (gen.CreateObservabilityPlaneResponseObject, error) {
	h.logger.Info("CreateObservabilityPlane called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateObservabilityPlane400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	opCR, err := convert[gen.ObservabilityPlane, openchoreov1alpha1.ObservabilityPlane](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateObservabilityPlane400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	opCR.Status = openchoreov1alpha1.ObservabilityPlaneStatus{}

	created, err := h.services.ObservabilityPlaneService.CreateObservabilityPlane(ctx, request.NamespaceName, &opCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateObservabilityPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, observabilityplanesvc.ErrObservabilityPlaneAlreadyExists) {
			return gen.CreateObservabilityPlane409JSONResponse{ConflictJSONResponse: conflict("ObservabilityPlane already exists")}, nil
		}
		h.logger.Error("Failed to create observability plane", "error", err)
		return gen.CreateObservabilityPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genOP, err := convert[openchoreov1alpha1.ObservabilityPlane, gen.ObservabilityPlane](*created)
	if err != nil {
		h.logger.Error("Failed to convert created observability plane", "error", err)
		return gen.CreateObservabilityPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ObservabilityPlane created successfully", "namespaceName", request.NamespaceName, "observabilityPlane", created.Name)
	return gen.CreateObservabilityPlane201JSONResponse(genOP), nil
}

// UpdateObservabilityPlane replaces an existing observability plane (full update).
func (h *Handler) UpdateObservabilityPlane(
	ctx context.Context,
	request gen.UpdateObservabilityPlaneRequestObject,
) (gen.UpdateObservabilityPlaneResponseObject, error) {
	h.logger.Info("UpdateObservabilityPlane called", "namespaceName", request.NamespaceName, "observabilityPlaneName", request.ObservabilityPlaneName)

	if request.Body == nil {
		return gen.UpdateObservabilityPlane400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	opCR, err := convert[gen.ObservabilityPlane, openchoreov1alpha1.ObservabilityPlane](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateObservabilityPlane400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	opCR.Status = openchoreov1alpha1.ObservabilityPlaneStatus{}

	// Ensure the name from the URL path is used
	opCR.Name = request.ObservabilityPlaneName

	updated, err := h.services.ObservabilityPlaneService.UpdateObservabilityPlane(ctx, request.NamespaceName, &opCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateObservabilityPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, observabilityplanesvc.ErrObservabilityPlaneNotFound) {
			return gen.UpdateObservabilityPlane404JSONResponse{NotFoundJSONResponse: notFound("ObservabilityPlane")}, nil
		}
		h.logger.Error("Failed to update observability plane", "error", err)
		return gen.UpdateObservabilityPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genOP, err := convert[openchoreov1alpha1.ObservabilityPlane, gen.ObservabilityPlane](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated observability plane", "error", err)
		return gen.UpdateObservabilityPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ObservabilityPlane updated successfully", "namespaceName", request.NamespaceName, "observabilityPlane", updated.Name)
	return gen.UpdateObservabilityPlane200JSONResponse(genOP), nil
}

// DeleteObservabilityPlane deletes an observability plane by name.
func (h *Handler) DeleteObservabilityPlane(
	ctx context.Context,
	request gen.DeleteObservabilityPlaneRequestObject,
) (gen.DeleteObservabilityPlaneResponseObject, error) {
	h.logger.Info("DeleteObservabilityPlane called", "namespaceName", request.NamespaceName, "observabilityPlaneName", request.ObservabilityPlaneName)

	err := h.services.ObservabilityPlaneService.DeleteObservabilityPlane(ctx, request.NamespaceName, request.ObservabilityPlaneName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteObservabilityPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, observabilityplanesvc.ErrObservabilityPlaneNotFound) {
			return gen.DeleteObservabilityPlane404JSONResponse{NotFoundJSONResponse: notFound("ObservabilityPlane")}, nil
		}
		h.logger.Error("Failed to delete observability plane", "error", err)
		return gen.DeleteObservabilityPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ObservabilityPlane deleted successfully", "namespaceName", request.NamespaceName, "observabilityPlane", request.ObservabilityPlaneName)
	return gen.DeleteObservabilityPlane204Response{}, nil
}
