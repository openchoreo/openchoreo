// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// ListTraits returns a list of traits
func (h *Handler) ListTraits(
	ctx context.Context,
	request gen.ListTraitsRequestObject,
) (gen.ListTraitsResponseObject, error) {
	h.logger.Debug("ListTraits called", "namespaceName", request.NamespaceName)

	traits, err := h.services.TraitService.ListTraits(ctx, request.NamespaceName)
	if err != nil {
		h.logger.Error("Failed to list traits", "error", err)
		return gen.ListTraits500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert to generated types
	items := make([]gen.Trait, 0, len(traits))
	for _, t := range traits {
		items = append(items, toGenTrait(t))
	}

	// TODO: Implement proper cursor-based pagination with Kubernetes continuation tokens
	return gen.ListTraits200JSONResponse{
		Items:      items,
		Pagination: gen.Pagination{},
	}, nil
}

// toGenTrait converts models.TraitResponse to gen.Trait
func toGenTrait(t *models.TraitResponse) gen.Trait {
	return gen.Trait{
		Name:        t.Name,
		DisplayName: ptr.To(t.DisplayName),
		Description: ptr.To(t.Description),
		CreatedAt:   t.CreatedAt,
	}
}

// GetTraitSchema returns the parameter schema for a trait
func (h *Handler) GetTraitSchema(
	ctx context.Context,
	request gen.GetTraitSchemaRequestObject,
) (gen.GetTraitSchemaResponseObject, error) {
	return nil, errNotImplemented
}
