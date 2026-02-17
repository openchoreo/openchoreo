// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	projectsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project"
)

// ListProjects returns a paginated list of projects within a namespace.
func (h *Handler) ListProjects(
	ctx context.Context,
	request gen.ListProjectsRequestObject,
) (gen.ListProjectsResponseObject, error) {
	h.logger.Debug("ListProjects called", "namespaceName", request.NamespaceName)

	projects, err := h.projectService.ListProjects(ctx, request.NamespaceName)
	if err != nil {
		h.logger.Error("Failed to list projects", "error", err)
		return gen.ListProjects500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.Project, gen.Project](projects)
	if err != nil {
		h.logger.Error("Failed to convert projects", "error", err)
		return gen.ListProjects500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListProjects200JSONResponse{
		Items:      items,
		Pagination: &gen.Pagination{},
	}, nil
}

// CreateProject creates a new project within a namespace.
func (h *Handler) CreateProject(
	ctx context.Context,
	request gen.CreateProjectRequestObject,
) (gen.CreateProjectResponseObject, error) {
	h.logger.Info("CreateProject called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateProject400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	projectCR, err := convert[gen.Project, openchoreov1alpha1.Project](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateProject400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	created, err := h.projectService.CreateProject(ctx, request.NamespaceName, &projectCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateProject403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, projectsvc.ErrProjectAlreadyExists) {
			return gen.CreateProject409JSONResponse{ConflictJSONResponse: conflict("Project already exists")}, nil
		}
		h.logger.Error("Failed to create project", "error", err)
		return gen.CreateProject500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genProject, err := convert[openchoreov1alpha1.Project, gen.Project](*created)
	if err != nil {
		h.logger.Error("Failed to convert created project", "error", err)
		return gen.CreateProject500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Project created successfully", "namespaceName", request.NamespaceName, "project", created.Name)
	return gen.CreateProject201JSONResponse(genProject), nil
}

// GetProject returns details of a specific project.
func (h *Handler) GetProject(
	ctx context.Context,
	request gen.GetProjectRequestObject,
) (gen.GetProjectResponseObject, error) {
	h.logger.Debug("GetProject called", "namespaceName", request.NamespaceName, "projectName", request.ProjectName)

	project, err := h.projectService.GetProject(ctx, request.NamespaceName, request.ProjectName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetProject403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, projectsvc.ErrProjectNotFound) {
			return gen.GetProject404JSONResponse{NotFoundJSONResponse: notFound("Project")}, nil
		}
		h.logger.Error("Failed to get project", "error", err)
		return gen.GetProject500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genProject, err := convert[openchoreov1alpha1.Project, gen.Project](*project)
	if err != nil {
		h.logger.Error("Failed to convert project", "error", err)
		return gen.GetProject500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetProject200JSONResponse(genProject), nil
}

// UpdateProject replaces an existing project (full update).
func (h *Handler) UpdateProject(
	ctx context.Context,
	request gen.UpdateProjectRequestObject,
) (gen.UpdateProjectResponseObject, error) {
	h.logger.Info("UpdateProject called", "namespaceName", request.NamespaceName, "projectName", request.ProjectName)

	if request.Body == nil {
		return gen.UpdateProject400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	projectCR, err := convert[gen.Project, openchoreov1alpha1.Project](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateProject400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	// Ensure the name from the URL path is used
	projectCR.Name = request.ProjectName

	updated, err := h.projectService.UpdateProject(ctx, request.NamespaceName, &projectCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateProject403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, projectsvc.ErrProjectNotFound) {
			return gen.UpdateProject404JSONResponse{NotFoundJSONResponse: notFound("Project")}, nil
		}
		h.logger.Error("Failed to update project", "error", err)
		return gen.UpdateProject500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genProject, err := convert[openchoreov1alpha1.Project, gen.Project](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated project", "error", err)
		return gen.UpdateProject500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Project updated successfully", "namespaceName", request.NamespaceName, "project", updated.Name)
	return gen.UpdateProject200JSONResponse(genProject), nil
}

// DeleteProject deletes a project by name.
func (h *Handler) DeleteProject(
	ctx context.Context,
	request gen.DeleteProjectRequestObject,
) (gen.DeleteProjectResponseObject, error) {
	h.logger.Info("DeleteProject called", "namespaceName", request.NamespaceName, "projectName", request.ProjectName)

	err := h.projectService.DeleteProject(ctx, request.NamespaceName, request.ProjectName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteProject403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, projectsvc.ErrProjectNotFound) {
			return gen.DeleteProject404JSONResponse{NotFoundJSONResponse: notFound("Project")}, nil
		}
		h.logger.Error("Failed to delete project", "error", err)
		return gen.DeleteProject500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Project deleted successfully", "namespaceName", request.NamespaceName, "project", request.ProjectName)
	return gen.DeleteProject204Response{}, nil
}
