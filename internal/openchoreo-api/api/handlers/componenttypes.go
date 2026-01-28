// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// ListComponentTypes returns a list of component types
func (h *Handler) ListComponentTypes(
	ctx context.Context,
	request gen.ListComponentTypesRequestObject,
) (gen.ListComponentTypesResponseObject, error) {
	h.logger.Debug("ListComponentTypes called", "namespaceName", request.NamespaceName)

	componentTypes, err := h.services.ComponentTypeService.ListComponentTypes(ctx, request.NamespaceName)
	if err != nil {
		h.logger.Error("Failed to list component types", "error", err)
		return gen.ListComponentTypes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert to generated types
	items := make([]gen.ComponentType, 0, len(componentTypes))
	for _, ct := range componentTypes {
		items = append(items, toGenComponentType(ct))
	}

	// TODO: Implement proper cursor-based pagination with Kubernetes continuation tokens
	return gen.ListComponentTypes200JSONResponse{
		Items:      items,
		Pagination: gen.Pagination{},
	}, nil
}

// toGenComponentType converts models.ComponentTypeResponse to gen.ComponentType
func toGenComponentType(ct *models.ComponentTypeResponse) gen.ComponentType {
	result := gen.ComponentType{
		Name:         ct.Name,
		DisplayName:  ptr.To(ct.DisplayName),
		Description:  ptr.To(ct.Description),
		WorkloadType: ct.WorkloadType,
		CreatedAt:    ct.CreatedAt,
	}
	if len(ct.AllowedWorkflows) > 0 {
		result.AllowedWorkflows = ptr.To(ct.AllowedWorkflows)
	}
	return result
}

// GetComponentTypeSchema returns the parameter schema for a component type
func (h *Handler) GetComponentTypeSchema(
	ctx context.Context,
	request gen.GetComponentTypeSchemaRequestObject,
) (gen.GetComponentTypeSchemaResponseObject, error) {
	return nil, errNotImplemented
}
