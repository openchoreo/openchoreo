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
	h.logger.Debug("ListComponents called", "namespaceName", request.NamespaceName, "projectName", request.ProjectName)

	components, err := h.services.ComponentService.ListComponents(
		ctx,
		request.NamespaceName,
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
	h.logger.Info("CreateComponent called", "namespaceName", request.NamespaceName, "projectName", request.ProjectName)

	if request.Body == nil {
		return gen.CreateComponent400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	// Convert to service request
	req := toModelCreateComponentRequest(request.Body)

	component, err := h.services.ComponentService.CreateComponent(
		ctx,
		request.NamespaceName,
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
		"namespaceName", request.NamespaceName,
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
		NamespaceName:     c.NamespaceName,
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

// GetComponent returns details of a specific component
func (h *Handler) GetComponent(
	ctx context.Context,
	request gen.GetComponentRequestObject,
) (gen.GetComponentResponseObject, error) {
	return nil, errNotImplemented
}

// PatchComponent updates a component with partial data
func (h *Handler) PatchComponent(
	ctx context.Context,
	request gen.PatchComponentRequestObject,
) (gen.PatchComponentResponseObject, error) {
	return nil, errNotImplemented
}

// GetComponentSchema returns the combined parameter schema for a component
func (h *Handler) GetComponentSchema(
	ctx context.Context,
	request gen.GetComponentSchemaRequestObject,
) (gen.GetComponentSchemaResponseObject, error) {
	return nil, errNotImplemented
}

// ListComponentTraits returns the traits attached to a component
func (h *Handler) ListComponentTraits(
	ctx context.Context,
	request gen.ListComponentTraitsRequestObject,
) (gen.ListComponentTraitsResponseObject, error) {
	return nil, errNotImplemented
}

// UpdateComponentTraits replaces the traits attached to a component
func (h *Handler) UpdateComponentTraits(
	ctx context.Context,
	request gen.UpdateComponentTraitsRequestObject,
) (gen.UpdateComponentTraitsResponseObject, error) {
	return nil, errNotImplemented
}

// ListComponentBindings returns deployment bindings for a component
func (h *Handler) ListComponentBindings(
	ctx context.Context,
	request gen.ListComponentBindingsRequestObject,
) (gen.ListComponentBindingsResponseObject, error) {
	return nil, errNotImplemented
}

// UpdateComponentBinding updates a component's deployment binding
func (h *Handler) UpdateComponentBinding(
	ctx context.Context,
	request gen.UpdateComponentBindingRequestObject,
) (gen.UpdateComponentBindingResponseObject, error) {
	return nil, errNotImplemented
}

// ListComponentReleases returns immutable release snapshots for a component
func (h *Handler) ListComponentReleases(
	ctx context.Context,
	request gen.ListComponentReleasesRequestObject,
) (gen.ListComponentReleasesResponseObject, error) {
	return nil, errNotImplemented
}

// CreateComponentRelease creates an immutable release snapshot
func (h *Handler) CreateComponentRelease(
	ctx context.Context,
	request gen.CreateComponentReleaseRequestObject,
) (gen.CreateComponentReleaseResponseObject, error) {
	return nil, errNotImplemented
}

// GetComponentRelease returns details of a specific component release
func (h *Handler) GetComponentRelease(
	ctx context.Context,
	request gen.GetComponentReleaseRequestObject,
) (gen.GetComponentReleaseResponseObject, error) {
	return nil, errNotImplemented
}

// GetComponentReleaseSchema returns the parameter schema for a component release
func (h *Handler) GetComponentReleaseSchema(
	ctx context.Context,
	request gen.GetComponentReleaseSchemaRequestObject,
) (gen.GetComponentReleaseSchemaResponseObject, error) {
	return nil, errNotImplemented
}

// ListReleaseBindings returns environment-specific release bindings
func (h *Handler) ListReleaseBindings(
	ctx context.Context,
	request gen.ListReleaseBindingsRequestObject,
) (gen.ListReleaseBindingsResponseObject, error) {
	return nil, errNotImplemented
}

// PatchReleaseBinding updates a release binding with environment-specific overrides
func (h *Handler) PatchReleaseBinding(
	ctx context.Context,
	request gen.PatchReleaseBindingRequestObject,
) (gen.PatchReleaseBindingResponseObject, error) {
	return nil, errNotImplemented
}

// GetEnvironmentRelease returns the deployed release for a component in an environment
func (h *Handler) GetEnvironmentRelease(
	ctx context.Context,
	request gen.GetEnvironmentReleaseRequestObject,
) (gen.GetEnvironmentReleaseResponseObject, error) {
	return nil, errNotImplemented
}

// DeployRelease deploys a component release to an environment
func (h *Handler) DeployRelease(
	ctx context.Context,
	request gen.DeployReleaseRequestObject,
) (gen.DeployReleaseResponseObject, error) {
	return nil, errNotImplemented
}

// PromoteComponent promotes a component release from one environment to another
func (h *Handler) PromoteComponent(
	ctx context.Context,
	request gen.PromoteComponentRequestObject,
) (gen.PromoteComponentResponseObject, error) {
	return nil, errNotImplemented
}

// GetComponentObserverURL returns the observer URL for component logs and metrics
func (h *Handler) GetComponentObserverURL(
	ctx context.Context,
	request gen.GetComponentObserverURLRequestObject,
) (gen.GetComponentObserverURLResponseObject, error) {
	return nil, errNotImplemented
}

// GetBuildObserverURL returns the observer URL for component build logs
func (h *Handler) GetBuildObserverURL(
	ctx context.Context,
	request gen.GetBuildObserverURLRequestObject,
) (gen.GetBuildObserverURLResponseObject, error) {
	return nil, errNotImplemented
}
