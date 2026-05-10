// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	resourcereleasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcereleasebinding"
)

// ListResourceReleaseBindings returns a paginated list of resource release bindings within a namespace.
func (h *Handler) ListResourceReleaseBindings(
	ctx context.Context,
	request gen.ListResourceReleaseBindingsRequestObject,
) (gen.ListResourceReleaseBindingsResponseObject, error) {
	h.logger.Debug("ListResourceReleaseBindings called", "namespaceName", request.NamespaceName)

	resourceName := ""
	if request.Params.Resource != nil {
		resourceName = *request.Params.Resource
	}

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.ResourceReleaseBindingService.ListResourceReleaseBindings(ctx, request.NamespaceName, resourceName, opts)
	if err != nil {
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.ListResourceReleaseBindings400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to list resource release bindings", "error", err)
		return gen.ListResourceReleaseBindings500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ResourceReleaseBinding, gen.ResourceReleaseBinding](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert resource release bindings", "error", err)
		return gen.ListResourceReleaseBindings500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListResourceReleaseBindings200JSONResponse{
		Items:      items,
		Pagination: ToPagination(result),
	}, nil
}

// CreateResourceReleaseBinding creates a new resource release binding within a namespace.
func (h *Handler) CreateResourceReleaseBinding(
	ctx context.Context,
	request gen.CreateResourceReleaseBindingRequestObject,
) (gen.CreateResourceReleaseBindingResponseObject, error) {
	h.logger.Info("CreateResourceReleaseBinding called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateResourceReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	rbCR, err := convert[gen.ResourceReleaseBinding, openchoreov1alpha1.ResourceReleaseBinding](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateResourceReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	created, err := h.services.ResourceReleaseBindingService.CreateResourceReleaseBinding(ctx, request.NamespaceName, &rbCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateResourceReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, resourcereleasebindingsvc.ErrResourceNotFound) {
			return gen.CreateResourceReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Referenced resource not found")}, nil
		}
		if errors.Is(err, resourcereleasebindingsvc.ErrResourceReleaseBindingAlreadyExists) {
			return gen.CreateResourceReleaseBinding409JSONResponse{ConflictJSONResponse: conflict("ResourceReleaseBinding already exists")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.CreateResourceReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to create resource release binding", "error", err)
		return gen.CreateResourceReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRB, err := convert[openchoreov1alpha1.ResourceReleaseBinding, gen.ResourceReleaseBinding](*created)
	if err != nil {
		h.logger.Error("Failed to convert created resource release binding", "error", err)
		return gen.CreateResourceReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ResourceReleaseBinding created successfully", "namespaceName", request.NamespaceName, "resourceReleaseBinding", created.Name)
	return gen.CreateResourceReleaseBinding201JSONResponse(genRB), nil
}

// GetResourceReleaseBinding returns details of a specific resource release binding.
func (h *Handler) GetResourceReleaseBinding(
	ctx context.Context,
	request gen.GetResourceReleaseBindingRequestObject,
) (gen.GetResourceReleaseBindingResponseObject, error) {
	h.logger.Debug("GetResourceReleaseBinding called", "namespaceName", request.NamespaceName, "resourceReleaseBindingName", request.ResourceReleaseBindingName)

	rb, err := h.services.ResourceReleaseBindingService.GetResourceReleaseBinding(ctx, request.NamespaceName, request.ResourceReleaseBindingName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetResourceReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, resourcereleasebindingsvc.ErrResourceReleaseBindingNotFound) {
			return gen.GetResourceReleaseBinding404JSONResponse{NotFoundJSONResponse: notFound("ResourceReleaseBinding")}, nil
		}
		h.logger.Error("Failed to get resource release binding", "error", err)
		return gen.GetResourceReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRB, err := convert[openchoreov1alpha1.ResourceReleaseBinding, gen.ResourceReleaseBinding](*rb)
	if err != nil {
		h.logger.Error("Failed to convert resource release binding", "error", err)
		return gen.GetResourceReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetResourceReleaseBinding200JSONResponse(genRB), nil
}

// UpdateResourceReleaseBinding replaces an existing resource release binding (full update).
func (h *Handler) UpdateResourceReleaseBinding(
	ctx context.Context,
	request gen.UpdateResourceReleaseBindingRequestObject,
) (gen.UpdateResourceReleaseBindingResponseObject, error) {
	h.logger.Info("UpdateResourceReleaseBinding called", "namespaceName", request.NamespaceName, "resourceReleaseBindingName", request.ResourceReleaseBindingName)

	if request.Body == nil {
		return gen.UpdateResourceReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	rbCR, err := convert[gen.ResourceReleaseBinding, openchoreov1alpha1.ResourceReleaseBinding](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateResourceReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	rbCR.Name = request.ResourceReleaseBindingName

	updated, err := h.services.ResourceReleaseBindingService.UpdateResourceReleaseBinding(ctx, request.NamespaceName, &rbCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateResourceReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, resourcereleasebindingsvc.ErrResourceReleaseBindingNotFound) {
			return gen.UpdateResourceReleaseBinding404JSONResponse{NotFoundJSONResponse: notFound("ResourceReleaseBinding")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.UpdateResourceReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to update resource release binding", "error", err)
		return gen.UpdateResourceReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRB, err := convert[openchoreov1alpha1.ResourceReleaseBinding, gen.ResourceReleaseBinding](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated resource release binding", "error", err)
		return gen.UpdateResourceReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ResourceReleaseBinding updated successfully", "namespaceName", request.NamespaceName, "resourceReleaseBinding", updated.Name)
	return gen.UpdateResourceReleaseBinding200JSONResponse(genRB), nil
}

// DeleteResourceReleaseBinding deletes a resource release binding by name.
func (h *Handler) DeleteResourceReleaseBinding(
	ctx context.Context,
	request gen.DeleteResourceReleaseBindingRequestObject,
) (gen.DeleteResourceReleaseBindingResponseObject, error) {
	h.logger.Info("DeleteResourceReleaseBinding called", "namespaceName", request.NamespaceName, "resourceReleaseBindingName", request.ResourceReleaseBindingName)

	err := h.services.ResourceReleaseBindingService.DeleteResourceReleaseBinding(ctx, request.NamespaceName, request.ResourceReleaseBindingName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteResourceReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, resourcereleasebindingsvc.ErrResourceReleaseBindingNotFound) {
			return gen.DeleteResourceReleaseBinding404JSONResponse{NotFoundJSONResponse: notFound("ResourceReleaseBinding")}, nil
		}
		h.logger.Error("Failed to delete resource release binding", "error", err)
		return gen.DeleteResourceReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ResourceReleaseBinding deleted successfully", "namespaceName", request.NamespaceName, "resourceReleaseBinding", request.ResourceReleaseBindingName)
	return gen.DeleteResourceReleaseBinding204Response{}, nil
}
