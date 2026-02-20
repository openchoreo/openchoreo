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
	traitsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/trait"
)

// ListTraits returns a paginated list of traits within a namespace.
func (h *Handler) ListTraits(
	ctx context.Context,
	request gen.ListTraitsRequestObject,
) (gen.ListTraitsResponseObject, error) {
	h.logger.Debug("ListTraits called", "namespaceName", request.NamespaceName)

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.traitService.ListTraits(ctx, request.NamespaceName, opts)
	if err != nil {
		h.logger.Error("Failed to list traits", "error", err)
		return gen.ListTraits500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.Trait, gen.Trait](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert traits", "error", err)
		return gen.ListTraits500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListTraits200JSONResponse{
		Items:      items,
		Pagination: ToPaginationPtr(result),
	}, nil
}

// CreateTrait creates a new trait within a namespace.
func (h *Handler) CreateTrait(
	ctx context.Context,
	request gen.CreateTraitRequestObject,
) (gen.CreateTraitResponseObject, error) {
	h.logger.Info("CreateTrait called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateTrait400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	tCR, err := convert[gen.Trait, openchoreov1alpha1.Trait](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateTrait400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	tCR.Status = openchoreov1alpha1.TraitStatus{}

	created, err := h.traitService.CreateTrait(ctx, request.NamespaceName, &tCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateTrait403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, traitsvc.ErrTraitAlreadyExists) {
			return gen.CreateTrait409JSONResponse{ConflictJSONResponse: conflict("Trait already exists")}, nil
		}
		h.logger.Error("Failed to create trait", "error", err)
		return gen.CreateTrait500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genTrait, err := convert[openchoreov1alpha1.Trait, gen.Trait](*created)
	if err != nil {
		h.logger.Error("Failed to convert created trait", "error", err)
		return gen.CreateTrait500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Trait created successfully", "namespaceName", request.NamespaceName, "trait", created.Name)
	return gen.CreateTrait201JSONResponse(genTrait), nil
}

// GetTrait returns details of a specific trait.
func (h *Handler) GetTrait(
	ctx context.Context,
	request gen.GetTraitRequestObject,
) (gen.GetTraitResponseObject, error) {
	h.logger.Debug("GetTrait called", "namespaceName", request.NamespaceName, "traitName", request.TraitName)

	t, err := h.traitService.GetTrait(ctx, request.NamespaceName, request.TraitName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetTrait403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, traitsvc.ErrTraitNotFound) {
			return gen.GetTrait404JSONResponse{NotFoundJSONResponse: notFound("Trait")}, nil
		}
		h.logger.Error("Failed to get trait", "error", err)
		return gen.GetTrait500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genTrait, err := convert[openchoreov1alpha1.Trait, gen.Trait](*t)
	if err != nil {
		h.logger.Error("Failed to convert trait", "error", err)
		return gen.GetTrait500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetTrait200JSONResponse(genTrait), nil
}

// UpdateTrait replaces an existing trait (full update).
func (h *Handler) UpdateTrait(
	ctx context.Context,
	request gen.UpdateTraitRequestObject,
) (gen.UpdateTraitResponseObject, error) {
	h.logger.Info("UpdateTrait called", "namespaceName", request.NamespaceName, "traitName", request.TraitName)

	if request.Body == nil {
		return gen.UpdateTrait400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	tCR, err := convert[gen.Trait, openchoreov1alpha1.Trait](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateTrait400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	tCR.Status = openchoreov1alpha1.TraitStatus{}

	// Ensure the name from the URL path is used
	tCR.Name = request.TraitName

	updated, err := h.traitService.UpdateTrait(ctx, request.NamespaceName, &tCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateTrait403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, traitsvc.ErrTraitNotFound) {
			return gen.UpdateTrait404JSONResponse{NotFoundJSONResponse: notFound("Trait")}, nil
		}
		h.logger.Error("Failed to update trait", "error", err)
		return gen.UpdateTrait500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genTrait, err := convert[openchoreov1alpha1.Trait, gen.Trait](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated trait", "error", err)
		return gen.UpdateTrait500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Trait updated successfully", "namespaceName", request.NamespaceName, "trait", updated.Name)
	return gen.UpdateTrait200JSONResponse(genTrait), nil
}

// DeleteTrait deletes a trait by name.
func (h *Handler) DeleteTrait(
	ctx context.Context,
	request gen.DeleteTraitRequestObject,
) (gen.DeleteTraitResponseObject, error) {
	h.logger.Info("DeleteTrait called", "namespaceName", request.NamespaceName, "traitName", request.TraitName)

	err := h.traitService.DeleteTrait(ctx, request.NamespaceName, request.TraitName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteTrait403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, traitsvc.ErrTraitNotFound) {
			return gen.DeleteTrait404JSONResponse{NotFoundJSONResponse: notFound("Trait")}, nil
		}
		h.logger.Error("Failed to delete trait", "error", err)
		return gen.DeleteTrait500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Trait deleted successfully", "namespaceName", request.NamespaceName, "trait", request.TraitName)
	return gen.DeleteTrait204Response{}, nil
}

// GetTraitSchema returns the parameter schema for a trait
func (h *Handler) GetTraitSchema(
	ctx context.Context,
	request gen.GetTraitSchemaRequestObject,
) (gen.GetTraitSchemaResponseObject, error) {
	h.logger.Debug("GetTraitSchema called", "namespaceName", request.NamespaceName, "traitName", request.TraitName)

	jsonSchema, err := h.traitService.GetTraitSchema(ctx, request.NamespaceName, request.TraitName)
	if err != nil {
		if errors.Is(err, traitsvc.ErrTraitNotFound) {
			return gen.GetTraitSchema404JSONResponse{NotFoundJSONResponse: notFound("Trait")}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetTraitSchema403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to get trait schema", "error", err)
		return gen.GetTraitSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert JSONSchemaProps to SchemaResponse (map[string]interface{})
	data, err := json.Marshal(jsonSchema)
	if err != nil {
		h.logger.Error("Failed to marshal schema", "error", err)
		return gen.GetTraitSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	var schemaResp gen.SchemaResponse
	if err := json.Unmarshal(data, &schemaResp); err != nil {
		h.logger.Error("Failed to unmarshal schema response", "error", err)
		return gen.GetTraitSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetTraitSchema200JSONResponse(schemaResp), nil
}
