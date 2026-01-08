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
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// ListComponents returns a paginated list of components within a project
func (h *Handler) ListComponents(
	ctx context.Context,
	request gen.ListComponentsRequestObject,
) (gen.ListComponentsResponseObject, error) {
	h.logger.Debug("ListComponents called", "orgName", request.OrgName, "projectName", request.ProjectName)

	components, err := h.services.ComponentService.ListComponents(
		ctx,
		request.OrgName,
		request.ProjectName,
	)
	if err != nil {
		h.logger.Error("Failed to list components", "error", err)
		return gen.ListComponents500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert to generated types
	items := make([]gen.Component, 0, len(components))
	for _, c := range components {
		items = append(items, toGenComponent(c))
	}

	// TODO: Implement proper cursor-based pagination with Kubernetes continuation tokens
	return gen.ListComponents200JSONResponse{
		Items:      items,
		Pagination: gen.Pagination{},
	}, nil
}

// CreateComponent creates a new component within a project
func (h *Handler) CreateComponent(
	ctx context.Context,
	request gen.CreateComponentRequestObject,
) (gen.CreateComponentResponseObject, error) {
	h.logger.Info("CreateComponent called", "orgName", request.OrgName, "projectName", request.ProjectName)

	if request.Body == nil {
		return gen.CreateComponent400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	// Convert to service request
	req := &models.CreateComponentRequest{
		Name:          request.Body.Name,
		DisplayName:   ptr.Deref(request.Body.DisplayName, ""),
		Description:   ptr.Deref(request.Body.Description, ""),
		ComponentType: ptr.Deref(request.Body.ComponentType, ""),
	}

	if request.Body.AutoDeploy != nil {
		req.AutoDeploy = request.Body.AutoDeploy
	}

	// TODO: Convert traits and workflow from gen types to models types
	// This requires more complex conversion logic for nested structures

	component, err := h.services.ComponentService.CreateComponent(
		ctx,
		request.OrgName,
		request.ProjectName,
		req,
	)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateComponent403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			return gen.CreateComponent404JSONResponse{NotFoundJSONResponse: notFound("Project")}, nil
		}
		if errors.Is(err, services.ErrComponentAlreadyExists) {
			return gen.CreateComponent409JSONResponse{ConflictJSONResponse: conflict("Component already exists")}, nil
		}
		h.logger.Error("Failed to create component", "error", err)
		return gen.CreateComponent500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Component created successfully",
		"orgName", request.OrgName,
		"projectName", request.ProjectName,
		"component", component.Name)

	return gen.CreateComponent201JSONResponse(toGenComponent(component)), nil
}

// toGenComponent converts models.ComponentResponse to gen.Component
func toGenComponent(c *models.ComponentResponse) gen.Component {
	uid, _ := uuid.Parse(c.UID)
	return gen.Component{
		Uid:         uid,
		Name:        c.Name,
		Type:        c.Type,
		ProjectName: c.ProjectName,
		OrgName:     c.OrgName,
		CreatedAt:   c.CreatedAt,
		DisplayName: ptr.To(c.DisplayName),
		Description: ptr.To(c.Description),
		Status:      ptr.To(c.Status),
		AutoDeploy:  ptr.To(c.AutoDeploy),
		// TODO: Convert workload and componentWorkflow fields
	}
}
