// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	endpointreleasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointreleasebinding"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// ListEndpointReleaseBindings returns a paginated list of endpoint release bindings within a namespace.
func (h *Handler) ListEndpointReleaseBindings(
	ctx context.Context,
	request gen.ListEndpointReleaseBindingsRequestObject,
) (gen.ListEndpointReleaseBindingsResponseObject, error) {
	h.logger.Debug("ListEndpointReleaseBindings called", "namespaceName", request.NamespaceName)

	endpointName := ""
	if request.Params.Endpoint != nil {
		endpointName = *request.Params.Endpoint
	}

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.EndpointReleaseBindingService.ListEndpointReleaseBindings(ctx, request.NamespaceName, endpointName, opts)
	if err != nil {
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.ListEndpointReleaseBindings400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to list endpoint release bindings", "error", err)
		return gen.ListEndpointReleaseBindings500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.EndpointReleaseBinding, gen.EndpointReleaseBinding](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert endpoint release bindings", "error", err)
		return gen.ListEndpointReleaseBindings500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListEndpointReleaseBindings200JSONResponse{
		Items:      items,
		Pagination: ToPagination(result),
	}, nil
}

// CreateEndpointReleaseBinding creates a new endpoint release binding within a namespace.
func (h *Handler) CreateEndpointReleaseBinding(
	ctx context.Context,
	request gen.CreateEndpointReleaseBindingRequestObject,
) (gen.CreateEndpointReleaseBindingResponseObject, error) {
	h.logger.Info("CreateEndpointReleaseBinding called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateEndpointReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	rbCR, err := convert[gen.EndpointReleaseBinding, openchoreov1alpha1.EndpointReleaseBinding](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateEndpointReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	created, err := h.services.EndpointReleaseBindingService.CreateEndpointReleaseBinding(ctx, request.NamespaceName, &rbCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateEndpointReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, endpointreleasebindingsvc.ErrEndpointNotFound) {
			return gen.CreateEndpointReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Referenced endpoint not found")}, nil
		}
		if errors.Is(err, endpointreleasebindingsvc.ErrEndpointReleaseBindingAlreadyExists) {
			return gen.CreateEndpointReleaseBinding409JSONResponse{ConflictJSONResponse: conflict("EndpointReleaseBinding already exists")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.CreateEndpointReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to create endpoint release binding", "error", err)
		return gen.CreateEndpointReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	audit.SetResource(ctx, &audit.Resource{Namespace: request.NamespaceName, UID: string(created.UID), Name: created.Name})

	genRB, err := convert[openchoreov1alpha1.EndpointReleaseBinding, gen.EndpointReleaseBinding](*created)
	if err != nil {
		h.logger.Error("Failed to convert created endpoint release binding", "error", err)
		return gen.CreateEndpointReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("EndpointReleaseBinding created successfully", "namespaceName", request.NamespaceName, "endpointReleaseBinding", created.Name)
	return gen.CreateEndpointReleaseBinding201JSONResponse(genRB), nil
}

// GetEndpointReleaseBinding returns details of a specific endpoint release binding.
func (h *Handler) GetEndpointReleaseBinding(
	ctx context.Context,
	request gen.GetEndpointReleaseBindingRequestObject,
) (gen.GetEndpointReleaseBindingResponseObject, error) {
	h.logger.Debug("GetEndpointReleaseBinding called", "namespaceName", request.NamespaceName, "endpointReleaseBindingName", request.EndpointReleaseBindingName)

	rb, err := h.services.EndpointReleaseBindingService.GetEndpointReleaseBinding(ctx, request.NamespaceName, request.EndpointReleaseBindingName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetEndpointReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, endpointreleasebindingsvc.ErrEndpointReleaseBindingNotFound) {
			return gen.GetEndpointReleaseBinding404JSONResponse{NotFoundJSONResponse: notFound("EndpointReleaseBinding")}, nil
		}
		h.logger.Error("Failed to get endpoint release binding", "error", err)
		return gen.GetEndpointReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRB, err := convert[openchoreov1alpha1.EndpointReleaseBinding, gen.EndpointReleaseBinding](*rb)
	if err != nil {
		h.logger.Error("Failed to convert endpoint release binding", "error", err)
		return gen.GetEndpointReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetEndpointReleaseBinding200JSONResponse(genRB), nil
}

// UpdateEndpointReleaseBinding replaces an existing endpoint release binding (full update).
func (h *Handler) UpdateEndpointReleaseBinding(
	ctx context.Context,
	request gen.UpdateEndpointReleaseBindingRequestObject,
) (gen.UpdateEndpointReleaseBindingResponseObject, error) {
	h.logger.Info("UpdateEndpointReleaseBinding called", "namespaceName", request.NamespaceName, "endpointReleaseBindingName", request.EndpointReleaseBindingName)

	if request.Body == nil {
		return gen.UpdateEndpointReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	rbCR, err := convert[gen.EndpointReleaseBinding, openchoreov1alpha1.EndpointReleaseBinding](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateEndpointReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	rbCR.Name = request.EndpointReleaseBindingName

	updated, err := h.services.EndpointReleaseBindingService.UpdateEndpointReleaseBinding(ctx, request.NamespaceName, &rbCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateEndpointReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, endpointreleasebindingsvc.ErrEndpointReleaseBindingNotFound) {
			return gen.UpdateEndpointReleaseBinding404JSONResponse{NotFoundJSONResponse: notFound("EndpointReleaseBinding")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.UpdateEndpointReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to update endpoint release binding", "error", err)
		return gen.UpdateEndpointReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	audit.SetResource(ctx, &audit.Resource{Namespace: request.NamespaceName, UID: string(updated.UID), Name: updated.Name})

	genRB, err := convert[openchoreov1alpha1.EndpointReleaseBinding, gen.EndpointReleaseBinding](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated endpoint release binding", "error", err)
		return gen.UpdateEndpointReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("EndpointReleaseBinding updated successfully", "namespaceName", request.NamespaceName, "endpointReleaseBinding", updated.Name)
	return gen.UpdateEndpointReleaseBinding200JSONResponse(genRB), nil
}

// DeleteEndpointReleaseBinding deletes a endpoint release binding by name.
func (h *Handler) DeleteEndpointReleaseBinding(
	ctx context.Context,
	request gen.DeleteEndpointReleaseBindingRequestObject,
) (gen.DeleteEndpointReleaseBindingResponseObject, error) {
	h.logger.Info("DeleteEndpointReleaseBinding called", "namespaceName", request.NamespaceName, "endpointReleaseBindingName", request.EndpointReleaseBindingName)

	err := h.services.EndpointReleaseBindingService.DeleteEndpointReleaseBinding(ctx, request.NamespaceName, request.EndpointReleaseBindingName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteEndpointReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, endpointreleasebindingsvc.ErrEndpointReleaseBindingNotFound) {
			return gen.DeleteEndpointReleaseBinding404JSONResponse{NotFoundJSONResponse: notFound("EndpointReleaseBinding")}, nil
		}
		h.logger.Error("Failed to delete endpoint release binding", "error", err)
		return gen.DeleteEndpointReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("EndpointReleaseBinding deleted successfully", "namespaceName", request.NamespaceName, "endpointReleaseBinding", request.EndpointReleaseBindingName)
	return gen.DeleteEndpointReleaseBinding204Response{}, nil
}
