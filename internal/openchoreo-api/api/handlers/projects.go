// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacy_services"
)

// ListProjects returns a paginated list of projects within an namespace
func (h *Handler) ListProjects(
	ctx context.Context,
	request gen.ListProjectsRequestObject,
) (gen.ListProjectsResponseObject, error) {
	h.logger.Debug("ListProjects called", "namespaceName", request.NamespaceName)

	projects, err := h.services.ProjectService.ListProjects(ctx, request.NamespaceName)
	if err != nil {
		h.logger.Error("Failed to list projects", "error", err)
		return gen.ListProjects500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert to generated types
	items := make([]gen.Project, 0, len(projects))
	for _, p := range projects {
		items = append(items, toGenProject(p))
	}

	// TODO: Implement proper cursor-based pagination with Kubernetes continuation tokens
	return gen.ListProjects200JSONResponse{
		Items:      items,
		Pagination: gen.Pagination{},
	}, nil
}

// CreateProject creates a new project within an namespace
func (h *Handler) CreateProject(
	ctx context.Context,
	request gen.CreateProjectRequestObject,
) (gen.CreateProjectResponseObject, error) {
	h.logger.Info("CreateProject called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateProject400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	// Convert to service request
	req := &models.CreateProjectRequest{
		Name:               request.Body.Name,
		DisplayName:        ptr.Deref(request.Body.DisplayName, ""),
		Description:        ptr.Deref(request.Body.Description, ""),
		DeploymentPipeline: ptr.Deref(request.Body.DeploymentPipeline, ""),
	}

	project, err := h.services.ProjectService.CreateProject(ctx, request.NamespaceName, req)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateProject403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrProjectAlreadyExists) {
			return gen.CreateProject409JSONResponse{ConflictJSONResponse: conflict("Project already exists")}, nil
		}
		h.logger.Error("Failed to create project", "error", err)
		return gen.CreateProject500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Project created successfully", "namespaceName", request.NamespaceName, "project", project.Name)
	return gen.CreateProject201JSONResponse(toGenProject(project)), nil
}

// GetProject returns details of a specific project
func (h *Handler) GetProject(
	ctx context.Context,
	request gen.GetProjectRequestObject,
) (gen.GetProjectResponseObject, error) {
	h.logger.Debug("GetProject called", "namespaceName", request.NamespaceName, "projectName", request.ProjectName)

	project, err := h.services.ProjectService.GetProject(
		ctx,
		request.NamespaceName,
		request.ProjectName,
	)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetProject403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			return gen.GetProject404JSONResponse{NotFoundJSONResponse: notFound("Project")}, nil
		}
		h.logger.Error("Failed to get project", "error", err)
		return gen.GetProject500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetProject200JSONResponse(toGenProject(project)), nil
}

// toGenProject converts models.ProjectResponse to gen.Project
func toGenProject(p *models.ProjectResponse) gen.Project {
	uid, _ := uuid.Parse(p.UID)
	return gen.Project{
		Uid:                uid,
		Name:               p.Name,
		NamespaceName:      p.NamespaceName,
		DisplayName:        ptr.To(p.DisplayName),
		Description:        ptr.To(p.Description),
		DeploymentPipeline: ptr.To(p.DeploymentPipeline),
		CreatedAt:          p.CreatedAt,
		Status:             ptr.To(p.Status),
	}
}
