// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	projectreleasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/projectrelease"
)

// ListProjectReleases returns a paginated list of project releases within a namespace.
func (h *Handler) ListProjectReleases(
	ctx context.Context,
	request gen.ListProjectReleasesRequestObject,
) (gen.ListProjectReleasesResponseObject, error) {
	h.logger.Debug("ListProjectReleases called", "namespaceName", request.NamespaceName)

	projectName := ""
	if request.Params.Project != nil {
		projectName = *request.Params.Project
	}

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.ProjectReleaseService.ListProjectReleases(ctx, request.NamespaceName, projectName, opts)
	if err != nil {
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.ListProjectReleases400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to list project releases", "error", err)
		return gen.ListProjectReleases500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ProjectRelease, gen.ProjectRelease](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert project releases", "error", err)
		return gen.ListProjectReleases500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	resp := gen.ListProjectReleases200JSONResponse{
		Items: items,
	}
	resp.Pagination = ToPagination(result)

	return resp, nil
}

// GetProjectRelease returns details of a specific project release.
func (h *Handler) GetProjectRelease(
	ctx context.Context,
	request gen.GetProjectReleaseRequestObject,
) (gen.GetProjectReleaseResponseObject, error) {
	h.logger.Debug("GetProjectRelease called", "namespaceName", request.NamespaceName, "projectReleaseName", request.ProjectReleaseName)

	pr, err := h.services.ProjectReleaseService.GetProjectRelease(ctx, request.NamespaceName, request.ProjectReleaseName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetProjectRelease403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, projectreleasesvc.ErrProjectReleaseNotFound) {
			return gen.GetProjectRelease404JSONResponse{NotFoundJSONResponse: notFound("ProjectRelease")}, nil
		}
		h.logger.Error("Failed to get project release", "error", err)
		return gen.GetProjectRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genPR, err := convert[openchoreov1alpha1.ProjectRelease, gen.ProjectRelease](*pr)
	if err != nil {
		h.logger.Error("Failed to convert project release", "error", err)
		return gen.GetProjectRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetProjectRelease200JSONResponse(genPR), nil
}

// CreateProjectRelease creates a new project release within a namespace.
func (h *Handler) CreateProjectRelease(
	ctx context.Context,
	request gen.CreateProjectReleaseRequestObject,
) (gen.CreateProjectReleaseResponseObject, error) {
	h.logger.Info("CreateProjectRelease called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateProjectRelease400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	prCR, err := convert[gen.ProjectRelease, openchoreov1alpha1.ProjectRelease](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateProjectRelease400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	created, err := h.services.ProjectReleaseService.CreateProjectRelease(ctx, request.NamespaceName, &prCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateProjectRelease403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, projectreleasesvc.ErrProjectReleaseAlreadyExists) {
			return gen.CreateProjectRelease409JSONResponse{ConflictJSONResponse: conflict("ProjectRelease already exists")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.CreateProjectRelease400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to create project release", "error", err)
		return gen.CreateProjectRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genPR, err := convert[openchoreov1alpha1.ProjectRelease, gen.ProjectRelease](*created)
	if err != nil {
		h.logger.Error("Failed to convert created project release", "error", err)
		return gen.CreateProjectRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ProjectRelease created successfully", "namespaceName", request.NamespaceName, "projectRelease", created.Name)
	return gen.CreateProjectRelease201JSONResponse(genPR), nil
}

// DeleteProjectRelease deletes a project release by name.
func (h *Handler) DeleteProjectRelease(
	ctx context.Context,
	request gen.DeleteProjectReleaseRequestObject,
) (gen.DeleteProjectReleaseResponseObject, error) {
	h.logger.Info("DeleteProjectRelease called", "namespaceName", request.NamespaceName, "projectReleaseName", request.ProjectReleaseName)

	err := h.services.ProjectReleaseService.DeleteProjectRelease(ctx, request.NamespaceName, request.ProjectReleaseName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteProjectRelease403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, projectreleasesvc.ErrProjectReleaseNotFound) {
			return gen.DeleteProjectRelease404JSONResponse{NotFoundJSONResponse: notFound("ProjectRelease")}, nil
		}
		h.logger.Error("Failed to delete project release", "error", err)
		return gen.DeleteProjectRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ProjectRelease deleted successfully", "namespaceName", request.NamespaceName, "projectRelease", request.ProjectReleaseName)
	return gen.DeleteProjectRelease204Response{}, nil
}
