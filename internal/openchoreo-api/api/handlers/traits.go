// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ListTraits returns a list of traits
func (h *Handler) ListTraits(
	ctx context.Context,
	request gen.ListTraitsRequestObject,
) (gen.ListTraitsResponseObject, error) {
	return nil, errNotImplemented
}

// GetTraitSchema returns the parameter schema for a trait
func (h *Handler) GetTraitSchema(
	ctx context.Context,
	request gen.GetTraitSchemaRequestObject,
) (gen.GetTraitSchemaResponseObject, error) {
	return nil, errNotImplemented
}
