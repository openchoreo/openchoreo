// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	clusterendpointtypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterendpointtype"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// ListClusterEndpointTypes returns a paginated list of cluster-scoped endpoint types.
func (h *Handler) ListClusterEndpointTypes(
	ctx context.Context,
	request gen.ListClusterEndpointTypesRequestObject,
) (gen.ListClusterEndpointTypesResponseObject, error) {
	h.logger.Debug("ListClusterEndpointTypes called")

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.ClusterEndpointTypeService.ListClusterEndpointTypes(ctx, opts)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListClusterEndpointTypes403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.ListClusterEndpointTypes400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to list cluster endpoint types", "error", err)
		return gen.ListClusterEndpointTypes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ClusterEndpointType, gen.ClusterEndpointType](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert cluster endpoint types", "error", err)
		return gen.ListClusterEndpointTypes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListClusterEndpointTypes200JSONResponse{
		Items:      items,
		Pagination: ToPagination(result),
	}, nil
}

// CreateClusterEndpointType creates a new cluster-scoped endpoint type.
func (h *Handler) CreateClusterEndpointType(
	ctx context.Context,
	request gen.CreateClusterEndpointTypeRequestObject,
) (gen.CreateClusterEndpointTypeResponseObject, error) {
	h.logger.Info("CreateClusterEndpointType called")

	if request.Body == nil {
		return gen.CreateClusterEndpointType400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	crtCR, err := convert[gen.ClusterEndpointType, openchoreov1alpha1.ClusterEndpointType](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateClusterEndpointType400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	created, err := h.services.ClusterEndpointTypeService.CreateClusterEndpointType(ctx, &crtCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateClusterEndpointType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterendpointtypesvc.ErrClusterEndpointTypeAlreadyExists) {
			return gen.CreateClusterEndpointType409JSONResponse{ConflictJSONResponse: conflict("Cluster endpoint type already exists")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.CreateClusterEndpointType400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to create cluster endpoint type", "error", err)
		return gen.CreateClusterEndpointType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	audit.SetResource(ctx, &audit.Resource{UID: string(created.UID), Name: created.Name})

	genCRT, err := convert[openchoreov1alpha1.ClusterEndpointType, gen.ClusterEndpointType](*created)
	if err != nil {
		h.logger.Error("Failed to convert created cluster endpoint type", "error", err)
		return gen.CreateClusterEndpointType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster endpoint type created successfully", "clusterEndpointType", created.Name)
	return gen.CreateClusterEndpointType201JSONResponse(genCRT), nil
}

// UpdateClusterEndpointType replaces an existing cluster-scoped endpoint type (full update).
func (h *Handler) UpdateClusterEndpointType(
	ctx context.Context,
	request gen.UpdateClusterEndpointTypeRequestObject,
) (gen.UpdateClusterEndpointTypeResponseObject, error) {
	h.logger.Info("UpdateClusterEndpointType called", "cetName", request.CetName)

	if request.Body == nil {
		return gen.UpdateClusterEndpointType400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	crtCR, err := convert[gen.ClusterEndpointType, openchoreov1alpha1.ClusterEndpointType](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateClusterEndpointType400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	crtCR.Name = request.CetName

	updated, err := h.services.ClusterEndpointTypeService.UpdateClusterEndpointType(ctx, &crtCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateClusterEndpointType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterendpointtypesvc.ErrClusterEndpointTypeNotFound) {
			return gen.UpdateClusterEndpointType404JSONResponse{NotFoundJSONResponse: notFound("ClusterEndpointType")}, nil
		}
		var validationErr *services.ValidationError
		if errors.As(err, &validationErr) {
			return gen.UpdateClusterEndpointType400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to update cluster endpoint type", "error", err)
		return gen.UpdateClusterEndpointType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	audit.SetResource(ctx, &audit.Resource{UID: string(updated.UID), Name: updated.Name})

	genCRT, err := convert[openchoreov1alpha1.ClusterEndpointType, gen.ClusterEndpointType](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated cluster endpoint type", "error", err)
		return gen.UpdateClusterEndpointType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster endpoint type updated successfully", "clusterEndpointType", updated.Name)
	return gen.UpdateClusterEndpointType200JSONResponse(genCRT), nil
}

// GetClusterEndpointType returns details of a specific cluster-scoped endpoint type.
func (h *Handler) GetClusterEndpointType(
	ctx context.Context,
	request gen.GetClusterEndpointTypeRequestObject,
) (gen.GetClusterEndpointTypeResponseObject, error) {
	h.logger.Debug("GetClusterEndpointType called", "cetName", request.CetName)

	crt, err := h.services.ClusterEndpointTypeService.GetClusterEndpointType(ctx, request.CetName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetClusterEndpointType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterendpointtypesvc.ErrClusterEndpointTypeNotFound) {
			return gen.GetClusterEndpointType404JSONResponse{NotFoundJSONResponse: notFound("ClusterEndpointType")}, nil
		}
		h.logger.Error("Failed to get cluster endpoint type", "error", err)
		return gen.GetClusterEndpointType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCRT, err := convert[openchoreov1alpha1.ClusterEndpointType, gen.ClusterEndpointType](*crt)
	if err != nil {
		h.logger.Error("Failed to convert cluster endpoint type", "error", err)
		return gen.GetClusterEndpointType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetClusterEndpointType200JSONResponse(genCRT), nil
}

// DeleteClusterEndpointType deletes a cluster-scoped endpoint type by name.
func (h *Handler) DeleteClusterEndpointType(
	ctx context.Context,
	request gen.DeleteClusterEndpointTypeRequestObject,
) (gen.DeleteClusterEndpointTypeResponseObject, error) {
	h.logger.Info("DeleteClusterEndpointType called", "cetName", request.CetName)

	err := h.services.ClusterEndpointTypeService.DeleteClusterEndpointType(ctx, request.CetName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteClusterEndpointType403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterendpointtypesvc.ErrClusterEndpointTypeNotFound) {
			return gen.DeleteClusterEndpointType404JSONResponse{NotFoundJSONResponse: notFound("ClusterEndpointType")}, nil
		}
		h.logger.Error("Failed to delete cluster endpoint type", "error", err)
		return gen.DeleteClusterEndpointType500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ClusterEndpointType deleted successfully", "clusterEndpointType", request.CetName)
	return gen.DeleteClusterEndpointType204Response{}, nil
}

// GetClusterEndpointTypeSchema returns the parameter schema for a cluster-scoped endpoint type.
func (h *Handler) GetClusterEndpointTypeSchema(
	ctx context.Context,
	request gen.GetClusterEndpointTypeSchemaRequestObject,
) (gen.GetClusterEndpointTypeSchemaResponseObject, error) {
	h.logger.Debug("GetClusterEndpointTypeSchema called", "name", request.CetName)

	rawSchema, err := h.services.ClusterEndpointTypeService.GetClusterEndpointTypeSchema(ctx, request.CetName)
	if err != nil {
		if errors.Is(err, clusterendpointtypesvc.ErrClusterEndpointTypeNotFound) {
			return gen.GetClusterEndpointTypeSchema404JSONResponse{NotFoundJSONResponse: notFound("ClusterEndpointType")}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetClusterEndpointTypeSchema403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to get cluster endpoint type schema", "error", err)
		return gen.GetClusterEndpointTypeSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetClusterEndpointTypeSchema200JSONResponse(rawSchema), nil
}
