// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// ListClusterBuildPlanes returns a list of cluster-scoped build planes
func (h *Handler) ListClusterBuildPlanes(
	ctx context.Context,
	request gen.ListClusterBuildPlanesRequestObject,
) (gen.ListClusterBuildPlanesResponseObject, error) {
	h.logger.Debug("ListClusterBuildPlanes called")

	buildplanes, err := h.services.ClusterBuildPlaneService.ListClusterBuildPlanes(ctx)
	if err != nil {
		h.logger.Error("Failed to list cluster buildplanes", "error", err)
		return gen.ListClusterBuildPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items := make([]gen.ClusterBuildPlane, 0, len(buildplanes))
	for _, bp := range buildplanes {
		items = append(items, toGenClusterBuildPlane(&bp))
	}

	return gen.ListClusterBuildPlanes200JSONResponse{
		Items:      items,
		Pagination: gen.Pagination{},
	}, nil
}

// toGenClusterBuildPlane converts models.ClusterBuildPlaneResponse to gen.ClusterBuildPlane
func toGenClusterBuildPlane(bp *models.ClusterBuildPlaneResponse) gen.ClusterBuildPlane {
	result := gen.ClusterBuildPlane{
		Name:        bp.Name,
		PlaneID:     bp.PlaneID,
		DisplayName: ptr.To(bp.DisplayName),
		Description: ptr.To(bp.Description),
		CreatedAt:   bp.CreatedAt,
		Status:      ptr.To(bp.Status),
	}
	if bp.ObservabilityPlaneRef != nil {
		result.ObservabilityPlaneRef = &gen.ClusterObservabilityPlaneRef{
			Kind: gen.ClusterObservabilityPlaneRefKind(bp.ObservabilityPlaneRef.Kind),
			Name: bp.ObservabilityPlaneRef.Name,
		}
	}
	return result
}
