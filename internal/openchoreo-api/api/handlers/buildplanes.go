// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// ListBuildPlanes returns a list of build planes
func (h *Handler) ListBuildPlanes(
	ctx context.Context,
	request gen.ListBuildPlanesRequestObject,
) (gen.ListBuildPlanesResponseObject, error) {
	h.logger.Debug("ListBuildPlanes called", "namespaceName", request.NamespaceName)

	buildplanes, err := h.services.BuildPlaneService.ListBuildPlanes(ctx, request.NamespaceName)
	if err != nil {
		h.logger.Error("Failed to list buildplanes", "error", err)
		return gen.ListBuildPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert to generated types
	items := make([]gen.BuildPlane, 0, len(buildplanes))
	for _, bp := range buildplanes {
		items = append(items, toGenBuildPlane(&bp))
	}

	// TODO: Implement proper cursor-based pagination with Kubernetes continuation tokens
	return gen.ListBuildPlanes200JSONResponse{
		Items:      items,
		Pagination: gen.Pagination{},
	}, nil
}

// toGenBuildPlane converts models.BuildPlaneResponse to gen.BuildPlane
func toGenBuildPlane(bp *models.BuildPlaneResponse) gen.BuildPlane {
	result := gen.BuildPlane{
		Name:        bp.Name,
		Namespace:   bp.Namespace,
		DisplayName: ptr.To(bp.DisplayName),
		Description: ptr.To(bp.Description),
		CreatedAt:   bp.CreatedAt,
		Status:      ptr.To(bp.Status),
	}
	if bp.ObservabilityPlaneRef != "" {
		result.ObservabilityPlaneRef = ptr.To(bp.ObservabilityPlaneRef)
	}
	return result
}
