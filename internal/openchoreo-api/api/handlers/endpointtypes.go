// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	endpointtypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointtype"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// ListEndpointTypes returns a paginated list of endpoint types within a namespace.
func (h *Handler) ListEndpointTypes(
	ctx context.Context,
	request gen.ListEndpointTypesRequestObject,
) (gen.ListEndpointTypesResponseObject, error) {
	h.logger.Debug("ListEndpointTypes called", "namespaceName", request.NamespaceName)

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.EndpointTypeService.ListEndpointTypes(ctx, request.NamespaceName, opts)
	if err != nil {
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.ListEndpointTypes400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to list endpoint types", "error", err)
		return gen.ListEndpointTypes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.EndpointType, gen.EndpointType](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert endpoint types", "error", err)
		return gen.ListEndpointTypes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListEndpointTypes200JSONResponse{
		Items:      items,
		Pagination: ToPagination(result),
	}, nil
}

// CreateEndpointType creates a new endpoint type within a namespace.
func (h *Handler) CreateEndpointType(
	ctx context.Context,
	request gen.CreateEndpointTypeRequestObject,
) (gen.CreateEndpointTypeResponseObject, error) {
	h.logger.Info("CreateEndpointType called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateEndpointType400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	rtCR, err := convert[gen.EndpointType, openchoreov1alpha1.EndpointType](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateEndpointType400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	created, err := h.services.EndpointTypeService.CreateEndpointType(ctx, request.NamespaceName, &rtCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateEndpointType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, endpointtypesvc.ErrEndpointTypeAlreadyExists) {
			return gen.CreateEndpointType409JSONResponse{ConflictJSONResponse: conflict("Endpoint type already exists")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.CreateEndpointType400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to create endpoint type", "error", err)
		return gen.CreateEndpointType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	audit.SetResource(ctx, &audit.Resource{Namespace: request.NamespaceName, UID: string(created.UID), Name: created.Name})

	genRT, err := convert[openchoreov1alpha1.EndpointType, gen.EndpointType](*created)
	if err != nil {
		h.logger.Error("Failed to convert created endpoint type", "error", err)
		return gen.CreateEndpointType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Endpoint type created successfully", "namespaceName", request.NamespaceName, "endpointType", created.Name)
	return gen.CreateEndpointType201JSONResponse(genRT), nil
}

// GetEndpointType returns details of a specific endpoint type.
func (h *Handler) GetEndpointType(
	ctx context.Context,
	request gen.GetEndpointTypeRequestObject,
) (gen.GetEndpointTypeResponseObject, error) {
	h.logger.Debug("GetEndpointType called", "namespaceName", request.NamespaceName, "etName", request.EtName)

	rt, err := h.services.EndpointTypeService.GetEndpointType(ctx, request.NamespaceName, request.EtName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetEndpointType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, endpointtypesvc.ErrEndpointTypeNotFound) {
			return gen.GetEndpointType404JSONResponse{NotFoundJSONResponse: notFound("Endpoint type")}, nil
		}
		h.logger.Error("Failed to get endpoint type", "error", err)
		return gen.GetEndpointType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRT, err := convert[openchoreov1alpha1.EndpointType, gen.EndpointType](*rt)
	if err != nil {
		h.logger.Error("Failed to convert endpoint type", "error", err)
		return gen.GetEndpointType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetEndpointType200JSONResponse(genRT), nil
}

// UpdateEndpointType replaces an existing endpoint type (full update).
func (h *Handler) UpdateEndpointType(
	ctx context.Context,
	request gen.UpdateEndpointTypeRequestObject,
) (gen.UpdateEndpointTypeResponseObject, error) {
	h.logger.Info("UpdateEndpointType called", "namespaceName", request.NamespaceName, "etName", request.EtName)

	if request.Body == nil {
		return gen.UpdateEndpointType400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	rtCR, err := convert[gen.EndpointType, openchoreov1alpha1.EndpointType](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateEndpointType400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	rtCR.Name = request.EtName

	updated, err := h.services.EndpointTypeService.UpdateEndpointType(ctx, request.NamespaceName, &rtCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateEndpointType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, endpointtypesvc.ErrEndpointTypeNotFound) {
			return gen.UpdateEndpointType404JSONResponse{NotFoundJSONResponse: notFound("Endpoint type")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.UpdateEndpointType400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to update endpoint type", "error", err)
		return gen.UpdateEndpointType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	audit.SetResource(ctx, &audit.Resource{Namespace: request.NamespaceName, UID: string(updated.UID), Name: updated.Name})

	genRT, err := convert[openchoreov1alpha1.EndpointType, gen.EndpointType](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated endpoint type", "error", err)
		return gen.UpdateEndpointType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Endpoint type updated successfully", "namespaceName", request.NamespaceName, "endpointType", updated.Name)
	return gen.UpdateEndpointType200JSONResponse(genRT), nil
}

// DeleteEndpointType deletes a endpoint type by name.
func (h *Handler) DeleteEndpointType(
	ctx context.Context,
	request gen.DeleteEndpointTypeRequestObject,
) (gen.DeleteEndpointTypeResponseObject, error) {
	h.logger.Info("DeleteEndpointType called", "namespaceName", request.NamespaceName, "etName", request.EtName)

	err := h.services.EndpointTypeService.DeleteEndpointType(ctx, request.NamespaceName, request.EtName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteEndpointType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, endpointtypesvc.ErrEndpointTypeNotFound) {
			return gen.DeleteEndpointType404JSONResponse{NotFoundJSONResponse: notFound("Endpoint type")}, nil
		}
		h.logger.Error("Failed to delete endpoint type", "error", err)
		return gen.DeleteEndpointType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Endpoint type deleted successfully", "namespaceName", request.NamespaceName, "endpointType", request.EtName)
	return gen.DeleteEndpointType204Response{}, nil
}

// GetEndpointTypeSchema returns the parameter schema for a endpoint type.
func (h *Handler) GetEndpointTypeSchema(
	ctx context.Context,
	request gen.GetEndpointTypeSchemaRequestObject,
) (gen.GetEndpointTypeSchemaResponseObject, error) {
	h.logger.Debug("GetEndpointTypeSchema called", "namespaceName", request.NamespaceName, "etName", request.EtName)

	rawSchema, err := h.services.EndpointTypeService.GetEndpointTypeSchema(ctx, request.NamespaceName, request.EtName)
	if err != nil {
		if errors.Is(err, endpointtypesvc.ErrEndpointTypeNotFound) {
			return gen.GetEndpointTypeSchema404JSONResponse{NotFoundJSONResponse: notFound("Endpoint type")}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetEndpointTypeSchema403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to get endpoint type schema", "error", err)
		return gen.GetEndpointTypeSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetEndpointTypeSchema200JSONResponse(rawSchema), nil
}
