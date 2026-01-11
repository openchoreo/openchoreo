// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ListComponentTypes returns a list of component types
func (h *Handler) ListComponentTypes(
	ctx context.Context,
	request gen.ListComponentTypesRequestObject,
) (gen.ListComponentTypesResponseObject, error) {
	return nil, errNotImplemented
}

// GetComponentTypeSchema returns the parameter schema for a component type
func (h *Handler) GetComponentTypeSchema(
	ctx context.Context,
	request gen.GetComponentTypeSchemaRequestObject,
) (gen.GetComponentTypeSchemaResponseObject, error) {
	return nil, errNotImplemented
}
