// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	resourcetypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcetype"
)

// ListResourceTypes returns a paginated list of resource types within a namespace.
func (h *Handler) ListResourceTypes(
	ctx context.Context,
	request gen.ListResourceTypesRequestObject,
) (gen.ListResourceTypesResponseObject, error) {
	h.logger.Debug("ListResourceTypes called", "namespaceName", request.NamespaceName)

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.ResourceTypeService.ListResourceTypes(ctx, request.NamespaceName, opts)
	if err != nil {
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.ListResourceTypes400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to list resource types", "error", err)
		return gen.ListResourceTypes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ResourceType, gen.ResourceType](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert resource types", "error", err)
		return gen.ListResourceTypes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListResourceTypes200JSONResponse{
		Items:      items,
		Pagination: ToPagination(result),
	}, nil
}

// CreateResourceType creates a new resource type within a namespace.
func (h *Handler) CreateResourceType(
	ctx context.Context,
	request gen.CreateResourceTypeRequestObject,
) (gen.CreateResourceTypeResponseObject, error) {
	h.logger.Info("CreateResourceType called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateResourceType400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	rtCR, err := convert[gen.ResourceType, openchoreov1alpha1.ResourceType](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateResourceType400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	created, err := h.services.ResourceTypeService.CreateResourceType(ctx, request.NamespaceName, &rtCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateResourceType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, resourcetypesvc.ErrResourceTypeAlreadyExists) {
			return gen.CreateResourceType409JSONResponse{ConflictJSONResponse: conflict("Resource type already exists")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.CreateResourceType400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to create resource type", "error", err)
		return gen.CreateResourceType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRT, err := convert[openchoreov1alpha1.ResourceType, gen.ResourceType](*created)
	if err != nil {
		h.logger.Error("Failed to convert created resource type", "error", err)
		return gen.CreateResourceType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Resource type created successfully", "namespaceName", request.NamespaceName, "resourceType", created.Name)
	return gen.CreateResourceType201JSONResponse(genRT), nil
}

// GetResourceType returns details of a specific resource type.
func (h *Handler) GetResourceType(
	ctx context.Context,
	request gen.GetResourceTypeRequestObject,
) (gen.GetResourceTypeResponseObject, error) {
	h.logger.Debug("GetResourceType called", "namespaceName", request.NamespaceName, "rtName", request.RtName)

	rt, err := h.services.ResourceTypeService.GetResourceType(ctx, request.NamespaceName, request.RtName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetResourceType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, resourcetypesvc.ErrResourceTypeNotFound) {
			return gen.GetResourceType404JSONResponse{NotFoundJSONResponse: notFound("Resource type")}, nil
		}
		h.logger.Error("Failed to get resource type", "error", err)
		return gen.GetResourceType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRT, err := convert[openchoreov1alpha1.ResourceType, gen.ResourceType](*rt)
	if err != nil {
		h.logger.Error("Failed to convert resource type", "error", err)
		return gen.GetResourceType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetResourceType200JSONResponse(genRT), nil
}

// UpdateResourceType replaces an existing resource type (full update).
func (h *Handler) UpdateResourceType(
	ctx context.Context,
	request gen.UpdateResourceTypeRequestObject,
) (gen.UpdateResourceTypeResponseObject, error) {
	h.logger.Info("UpdateResourceType called", "namespaceName", request.NamespaceName, "rtName", request.RtName)

	if request.Body == nil {
		return gen.UpdateResourceType400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	rtCR, err := convert[gen.ResourceType, openchoreov1alpha1.ResourceType](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateResourceType400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	rtCR.Name = request.RtName

	updated, err := h.services.ResourceTypeService.UpdateResourceType(ctx, request.NamespaceName, &rtCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateResourceType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, resourcetypesvc.ErrResourceTypeNotFound) {
			return gen.UpdateResourceType404JSONResponse{NotFoundJSONResponse: notFound("Resource type")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.UpdateResourceType400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to update resource type", "error", err)
		return gen.UpdateResourceType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRT, err := convert[openchoreov1alpha1.ResourceType, gen.ResourceType](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated resource type", "error", err)
		return gen.UpdateResourceType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Resource type updated successfully", "namespaceName", request.NamespaceName, "resourceType", updated.Name)
	return gen.UpdateResourceType200JSONResponse(genRT), nil
}

// DeleteResourceType deletes a resource type by name.
func (h *Handler) DeleteResourceType(
	ctx context.Context,
	request gen.DeleteResourceTypeRequestObject,
) (gen.DeleteResourceTypeResponseObject, error) {
	h.logger.Info("DeleteResourceType called", "namespaceName", request.NamespaceName, "rtName", request.RtName)

	err := h.services.ResourceTypeService.DeleteResourceType(ctx, request.NamespaceName, request.RtName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteResourceType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, resourcetypesvc.ErrResourceTypeNotFound) {
			return gen.DeleteResourceType404JSONResponse{NotFoundJSONResponse: notFound("Resource type")}, nil
		}
		h.logger.Error("Failed to delete resource type", "error", err)
		return gen.DeleteResourceType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Resource type deleted successfully", "namespaceName", request.NamespaceName, "resourceType", request.RtName)
	return gen.DeleteResourceType204Response{}, nil
}

// GetResourceTypeSchema returns the parameter schema for a resource type.
func (h *Handler) GetResourceTypeSchema(
	ctx context.Context,
	request gen.GetResourceTypeSchemaRequestObject,
) (gen.GetResourceTypeSchemaResponseObject, error) {
	h.logger.Debug("GetResourceTypeSchema called", "namespaceName", request.NamespaceName, "rtName", request.RtName)

	rawSchema, err := h.services.ResourceTypeService.GetResourceTypeSchema(ctx, request.NamespaceName, request.RtName)
	if err != nil {
		if errors.Is(err, resourcetypesvc.ErrResourceTypeNotFound) {
			return gen.GetResourceTypeSchema404JSONResponse{NotFoundJSONResponse: notFound("Resource type")}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetResourceTypeSchema403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to get resource type schema", "error", err)
		return gen.GetResourceTypeSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetResourceTypeSchema200JSONResponse(rawSchema), nil
}
