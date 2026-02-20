// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	releasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding"
)

// ListReleaseBindingsFlat returns a paginated list of release bindings within a namespace.
func (h *Handler) ListReleaseBindingsFlat(
	ctx context.Context,
	request gen.ListReleaseBindingsFlatRequestObject,
) (gen.ListReleaseBindingsFlatResponseObject, error) {
	h.logger.Debug("ListReleaseBindingsFlat called", "namespaceName", request.NamespaceName)

	componentName := ""
	if request.Params.Component != nil {
		componentName = *request.Params.Component
	}

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.releaseBindingService.ListReleaseBindings(ctx, request.NamespaceName, componentName, opts)
	if err != nil {
		h.logger.Error("Failed to list release bindings", "error", err)
		return gen.ListReleaseBindingsFlat500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ReleaseBinding, gen.ReleaseBinding](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert release bindings", "error", err)
		return gen.ListReleaseBindingsFlat500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	resp := gen.ListReleaseBindingsFlat200JSONResponse{
		Items: items,
	}
	if p := ToPaginationPtr(result); p != nil {
		resp.Pagination = *p
	}

	return resp, nil
}

// CreateReleaseBinding creates a new release binding within a namespace.
func (h *Handler) CreateReleaseBinding(
	ctx context.Context,
	request gen.CreateReleaseBindingRequestObject,
) (gen.CreateReleaseBindingResponseObject, error) {
	h.logger.Info("CreateReleaseBinding called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	rbCR, err := convert[gen.ReleaseBinding, openchoreov1alpha1.ReleaseBinding](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	rbCR.Status = openchoreov1alpha1.ReleaseBindingStatus{}

	created, err := h.releaseBindingService.CreateReleaseBinding(ctx, request.NamespaceName, &rbCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, releasebindingsvc.ErrReleaseBindingAlreadyExists) {
			return gen.CreateReleaseBinding409JSONResponse{ConflictJSONResponse: conflict("ReleaseBinding already exists")}, nil
		}
		if errors.Is(err, releasebindingsvc.ErrComponentNotFound) {
			return gen.CreateReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Referenced component not found")}, nil
		}
		h.logger.Error("Failed to create release binding", "error", err)
		return gen.CreateReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRB, err := convert[openchoreov1alpha1.ReleaseBinding, gen.ReleaseBinding](*created)
	if err != nil {
		h.logger.Error("Failed to convert created release binding", "error", err)
		return gen.CreateReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ReleaseBinding created successfully", "namespaceName", request.NamespaceName, "releaseBinding", created.Name)
	return gen.CreateReleaseBinding201JSONResponse(genRB), nil
}

// GetReleaseBinding returns details of a specific release binding.
func (h *Handler) GetReleaseBinding(
	ctx context.Context,
	request gen.GetReleaseBindingRequestObject,
) (gen.GetReleaseBindingResponseObject, error) {
	h.logger.Debug("GetReleaseBinding called", "namespaceName", request.NamespaceName, "releaseBindingName", request.ReleaseBindingName)

	rb, err := h.releaseBindingService.GetReleaseBinding(ctx, request.NamespaceName, request.ReleaseBindingName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, releasebindingsvc.ErrReleaseBindingNotFound) {
			return gen.GetReleaseBinding404JSONResponse{NotFoundJSONResponse: notFound("ReleaseBinding")}, nil
		}
		h.logger.Error("Failed to get release binding", "error", err)
		return gen.GetReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRB, err := convert[openchoreov1alpha1.ReleaseBinding, gen.ReleaseBinding](*rb)
	if err != nil {
		h.logger.Error("Failed to convert release binding", "error", err)
		return gen.GetReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetReleaseBinding200JSONResponse(genRB), nil
}

// UpdateReleaseBinding replaces an existing release binding (full update).
func (h *Handler) UpdateReleaseBinding(
	ctx context.Context,
	request gen.UpdateReleaseBindingRequestObject,
) (gen.UpdateReleaseBindingResponseObject, error) {
	h.logger.Info("UpdateReleaseBinding called", "namespaceName", request.NamespaceName, "releaseBindingName", request.ReleaseBindingName)

	if request.Body == nil {
		return gen.UpdateReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	rbCR, err := convert[gen.ReleaseBinding, openchoreov1alpha1.ReleaseBinding](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	rbCR.Status = openchoreov1alpha1.ReleaseBindingStatus{}

	// Ensure the name from the URL path is used
	rbCR.Name = request.ReleaseBindingName

	updated, err := h.releaseBindingService.UpdateReleaseBinding(ctx, request.NamespaceName, &rbCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, releasebindingsvc.ErrReleaseBindingNotFound) {
			return gen.UpdateReleaseBinding404JSONResponse{NotFoundJSONResponse: notFound("ReleaseBinding")}, nil
		}
		h.logger.Error("Failed to update release binding", "error", err)
		return gen.UpdateReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRB, err := convert[openchoreov1alpha1.ReleaseBinding, gen.ReleaseBinding](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated release binding", "error", err)
		return gen.UpdateReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ReleaseBinding updated successfully", "namespaceName", request.NamespaceName, "releaseBinding", updated.Name)
	return gen.UpdateReleaseBinding200JSONResponse(genRB), nil
}

// DeleteReleaseBinding deletes a release binding by name.
func (h *Handler) DeleteReleaseBinding(
	ctx context.Context,
	request gen.DeleteReleaseBindingRequestObject,
) (gen.DeleteReleaseBindingResponseObject, error) {
	h.logger.Info("DeleteReleaseBinding called", "namespaceName", request.NamespaceName, "releaseBindingName", request.ReleaseBindingName)

	err := h.releaseBindingService.DeleteReleaseBinding(ctx, request.NamespaceName, request.ReleaseBindingName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, releasebindingsvc.ErrReleaseBindingNotFound) {
			return gen.DeleteReleaseBinding404JSONResponse{NotFoundJSONResponse: notFound("ReleaseBinding")}, nil
		}
		h.logger.Error("Failed to delete release binding", "error", err)
		return gen.DeleteReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ReleaseBinding deleted successfully", "namespaceName", request.NamespaceName, "releaseBinding", request.ReleaseBindingName)
	return gen.DeleteReleaseBinding204Response{}, nil
}
