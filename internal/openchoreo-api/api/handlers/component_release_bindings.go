// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"net/http"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	componentcomponentreleasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componentreleasebinding"
)

// ListComponentReleaseBindings returns a paginated list of release bindings within a namespace.
func (h *Handler) ListComponentReleaseBindings(
	ctx context.Context,
	request gen.ListComponentReleaseBindingsRequestObject,
) (gen.ListComponentReleaseBindingsResponseObject, error) {
	h.logger.Debug("ListComponentReleaseBindings called", "namespaceName", request.NamespaceName)

	componentName := ""
	if request.Params.Component != nil {
		componentName = *request.Params.Component
	}

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.ComponentReleaseBindingService.ListComponentComponentReleaseBindings(ctx, request.NamespaceName, componentName, opts)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListComponentReleaseBindings403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, componentcomponentreleasebindingsvc.ErrComponentNotFound) {
			return gen.ListComponentReleaseBindings404JSONResponse{NotFoundJSONResponse: notFound("Component")}, nil
		}
		if validationErr, ok := errors.AsType[*services.ValidationError](err); ok {
			return gen.ListComponentReleaseBindings400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to list release bindings", "error", err)
		return gen.ListComponentReleaseBindings500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ComponentReleaseBinding, gen.ComponentReleaseBinding](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert release bindings", "error", err)
		return gen.ListComponentReleaseBindings500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	resp := gen.ListComponentReleaseBindings200JSONResponse{
		Items: items,
	}
	resp.Pagination = ToPagination(result)

	return resp, nil
}

