// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	componenttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componenttype"
)

// ListComponentTypes returns a paginated list of component types within a namespace.
func (h *Handler) ListComponentTypes(
	ctx context.Context,
	request gen.ListComponentTypesRequestObject,
) (gen.ListComponentTypesResponseObject, error) {
	h.logger.Debug("ListComponentTypes called", "namespaceName", request.NamespaceName)

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.componentTypeService.ListComponentTypes(ctx, request.NamespaceName, opts)
	if err != nil {
		h.logger.Error("Failed to list component types", "error", err)
		return gen.ListComponentTypes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ComponentType, gen.ComponentType](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert component types", "error", err)
		return gen.ListComponentTypes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListComponentTypes200JSONResponse{
		Items:      items,
		Pagination: ToPaginationPtr(result),
	}, nil
}

// CreateComponentType creates a new component type within a namespace.
func (h *Handler) CreateComponentType(
	ctx context.Context,
	request gen.CreateComponentTypeRequestObject,
) (gen.CreateComponentTypeResponseObject, error) {
	h.logger.Info("CreateComponentType called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateComponentType400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	ctCR, err := convert[gen.ComponentType, openchoreov1alpha1.ComponentType](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateComponentType400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	ctCR.Status = openchoreov1alpha1.ComponentTypeStatus{}

	created, err := h.componentTypeService.CreateComponentType(ctx, request.NamespaceName, &ctCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateComponentType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, componenttypesvc.ErrComponentTypeAlreadyExists) {
			return gen.CreateComponentType409JSONResponse{ConflictJSONResponse: conflict("Component type already exists")}, nil
		}
		h.logger.Error("Failed to create component type", "error", err)
		return gen.CreateComponentType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCT, err := convert[openchoreov1alpha1.ComponentType, gen.ComponentType](*created)
	if err != nil {
		h.logger.Error("Failed to convert created component type", "error", err)
		return gen.CreateComponentType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Component type created successfully", "namespaceName", request.NamespaceName, "componentType", created.Name)
	return gen.CreateComponentType201JSONResponse(genCT), nil
}

// GetComponentType returns details of a specific component type.
func (h *Handler) GetComponentType(
	ctx context.Context,
	request gen.GetComponentTypeRequestObject,
) (gen.GetComponentTypeResponseObject, error) {
	h.logger.Debug("GetComponentType called", "namespaceName", request.NamespaceName, "ctName", request.CtName)

	ct, err := h.componentTypeService.GetComponentType(ctx, request.NamespaceName, request.CtName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetComponentType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, componenttypesvc.ErrComponentTypeNotFound) {
			return gen.GetComponentType404JSONResponse{NotFoundJSONResponse: notFound("Component type")}, nil
		}
		h.logger.Error("Failed to get component type", "error", err)
		return gen.GetComponentType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCT, err := convert[openchoreov1alpha1.ComponentType, gen.ComponentType](*ct)
	if err != nil {
		h.logger.Error("Failed to convert component type", "error", err)
		return gen.GetComponentType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetComponentType200JSONResponse(genCT), nil
}

// UpdateComponentType replaces an existing component type (full update).
func (h *Handler) UpdateComponentType(
	ctx context.Context,
	request gen.UpdateComponentTypeRequestObject,
) (gen.UpdateComponentTypeResponseObject, error) {
	h.logger.Info("UpdateComponentType called", "namespaceName", request.NamespaceName, "ctName", request.CtName)

	if request.Body == nil {
		return gen.UpdateComponentType400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	ctCR, err := convert[gen.ComponentType, openchoreov1alpha1.ComponentType](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateComponentType400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	ctCR.Status = openchoreov1alpha1.ComponentTypeStatus{}

	// Ensure the name from the URL path is used
	ctCR.Name = request.CtName

	updated, err := h.componentTypeService.UpdateComponentType(ctx, request.NamespaceName, &ctCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateComponentType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, componenttypesvc.ErrComponentTypeNotFound) {
			return gen.UpdateComponentType404JSONResponse{NotFoundJSONResponse: notFound("Component type")}, nil
		}
		h.logger.Error("Failed to update component type", "error", err)
		return gen.UpdateComponentType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCT, err := convert[openchoreov1alpha1.ComponentType, gen.ComponentType](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated component type", "error", err)
		return gen.UpdateComponentType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Component type updated successfully", "namespaceName", request.NamespaceName, "componentType", updated.Name)
	return gen.UpdateComponentType200JSONResponse(genCT), nil
}

// DeleteComponentType deletes a component type by name.
func (h *Handler) DeleteComponentType(
	ctx context.Context,
	request gen.DeleteComponentTypeRequestObject,
) (gen.DeleteComponentTypeResponseObject, error) {
	h.logger.Info("DeleteComponentType called", "namespaceName", request.NamespaceName, "ctName", request.CtName)

	err := h.componentTypeService.DeleteComponentType(ctx, request.NamespaceName, request.CtName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteComponentType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, componenttypesvc.ErrComponentTypeNotFound) {
			return gen.DeleteComponentType404JSONResponse{NotFoundJSONResponse: notFound("Component type")}, nil
		}
		h.logger.Error("Failed to delete component type", "error", err)
		return gen.DeleteComponentType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Component type deleted successfully", "namespaceName", request.NamespaceName, "componentType", request.CtName)
	return gen.DeleteComponentType204Response{}, nil
}

// GetComponentTypeSchema returns the parameter schema for a component type
func (h *Handler) GetComponentTypeSchema(
	ctx context.Context,
	request gen.GetComponentTypeSchemaRequestObject,
) (gen.GetComponentTypeSchemaResponseObject, error) {
	return nil, errNotImplemented
}
