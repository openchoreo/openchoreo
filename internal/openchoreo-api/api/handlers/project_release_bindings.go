// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	projectreleasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/projectreleasebinding"
)

// ListProjectReleaseBindings returns a paginated list of project release bindings within a namespace.
func (h *Handler) ListProjectReleaseBindings(
	ctx context.Context,
	request gen.ListProjectReleaseBindingsRequestObject,
) (gen.ListProjectReleaseBindingsResponseObject, error) {
	h.logger.Debug("ListProjectReleaseBindings called", "namespaceName", request.NamespaceName)

	projectName := ""
	if request.Params.Project != nil {
		projectName = *request.Params.Project
	}

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.ProjectReleaseBindingService.ListProjectReleaseBindings(ctx, request.NamespaceName, projectName, opts)
	if err != nil {
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.ListProjectReleaseBindings400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to list project release bindings", "error", err)
		return gen.ListProjectReleaseBindings500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ProjectReleaseBinding, gen.ProjectReleaseBinding](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert project release bindings", "error", err)
		return gen.ListProjectReleaseBindings500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListProjectReleaseBindings200JSONResponse{
		Items:      items,
		Pagination: ToPagination(result),
	}, nil
}

// CreateProjectReleaseBinding creates a new project release binding within a namespace.
func (h *Handler) CreateProjectReleaseBinding(
	ctx context.Context,
	request gen.CreateProjectReleaseBindingRequestObject,
) (gen.CreateProjectReleaseBindingResponseObject, error) {
	h.logger.Info("CreateProjectReleaseBinding called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateProjectReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	rbCR, err := convert[gen.ProjectReleaseBinding, openchoreov1alpha1.ProjectReleaseBinding](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateProjectReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	created, err := h.services.ProjectReleaseBindingService.CreateProjectReleaseBinding(ctx, request.NamespaceName, &rbCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateProjectReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, projectreleasebindingsvc.ErrProjectNotFound) {
			return gen.CreateProjectReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Referenced project not found")}, nil
		}
		if errors.Is(err, projectreleasebindingsvc.ErrProjectReleaseBindingAlreadyExists) {
			return gen.CreateProjectReleaseBinding409JSONResponse{ConflictJSONResponse: conflict("ProjectReleaseBinding already exists")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.CreateProjectReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to create project release binding", "error", err)
		return gen.CreateProjectReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRB, err := convert[openchoreov1alpha1.ProjectReleaseBinding, gen.ProjectReleaseBinding](*created)
	if err != nil {
		h.logger.Error("Failed to convert created project release binding", "error", err)
		return gen.CreateProjectReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ProjectReleaseBinding created successfully", "namespaceName", request.NamespaceName, "projectReleaseBinding", created.Name)
	return gen.CreateProjectReleaseBinding201JSONResponse(genRB), nil
}

// GetProjectReleaseBinding returns details of a specific project release binding.
func (h *Handler) GetProjectReleaseBinding(
	ctx context.Context,
	request gen.GetProjectReleaseBindingRequestObject,
) (gen.GetProjectReleaseBindingResponseObject, error) {
	h.logger.Debug("GetProjectReleaseBinding called", "namespaceName", request.NamespaceName, "projectReleaseBindingName", request.ProjectReleaseBindingName)

	rb, err := h.services.ProjectReleaseBindingService.GetProjectReleaseBinding(ctx, request.NamespaceName, request.ProjectReleaseBindingName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetProjectReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, projectreleasebindingsvc.ErrProjectReleaseBindingNotFound) {
			return gen.GetProjectReleaseBinding404JSONResponse{NotFoundJSONResponse: notFound("ProjectReleaseBinding")}, nil
		}
		h.logger.Error("Failed to get project release binding", "error", err)
		return gen.GetProjectReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRB, err := convert[openchoreov1alpha1.ProjectReleaseBinding, gen.ProjectReleaseBinding](*rb)
	if err != nil {
		h.logger.Error("Failed to convert project release binding", "error", err)
		return gen.GetProjectReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetProjectReleaseBinding200JSONResponse(genRB), nil
}

// UpdateProjectReleaseBinding replaces an existing project release binding (full update).
func (h *Handler) UpdateProjectReleaseBinding(
	ctx context.Context,
	request gen.UpdateProjectReleaseBindingRequestObject,
) (gen.UpdateProjectReleaseBindingResponseObject, error) {
	h.logger.Info("UpdateProjectReleaseBinding called", "namespaceName", request.NamespaceName, "projectReleaseBindingName", request.ProjectReleaseBindingName)

	if request.Body == nil {
		return gen.UpdateProjectReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	rbCR, err := convert[gen.ProjectReleaseBinding, openchoreov1alpha1.ProjectReleaseBinding](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateProjectReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	rbCR.Name = request.ProjectReleaseBindingName

	updated, err := h.services.ProjectReleaseBindingService.UpdateProjectReleaseBinding(ctx, request.NamespaceName, &rbCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateProjectReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, projectreleasebindingsvc.ErrProjectReleaseBindingNotFound) {
			return gen.UpdateProjectReleaseBinding404JSONResponse{NotFoundJSONResponse: notFound("ProjectReleaseBinding")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.UpdateProjectReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to update project release binding", "error", err)
		return gen.UpdateProjectReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRB, err := convert[openchoreov1alpha1.ProjectReleaseBinding, gen.ProjectReleaseBinding](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated project release binding", "error", err)
		return gen.UpdateProjectReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ProjectReleaseBinding updated successfully", "namespaceName", request.NamespaceName, "projectReleaseBinding", updated.Name)
	return gen.UpdateProjectReleaseBinding200JSONResponse(genRB), nil
}

// DeleteProjectReleaseBinding deletes a project release binding by name.
func (h *Handler) DeleteProjectReleaseBinding(
	ctx context.Context,
	request gen.DeleteProjectReleaseBindingRequestObject,
) (gen.DeleteProjectReleaseBindingResponseObject, error) {
	h.logger.Info("DeleteProjectReleaseBinding called", "namespaceName", request.NamespaceName, "projectReleaseBindingName", request.ProjectReleaseBindingName)

	err := h.services.ProjectReleaseBindingService.DeleteProjectReleaseBinding(ctx, request.NamespaceName, request.ProjectReleaseBindingName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteProjectReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, projectreleasebindingsvc.ErrProjectReleaseBindingNotFound) {
			return gen.DeleteProjectReleaseBinding404JSONResponse{NotFoundJSONResponse: notFound("ProjectReleaseBinding")}, nil
		}
		h.logger.Error("Failed to delete project release binding", "error", err)
		return gen.DeleteProjectReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ProjectReleaseBinding deleted successfully", "namespaceName", request.NamespaceName, "projectReleaseBinding", request.ProjectReleaseBindingName)
	return gen.DeleteProjectReleaseBinding204Response{}, nil
}
