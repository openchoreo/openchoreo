// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	svcerrors "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	componentsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/component"
	projectsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project"
)

// ListComponents returns a paginated list of components within a namespace.
func (h *Handler) ListComponents(
	ctx context.Context,
	request gen.ListComponentsRequestObject,
) (gen.ListComponentsResponseObject, error) {
	h.logger.Debug("ListComponents called", "namespaceName", request.NamespaceName)

	projectName := ""
	if request.Params.Project != nil {
		projectName = *request.Params.Project
	}

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.componentService.ListComponents(ctx, request.NamespaceName, projectName, opts)
	if err != nil {
		if errors.Is(err, projectsvc.ErrProjectNotFound) {
			return gen.ListComponents404JSONResponse{NotFoundJSONResponse: notFound("Project")}, nil
		}
		h.logger.Error("Failed to list components", "error", err)
		return gen.ListComponents500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.Component, gen.Component](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert components", "error", err)
		return gen.ListComponents500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListComponents200JSONResponse{
		Items:      items,
		Pagination: ToPaginationPtr(result),
	}, nil
}

// CreateComponent creates a new component within a namespace.
func (h *Handler) CreateComponent(
	ctx context.Context,
	request gen.CreateComponentRequestObject,
) (gen.CreateComponentResponseObject, error) {
	h.logger.Info("CreateComponent called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateComponent400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	componentCR, err := convert[gen.Component, openchoreov1alpha1.Component](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateComponent400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	if componentCR.Namespace != "" && componentCR.Namespace != request.NamespaceName {
		return gen.CreateComponent400JSONResponse{BadRequestJSONResponse: badRequest("Namespace in body does not match path")}, nil
	}
	componentCR.Status = openchoreov1alpha1.ComponentStatus{}

	created, err := h.componentService.CreateComponent(ctx, request.NamespaceName, &componentCR)
	if err != nil {
		if errors.Is(err, svcerrors.ErrForbidden) {
			return gen.CreateComponent403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, projectsvc.ErrProjectNotFound) {
			return gen.CreateComponent400JSONResponse{BadRequestJSONResponse: badRequest("Referenced project not found")}, nil
		}
		if errors.Is(err, componentsvc.ErrComponentAlreadyExists) {
			return gen.CreateComponent409JSONResponse{ConflictJSONResponse: conflict("Component already exists")}, nil
		}
		h.logger.Error("Failed to create component", "error", err)
		return gen.CreateComponent500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genComponent, err := convert[openchoreov1alpha1.Component, gen.Component](*created)
	if err != nil {
		h.logger.Error("Failed to convert created component", "error", err)
		return gen.CreateComponent500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Component created successfully", "namespaceName", request.NamespaceName, "component", created.Name)
	return gen.CreateComponent201JSONResponse(genComponent), nil
}

// toGenComponent converts models.ComponentResponse to gen.Component (K8s-native shape).
// Used by legacy sub-resource methods that still call the legacy service layer.
func toGenComponent(c *models.ComponentResponse) gen.Component {
	uid := c.UID
	metadata := gen.ObjectMeta{
		Name:              c.Name,
		Namespace:         ptr.To(c.NamespaceName),
		Uid:               ptr.To(uid),
		CreationTimestamp: ptr.To(c.CreatedAt),
	}

	componentTypeName := gen.ComponentSpecComponentTypeKind("ComponentType")
	spec := &gen.ComponentSpec{
		Owner: struct {
			ProjectName string `json:"projectName"`
		}{ProjectName: c.ProjectName},
		ComponentType: struct {
			Kind *gen.ComponentSpecComponentTypeKind `json:"kind,omitempty"`
			Name string                              `json:"name"`
		}{
			Kind: &componentTypeName,
			Name: c.Type,
		},
		AutoDeploy: ptr.To(c.AutoDeploy),
	}

	if c.ComponentWorkflow != nil {
		spec.Workflow = toGenComponentWorkflowConfig(c.ComponentWorkflow)
	}

	return gen.Component{
		Metadata: metadata,
		Spec:     spec,
	}
}

// toGenComponentWorkflowConfig converts models.ComponentWorkflow to gen.ComponentWorkflowConfig
func toGenComponentWorkflowConfig(cw *models.ComponentWorkflow) *gen.ComponentWorkflowConfig {
	if cw == nil {
		return nil
	}

	config := &gen.ComponentWorkflowConfig{
		Name: ptr.To(cw.Name),
	}

	// Convert Parameters from runtime.RawExtension to map[string]interface{}
	if cw.Parameters != nil && cw.Parameters.Raw != nil {
		var params map[string]interface{}
		if err := json.Unmarshal(cw.Parameters.Raw, &params); err == nil {
			config.Parameters = &params
		}
	}

	// Convert SystemParameters
	if cw.SystemParameters != nil {
		config.SystemParameters = &struct {
			Repository *struct {
				AppPath  *string `json:"appPath,omitempty"`
				Revision *struct {
					Branch *string `json:"branch,omitempty"`
					Commit *string `json:"commit,omitempty"`
				} `json:"revision,omitempty"`
				Url *string `json:"url,omitempty"` //nolint
			} `json:"repository,omitempty"`
		}{
			Repository: &struct {
				AppPath  *string `json:"appPath,omitempty"`
				Revision *struct {
					Branch *string `json:"branch,omitempty"`
					Commit *string `json:"commit,omitempty"`
				} `json:"revision,omitempty"`
				Url *string `json:"url,omitempty"` //nolint
			}{
				AppPath: ptr.To(cw.SystemParameters.Repository.AppPath),
				Revision: &struct {
					Branch *string `json:"branch,omitempty"`
					Commit *string `json:"commit,omitempty"`
				}{
					Branch: ptr.To(cw.SystemParameters.Repository.Revision.Branch),
					Commit: ptr.To(cw.SystemParameters.Repository.Revision.Commit),
				},
				Url: ptr.To(cw.SystemParameters.Repository.URL),
			},
		}
	}

	return config
}

// toModelCreateComponentRequest converts gen.CreateComponentRequest to models.CreateComponentRequest
func toModelCreateComponentRequest(req *gen.CreateComponentRequest) *models.CreateComponentRequest {
	if req == nil {
		return nil
	}

	var componentTypeRef *models.ComponentTypeRef
	if req.ComponentType != nil && *req.ComponentType != "" {
		componentTypeRef = &models.ComponentTypeRef{
			Kind: "ComponentType",
			Name: *req.ComponentType,
		}
	}

	return &models.CreateComponentRequest{
		Name:              req.Name,
		DisplayName:       ptr.Deref(req.DisplayName, ""),
		Description:       ptr.Deref(req.Description, ""),
		ComponentType:     componentTypeRef,
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
		if t.Kind != nil {
			result[i].Kind = string(*t.Kind)
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

// GetComponent returns details of a specific component.
func (h *Handler) GetComponent(
	ctx context.Context,
	request gen.GetComponentRequestObject,
) (gen.GetComponentResponseObject, error) {
	h.logger.Debug("GetComponent called", "namespaceName", request.NamespaceName, "componentName", request.ComponentName)

	component, err := h.componentService.GetComponent(ctx, request.NamespaceName, request.ComponentName)
	if err != nil {
		if errors.Is(err, svcerrors.ErrForbidden) {
			return gen.GetComponent403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, componentsvc.ErrComponentNotFound) {
			return gen.GetComponent404JSONResponse{NotFoundJSONResponse: notFound("Component")}, nil
		}
		h.logger.Error("Failed to get component", "error", err)
		return gen.GetComponent500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genComponent, err := convert[openchoreov1alpha1.Component, gen.Component](*component)
	if err != nil {
		h.logger.Error("Failed to convert component", "error", err)
		return gen.GetComponent500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetComponent200JSONResponse(genComponent), nil
}

// UpdateComponent replaces an existing component (full update).
func (h *Handler) UpdateComponent(
	ctx context.Context,
	request gen.UpdateComponentRequestObject,
) (gen.UpdateComponentResponseObject, error) {
	h.logger.Info("UpdateComponent called", "namespaceName", request.NamespaceName, "componentName", request.ComponentName)

	if request.Body == nil {
		return gen.UpdateComponent400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	componentCR, err := convert[gen.Component, openchoreov1alpha1.Component](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateComponent400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	if componentCR.Namespace != "" && componentCR.Namespace != request.NamespaceName {
		return gen.UpdateComponent400JSONResponse{BadRequestJSONResponse: badRequest("Namespace in body does not match path")}, nil
	}
	componentCR.Status = openchoreov1alpha1.ComponentStatus{}
	componentCR.Name = request.ComponentName

	updated, err := h.componentService.UpdateComponent(ctx, request.NamespaceName, &componentCR)
	if err != nil {
		if errors.Is(err, svcerrors.ErrForbidden) {
			return gen.UpdateComponent403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, componentsvc.ErrComponentNotFound) {
			return gen.UpdateComponent404JSONResponse{NotFoundJSONResponse: notFound("Component")}, nil
		}
		h.logger.Error("Failed to update component", "error", err)
		return gen.UpdateComponent500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genComponent, err := convert[openchoreov1alpha1.Component, gen.Component](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated component", "error", err)
		return gen.UpdateComponent500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Component updated successfully", "namespaceName", request.NamespaceName, "component", updated.Name)
	return gen.UpdateComponent200JSONResponse(genComponent), nil
}

// DeleteComponent deletes a component by name.
func (h *Handler) DeleteComponent(
	ctx context.Context,
	request gen.DeleteComponentRequestObject,
) (gen.DeleteComponentResponseObject, error) {
	h.logger.Info("DeleteComponent called", "namespaceName", request.NamespaceName, "componentName", request.ComponentName)

	err := h.componentService.DeleteComponent(ctx, request.NamespaceName, request.ComponentName)
	if err != nil {
		if errors.Is(err, svcerrors.ErrForbidden) {
			return gen.DeleteComponent403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, componentsvc.ErrComponentNotFound) {
			return gen.DeleteComponent404JSONResponse{NotFoundJSONResponse: notFound("Component")}, nil
		}
		h.logger.Error("Failed to delete component", "error", err)
		return gen.DeleteComponent500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Component deleted successfully", "namespaceName", request.NamespaceName, "component", request.ComponentName)
	return gen.DeleteComponent204Response{}, nil
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
	h.logger.Info("ListComponentReleases called",
		"namespaceName", request.NamespaceName,
		"projectName", request.ProjectName,
		"componentName", request.ComponentName)

	releases, err := h.services.ComponentService.ListComponentReleases(
		ctx,
		request.NamespaceName,
		request.ProjectName,
		request.ComponentName,
	)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListComponentReleases403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			return gen.ListComponentReleases404JSONResponse{NotFoundJSONResponse: notFound("Project")}, nil
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			return gen.ListComponentReleases404JSONResponse{NotFoundJSONResponse: notFound("Component")}, nil
		}
		h.logger.Error("Failed to list component releases", "error", err)
		return gen.ListComponentReleases500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	result := gen.ComponentReleaseList{
		Items: make([]gen.ComponentRelease, 0, len(releases)),
	}
	for _, release := range releases {
		result.Items = append(result.Items, toGenComponentRelease(release))
	}

	return gen.ListComponentReleases200JSONResponse(result), nil
}

// CreateComponentRelease creates an immutable release snapshot
func (h *Handler) CreateComponentRelease(
	ctx context.Context,
	request gen.CreateComponentReleaseRequestObject,
) (gen.CreateComponentReleaseResponseObject, error) {
	h.logger.Info("CreateComponentRelease called",
		"namespaceName", request.NamespaceName,
		"projectName", request.ProjectName,
		"componentName", request.ComponentName)

	releaseName := ""
	if request.Body != nil && request.Body.ReleaseName != nil {
		releaseName = *request.Body.ReleaseName
	}

	release, err := h.services.ComponentService.CreateComponentRelease(
		ctx,
		request.NamespaceName,
		request.ProjectName,
		request.ComponentName,
		releaseName,
	)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateComponentRelease403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			return gen.CreateComponentRelease404JSONResponse{NotFoundJSONResponse: notFound("Project")}, nil
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			return gen.CreateComponentRelease404JSONResponse{NotFoundJSONResponse: notFound("Component")}, nil
		}
		if errors.Is(err, services.ErrWorkloadNotFound) {
			return gen.CreateComponentRelease404JSONResponse{NotFoundJSONResponse: notFound("Workload")}, nil
		}
		h.logger.Error("Failed to create component release", "error", err)
		return gen.CreateComponentRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.CreateComponentRelease201JSONResponse(toGenComponentRelease(release)), nil
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
	h.logger.Info("ListReleaseBindings called",
		"namespaceName", request.NamespaceName,
		"projectName", request.ProjectName,
		"componentName", request.ComponentName)

	// Get environments filter from query params
	var environments []string
	if request.Params.Environment != nil {
		environments = *request.Params.Environment
	}

	bindings, err := h.services.ComponentService.ListReleaseBindings(
		ctx,
		request.NamespaceName,
		request.ProjectName,
		request.ComponentName,
		environments,
	)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListReleaseBindings403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			return gen.ListReleaseBindings404JSONResponse{NotFoundJSONResponse: notFound("Project")}, nil
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			return gen.ListReleaseBindings404JSONResponse{NotFoundJSONResponse: notFound("Component")}, nil
		}
		h.logger.Error("Failed to list release bindings", "error", err)
		return gen.ListReleaseBindings500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	result := gen.ReleaseBindingList{
		Items: make([]gen.ReleaseBinding, 0, len(bindings)),
	}
	for _, binding := range bindings {
		result.Items = append(result.Items, toGenReleaseBinding(binding))
	}

	return gen.ListReleaseBindings200JSONResponse(result), nil
}

// PatchReleaseBinding updates a release binding with environment-specific overrides
func (h *Handler) PatchReleaseBinding(
	ctx context.Context,
	request gen.PatchReleaseBindingRequestObject,
) (gen.PatchReleaseBindingResponseObject, error) {
	h.logger.Info("PatchReleaseBinding called",
		"namespaceName", request.NamespaceName,
		"projectName", request.ProjectName,
		"componentName", request.ComponentName,
		"bindingName", request.BindingName)

	if request.Body == nil {
		return gen.PatchReleaseBinding400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	patchReq := toModelPatchReleaseBindingRequest(request.Body)

	binding, err := h.services.ComponentService.PatchReleaseBinding(
		ctx,
		request.NamespaceName,
		request.ProjectName,
		request.ComponentName,
		request.BindingName,
		patchReq,
	)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.PatchReleaseBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			return gen.PatchReleaseBinding404JSONResponse{NotFoundJSONResponse: notFound("Project")}, nil
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			return gen.PatchReleaseBinding404JSONResponse{NotFoundJSONResponse: notFound("Component")}, nil
		}
		if errors.Is(err, services.ErrReleaseBindingNotFound) {
			return gen.PatchReleaseBinding404JSONResponse{NotFoundJSONResponse: notFound("ReleaseBinding")}, nil
		}
		h.logger.Error("Failed to patch release binding", "error", err)
		return gen.PatchReleaseBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.PatchReleaseBinding200JSONResponse(toGenReleaseBinding(binding)), nil
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
	h.logger.Info("DeployRelease called",
		"namespaceName", request.NamespaceName,
		"projectName", request.ProjectName,
		"componentName", request.ComponentName)

	if request.Body == nil {
		return gen.DeployRelease400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	deployReq := toModelDeployReleaseRequest(request.Body)

	binding, err := h.services.ComponentService.DeployRelease(
		ctx,
		request.NamespaceName,
		request.ProjectName,
		request.ComponentName,
		deployReq,
	)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeployRelease403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			return gen.DeployRelease404JSONResponse{NotFoundJSONResponse: notFound("Project")}, nil
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			return gen.DeployRelease404JSONResponse{NotFoundJSONResponse: notFound("Component")}, nil
		}
		if errors.Is(err, services.ErrComponentReleaseNotFound) {
			return gen.DeployRelease404JSONResponse{NotFoundJSONResponse: notFound("ComponentRelease")}, nil
		}
		h.logger.Error("Failed to deploy release", "error", err)
		return gen.DeployRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.DeployRelease201JSONResponse(toGenReleaseBinding(binding)), nil
}

// PromoteComponent promotes a component release from one environment to another
func (h *Handler) PromoteComponent(
	ctx context.Context,
	request gen.PromoteComponentRequestObject,
) (gen.PromoteComponentResponseObject, error) {
	h.logger.Info("PromoteComponent called",
		"namespaceName", request.NamespaceName,
		"projectName", request.ProjectName,
		"componentName", request.ComponentName)

	if request.Body == nil {
		return gen.PromoteComponent400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	promoteReq := toModelPromoteComponentRequest(request.Body)

	payload := services.PromoteComponentPayload{
		PromoteComponentRequest: *promoteReq,
		NamespaceName:           request.NamespaceName,
		ProjectName:             request.ProjectName,
		ComponentName:           request.ComponentName,
	}

	binding, err := h.services.ComponentService.PromoteComponent(ctx, &payload)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.PromoteComponent403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrProjectNotFound) {
			return gen.PromoteComponent404JSONResponse{NotFoundJSONResponse: notFound("Project")}, nil
		}
		if errors.Is(err, services.ErrComponentNotFound) {
			return gen.PromoteComponent404JSONResponse{NotFoundJSONResponse: notFound("Component")}, nil
		}
		if errors.Is(err, services.ErrReleaseBindingNotFound) {
			return gen.PromoteComponent404JSONResponse{NotFoundJSONResponse: notFound("ReleaseBinding")}, nil
		}
		h.logger.Error("Failed to promote component", "error", err)
		return gen.PromoteComponent500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.PromoteComponent200JSONResponse(toGenReleaseBinding(binding)), nil
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

// Converter functions

// toGenComponentRelease converts models.ComponentReleaseResponse to gen.ComponentRelease
func toGenComponentRelease(r *models.ComponentReleaseResponse) gen.ComponentRelease {
	if r == nil {
		return gen.ComponentRelease{}
	}

	result := gen.ComponentRelease{
		Name:          r.Name,
		ComponentName: r.ComponentName,
		ProjectName:   r.ProjectName,
		NamespaceName: r.NamespaceName,
		CreatedAt:     r.CreatedAt,
	}

	if r.Status != "" {
		result.Status = &r.Status
	}

	return result
}

// toGenReleaseBinding converts models.ReleaseBindingResponse to gen.ReleaseBinding
func toGenReleaseBinding(r *models.ReleaseBindingResponse) gen.ReleaseBinding {
	if r == nil {
		return gen.ReleaseBinding{}
	}

	result := gen.ReleaseBinding{
		Name:          r.Name,
		ComponentName: r.ComponentName,
		ProjectName:   r.ProjectName,
		NamespaceName: r.NamespaceName,
		Environment:   r.Environment,
		CreatedAt:     r.CreatedAt,
	}

	if r.ReleaseName != "" {
		result.ReleaseName = &r.ReleaseName
	}

	if len(r.ComponentTypeEnvOverrides) > 0 {
		result.ComponentTypeEnvOverrides = &r.ComponentTypeEnvOverrides
	}

	if len(r.TraitOverrides) > 0 {
		result.TraitOverrides = &r.TraitOverrides
	}

	if r.WorkloadOverrides != nil {
		result.WorkloadOverrides = toGenWorkloadOverrides(r.WorkloadOverrides)
	}

	return result
}

// toGenWorkloadOverrides converts models.WorkloadOverrides to gen.WorkloadOverrides
func toGenWorkloadOverrides(w *models.WorkloadOverrides) *gen.WorkloadOverrides {
	if w == nil {
		return nil
	}

	result := &gen.WorkloadOverrides{}

	if len(w.Containers) > 0 {
		containers := make(map[string]gen.ContainerOverride, len(w.Containers))
		for name, c := range w.Containers {
			container := gen.ContainerOverride{}

			if len(c.Env) > 0 {
				envVars := make([]gen.EnvVar, 0, len(c.Env))
				for _, e := range c.Env {
					envVar := gen.EnvVar{
						Key: e.Key,
					}
					if e.Value != "" {
						envVar.Value = &e.Value
					}
					if e.ValueFrom != nil && e.ValueFrom.SecretRef != nil {
						envVar.ValueFrom = &gen.EnvVarValueFrom{
							SecretRef: &struct {
								Key  *string `json:"key,omitempty"`
								Name *string `json:"name,omitempty"`
							}{
								Key:  ptr.To(e.ValueFrom.SecretRef.Key),
								Name: ptr.To(e.ValueFrom.SecretRef.Name),
							},
						}
					}
					envVars = append(envVars, envVar)
				}
				container.Env = &envVars
			}

			if len(c.Files) > 0 {
				fileVars := make([]gen.FileVar, 0, len(c.Files))
				for _, f := range c.Files {
					fileVar := gen.FileVar{
						Key:       f.Key,
						MountPath: f.MountPath,
					}
					if f.Value != "" {
						fileVar.Value = &f.Value
					}
					if f.ValueFrom != nil && f.ValueFrom.SecretRef != nil {
						fileVar.ValueFrom = &gen.EnvVarValueFrom{
							SecretRef: &struct {
								Key  *string `json:"key,omitempty"`
								Name *string `json:"name,omitempty"`
							}{
								Key:  ptr.To(f.ValueFrom.SecretRef.Key),
								Name: ptr.To(f.ValueFrom.SecretRef.Name),
							},
						}
					}
					fileVars = append(fileVars, fileVar)
				}
				container.Files = &fileVars
			}

			containers[name] = container
		}
		result.Containers = &containers
	}

	return result
}

// toModelPatchReleaseBindingRequest converts gen.PatchReleaseBindingRequest to models.PatchReleaseBindingRequest
func toModelPatchReleaseBindingRequest(req *gen.PatchReleaseBindingRequest) *models.PatchReleaseBindingRequest {
	if req == nil {
		return nil
	}

	result := &models.PatchReleaseBindingRequest{}

	if req.ReleaseName != nil {
		result.ReleaseName = *req.ReleaseName
	}

	if req.ComponentTypeEnvOverrides != nil {
		result.ComponentTypeEnvOverrides = *req.ComponentTypeEnvOverrides
	}

	if req.TraitOverrides != nil {
		// Convert map[string]interface{} to map[string]map[string]interface{}
		traitOverrides := make(map[string]map[string]interface{})
		for traitName, traitData := range *req.TraitOverrides {
			if traitMap, ok := traitData.(map[string]interface{}); ok {
				traitOverrides[traitName] = traitMap
			}
		}
		result.TraitOverrides = traitOverrides
	}

	if req.WorkloadOverrides != nil {
		result.WorkloadOverrides = toModelWorkloadOverrides(req.WorkloadOverrides)
	}

	return result
}

// toModelWorkloadOverrides converts gen.WorkloadOverrides to models.WorkloadOverrides
func toModelWorkloadOverrides(w *gen.WorkloadOverrides) *models.WorkloadOverrides {
	if w == nil {
		return nil
	}

	result := &models.WorkloadOverrides{}

	if w.Containers != nil && len(*w.Containers) > 0 {
		containers := make(map[string]models.ContainerOverride, len(*w.Containers))
		for name, c := range *w.Containers {
			container := models.ContainerOverride{}

			if c.Env != nil && len(*c.Env) > 0 {
				envVars := make([]models.EnvVar, 0, len(*c.Env))
				for _, e := range *c.Env {
					envVar := models.EnvVar{
						Key: e.Key,
					}
					if e.Value != nil {
						envVar.Value = *e.Value
					}
					if e.ValueFrom != nil && e.ValueFrom.SecretRef != nil {
						envVar.ValueFrom = &models.EnvVarValueFrom{
							SecretRef: &models.SecretKeyRef{
								Key:  *e.ValueFrom.SecretRef.Key,
								Name: *e.ValueFrom.SecretRef.Name,
							},
						}
					}
					envVars = append(envVars, envVar)
				}
				container.Env = envVars
			}

			if c.Files != nil && len(*c.Files) > 0 {
				fileVars := make([]models.FileVar, 0, len(*c.Files))
				for _, f := range *c.Files {
					fileVar := models.FileVar{
						Key:       f.Key,
						MountPath: f.MountPath,
					}
					if f.Value != nil {
						fileVar.Value = *f.Value
					}
					if f.ValueFrom != nil && f.ValueFrom.SecretRef != nil {
						fileVar.ValueFrom = &models.EnvVarValueFrom{
							SecretRef: &models.SecretKeyRef{
								Key:  *f.ValueFrom.SecretRef.Key,
								Name: *f.ValueFrom.SecretRef.Name,
							},
						}
					}
					fileVars = append(fileVars, fileVar)
				}
				container.Files = fileVars
			}

			containers[name] = container
		}
		result.Containers = containers
	}

	return result
}

// toModelDeployReleaseRequest converts gen.DeployReleaseRequest to models.DeployReleaseRequest
func toModelDeployReleaseRequest(req *gen.DeployReleaseRequest) *models.DeployReleaseRequest {
	if req == nil {
		return nil
	}

	return &models.DeployReleaseRequest{
		ReleaseName: req.ReleaseName,
	}
}

// toModelPromoteComponentRequest converts gen.PromoteComponentRequest to models.PromoteComponentRequest
func toModelPromoteComponentRequest(req *gen.PromoteComponentRequest) *models.PromoteComponentRequest {
	if req == nil {
		return nil
	}

	return &models.PromoteComponentRequest{
		SourceEnvironment: req.SourceEnv,
		TargetEnvironment: req.TargetEnv,
	}
}
