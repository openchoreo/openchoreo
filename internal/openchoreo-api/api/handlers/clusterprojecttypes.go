// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	clusterprojecttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterprojecttype"
)

// ListClusterProjectTypes returns a paginated list of cluster-scoped project types.
func (h *Handler) ListClusterProjectTypes(
	ctx context.Context,
	request gen.ListClusterProjectTypesRequestObject,
) (gen.ListClusterProjectTypesResponseObject, error) {
	h.logger.Debug("ListClusterProjectTypes called")

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.ClusterProjectTypeService.ListClusterProjectTypes(ctx, opts)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListClusterProjectTypes403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.ListClusterProjectTypes400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to list cluster project types", "error", err)
		return gen.ListClusterProjectTypes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ClusterProjectType, gen.ClusterProjectType](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert cluster project types", "error", err)
		return gen.ListClusterProjectTypes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListClusterProjectTypes200JSONResponse{
		Items:      items,
		Pagination: ToPagination(result),
	}, nil
}

// CreateClusterProjectType creates a new cluster-scoped project type.
func (h *Handler) CreateClusterProjectType(
	ctx context.Context,
	request gen.CreateClusterProjectTypeRequestObject,
) (gen.CreateClusterProjectTypeResponseObject, error) {
	h.logger.Info("CreateClusterProjectType called")

	if request.Body == nil {
		return gen.CreateClusterProjectType400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	cptCR, err := convert[gen.ClusterProjectType, openchoreov1alpha1.ClusterProjectType](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateClusterProjectType400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	created, err := h.services.ClusterProjectTypeService.CreateClusterProjectType(ctx, &cptCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateClusterProjectType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterprojecttypesvc.ErrClusterProjectTypeAlreadyExists) {
			return gen.CreateClusterProjectType409JSONResponse{ConflictJSONResponse: conflict("Cluster project type already exists")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.CreateClusterProjectType400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to create cluster project type", "error", err)
		return gen.CreateClusterProjectType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCPT, err := convert[openchoreov1alpha1.ClusterProjectType, gen.ClusterProjectType](*created)
	if err != nil {
		h.logger.Error("Failed to convert created cluster project type", "error", err)
		return gen.CreateClusterProjectType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster project type created successfully", "clusterProjectType", created.Name)
	return gen.CreateClusterProjectType201JSONResponse(genCPT), nil
}

// UpdateClusterProjectType replaces an existing cluster-scoped project type (full update).
func (h *Handler) UpdateClusterProjectType(
	ctx context.Context,
	request gen.UpdateClusterProjectTypeRequestObject,
) (gen.UpdateClusterProjectTypeResponseObject, error) {
	h.logger.Info("UpdateClusterProjectType called", "cptName", request.CptName)

	if request.Body == nil {
		return gen.UpdateClusterProjectType400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	cptCR, err := convert[gen.ClusterProjectType, openchoreov1alpha1.ClusterProjectType](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateClusterProjectType400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	cptCR.Name = request.CptName

	updated, err := h.services.ClusterProjectTypeService.UpdateClusterProjectType(ctx, &cptCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateClusterProjectType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterprojecttypesvc.ErrClusterProjectTypeNotFound) {
			return gen.UpdateClusterProjectType404JSONResponse{NotFoundJSONResponse: notFound("ClusterProjectType")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.UpdateClusterProjectType400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to update cluster project type", "error", err)
		return gen.UpdateClusterProjectType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCPT, err := convert[openchoreov1alpha1.ClusterProjectType, gen.ClusterProjectType](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated cluster project type", "error", err)
		return gen.UpdateClusterProjectType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster project type updated successfully", "clusterProjectType", updated.Name)
	return gen.UpdateClusterProjectType200JSONResponse(genCPT), nil
}

// GetClusterProjectType returns details of a specific cluster-scoped project type.
func (h *Handler) GetClusterProjectType(
	ctx context.Context,
	request gen.GetClusterProjectTypeRequestObject,
) (gen.GetClusterProjectTypeResponseObject, error) {
	h.logger.Debug("GetClusterProjectType called", "cptName", request.CptName)

	cpt, err := h.services.ClusterProjectTypeService.GetClusterProjectType(ctx, request.CptName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetClusterProjectType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterprojecttypesvc.ErrClusterProjectTypeNotFound) {
			return gen.GetClusterProjectType404JSONResponse{NotFoundJSONResponse: notFound("ClusterProjectType")}, nil
		}
		h.logger.Error("Failed to get cluster project type", "error", err)
		return gen.GetClusterProjectType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCPT, err := convert[openchoreov1alpha1.ClusterProjectType, gen.ClusterProjectType](*cpt)
	if err != nil {
		h.logger.Error("Failed to convert cluster project type", "error", err)
		return gen.GetClusterProjectType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetClusterProjectType200JSONResponse(genCPT), nil
}

// DeleteClusterProjectType deletes a cluster-scoped project type by name.
func (h *Handler) DeleteClusterProjectType(
	ctx context.Context,
	request gen.DeleteClusterProjectTypeRequestObject,
) (gen.DeleteClusterProjectTypeResponseObject, error) {
	h.logger.Info("DeleteClusterProjectType called", "cptName", request.CptName)

	err := h.services.ClusterProjectTypeService.DeleteClusterProjectType(ctx, request.CptName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteClusterProjectType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterprojecttypesvc.ErrClusterProjectTypeNotFound) {
			return gen.DeleteClusterProjectType404JSONResponse{NotFoundJSONResponse: notFound("ClusterProjectType")}, nil
		}
		h.logger.Error("Failed to delete cluster project type", "error", err)
		return gen.DeleteClusterProjectType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ClusterProjectType deleted successfully", "clusterProjectType", request.CptName)
	return gen.DeleteClusterProjectType204Response{}, nil
}

// GetClusterProjectTypeSchema returns the parameter schema for a cluster-scoped project type.
func (h *Handler) GetClusterProjectTypeSchema(
	ctx context.Context,
	request gen.GetClusterProjectTypeSchemaRequestObject,
) (gen.GetClusterProjectTypeSchemaResponseObject, error) {
	h.logger.Debug("GetClusterProjectTypeSchema called", "name", request.CptName)

	rawSchema, err := h.services.ClusterProjectTypeService.GetClusterProjectTypeSchema(ctx, request.CptName)
	if err != nil {
		if errors.Is(err, clusterprojecttypesvc.ErrClusterProjectTypeNotFound) {
			return gen.GetClusterProjectTypeSchema404JSONResponse{NotFoundJSONResponse: notFound("ClusterProjectType")}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetClusterProjectTypeSchema403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to get cluster project type schema", "error", err)
		return gen.GetClusterProjectTypeSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetClusterProjectTypeSchema200JSONResponse(rawSchema), nil
}
