// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"net/http"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	clusterhooksvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterhook"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// ListClusterHooks returns a paginated list of cluster-scoped hooks.
func (h *Handler) ListClusterHooks(
	ctx context.Context,
	request gen.ListClusterHooksRequestObject,
) (gen.ListClusterHooksResponseObject, error) {
	h.logger.Debug("ListClusterHooks called")

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.ClusterHookService.ListClusterHooks(ctx, opts)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListClusterHooks403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if validationErr, ok := errors.AsType[*services.ValidationError](err); ok {
			return gen.ListClusterHooks400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to list cluster hooks", "error", err)
		return gen.ListClusterHooks500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ClusterHook, gen.ClusterHook](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert cluster hooks", "error", err)
		return gen.ListClusterHooks500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListClusterHooks200JSONResponse{
		Items:      items,
		Pagination: ToPagination(result),
	}, nil
}

// CreateClusterHook creates a new cluster-scoped hook.
func (h *Handler) CreateClusterHook(
	ctx context.Context,
	request gen.CreateClusterHookRequestObject,
) (gen.CreateClusterHookResponseObject, error) {
	h.logger.Info("CreateClusterHook called")

	if request.Body == nil {
		return gen.CreateClusterHook400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	chCR, err := convert[gen.ClusterHook, openchoreov1alpha1.ClusterHook](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateClusterHook400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	created, err := h.services.ClusterHookService.CreateClusterHook(ctx, &chCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateClusterHook403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterhooksvc.ErrClusterHookAlreadyExists) {
			return gen.CreateClusterHook409JSONResponse{ConflictJSONResponse: conflict("Cluster hook already exists")}, nil
		}
		if validationErr, ok := errors.AsType[*services.ValidationError](err); ok {
			if validationErr.StatusCode == http.StatusUnprocessableEntity {
				return gen.CreateClusterHook422JSONResponse{UnprocessableContentJSONResponse: unprocessableContent(validationErr.Msg)}, nil
			}
			return gen.CreateClusterHook400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to create cluster hook", "error", err)
		return gen.CreateClusterHook500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	audit.SetResource(ctx, &audit.Resource{UID: string(created.UID), Name: created.Name})

	genCH, err := convert[openchoreov1alpha1.ClusterHook, gen.ClusterHook](*created)
	if err != nil {
		h.logger.Error("Failed to convert created cluster hook", "error", err)
		return gen.CreateClusterHook500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster hook created successfully", "clusterHook", created.Name)
	return gen.CreateClusterHook201JSONResponse(genCH), nil
}

// UpdateClusterHook replaces an existing cluster-scoped hook (full update).
func (h *Handler) UpdateClusterHook(
	ctx context.Context,
	request gen.UpdateClusterHookRequestObject,
) (gen.UpdateClusterHookResponseObject, error) {
	h.logger.Info("UpdateClusterHook called", "clusterHookName", request.ClusterHookName)

	if request.Body == nil {
		return gen.UpdateClusterHook400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	chCR, err := convert[gen.ClusterHook, openchoreov1alpha1.ClusterHook](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateClusterHook400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	// Ensure the name from the URL path is used
	chCR.Name = request.ClusterHookName

	updated, err := h.services.ClusterHookService.UpdateClusterHook(ctx, &chCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateClusterHook403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterhooksvc.ErrClusterHookNotFound) {
			return gen.UpdateClusterHook404JSONResponse{NotFoundJSONResponse: notFound("ClusterHook")}, nil
		}
		if validationErr, ok := errors.AsType[*services.ValidationError](err); ok {
			if validationErr.StatusCode == http.StatusUnprocessableEntity {
				return gen.UpdateClusterHook422JSONResponse{UnprocessableContentJSONResponse: unprocessableContent(validationErr.Msg)}, nil
			}
			return gen.UpdateClusterHook400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to update cluster hook", "error", err)
		return gen.UpdateClusterHook500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	audit.SetResource(ctx, &audit.Resource{UID: string(updated.UID), Name: updated.Name})

	genCH, err := convert[openchoreov1alpha1.ClusterHook, gen.ClusterHook](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated cluster hook", "error", err)
		return gen.UpdateClusterHook500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster hook updated successfully", "clusterHook", updated.Name)
	return gen.UpdateClusterHook200JSONResponse(genCH), nil
}

// GetClusterHook returns details of a specific cluster-scoped hook.
func (h *Handler) GetClusterHook(
	ctx context.Context,
	request gen.GetClusterHookRequestObject,
) (gen.GetClusterHookResponseObject, error) {
	h.logger.Debug("GetClusterHook called", "clusterHookName", request.ClusterHookName)

	hook, err := h.services.ClusterHookService.GetClusterHook(ctx, request.ClusterHookName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetClusterHook403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterhooksvc.ErrClusterHookNotFound) {
			return gen.GetClusterHook404JSONResponse{NotFoundJSONResponse: notFound("ClusterHook")}, nil
		}
		h.logger.Error("Failed to get cluster hook", "error", err)
		return gen.GetClusterHook500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genHook, err := convert[openchoreov1alpha1.ClusterHook, gen.ClusterHook](*hook)
	if err != nil {
		h.logger.Error("Failed to convert cluster hook", "error", err)
		return gen.GetClusterHook500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetClusterHook200JSONResponse(genHook), nil
}

// DeleteClusterHook deletes a cluster-scoped hook by name.
func (h *Handler) DeleteClusterHook(
	ctx context.Context,
	request gen.DeleteClusterHookRequestObject,
) (gen.DeleteClusterHookResponseObject, error) {
	h.logger.Info("DeleteClusterHook called", "clusterHookName", request.ClusterHookName)

	err := h.services.ClusterHookService.DeleteClusterHook(ctx, request.ClusterHookName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteClusterHook403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterhooksvc.ErrClusterHookNotFound) {
			return gen.DeleteClusterHook404JSONResponse{NotFoundJSONResponse: notFound("ClusterHook")}, nil
		}
		h.logger.Error("Failed to delete cluster hook", "error", err)
		return gen.DeleteClusterHook500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ClusterHook deleted successfully", "clusterHook", request.ClusterHookName)
	return gen.DeleteClusterHook204Response{}, nil
}
