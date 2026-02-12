// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// ListClusterObservabilityPlanes returns a list of cluster-scoped observability planes
func (h *Handler) ListClusterObservabilityPlanes(
	ctx context.Context,
	request gen.ListClusterObservabilityPlanesRequestObject,
) (gen.ListClusterObservabilityPlanesResponseObject, error) {
	h.logger.Debug("ListClusterObservabilityPlanes called")

	observabilityPlanes, err := h.services.ClusterObservabilityPlaneService.ListClusterObservabilityPlanes(ctx)
	if err != nil {
		h.logger.Error("Failed to list cluster observability planes", "error", err)
		return gen.ListClusterObservabilityPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items := make([]gen.ClusterObservabilityPlane, 0, len(observabilityPlanes))
	for _, op := range observabilityPlanes {
		items = append(items, toGenClusterObservabilityPlane(&op))
	}

	return gen.ListClusterObservabilityPlanes200JSONResponse{
		Items:      items,
		Pagination: gen.Pagination{},
	}, nil
}

// toGenClusterObservabilityPlane converts models.ClusterObservabilityPlaneResponse to gen.ClusterObservabilityPlane
func toGenClusterObservabilityPlane(op *models.ClusterObservabilityPlaneResponse) gen.ClusterObservabilityPlane {
	result := gen.ClusterObservabilityPlane{
		Name:        op.Name,
		PlaneID:     op.PlaneID,
		DisplayName: ptr.To(op.DisplayName),
		Description: ptr.To(op.Description),
		CreatedAt:   op.CreatedAt,
		Status:      ptr.To(op.Status),
	}
	if op.ObserverURL != "" {
		result.ObserverURL = ptr.To(op.ObserverURL)
	}
	if op.RCAAgentURL != "" {
		result.RcaAgentURL = ptr.To(op.RCAAgentURL)
	}
	return result
}
