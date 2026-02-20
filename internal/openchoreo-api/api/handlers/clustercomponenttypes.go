// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	clustercomponenttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clustercomponenttype"
)

// ListClusterComponentTypes returns a paginated list of cluster-scoped component types.
func (h *Handler) ListClusterComponentTypes(
	ctx context.Context,
	request gen.ListClusterComponentTypesRequestObject,
) (gen.ListClusterComponentTypesResponseObject, error) {
	h.logger.Debug("ListClusterComponentTypes called")

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.clusterComponentTypeService.ListClusterComponentTypes(ctx, opts)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListClusterComponentTypes403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to list cluster component types", "error", err)
		return gen.ListClusterComponentTypes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ClusterComponentType, gen.ClusterComponentType](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert cluster component types", "error", err)
		return gen.ListClusterComponentTypes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListClusterComponentTypes200JSONResponse{
		Items:      items,
		Pagination: ToPaginationPtr(result),
	}, nil
}

// CreateClusterComponentType creates a new cluster-scoped component type.
func (h *Handler) CreateClusterComponentType(
	ctx context.Context,
	request gen.CreateClusterComponentTypeRequestObject,
) (gen.CreateClusterComponentTypeResponseObject, error) {
	h.logger.Info("CreateClusterComponentType called")

	if request.Body == nil {
		return gen.CreateClusterComponentType400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	cctCR, err := convert[gen.ClusterComponentType, openchoreov1alpha1.ClusterComponentType](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateClusterComponentType400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	cctCR.Status = openchoreov1alpha1.ClusterComponentTypeStatus{}

	created, err := h.clusterComponentTypeService.CreateClusterComponentType(ctx, &cctCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateClusterComponentType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clustercomponenttypesvc.ErrClusterComponentTypeAlreadyExists) {
			return gen.CreateClusterComponentType409JSONResponse{ConflictJSONResponse: conflict("Cluster component type already exists")}, nil
		}
		h.logger.Error("Failed to create cluster component type", "error", err)
		return gen.CreateClusterComponentType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCCT, err := convert[openchoreov1alpha1.ClusterComponentType, gen.ClusterComponentType](*created)
	if err != nil {
		h.logger.Error("Failed to convert created cluster component type", "error", err)
		return gen.CreateClusterComponentType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster component type created successfully", "clusterComponentType", created.Name)
	return gen.CreateClusterComponentType201JSONResponse(genCCT), nil
}

// UpdateClusterComponentType replaces an existing cluster-scoped component type (full update).
func (h *Handler) UpdateClusterComponentType(
	ctx context.Context,
	request gen.UpdateClusterComponentTypeRequestObject,
) (gen.UpdateClusterComponentTypeResponseObject, error) {
	h.logger.Info("UpdateClusterComponentType called", "cctName", request.CctName)

	if request.Body == nil {
		return gen.UpdateClusterComponentType400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	cctCR, err := convert[gen.ClusterComponentType, openchoreov1alpha1.ClusterComponentType](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateClusterComponentType400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	cctCR.Status = openchoreov1alpha1.ClusterComponentTypeStatus{}

	// Ensure the name from the URL path is used
	cctCR.Name = request.CctName

	updated, err := h.clusterComponentTypeService.UpdateClusterComponentType(ctx, &cctCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateClusterComponentType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clustercomponenttypesvc.ErrClusterComponentTypeNotFound) {
			return gen.UpdateClusterComponentType404JSONResponse{NotFoundJSONResponse: notFound("ClusterComponentType")}, nil
		}
		h.logger.Error("Failed to update cluster component type", "error", err)
		return gen.UpdateClusterComponentType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCCT, err := convert[openchoreov1alpha1.ClusterComponentType, gen.ClusterComponentType](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated cluster component type", "error", err)
		return gen.UpdateClusterComponentType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster component type updated successfully", "clusterComponentType", updated.Name)
	return gen.UpdateClusterComponentType200JSONResponse(genCCT), nil
}

// GetClusterComponentType returns details of a specific cluster-scoped component type.
func (h *Handler) GetClusterComponentType(
	ctx context.Context,
	request gen.GetClusterComponentTypeRequestObject,
) (gen.GetClusterComponentTypeResponseObject, error) {
	h.logger.Debug("GetClusterComponentType called", "cctName", request.CctName)

	cct, err := h.clusterComponentTypeService.GetClusterComponentType(ctx, request.CctName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetClusterComponentType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clustercomponenttypesvc.ErrClusterComponentTypeNotFound) {
			return gen.GetClusterComponentType404JSONResponse{NotFoundJSONResponse: notFound("ClusterComponentType")}, nil
		}
		h.logger.Error("Failed to get cluster component type", "error", err)
		return gen.GetClusterComponentType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCCT, err := convert[openchoreov1alpha1.ClusterComponentType, gen.ClusterComponentType](*cct)
	if err != nil {
		h.logger.Error("Failed to convert cluster component type", "error", err)
		return gen.GetClusterComponentType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetClusterComponentType200JSONResponse(genCCT), nil
}

// GetClusterComponentTypeSchema returns the parameter schema for a cluster-scoped component type.
func (h *Handler) GetClusterComponentTypeSchema(
	ctx context.Context,
	request gen.GetClusterComponentTypeSchemaRequestObject,
) (gen.GetClusterComponentTypeSchemaResponseObject, error) {
	h.logger.Debug("GetClusterComponentTypeSchema called", "name", request.CctName)

	jsonSchema, err := h.clusterComponentTypeService.GetClusterComponentTypeSchema(ctx, request.CctName)
	if err != nil {
		if errors.Is(err, clustercomponenttypesvc.ErrClusterComponentTypeNotFound) {
			return gen.GetClusterComponentTypeSchema404JSONResponse{NotFoundJSONResponse: notFound("ClusterComponentType")}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetClusterComponentTypeSchema403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to get cluster component type schema", "error", err)
		return gen.GetClusterComponentTypeSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert JSONSchemaProps to SchemaResponse (map[string]interface{})
	data, err := json.Marshal(jsonSchema)
	if err != nil {
		h.logger.Error("Failed to marshal schema", "error", err)
		return gen.GetClusterComponentTypeSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	var schemaResp gen.SchemaResponse
	if err := json.Unmarshal(data, &schemaResp); err != nil {
		h.logger.Error("Failed to unmarshal schema response", "error", err)
		return gen.GetClusterComponentTypeSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetClusterComponentTypeSchema200JSONResponse(schemaResp), nil
}
