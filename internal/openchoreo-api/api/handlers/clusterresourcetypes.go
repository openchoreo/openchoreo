// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	clusterresourcetypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterresourcetype"
)

// ListClusterResourceTypes returns a paginated list of cluster-scoped resource types.
func (h *Handler) ListClusterResourceTypes(
	ctx context.Context,
	request gen.ListClusterResourceTypesRequestObject,
) (gen.ListClusterResourceTypesResponseObject, error) {
	h.logger.Debug("ListClusterResourceTypes called")

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.ClusterResourceTypeService.ListClusterResourceTypes(ctx, opts)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListClusterResourceTypes403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.ListClusterResourceTypes400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to list cluster resource types", "error", err)
		return gen.ListClusterResourceTypes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ClusterResourceType, gen.ClusterResourceType](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert cluster resource types", "error", err)
		return gen.ListClusterResourceTypes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListClusterResourceTypes200JSONResponse{
		Items:      items,
		Pagination: ToPagination(result),
	}, nil
}

// CreateClusterResourceType creates a new cluster-scoped resource type.
func (h *Handler) CreateClusterResourceType(
	ctx context.Context,
	request gen.CreateClusterResourceTypeRequestObject,
) (gen.CreateClusterResourceTypeResponseObject, error) {
	h.logger.Info("CreateClusterResourceType called")

	if request.Body == nil {
		return gen.CreateClusterResourceType400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	crtCR, err := convert[gen.ClusterResourceType, openchoreov1alpha1.ClusterResourceType](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateClusterResourceType400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	created, err := h.services.ClusterResourceTypeService.CreateClusterResourceType(ctx, &crtCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateClusterResourceType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterresourcetypesvc.ErrClusterResourceTypeAlreadyExists) {
			return gen.CreateClusterResourceType409JSONResponse{ConflictJSONResponse: conflict("Cluster resource type already exists")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.CreateClusterResourceType400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to create cluster resource type", "error", err)
		return gen.CreateClusterResourceType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCRT, err := convert[openchoreov1alpha1.ClusterResourceType, gen.ClusterResourceType](*created)
	if err != nil {
		h.logger.Error("Failed to convert created cluster resource type", "error", err)
		return gen.CreateClusterResourceType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster resource type created successfully", "clusterResourceType", created.Name)
	return gen.CreateClusterResourceType201JSONResponse(genCRT), nil
}

// UpdateClusterResourceType replaces an existing cluster-scoped resource type (full update).
func (h *Handler) UpdateClusterResourceType(
	ctx context.Context,
	request gen.UpdateClusterResourceTypeRequestObject,
) (gen.UpdateClusterResourceTypeResponseObject, error) {
	h.logger.Info("UpdateClusterResourceType called", "crtName", request.CrtName)

	if request.Body == nil {
		return gen.UpdateClusterResourceType400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	crtCR, err := convert[gen.ClusterResourceType, openchoreov1alpha1.ClusterResourceType](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateClusterResourceType400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	crtCR.Name = request.CrtName

	updated, err := h.services.ClusterResourceTypeService.UpdateClusterResourceType(ctx, &crtCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateClusterResourceType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterresourcetypesvc.ErrClusterResourceTypeNotFound) {
			return gen.UpdateClusterResourceType404JSONResponse{NotFoundJSONResponse: notFound("ClusterResourceType")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.UpdateClusterResourceType400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to update cluster resource type", "error", err)
		return gen.UpdateClusterResourceType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCRT, err := convert[openchoreov1alpha1.ClusterResourceType, gen.ClusterResourceType](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated cluster resource type", "error", err)
		return gen.UpdateClusterResourceType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster resource type updated successfully", "clusterResourceType", updated.Name)
	return gen.UpdateClusterResourceType200JSONResponse(genCRT), nil
}

// GetClusterResourceType returns details of a specific cluster-scoped resource type.
func (h *Handler) GetClusterResourceType(
	ctx context.Context,
	request gen.GetClusterResourceTypeRequestObject,
) (gen.GetClusterResourceTypeResponseObject, error) {
	h.logger.Debug("GetClusterResourceType called", "crtName", request.CrtName)

	crt, err := h.services.ClusterResourceTypeService.GetClusterResourceType(ctx, request.CrtName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetClusterResourceType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterresourcetypesvc.ErrClusterResourceTypeNotFound) {
			return gen.GetClusterResourceType404JSONResponse{NotFoundJSONResponse: notFound("ClusterResourceType")}, nil
		}
		h.logger.Error("Failed to get cluster resource type", "error", err)
		return gen.GetClusterResourceType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCRT, err := convert[openchoreov1alpha1.ClusterResourceType, gen.ClusterResourceType](*crt)
	if err != nil {
		h.logger.Error("Failed to convert cluster resource type", "error", err)
		return gen.GetClusterResourceType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetClusterResourceType200JSONResponse(genCRT), nil
}

// DeleteClusterResourceType deletes a cluster-scoped resource type by name.
func (h *Handler) DeleteClusterResourceType(
	ctx context.Context,
	request gen.DeleteClusterResourceTypeRequestObject,
) (gen.DeleteClusterResourceTypeResponseObject, error) {
	h.logger.Info("DeleteClusterResourceType called", "crtName", request.CrtName)

	err := h.services.ClusterResourceTypeService.DeleteClusterResourceType(ctx, request.CrtName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteClusterResourceType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterresourcetypesvc.ErrClusterResourceTypeNotFound) {
			return gen.DeleteClusterResourceType404JSONResponse{NotFoundJSONResponse: notFound("ClusterResourceType")}, nil
		}
		h.logger.Error("Failed to delete cluster resource type", "error", err)
		return gen.DeleteClusterResourceType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ClusterResourceType deleted successfully", "clusterResourceType", request.CrtName)
	return gen.DeleteClusterResourceType204Response{}, nil
}

// GetClusterResourceTypeSchema returns the parameter schema for a cluster-scoped resource type.
func (h *Handler) GetClusterResourceTypeSchema(
	ctx context.Context,
	request gen.GetClusterResourceTypeSchemaRequestObject,
) (gen.GetClusterResourceTypeSchemaResponseObject, error) {
	h.logger.Debug("GetClusterResourceTypeSchema called", "name", request.CrtName)

	rawSchema, err := h.services.ClusterResourceTypeService.GetClusterResourceTypeSchema(ctx, request.CrtName)
	if err != nil {
		if errors.Is(err, clusterresourcetypesvc.ErrClusterResourceTypeNotFound) {
			return gen.GetClusterResourceTypeSchema404JSONResponse{NotFoundJSONResponse: notFound("ClusterResourceType")}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetClusterResourceTypeSchema403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to get cluster resource type schema", "error", err)
		return gen.GetClusterResourceTypeSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetClusterResourceTypeSchema200JSONResponse(rawSchema), nil
}
