// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/runtime"
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
	req := toModelCreateComponentRequest(request.Body)

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

// toModelCreateComponentRequest converts gen.CreateComponentRequest to models.CreateComponentRequest
func toModelCreateComponentRequest(req *gen.CreateComponentRequest) *models.CreateComponentRequest {
	if req == nil {
		return nil
	}

	return &models.CreateComponentRequest{
		Name:              req.Name,
		DisplayName:       ptr.Deref(req.DisplayName, ""),
		Description:       ptr.Deref(req.Description, ""),
		Type:              ptr.Deref(req.Type, ""),
		ComponentType:     ptr.Deref(req.ComponentType, ""),
		AutoDeploy:        req.AutoDeploy,
		Parameters:        mapToRawExtension(req.Parameters),
		Traits:            toModelTraits(req.Traits),
		ComponentWorkflow: toModelComponentWorkflow(req.Workflow),
	}
}

// toModelTraits converts *[]gen.ComponentTraitInput to []models.ComponentTrait
func toModelTraits(traits *[]gen.ComponentTraitInput) []models.ComponentTrait {
	if traits == nil || len(*traits) == 0 {
		return nil
	}

	result := make([]models.ComponentTrait, len(*traits))
	for i, t := range *traits {
		result[i] = models.ComponentTrait{
			Name:         t.Name,
			InstanceName: t.InstanceName,
			Parameters:   mapToRawExtension(t.Parameters),
		}
	}
	return result
}

// toModelComponentWorkflow converts *gen.ComponentWorkflowInput to *models.ComponentWorkflow
func toModelComponentWorkflow(workflow *gen.ComponentWorkflowInput) *models.ComponentWorkflow {
	if workflow == nil {
		return nil
	}

	return &models.ComponentWorkflow{
		Name: workflow.Name,
		SystemParameters: &models.ComponentWorkflowSystemParams{
			Repository: models.ComponentWorkflowRepository{
				URL:     workflow.SystemParameters.Repository.Url,
				AppPath: workflow.SystemParameters.Repository.AppPath,
				Revision: models.ComponentWorkflowRepositoryRevision{
					Branch: workflow.SystemParameters.Repository.Revision.Branch,
					Commit: ptr.Deref(workflow.SystemParameters.Repository.Revision.Commit, ""),
				},
			},
		},
		Parameters: mapToRawExtension(workflow.Parameters),
	}
}

// mapToRawExtension converts *map[string]interface{} to *runtime.RawExtension
func mapToRawExtension(m *map[string]interface{}) *runtime.RawExtension {
	if m == nil || len(*m) == 0 {
		return nil
	}

	data, err := json.Marshal(m)
	if err != nil {
		return nil
	}

	return &runtime.RawExtension{Raw: data}
}
