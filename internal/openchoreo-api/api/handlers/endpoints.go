// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	endpointsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpoint"
	projectsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// ListEndpoints returns a paginated list of endpoints within a namespace.
func (h *Handler) ListEndpoints(
	ctx context.Context,
	request gen.ListEndpointsRequestObject,
) (gen.ListEndpointsResponseObject, error) {
	h.logger.Debug("ListEndpoints called", "namespaceName", request.NamespaceName)

	projectName := ""
	if request.Params.Project != nil {
		projectName = *request.Params.Project
	}

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.EndpointService.ListEndpoints(ctx, request.NamespaceName, projectName, opts)
	if err != nil {
		if errors.Is(err, projectsvc.ErrProjectNotFound) {
			return gen.ListEndpoints404JSONResponse{NotFoundJSONResponse: notFound("Project")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.ListEndpoints400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to list endpoints", "error", err)
		return gen.ListEndpoints500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.Endpoint, gen.Endpoint](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert endpoints", "error", err)
		return gen.ListEndpoints500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListEndpoints200JSONResponse{
		Items:      items,
		Pagination: ToPagination(result),
	}, nil
}

// CreateEndpoint creates a new endpoint within a namespace.
func (h *Handler) CreateEndpoint(
	ctx context.Context,
	request gen.CreateEndpointRequestObject,
) (gen.CreateEndpointResponseObject, error) {
	h.logger.Info("CreateEndpoint called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateEndpoint400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	rCR, err := convert[gen.Endpoint, openchoreov1alpha1.Endpoint](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateEndpoint400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	created, err := h.services.EndpointService.CreateEndpoint(ctx, request.NamespaceName, &rCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateEndpoint403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, projectsvc.ErrProjectNotFound) {
			return gen.CreateEndpoint400JSONResponse{BadRequestJSONResponse: badRequest("Referenced project not found")}, nil
		}
		if errors.Is(err, endpointsvc.ErrEndpointAlreadyExists) {
			return gen.CreateEndpoint409JSONResponse{ConflictJSONResponse: conflict("Endpoint already exists")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.CreateEndpoint400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to create endpoint", "error", err)
		return gen.CreateEndpoint500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	audit.SetResource(ctx, &audit.Resource{Namespace: request.NamespaceName, UID: string(created.UID), Name: created.Name})

	genR, err := convert[openchoreov1alpha1.Endpoint, gen.Endpoint](*created)
	if err != nil {
		h.logger.Error("Failed to convert created endpoint", "error", err)
		return gen.CreateEndpoint500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Endpoint created successfully", "namespaceName", request.NamespaceName, "endpoint", created.Name)
	return gen.CreateEndpoint201JSONResponse(genR), nil
}

// GetEndpoint returns details of a specific endpoint.
func (h *Handler) GetEndpoint(
	ctx context.Context,
	request gen.GetEndpointRequestObject,
) (gen.GetEndpointResponseObject, error) {
	h.logger.Debug("GetEndpoint called", "namespaceName", request.NamespaceName, "endpointName", request.EndpointName)

	r, err := h.services.EndpointService.GetEndpoint(ctx, request.NamespaceName, request.EndpointName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetEndpoint403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, endpointsvc.ErrEndpointNotFound) {
			return gen.GetEndpoint404JSONResponse{NotFoundJSONResponse: notFound("Endpoint")}, nil
		}
		h.logger.Error("Failed to get endpoint", "error", err)
		return gen.GetEndpoint500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genR, err := convert[openchoreov1alpha1.Endpoint, gen.Endpoint](*r)
	if err != nil {
		h.logger.Error("Failed to convert endpoint", "error", err)
		return gen.GetEndpoint500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetEndpoint200JSONResponse(genR), nil
}

// UpdateEndpoint replaces an existing endpoint (full update).
func (h *Handler) UpdateEndpoint(
	ctx context.Context,
	request gen.UpdateEndpointRequestObject,
) (gen.UpdateEndpointResponseObject, error) {
	h.logger.Info("UpdateEndpoint called", "namespaceName", request.NamespaceName, "endpointName", request.EndpointName)

	if request.Body == nil {
		return gen.UpdateEndpoint400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	rCR, err := convert[gen.Endpoint, openchoreov1alpha1.Endpoint](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateEndpoint400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	rCR.Name = request.EndpointName

	updated, err := h.services.EndpointService.UpdateEndpoint(ctx, request.NamespaceName, &rCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateEndpoint403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, endpointsvc.ErrEndpointNotFound) {
			return gen.UpdateEndpoint404JSONResponse{NotFoundJSONResponse: notFound("Endpoint")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.UpdateEndpoint400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to update endpoint", "error", err)
		return gen.UpdateEndpoint500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	audit.SetResource(ctx, &audit.Resource{Namespace: request.NamespaceName, UID: string(updated.UID), Name: updated.Name})

	genR, err := convert[openchoreov1alpha1.Endpoint, gen.Endpoint](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated endpoint", "error", err)
		return gen.UpdateEndpoint500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Endpoint updated successfully", "namespaceName", request.NamespaceName, "endpoint", updated.Name)
	return gen.UpdateEndpoint200JSONResponse(genR), nil
}

// DeleteEndpoint deletes a endpoint by name.
func (h *Handler) DeleteEndpoint(
	ctx context.Context,
	request gen.DeleteEndpointRequestObject,
) (gen.DeleteEndpointResponseObject, error) {
	h.logger.Info("DeleteEndpoint called", "namespaceName", request.NamespaceName, "endpointName", request.EndpointName)

	err := h.services.EndpointService.DeleteEndpoint(ctx, request.NamespaceName, request.EndpointName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteEndpoint403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, endpointsvc.ErrEndpointNotFound) {
			return gen.DeleteEndpoint404JSONResponse{NotFoundJSONResponse: notFound("Endpoint")}, nil
		}
		h.logger.Error("Failed to delete endpoint", "error", err)
		return gen.DeleteEndpoint500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Endpoint deleted successfully", "namespaceName", request.NamespaceName, "endpoint", request.EndpointName)
	return gen.DeleteEndpoint204Response{}, nil
}
