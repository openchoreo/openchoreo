// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	defaultClusterDataPlaneLimit = 20
	minClusterDataPlaneLimit     = 1
	maxClusterDataPlaneLimit     = 100
)

// ListClusterDataPlanes returns a paginated list of cluster-scoped data planes
func (h *Handler) ListClusterDataPlanes(
	ctx context.Context,
	request gen.ListClusterDataPlanesRequestObject,
) (gen.ListClusterDataPlanesResponseObject, error) {
	h.logger.Debug("ListClusterDataPlanes called")

	limit := defaultClusterDataPlaneLimit
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
		if limit < minClusterDataPlaneLimit || limit > maxClusterDataPlaneLimit {
			return gen.ListClusterDataPlanes400JSONResponse{
				BadRequestJSONResponse: badRequest(
					fmt.Sprintf("limit must be between %d and %d", minClusterDataPlaneLimit, maxClusterDataPlaneLimit),
				),
			}, nil
		}
	}
	var cursor string
	if request.Params.Cursor != nil {
		cursor = *request.Params.Cursor
	}

	result, err := h.services.ClusterDataPlaneService.ListClusterDataPlanesPaginated(ctx, limit, cursor)
	if err != nil {
		h.logger.Error("Failed to list cluster dataplanes", "error", err)
		return gen.ListClusterDataPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items := make([]gen.ClusterDataPlane, 0, len(result.Items))
	for _, dp := range result.Items {
		items = append(items, toGenClusterDataPlane(dp))
	}

	pagination := gen.Pagination{
		RemainingCount: result.RemainingCount,
	}
	if result.NextCursor != "" {
		pagination.NextCursor = &result.NextCursor
	}

	return gen.ListClusterDataPlanes200JSONResponse{
		Items:      items,
		Pagination: pagination,
	}, nil
}

// CreateClusterDataPlane creates a new cluster-scoped data plane
func (h *Handler) CreateClusterDataPlane(
	ctx context.Context,
	request gen.CreateClusterDataPlaneRequestObject,
) (gen.CreateClusterDataPlaneResponseObject, error) {
	h.logger.Debug("CreateClusterDataPlane called")

	if request.Body == nil {
		return gen.CreateClusterDataPlane400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	req := &models.CreateClusterDataPlaneRequest{
		Name:                    request.Body.Name,
		DisplayName:             ptr.Deref(request.Body.DisplayName, ""),
		Description:             ptr.Deref(request.Body.Description, ""),
		PlaneID:                 request.Body.PlaneID,
		ClusterAgentClientCA:    request.Body.ClusterAgentClientCA,
		PublicVirtualHost:       request.Body.PublicVirtualHost,
		OrganizationVirtualHost: request.Body.OrganizationVirtualHost,
		PublicHTTPPort:          request.Body.PublicHTTPPort,
		PublicHTTPSPort:         request.Body.PublicHTTPSPort,
		OrganizationHTTPPort:    request.Body.OrganizationHTTPPort,
		OrganizationHTTPSPort:   request.Body.OrganizationHTTPSPort,
	}
	if request.Body.ObservabilityPlaneRef != nil {
		req.ObservabilityPlaneRef = &models.ObservabilityPlaneRef{
			Kind: string(request.Body.ObservabilityPlaneRef.Kind),
			Name: request.Body.ObservabilityPlaneRef.Name,
		}
	}

	if err := req.Validate(); err != nil {
		return gen.CreateClusterDataPlane400JSONResponse{BadRequestJSONResponse: badRequest(err.Error())}, nil
	}

	dataplane, err := h.services.ClusterDataPlaneService.CreateClusterDataPlane(ctx, req)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateClusterDataPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrClusterDataPlaneAlreadyExists) {
			return gen.CreateClusterDataPlane409JSONResponse{ConflictJSONResponse: conflict("ClusterDataPlane already exists")}, nil
		}
		h.logger.Error("Failed to create cluster dataplane", "error", err, "clusterDataPlane", req.Name)
		return gen.CreateClusterDataPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.CreateClusterDataPlane201JSONResponse(toGenClusterDataPlane(dataplane)), nil
}

// GetClusterDataPlane returns details of a specific cluster-scoped data plane
func (h *Handler) GetClusterDataPlane(
	ctx context.Context,
	request gen.GetClusterDataPlaneRequestObject,
) (gen.GetClusterDataPlaneResponseObject, error) {
	h.logger.Debug("GetClusterDataPlane called", "clusterDataPlane", request.CdpName)

	dataplane, err := h.services.ClusterDataPlaneService.GetClusterDataPlane(ctx, request.CdpName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetClusterDataPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrClusterDataPlaneNotFound) {
			return gen.GetClusterDataPlane404JSONResponse{NotFoundJSONResponse: notFound("ClusterDataPlane")}, nil
		}
		h.logger.Error("Failed to get cluster dataplane", "error", err, "clusterDataPlane", request.CdpName)
		return gen.GetClusterDataPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetClusterDataPlane200JSONResponse(toGenClusterDataPlane(dataplane)), nil
}

// toGenClusterDataPlane converts models.ClusterDataPlaneResponse to gen.ClusterDataPlane
func toGenClusterDataPlane(dp *models.ClusterDataPlaneResponse) gen.ClusterDataPlane {
	result := gen.ClusterDataPlane{
		Name:                    dp.Name,
		PlaneID:                 dp.PlaneID,
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
	if dp.ObservabilityPlaneRef != nil {
		result.ObservabilityPlaneRef = &gen.ClusterObservabilityPlaneRef{
			Kind: gen.ClusterObservabilityPlaneRefKind(dp.ObservabilityPlaneRef.Kind),
			Name: dp.ObservabilityPlaneRef.Name,
		}
	}
	return result
}
