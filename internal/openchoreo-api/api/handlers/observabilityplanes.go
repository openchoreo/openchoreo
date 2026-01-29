// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// ListObservabilityPlanes returns a list of observability planes
func (h *Handler) ListObservabilityPlanes(
	ctx context.Context,
	request gen.ListObservabilityPlanesRequestObject,
) (gen.ListObservabilityPlanesResponseObject, error) {
	h.logger.Debug("ListObservabilityPlanes called", "namespaceName", request.NamespaceName)

	observabilityPlanes, err := h.services.ObservabilityPlaneService.ListObservabilityPlanes(ctx, request.NamespaceName)
	if err != nil {
		h.logger.Error("Failed to list observability planes", "error", err)
		return gen.ListObservabilityPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert to generated types
	items := make([]gen.ObservabilityPlane, 0, len(observabilityPlanes))
	for _, op := range observabilityPlanes {
		items = append(items, toGenObservabilityPlane(&op))
	}

	// TODO: Implement proper cursor-based pagination with Kubernetes continuation tokens
	return gen.ListObservabilityPlanes200JSONResponse{
		Items:      items,
		Pagination: gen.Pagination{},
	}, nil
}

// toGenObservabilityPlane converts models.ObservabilityPlaneResponse to gen.ObservabilityPlane
func toGenObservabilityPlane(op *models.ObservabilityPlaneResponse) gen.ObservabilityPlane {
	return gen.ObservabilityPlane{
		Name:        op.Name,
		Namespace:   op.Namespace,
		DisplayName: ptr.To(op.DisplayName),
		Description: ptr.To(op.Description),
		CreatedAt:   op.CreatedAt,
		Status:      ptr.To(op.Status),
	}
}
