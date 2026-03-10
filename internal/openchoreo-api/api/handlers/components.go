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

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.ComponentService.ListComponents(ctx, request.NamespaceName, projectName, opts)
	if err != nil {
		if errors.Is(err, projectsvc.ErrProjectNotFound) {
			return gen.ListComponents404JSONResponse{NotFoundJSONResponse: notFound("Project")}, nil
		}
		var validationErr *svcerrors.ValidationError
		if errors.As(err, &validationErr) {
			return gen.ListComponents400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
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
		Pagination: ToPagination(result),
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
	created, err := h.services.ComponentService.CreateComponent(ctx, request.NamespaceName, &componentCR)
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
		var validationErr *svcerrors.ValidationError
		if errors.As(err, &validationErr) {
			return gen.CreateComponent400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
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
		Name:           req.Name,
		DisplayName:    ptr.Deref(req.DisplayName, ""),
		Description:    ptr.Deref(req.Description, ""),
		ComponentType:  componentTypeRef,
		AutoDeploy:     req.AutoDeploy,
		Parameters:     mapToRawExtension(req.Parameters),
		Traits:         toModelTraits(req.Traits),
		WorkflowConfig: toModelWorkflowConfig(req.Workflow),
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

// toModelWorkflowConfig converts *gen.ComponentWorkflowInput to *models.WorkflowConfig
func toModelWorkflowConfig(workflow *gen.ComponentWorkflowInput) *models.WorkflowConfig {
	if workflow == nil {
		return nil
	}

	return &models.WorkflowConfig{
		Name:       workflow.Name,
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

	component, err := h.services.ComponentService.GetComponent(ctx, request.NamespaceName, request.ComponentName)
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
	componentCR.Name = request.ComponentName

	updated, err := h.services.ComponentService.UpdateComponent(ctx, request.NamespaceName, &componentCR)
	if err != nil {
		if errors.Is(err, svcerrors.ErrForbidden) {
			return gen.UpdateComponent403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, componentsvc.ErrComponentNotFound) {
			return gen.UpdateComponent404JSONResponse{NotFoundJSONResponse: notFound("Component")}, nil
		}
		var validationErr *svcerrors.ValidationError
		if errors.As(err, &validationErr) {
			return gen.UpdateComponent400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
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

	err := h.services.ComponentService.DeleteComponent(ctx, request.NamespaceName, request.ComponentName)
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
	h.logger.Debug("GetComponentSchema called", "namespaceName", request.NamespaceName, "componentName", request.ComponentName)

	schema, err := h.services.ComponentService.GetComponentSchema(ctx, request.NamespaceName, request.ComponentName)
	if err != nil {
		if errors.Is(err, svcerrors.ErrForbidden) {
			return gen.GetComponentSchema403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, componentsvc.ErrComponentNotFound) {
			return gen.GetComponentSchema404JSONResponse{NotFoundJSONResponse: notFound("Component")}, nil
		}
		if errors.Is(err, componentsvc.ErrComponentTypeNotFound) {
			return gen.GetComponentSchema404JSONResponse{NotFoundJSONResponse: notFound("ComponentType")}, nil
		}
		h.logger.Error("Failed to get component schema", "error", err)
		return gen.GetComponentSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genSchema, err := convert[any, gen.SchemaResponse](schema)
	if err != nil {
		h.logger.Error("Failed to convert schema response", "error", err)
		return gen.GetComponentSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetComponentSchema200JSONResponse(genSchema), nil
}

// GenerateRelease generates an immutable release snapshot from the current component state
func (h *Handler) GenerateRelease(
	ctx context.Context,
	request gen.GenerateReleaseRequestObject,
) (gen.GenerateReleaseResponseObject, error) {
	h.logger.Info("GenerateRelease called",
		"namespaceName", request.NamespaceName,
		"componentName", request.ComponentName)

	if request.Body == nil {
		return gen.GenerateRelease400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	releaseName := ""
	if request.Body.ReleaseName != nil {
		releaseName = *request.Body.ReleaseName
	}

	release, err := h.services.ComponentService.GenerateRelease(ctx, request.NamespaceName, request.ComponentName,
		&componentsvc.GenerateReleaseRequest{ReleaseName: releaseName})
	if err != nil {
		if errors.Is(err, svcerrors.ErrForbidden) {
			return gen.GenerateRelease403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, componentsvc.ErrComponentNotFound) {
			return gen.GenerateRelease404JSONResponse{NotFoundJSONResponse: notFound("Component")}, nil
		}
		if errors.Is(err, componentsvc.ErrWorkloadNotFound) {
			return gen.GenerateRelease404JSONResponse{NotFoundJSONResponse: notFound("Workload")}, nil
		}
		if errors.Is(err, componentsvc.ErrValidation) {
			return gen.GenerateRelease400JSONResponse{BadRequestJSONResponse: badRequest(err.Error())}, nil
		}
		if errors.Is(err, componentsvc.ErrComponentTypeNotFound) {
			return gen.GenerateRelease404JSONResponse{NotFoundJSONResponse: notFound("ComponentType")}, nil
		}
		if errors.Is(err, componentsvc.ErrTraitNotFound) {
			return gen.GenerateRelease404JSONResponse{NotFoundJSONResponse: notFound("Trait")}, nil
		}
		h.logger.Error("Failed to generate release", "error", err)
		return gen.GenerateRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRelease, err := convert[openchoreov1alpha1.ComponentRelease, gen.ComponentRelease](*release)
	if err != nil {
		h.logger.Error("Failed to convert component release", "error", err)
		return gen.GenerateRelease500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GenerateRelease201JSONResponse(genRelease), nil
}

// Converter functions
