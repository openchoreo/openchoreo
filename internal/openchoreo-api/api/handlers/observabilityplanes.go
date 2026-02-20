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

	result, err := h.observabilityPlaneService.ListObservabilityPlanes(ctx, request.NamespaceName, opts)
	if err != nil {
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

	op, err := h.observabilityPlaneService.GetObservabilityPlane(ctx, request.NamespaceName, request.ObservabilityPlaneName)
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
