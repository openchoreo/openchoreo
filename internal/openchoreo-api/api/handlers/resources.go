// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	projectsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project"
	resourcesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resource"
)

// ListResources returns a paginated list of resources within a namespace.
func (h *Handler) ListResources(
	ctx context.Context,
	request gen.ListResourcesRequestObject,
) (gen.ListResourcesResponseObject, error) {
	h.logger.Debug("ListResources called", "namespaceName", request.NamespaceName)

	projectName := ""
	if request.Params.Project != nil {
		projectName = *request.Params.Project
	}

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.ResourceService.ListResources(ctx, request.NamespaceName, projectName, opts)
	if err != nil {
		if errors.Is(err, projectsvc.ErrProjectNotFound) {
			return gen.ListResources404JSONResponse{NotFoundJSONResponse: notFound("Project")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.ListResources400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to list resources", "error", err)
		return gen.ListResources500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.Resource, gen.ResourceInstance](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert resources", "error", err)
		return gen.ListResources500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListResources200JSONResponse{
		Items:      items,
		Pagination: ToPagination(result),
	}, nil
}

// CreateResource creates a new resource within a namespace.
func (h *Handler) CreateResource(
	ctx context.Context,
	request gen.CreateResourceRequestObject,
) (gen.CreateResourceResponseObject, error) {
	h.logger.Info("CreateResource called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateResource400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	rCR, err := convert[gen.ResourceInstance, openchoreov1alpha1.Resource](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateResource400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	created, err := h.services.ResourceService.CreateResource(ctx, request.NamespaceName, &rCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateResource403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, projectsvc.ErrProjectNotFound) {
			return gen.CreateResource400JSONResponse{BadRequestJSONResponse: badRequest("Referenced project not found")}, nil
		}
		if errors.Is(err, resourcesvc.ErrResourceAlreadyExists) {
			return gen.CreateResource409JSONResponse{ConflictJSONResponse: conflict("Resource already exists")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.CreateResource400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to create resource", "error", err)
		return gen.CreateResource500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genR, err := convert[openchoreov1alpha1.Resource, gen.ResourceInstance](*created)
	if err != nil {
		h.logger.Error("Failed to convert created resource", "error", err)
		return gen.CreateResource500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Resource created successfully", "namespaceName", request.NamespaceName, "resource", created.Name)
	return gen.CreateResource201JSONResponse(genR), nil
}

// GetResource returns details of a specific resource.
func (h *Handler) GetResource(
	ctx context.Context,
	request gen.GetResourceRequestObject,
) (gen.GetResourceResponseObject, error) {
	h.logger.Debug("GetResource called", "namespaceName", request.NamespaceName, "resourceName", request.ResourceName)

	r, err := h.services.ResourceService.GetResource(ctx, request.NamespaceName, request.ResourceName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetResource403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, resourcesvc.ErrResourceNotFound) {
			return gen.GetResource404JSONResponse{NotFoundJSONResponse: notFound("Resource")}, nil
		}
		h.logger.Error("Failed to get resource", "error", err)
		return gen.GetResource500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genR, err := convert[openchoreov1alpha1.Resource, gen.ResourceInstance](*r)
	if err != nil {
		h.logger.Error("Failed to convert resource", "error", err)
		return gen.GetResource500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetResource200JSONResponse(genR), nil
}

// UpdateResource replaces an existing resource (full update).
func (h *Handler) UpdateResource(
	ctx context.Context,
	request gen.UpdateResourceRequestObject,
) (gen.UpdateResourceResponseObject, error) {
	h.logger.Info("UpdateResource called", "namespaceName", request.NamespaceName, "resourceName", request.ResourceName)

	if request.Body == nil {
		return gen.UpdateResource400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	rCR, err := convert[gen.ResourceInstance, openchoreov1alpha1.Resource](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateResource400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	rCR.Name = request.ResourceName

	updated, err := h.services.ResourceService.UpdateResource(ctx, request.NamespaceName, &rCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateResource403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, resourcesvc.ErrResourceNotFound) {
			return gen.UpdateResource404JSONResponse{NotFoundJSONResponse: notFound("Resource")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.UpdateResource400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to update resource", "error", err)
		return gen.UpdateResource500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genR, err := convert[openchoreov1alpha1.Resource, gen.ResourceInstance](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated resource", "error", err)
		return gen.UpdateResource500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Resource updated successfully", "namespaceName", request.NamespaceName, "resource", updated.Name)
	return gen.UpdateResource200JSONResponse(genR), nil
}

// DeleteResource deletes a resource by name.
func (h *Handler) DeleteResource(
	ctx context.Context,
	request gen.DeleteResourceRequestObject,
) (gen.DeleteResourceResponseObject, error) {
	h.logger.Info("DeleteResource called", "namespaceName", request.NamespaceName, "resourceName", request.ResourceName)

	err := h.services.ResourceService.DeleteResource(ctx, request.NamespaceName, request.ResourceName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteResource403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, resourcesvc.ErrResourceNotFound) {
			return gen.DeleteResource404JSONResponse{NotFoundJSONResponse: notFound("Resource")}, nil
		}
		h.logger.Error("Failed to delete resource", "error", err)
		return gen.DeleteResource500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Resource deleted successfully", "namespaceName", request.NamespaceName, "resource", request.ResourceName)
	return gen.DeleteResource204Response{}, nil
}