// CreateComponentReleaseBinding creates a new release binding within a namespace.
func (h *Handler) CreateComponentReleaseBinding(
	ctx context.Context,
	request gen.CreateComponentReleaseBindingRequestObject,
) (gen.CreateComponentReleaseBindingResponseObject, error) {
	h.logger.Info("CreateComponentReleaseBinding called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateComponentReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	rbCR, err := convert[gen.ComponentReleaseBinding, openchoreov1alpha1.ComponentReleaseBinding](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateComponentReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	if rbCR.Namespace != "" && rbCR.Namespace != request.NamespaceName {
		return gen.CreateComponentReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Namespace in body does not match path")}, nil
	}
	rbCR.Namespace = request.NamespaceName

	created, err := h.services.ComponentReleaseBindingService.CreateComponentReleaseBinding(ctx, request.NamespaceName, &rbCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateComponentReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, componentcomponentreleasebindingsvc.ErrComponentReleaseBindingAlreadyExists) {
			return gen.CreateComponentReleaseBinding409JSONResponse{ConflictJSONResponse: conflict("ComponentReleaseBinding already exists")}, nil
		}
		if errors.Is(err, componentcomponentreleasebindingsvc.ErrComponentNotFound) {
			return gen.CreateComponentReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Referenced component not found")}, nil
		}
		if validationErr, ok := errors.AsType[*services.ValidationError](err); ok {
			if validationErr.StatusCode == http.StatusUnprocessableEntity {
				return gen.CreateComponentReleaseBinding422JSONResponse{UnprocessableContentJSONResponse: unprocessableContent(validationErr.Msg)}, nil
			}
			return gen.CreateComponentReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to create release binding", "error", err)
		return gen.CreateComponentReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRB, err := convert[openchoreov1alpha1.ComponentReleaseBinding, gen.ComponentReleaseBinding](*created)
	if err != nil {
		h.logger.Error("Failed to convert created release binding", "error", err)
		return gen.CreateComponentReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ComponentReleaseBinding created successfully", "namespaceName", request.NamespaceName, "componentReleaseBinding", created.Name)
	return gen.CreateComponentReleaseBinding201JSONResponse(genRB), nil
}

// GetComponentReleaseBinding returns details of a specific release binding.
func (h *Handler) GetComponentReleaseBinding(
	ctx context.Context,
	request gen.GetComponentReleaseBindingRequestObject,
) (gen.GetComponentReleaseBindingResponseObject, error) {
	h.logger.Debug("GetComponentReleaseBinding called", "namespaceName", request.NamespaceName, "componentReleaseBindingName", request.ComponentReleaseBindingName)

	rb, err := h.services.ComponentReleaseBindingService.GetComponentReleaseBinding(ctx, request.NamespaceName, request.ComponentReleaseBindingName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetComponentReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, componentcomponentreleasebindingsvc.ErrComponentReleaseBindingNotFound) {
			return gen.GetComponentReleaseBinding404JSONResponse{NotFoundJSONResponse: notFound("ComponentReleaseBinding")}, nil
		}
		h.logger.Error("Failed to get release binding", "error", err)
		return gen.GetComponentReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRB, err := convert[openchoreov1alpha1.ComponentReleaseBinding, gen.ComponentReleaseBinding](*rb)
	if err != nil {
		h.logger.Error("Failed to convert release binding", "error", err)
		return gen.GetComponentReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetComponentReleaseBinding200JSONResponse(genRB), nil
}

// UpdateComponentReleaseBinding replaces an existing release binding (full update).
func (h *Handler) UpdateComponentReleaseBinding(
	ctx context.Context,
	request gen.UpdateComponentReleaseBindingRequestObject,
) (gen.UpdateComponentReleaseBindingResponseObject, error) {
	h.logger.Info("UpdateComponentReleaseBinding called", "namespaceName", request.NamespaceName, "componentReleaseBindingName", request.ComponentReleaseBindingName)

	if request.Body == nil {
		return gen.UpdateComponentReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	rbCR, err := convert[gen.ComponentReleaseBinding, openchoreov1alpha1.ComponentReleaseBinding](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateComponentReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	if rbCR.Namespace != "" && rbCR.Namespace != request.NamespaceName {
		return gen.UpdateComponentReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Namespace in body does not match path")}, nil
	}
	rbCR.Namespace = request.NamespaceName

	// Ensure the name from the URL path is used
	rbCR.Name = request.ComponentReleaseBindingName

	updated, err := h.services.ComponentReleaseBindingService.UpdateComponentReleaseBinding(ctx, request.NamespaceName, &rbCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateComponentReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, componentcomponentreleasebindingsvc.ErrComponentReleaseBindingNotFound) {
			return gen.UpdateComponentReleaseBinding404JSONResponse{NotFoundJSONResponse: notFound("ComponentReleaseBinding")}, nil
		}
		if validationErr, ok := errors.AsType[*services.ValidationError](err); ok {
			if validationErr.StatusCode == http.StatusUnprocessableEntity {
				return gen.UpdateComponentReleaseBinding422JSONResponse{UnprocessableContentJSONResponse: unprocessableContent(validationErr.Msg)}, nil
			}
			return gen.UpdateComponentReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to update release binding", "error", err)
		return gen.UpdateComponentReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRB, err := convert[openchoreov1alpha1.ComponentReleaseBinding, gen.ComponentReleaseBinding](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated release binding", "error", err)
		return gen.UpdateComponentReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ComponentReleaseBinding updated successfully", "namespaceName", request.NamespaceName, "componentReleaseBinding", updated.Name)
	return gen.UpdateComponentReleaseBinding200JSONResponse(genRB), nil
}

// DeleteComponentReleaseBinding deletes a release binding by name.
func (h *Handler) DeleteComponentReleaseBinding(
	ctx context.Context,
	request gen.DeleteComponentReleaseBindingRequestObject,
) (gen.DeleteComponentReleaseBindingResponseObject, error) {
	h.logger.Info("DeleteComponentReleaseBinding called", "namespaceName", request.NamespaceName, "componentReleaseBindingName", request.ComponentReleaseBindingName)

	err := h.services.ComponentReleaseBindingService.DeleteComponentReleaseBinding(ctx, request.NamespaceName, request.ComponentReleaseBindingName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteComponentReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, componentcomponentreleasebindingsvc.ErrComponentReleaseBindingNotFound) {
			return gen.DeleteComponentReleaseBinding404JSONResponse{NotFoundJSONResponse: notFound("ComponentReleaseBinding")}, nil
		}
		h.logger.Error("Failed to delete release binding", "error", err)
		return gen.DeleteComponentReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ComponentReleaseBinding deleted successfully", "namespaceName", request.NamespaceName, "componentReleaseBinding", request.ComponentReleaseBindingName)
	return gen.DeleteComponentReleaseBinding204Response{}, nil
}
