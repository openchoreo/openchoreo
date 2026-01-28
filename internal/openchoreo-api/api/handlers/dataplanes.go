// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// ListDataPlanes returns a paginated list of data planes
func (h *Handler) ListDataPlanes(
	ctx context.Context,
	request gen.ListDataPlanesRequestObject,
) (gen.ListDataPlanesResponseObject, error) {
	h.logger.Debug("ListDataPlanes called", "namespaceName", request.NamespaceName)

	dataplanes, err := h.services.DataPlaneService.ListDataPlanes(ctx, request.NamespaceName)
	if err != nil {
		h.logger.Error("Failed to list dataplanes", "error", err)
		return gen.ListDataPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert to generated types
	items := make([]gen.DataPlane, 0, len(dataplanes))
	for _, dp := range dataplanes {
		items = append(items, toGenDataPlane(dp))
	}

	// TODO: Implement proper cursor-based pagination with Kubernetes continuation tokens
	return gen.ListDataPlanes200JSONResponse{
		Items:      items,
		Pagination: gen.Pagination{},
	}, nil
}

// CreateDataPlane creates a new data plane
func (h *Handler) CreateDataPlane(
	ctx context.Context,
	request gen.CreateDataPlaneRequestObject,
) (gen.CreateDataPlaneResponseObject, error) {
	return nil, errNotImplemented
}

// GetDataPlane returns details of a specific data plane
func (h *Handler) GetDataPlane(
	ctx context.Context,
	request gen.GetDataPlaneRequestObject,
) (gen.GetDataPlaneResponseObject, error) {
	return nil, errNotImplemented
}

// toGenDataPlane converts models.DataPlaneResponse to gen.DataPlane
func toGenDataPlane(dp *models.DataPlaneResponse) gen.DataPlane {
	result := gen.DataPlane{
		Name:                    dp.Name,
		Namespace:               dp.Namespace,
		DisplayName:             ptr.To(dp.DisplayName),
		Description:             ptr.To(dp.Description),
		PublicVirtualHost:       dp.PublicVirtualHost,
		OrganizationVirtualHost: dp.OrganizationVirtualHost,
		PublicHTTPPort:          dp.PublicHTTPPort,
		PublicHTTPSPort:         dp.PublicHTTPSPort,
		OrganizationHTTPPort:    dp.OrganizationHTTPPort,
		OrganizationHTTPSPort:   dp.OrganizationHTTPSPort,
		CreatedAt:               dp.CreatedAt,
		Status:                  ptr.To(dp.Status),
	}
	if len(dp.ImagePullSecretRefs) > 0 {
		result.ImagePullSecretRefs = ptr.To(dp.ImagePullSecretRefs)
	}
	if dp.SecretStoreRef != "" {
		result.SecretStoreRef = ptr.To(dp.SecretStoreRef)
	}
	if dp.ObservabilityPlaneRef != "" {
		result.ObservabilityPlaneRef = ptr.To(dp.ObservabilityPlaneRef)
	}
	return result
}
