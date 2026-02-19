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

// ListClusterTraits returns a list of cluster-scoped traits
func (h *Handler) ListClusterTraits(
	ctx context.Context,
	request gen.ListClusterTraitsRequestObject,
) (gen.ListClusterTraitsResponseObject, error) {
	h.logger.Debug("ListClusterTraits called")

	traits, err := h.services.ClusterTraitService.ListClusterTraits(ctx)
	if err != nil {
		h.logger.Error("Failed to list cluster traits", "error", err)
		return gen.ListClusterTraits500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items := make([]gen.ClusterTrait, 0, len(traits))
	for _, t := range traits {
		items = append(items, toGenClusterTrait(t))
	}

	return gen.ListClusterTraits200JSONResponse{
		Items:      items,
		Pagination: gen.Pagination{},
	}, nil
}

// GetClusterTraitSchema returns the parameter schema for a cluster-scoped trait
func (h *Handler) GetClusterTraitSchema(
	ctx context.Context,
	request gen.GetClusterTraitSchemaRequestObject,
) (gen.GetClusterTraitSchemaResponseObject, error) {
	h.logger.Debug("GetClusterTraitSchema called", "name", request.ClusterTraitName)

	jsonSchema, err := h.services.ClusterTraitService.GetClusterTraitSchema(ctx, request.ClusterTraitName)
	if err != nil {
		if errors.Is(err, services.ErrClusterTraitNotFound) {
			return gen.GetClusterTraitSchema404JSONResponse{NotFoundJSONResponse: notFound("cluster trait")}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetClusterTraitSchema403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to get cluster trait schema", "error", err)
		return gen.GetClusterTraitSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert JSONSchemaProps to SchemaResponse (map[string]interface{})
	data, err := json.Marshal(jsonSchema)
	if err != nil {
		h.logger.Error("Failed to marshal schema", "error", err)
		return gen.GetClusterTraitSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	var schemaResp gen.SchemaResponse
	if err := json.Unmarshal(data, &schemaResp); err != nil {
		h.logger.Error("Failed to unmarshal schema response", "error", err)
		return gen.GetClusterTraitSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetClusterTraitSchema200JSONResponse(schemaResp), nil
}

func toGenClusterTrait(t *models.TraitResponse) gen.ClusterTrait {
	return gen.ClusterTrait{
		Name:        t.Name,
		DisplayName: ptr.To(t.DisplayName),
		Description: ptr.To(t.Description),
		CreatedAt:   t.CreatedAt,
	}
}
