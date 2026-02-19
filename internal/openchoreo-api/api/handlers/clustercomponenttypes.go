// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// ListClusterComponentTypes returns a list of cluster-scoped component types
func (h *Handler) ListClusterComponentTypes(
	ctx context.Context,
	request gen.ListClusterComponentTypesRequestObject,
) (gen.ListClusterComponentTypesResponseObject, error) {
	h.logger.Debug("ListClusterComponentTypes called")

	componentTypes, err := h.services.ClusterComponentTypeService.ListClusterComponentTypes(ctx)
	if err != nil {
		h.logger.Error("Failed to list cluster component types", "error", err)
		return gen.ListClusterComponentTypes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items := make([]gen.ClusterComponentType, 0, len(componentTypes))
	for _, ct := range componentTypes {
		items = append(items, toGenClusterComponentType(ct))
	}

	return gen.ListClusterComponentTypes200JSONResponse{
		Items:      items,
		Pagination: gen.Pagination{},
	}, nil
}

// GetClusterComponentTypeSchema returns the parameter schema for a cluster-scoped component type
func (h *Handler) GetClusterComponentTypeSchema(
	ctx context.Context,
	request gen.GetClusterComponentTypeSchemaRequestObject,
) (gen.GetClusterComponentTypeSchemaResponseObject, error) {
	h.logger.Debug("GetClusterComponentTypeSchema called", "name", request.CctName)

	jsonSchema, err := h.services.ClusterComponentTypeService.GetClusterComponentTypeSchema(ctx, request.CctName)
	if err != nil {
		if errors.Is(err, services.ErrClusterComponentTypeNotFound) {
			return gen.GetClusterComponentTypeSchema404JSONResponse{NotFoundJSONResponse: notFound("cluster component type")}, nil
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

func toGenClusterComponentType(ct *models.ComponentTypeResponse) gen.ClusterComponentType {
	result := gen.ClusterComponentType{
		Name:         ct.Name,
		DisplayName:  ptr.To(ct.DisplayName),
		Description:  ptr.To(ct.Description),
		WorkloadType: ct.WorkloadType,
		CreatedAt:    ct.CreatedAt,
	}
	if len(ct.AllowedWorkflows) > 0 {
		result.AllowedWorkflows = ptr.To(ct.AllowedWorkflows)
	}
	if len(ct.AllowedTraits) > 0 {
		traitStrings := make([]string, len(ct.AllowedTraits))
		for i, t := range ct.AllowedTraits {
			traitStrings[i] = t.Name
			if t.Kind != "" && t.Kind != "Trait" {
				traitStrings[i] = t.Kind + ":" + t.Name
			}
		}
		result.AllowedTraits = ptr.To(traitStrings)
	}
	return result
}
