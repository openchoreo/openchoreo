// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	projecttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/projecttype"
)

// ListProjectTypes returns a paginated list of project types within a namespace.
func (h *Handler) ListProjectTypes(
	ctx context.Context,
	request gen.ListProjectTypesRequestObject,
) (gen.ListProjectTypesResponseObject, error) {
	h.logger.Debug("ListProjectTypes called", "namespaceName", request.NamespaceName)

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.ProjectTypeService.ListProjectTypes(ctx, request.NamespaceName, opts)
	if err != nil {
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.ListProjectTypes400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to list project types", "error", err)
		return gen.ListProjectTypes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ProjectType, gen.ProjectType](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert project types", "error", err)
		return gen.ListProjectTypes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListProjectTypes200JSONResponse{
		Items:      items,
		Pagination: ToPagination(result),
	}, nil
}

// CreateProjectType creates a new project type within a namespace.
func (h *Handler) CreateProjectType(
	ctx context.Context,
	request gen.CreateProjectTypeRequestObject,
) (gen.CreateProjectTypeResponseObject, error) {
	h.logger.Info("CreateProjectType called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateProjectType400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	ptCR, err := convert[gen.ProjectType, openchoreov1alpha1.ProjectType](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateProjectType400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	created, err := h.services.ProjectTypeService.CreateProjectType(ctx, request.NamespaceName, &ptCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateProjectType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, projecttypesvc.ErrProjectTypeAlreadyExists) {
			return gen.CreateProjectType409JSONResponse{ConflictJSONResponse: conflict("Project type already exists")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.CreateProjectType400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to create project type", "error", err)
		return gen.CreateProjectType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genPT, err := convert[openchoreov1alpha1.ProjectType, gen.ProjectType](*created)
	if err != nil {
		h.logger.Error("Failed to convert created project type", "error", err)
		return gen.CreateProjectType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Project type created successfully", "namespaceName", request.NamespaceName, "projectType", created.Name)
	return gen.CreateProjectType201JSONResponse(genPT), nil
}

// GetProjectType returns details of a specific project type.
func (h *Handler) GetProjectType(
	ctx context.Context,
	request gen.GetProjectTypeRequestObject,
) (gen.GetProjectTypeResponseObject, error) {
	h.logger.Debug("GetProjectType called", "namespaceName", request.NamespaceName, "ptName", request.PtName)

	pt, err := h.services.ProjectTypeService.GetProjectType(ctx, request.NamespaceName, request.PtName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetProjectType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, projecttypesvc.ErrProjectTypeNotFound) {
			return gen.GetProjectType404JSONResponse{NotFoundJSONResponse: notFound("Project type")}, nil
		}
		h.logger.Error("Failed to get project type", "error", err)
		return gen.GetProjectType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genPT, err := convert[openchoreov1alpha1.ProjectType, gen.ProjectType](*pt)
	if err != nil {
		h.logger.Error("Failed to convert project type", "error", err)
		return gen.GetProjectType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetProjectType200JSONResponse(genPT), nil
}

// UpdateProjectType replaces an existing project type (full update).
func (h *Handler) UpdateProjectType(
	ctx context.Context,
	request gen.UpdateProjectTypeRequestObject,
) (gen.UpdateProjectTypeResponseObject, error) {
	h.logger.Info("UpdateProjectType called", "namespaceName", request.NamespaceName, "ptName", request.PtName)

	if request.Body == nil {
		return gen.UpdateProjectType400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	ptCR, err := convert[gen.ProjectType, openchoreov1alpha1.ProjectType](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateProjectType400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	ptCR.Name = request.PtName

	updated, err := h.services.ProjectTypeService.UpdateProjectType(ctx, request.NamespaceName, &ptCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateProjectType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, projecttypesvc.ErrProjectTypeNotFound) {
			return gen.UpdateProjectType404JSONResponse{NotFoundJSONResponse: notFound("Project type")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.UpdateProjectType400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to update project type", "error", err)
		return gen.UpdateProjectType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genPT, err := convert[openchoreov1alpha1.ProjectType, gen.ProjectType](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated project type", "error", err)
		return gen.UpdateProjectType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Project type updated successfully", "namespaceName", request.NamespaceName, "projectType", updated.Name)
	return gen.UpdateProjectType200JSONResponse(genPT), nil
}

// DeleteProjectType deletes a project type by name.
func (h *Handler) DeleteProjectType(
	ctx context.Context,
	request gen.DeleteProjectTypeRequestObject,
) (gen.DeleteProjectTypeResponseObject, error) {
	h.logger.Info("DeleteProjectType called", "namespaceName", request.NamespaceName, "ptName", request.PtName)

	err := h.services.ProjectTypeService.DeleteProjectType(ctx, request.NamespaceName, request.PtName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteProjectType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, projecttypesvc.ErrProjectTypeNotFound) {
			return gen.DeleteProjectType404JSONResponse{NotFoundJSONResponse: notFound("Project type")}, nil
		}
		h.logger.Error("Failed to delete project type", "error", err)
		return gen.DeleteProjectType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Project type deleted successfully", "namespaceName", request.NamespaceName, "projectType", request.PtName)
	return gen.DeleteProjectType204Response{}, nil
}

// GetProjectTypeSchema returns the parameter schema for a project type.
func (h *Handler) GetProjectTypeSchema(
	ctx context.Context,
	request gen.GetProjectTypeSchemaRequestObject,
) (gen.GetProjectTypeSchemaResponseObject, error) {
	h.logger.Debug("GetProjectTypeSchema called", "namespaceName", request.NamespaceName, "ptName", request.PtName)

	rawSchema, err := h.services.ProjectTypeService.GetProjectTypeSchema(ctx, request.NamespaceName, request.PtName)
	if err != nil {
		if errors.Is(err, projecttypesvc.ErrProjectTypeNotFound) {
			return gen.GetProjectTypeSchema404JSONResponse{NotFoundJSONResponse: notFound("Project type")}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetProjectTypeSchema403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to get project type schema", "error", err)
		return gen.GetProjectTypeSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetProjectTypeSchema200JSONResponse(rawSchema), nil
}
