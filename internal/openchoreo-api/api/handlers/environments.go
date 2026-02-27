// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"strings"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	environmentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/environment"
)

// ListEnvironments returns a paginated list of environments within a namespace.
func (h *Handler) ListEnvironments(
	ctx context.Context,
	request gen.ListEnvironmentsRequestObject,
) (gen.ListEnvironmentsResponseObject, error) {
	h.logger.Debug("ListEnvironments called", "namespaceName", request.NamespaceName)

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.services.EnvironmentService.ListEnvironments(ctx, request.NamespaceName, opts)
	if err != nil {
		h.logger.Error("Failed to list environments", "error", err)
		return gen.ListEnvironments500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.Environment, gen.Environment](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert environments", "error", err)
		return gen.ListEnvironments500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListEnvironments200JSONResponse{
		Items:      items,
		Pagination: ToPagination(result),
	}, nil
}

// CreateEnvironment creates a new environment within a namespace.
func (h *Handler) CreateEnvironment(
	ctx context.Context,
	request gen.CreateEnvironmentRequestObject,
) (gen.CreateEnvironmentResponseObject, error) {
	h.logger.Info("CreateEnvironment called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateEnvironment400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	if strings.TrimSpace(request.Body.Metadata.Name) == "" {
		return gen.CreateEnvironment400JSONResponse{BadRequestJSONResponse: badRequest("metadata.name is required")}, nil
	}

	envCR, err := convert[gen.Environment, openchoreov1alpha1.Environment](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateEnvironment400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	envCR.Status = openchoreov1alpha1.EnvironmentStatus{}

	created, err := h.services.EnvironmentService.CreateEnvironment(ctx, request.NamespaceName, &envCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateEnvironment403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, environmentsvc.ErrEnvironmentAlreadyExists) {
			return gen.CreateEnvironment409JSONResponse{ConflictJSONResponse: conflict("Environment already exists")}, nil
		}
		if errors.Is(err, environmentsvc.ErrDataPlaneNotFound) {
			return gen.CreateEnvironment400JSONResponse{BadRequestJSONResponse: badRequest("DataPlane not found")}, nil
		}
		h.logger.Error("Failed to create environment", "error", err)
		return gen.CreateEnvironment500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genEnv, err := convert[openchoreov1alpha1.Environment, gen.Environment](*created)
	if err != nil {
		h.logger.Error("Failed to convert created environment", "error", err)
		return gen.CreateEnvironment500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Environment created successfully", "namespaceName", request.NamespaceName, "environment", created.Name)
	return gen.CreateEnvironment201JSONResponse(genEnv), nil
}

// GetEnvironment returns details of a specific environment.
func (h *Handler) GetEnvironment(
	ctx context.Context,
	request gen.GetEnvironmentRequestObject,
) (gen.GetEnvironmentResponseObject, error) {
	h.logger.Debug("GetEnvironment called", "namespaceName", request.NamespaceName, "envName", request.EnvName)

	environment, err := h.services.EnvironmentService.GetEnvironment(ctx, request.NamespaceName, request.EnvName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetEnvironment403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, environmentsvc.ErrEnvironmentNotFound) {
			return gen.GetEnvironment404JSONResponse{NotFoundJSONResponse: notFound("Environment")}, nil
		}
		h.logger.Error("Failed to get environment", "error", err, "namespace", request.NamespaceName, "env", request.EnvName)
		return gen.GetEnvironment500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genEnv, err := convert[openchoreov1alpha1.Environment, gen.Environment](*environment)
	if err != nil {
		h.logger.Error("Failed to convert environment", "error", err)
		return gen.GetEnvironment500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetEnvironment200JSONResponse(genEnv), nil
}

// UpdateEnvironment replaces an existing environment (full update).
func (h *Handler) UpdateEnvironment(
	ctx context.Context,
	request gen.UpdateEnvironmentRequestObject,
) (gen.UpdateEnvironmentResponseObject, error) {
	h.logger.Info("UpdateEnvironment called", "namespaceName", request.NamespaceName, "envName", request.EnvName)

	if request.Body == nil {
		return gen.UpdateEnvironment400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	envCR, err := convert[gen.Environment, openchoreov1alpha1.Environment](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateEnvironment400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	envCR.Status = openchoreov1alpha1.EnvironmentStatus{}

	// Ensure the name from the URL path is used
	envCR.Name = request.EnvName

	updated, err := h.services.EnvironmentService.UpdateEnvironment(ctx, request.NamespaceName, &envCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateEnvironment403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, environmentsvc.ErrEnvironmentNotFound) {
			return gen.UpdateEnvironment404JSONResponse{NotFoundJSONResponse: notFound("Environment")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.UpdateEnvironment400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to update environment", "error", err)
		return gen.UpdateEnvironment500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genEnv, err := convert[openchoreov1alpha1.Environment, gen.Environment](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated environment", "error", err)
		return gen.UpdateEnvironment500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Environment updated successfully", "namespaceName", request.NamespaceName, "environment", updated.Name)
	return gen.UpdateEnvironment200JSONResponse(genEnv), nil
}

// DeleteEnvironment deletes an environment by name.
func (h *Handler) DeleteEnvironment(
	ctx context.Context,
	request gen.DeleteEnvironmentRequestObject,
) (gen.DeleteEnvironmentResponseObject, error) {
	h.logger.Info("DeleteEnvironment called", "namespaceName", request.NamespaceName, "envName", request.EnvName)

	err := h.services.EnvironmentService.DeleteEnvironment(ctx, request.NamespaceName, request.EnvName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteEnvironment403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, environmentsvc.ErrEnvironmentNotFound) {
			return gen.DeleteEnvironment404JSONResponse{NotFoundJSONResponse: notFound("Environment")}, nil
		}
		h.logger.Error("Failed to delete environment", "error", err)
		return gen.DeleteEnvironment500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Environment deleted successfully", "namespaceName", request.NamespaceName, "environment", request.EnvName)
	return gen.DeleteEnvironment204Response{}, nil
}

// GetEnvironmentObserverURL returns the observer URL for an environment.
func (h *Handler) GetEnvironmentObserverURL(
	ctx context.Context,
	request gen.GetEnvironmentObserverURLRequestObject,
) (gen.GetEnvironmentObserverURLResponseObject, error) {
	h.logger.Debug("GetEnvironmentObserverURL called", "namespaceName", request.NamespaceName, "envName", request.EnvName)

	result, err := h.services.EnvironmentService.GetObserverURL(ctx, request.NamespaceName, request.EnvName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetEnvironmentObserverURL403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, environmentsvc.ErrEnvironmentNotFound) {
			return gen.GetEnvironmentObserverURL404JSONResponse{NotFoundJSONResponse: notFound("Environment")}, nil
		}
		if errors.Is(err, environmentsvc.ErrDataPlaneNotFound) {
			return gen.GetEnvironmentObserverURL404JSONResponse{NotFoundJSONResponse: notFound("DataPlane")}, nil
		}
		h.logger.Error("Failed to get environment observer URL", "error", err, "namespace", request.NamespaceName, "env", request.EnvName)
		return gen.GetEnvironmentObserverURL500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	response := gen.ObserverURLResponse{
		ObserverUrl: &result.ObserverURL,
	}
	if result.Message != "" {
		response.Message = &result.Message
	}

	return gen.GetEnvironmentObserverURL200JSONResponse(response), nil
}

// GetRCAAgentURL returns the RCA agent URL for an environment.
func (h *Handler) GetRCAAgentURL(
	ctx context.Context,
	request gen.GetRCAAgentURLRequestObject,
) (gen.GetRCAAgentURLResponseObject, error) {
	h.logger.Debug("GetRCAAgentURL called", "namespaceName", request.NamespaceName, "envName", request.EnvName)

	result, err := h.services.EnvironmentService.GetRCAAgentURL(ctx, request.NamespaceName, request.EnvName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetRCAAgentURL403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, environmentsvc.ErrEnvironmentNotFound) {
			return gen.GetRCAAgentURL404JSONResponse{NotFoundJSONResponse: notFound("Environment")}, nil
		}
		if errors.Is(err, environmentsvc.ErrDataPlaneNotFound) {
			return gen.GetRCAAgentURL404JSONResponse{NotFoundJSONResponse: notFound("DataPlane")}, nil
		}
		h.logger.Error("Failed to get RCA agent URL", "error", err, "namespace", request.NamespaceName, "env", request.EnvName)
		return gen.GetRCAAgentURL500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	response := gen.RCAAgentURLResponse{
		RcaAgentUrl: &result.RCAAgentURL,
	}
	if result.Message != "" {
		response.Message = &result.Message
	}

	return gen.GetRCAAgentURL200JSONResponse(response), nil
}
