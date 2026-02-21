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
	clustertraitsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clustertrait"
)

// ListClusterTraits returns a paginated list of cluster-scoped traits.
func (h *Handler) ListClusterTraits(
	ctx context.Context,
	request gen.ListClusterTraitsRequestObject,
) (gen.ListClusterTraitsResponseObject, error) {
	h.logger.Debug("ListClusterTraits called")

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.services.ClusterTraitService.ListClusterTraits(ctx, opts)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListClusterTraits403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to list cluster traits", "error", err)
		return gen.ListClusterTraits500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ClusterTrait, gen.ClusterTrait](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert cluster traits", "error", err)
		return gen.ListClusterTraits500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListClusterTraits200JSONResponse{
		Items:      items,
		Pagination: ToPaginationPtr(result),
	}, nil
}

// CreateClusterTrait creates a new cluster-scoped trait.
func (h *Handler) CreateClusterTrait(
	ctx context.Context,
	request gen.CreateClusterTraitRequestObject,
) (gen.CreateClusterTraitResponseObject, error) {
	h.logger.Info("CreateClusterTrait called")

	if request.Body == nil {
		return gen.CreateClusterTrait400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	ctCR, err := convert[gen.ClusterTrait, openchoreov1alpha1.ClusterTrait](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateClusterTrait400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	ctCR.Status = openchoreov1alpha1.ClusterTraitStatus{}

	created, err := h.services.ClusterTraitService.CreateClusterTrait(ctx, &ctCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateClusterTrait403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clustertraitsvc.ErrClusterTraitAlreadyExists) {
			return gen.CreateClusterTrait409JSONResponse{ConflictJSONResponse: conflict("Cluster trait already exists")}, nil
		}
		h.logger.Error("Failed to create cluster trait", "error", err)
		return gen.CreateClusterTrait500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCT, err := convert[openchoreov1alpha1.ClusterTrait, gen.ClusterTrait](*created)
	if err != nil {
		h.logger.Error("Failed to convert created cluster trait", "error", err)
		return gen.CreateClusterTrait500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster trait created successfully", "clusterTrait", created.Name)
	return gen.CreateClusterTrait201JSONResponse(genCT), nil
}

// UpdateClusterTrait replaces an existing cluster-scoped trait (full update).
func (h *Handler) UpdateClusterTrait(
	ctx context.Context,
	request gen.UpdateClusterTraitRequestObject,
) (gen.UpdateClusterTraitResponseObject, error) {
	h.logger.Info("UpdateClusterTrait called", "clusterTraitName", request.ClusterTraitName)

	if request.Body == nil {
		return gen.UpdateClusterTrait400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	ctCR, err := convert[gen.ClusterTrait, openchoreov1alpha1.ClusterTrait](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateClusterTrait400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	ctCR.Status = openchoreov1alpha1.ClusterTraitStatus{}

	// Ensure the name from the URL path is used
	ctCR.Name = request.ClusterTraitName

	updated, err := h.services.ClusterTraitService.UpdateClusterTrait(ctx, &ctCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateClusterTrait403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clustertraitsvc.ErrClusterTraitNotFound) {
			return gen.UpdateClusterTrait404JSONResponse{NotFoundJSONResponse: notFound("ClusterTrait")}, nil
		}
		h.logger.Error("Failed to update cluster trait", "error", err)
		return gen.UpdateClusterTrait500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCT, err := convert[openchoreov1alpha1.ClusterTrait, gen.ClusterTrait](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated cluster trait", "error", err)
		return gen.UpdateClusterTrait500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster trait updated successfully", "clusterTrait", updated.Name)
	return gen.UpdateClusterTrait200JSONResponse(genCT), nil
}

// GetClusterTrait returns details of a specific cluster-scoped trait.
func (h *Handler) GetClusterTrait(
	ctx context.Context,
	request gen.GetClusterTraitRequestObject,
) (gen.GetClusterTraitResponseObject, error) {
	h.logger.Debug("GetClusterTrait called", "clusterTraitName", request.ClusterTraitName)

	trait, err := h.services.ClusterTraitService.GetClusterTrait(ctx, request.ClusterTraitName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetClusterTrait403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clustertraitsvc.ErrClusterTraitNotFound) {
			return gen.GetClusterTrait404JSONResponse{NotFoundJSONResponse: notFound("ClusterTrait")}, nil
		}
		h.logger.Error("Failed to get cluster trait", "error", err)
		return gen.GetClusterTrait500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genTrait, err := convert[openchoreov1alpha1.ClusterTrait, gen.ClusterTrait](*trait)
	if err != nil {
		h.logger.Error("Failed to convert cluster trait", "error", err)
		return gen.GetClusterTrait500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetClusterTrait200JSONResponse(genTrait), nil
}

// DeleteClusterTrait deletes a cluster-scoped trait by name.
func (h *Handler) DeleteClusterTrait(
	ctx context.Context,
	request gen.DeleteClusterTraitRequestObject,
) (gen.DeleteClusterTraitResponseObject, error) {
	h.logger.Info("DeleteClusterTrait called", "clusterTraitName", request.ClusterTraitName)

	err := h.services.ClusterTraitService.DeleteClusterTrait(ctx, request.ClusterTraitName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteClusterTrait403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clustertraitsvc.ErrClusterTraitNotFound) {
			return gen.DeleteClusterTrait404JSONResponse{NotFoundJSONResponse: notFound("ClusterTrait")}, nil
		}
		h.logger.Error("Failed to delete cluster trait", "error", err)
		return gen.DeleteClusterTrait500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ClusterTrait deleted successfully", "clusterTrait", request.ClusterTraitName)
	return gen.DeleteClusterTrait204Response{}, nil
}

// GetClusterTraitSchema returns the parameter schema for a cluster-scoped trait.
func (h *Handler) GetClusterTraitSchema(
	ctx context.Context,
	request gen.GetClusterTraitSchemaRequestObject,
) (gen.GetClusterTraitSchemaResponseObject, error) {
	h.logger.Debug("GetClusterTraitSchema called", "name", request.ClusterTraitName)

	jsonSchema, err := h.services.ClusterTraitService.GetClusterTraitSchema(ctx, request.ClusterTraitName)
	if err != nil {
		if errors.Is(err, clustertraitsvc.ErrClusterTraitNotFound) {
			return gen.GetClusterTraitSchema404JSONResponse{NotFoundJSONResponse: notFound("ClusterTrait")}, nil
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
