// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	workloadsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workload"
)

// ListWorkloads returns a paginated list of workloads within a namespace.
func (h *Handler) ListWorkloads(
	ctx context.Context,
	request gen.ListWorkloadsRequestObject,
) (gen.ListWorkloadsResponseObject, error) {
	h.logger.Debug("ListWorkloads called", "namespaceName", request.NamespaceName)

	componentName := ""
	if request.Params.Component != nil {
		componentName = *request.Params.Component
	}

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.services.WorkloadService.ListWorkloads(ctx, request.NamespaceName, componentName, opts)
	if err != nil {
		h.logger.Error("Failed to list workloads", "error", err)
		return gen.ListWorkloads500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.Workload, gen.Workload](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert workloads", "error", err)
		return gen.ListWorkloads500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListWorkloads200JSONResponse{
		Items:      items,
		Pagination: ToPaginationPtr(result),
	}, nil
}

// CreateWorkload creates a new workload within a namespace.
func (h *Handler) CreateWorkload(
	ctx context.Context,
	request gen.CreateWorkloadRequestObject,
) (gen.CreateWorkloadResponseObject, error) {
	h.logger.Info("CreateWorkload called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateWorkload400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	wCR, err := convert[gen.Workload, openchoreov1alpha1.Workload](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateWorkload400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	wCR.Status = openchoreov1alpha1.WorkloadStatus{}

	created, err := h.services.WorkloadService.CreateWorkload(ctx, request.NamespaceName, &wCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateWorkload403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, workloadsvc.ErrWorkloadAlreadyExists) {
			return gen.CreateWorkload409JSONResponse{ConflictJSONResponse: conflict("Workload already exists")}, nil
		}
		if errors.Is(err, workloadsvc.ErrComponentNotFound) {
			return gen.CreateWorkload400JSONResponse{BadRequestJSONResponse: badRequest("Referenced component not found")}, nil
		}
		h.logger.Error("Failed to create workload", "error", err)
		return gen.CreateWorkload500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genWorkload, err := convert[openchoreov1alpha1.Workload, gen.Workload](*created)
	if err != nil {
		h.logger.Error("Failed to convert created workload", "error", err)
		return gen.CreateWorkload500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Workload created successfully", "namespaceName", request.NamespaceName, "workload", created.Name)
	return gen.CreateWorkload201JSONResponse(genWorkload), nil
}

// GetWorkload returns details of a specific workload.
func (h *Handler) GetWorkload(
	ctx context.Context,
	request gen.GetWorkloadRequestObject,
) (gen.GetWorkloadResponseObject, error) {
	h.logger.Debug("GetWorkload called", "namespaceName", request.NamespaceName, "workloadName", request.WorkloadName)

	w, err := h.services.WorkloadService.GetWorkload(ctx, request.NamespaceName, request.WorkloadName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetWorkload403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, workloadsvc.ErrWorkloadNotFound) {
			return gen.GetWorkload404JSONResponse{NotFoundJSONResponse: notFound("Workload")}, nil
		}
		h.logger.Error("Failed to get workload", "error", err)
		return gen.GetWorkload500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genWorkload, err := convert[openchoreov1alpha1.Workload, gen.Workload](*w)
	if err != nil {
		h.logger.Error("Failed to convert workload", "error", err)
		return gen.GetWorkload500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetWorkload200JSONResponse(genWorkload), nil
}

// UpdateWorkload replaces an existing workload (full update).
func (h *Handler) UpdateWorkload(
	ctx context.Context,
	request gen.UpdateWorkloadRequestObject,
) (gen.UpdateWorkloadResponseObject, error) {
	h.logger.Info("UpdateWorkload called", "namespaceName", request.NamespaceName, "workloadName", request.WorkloadName)

	if request.Body == nil {
		return gen.UpdateWorkload400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	wCR, err := convert[gen.Workload, openchoreov1alpha1.Workload](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateWorkload400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	wCR.Status = openchoreov1alpha1.WorkloadStatus{}

	// Ensure the name from the URL path is used
	wCR.Name = request.WorkloadName

	updated, err := h.services.WorkloadService.UpdateWorkload(ctx, request.NamespaceName, &wCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateWorkload403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, workloadsvc.ErrWorkloadNotFound) {
			return gen.UpdateWorkload404JSONResponse{NotFoundJSONResponse: notFound("Workload")}, nil
		}
		h.logger.Error("Failed to update workload", "error", err)
		return gen.UpdateWorkload500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genWorkload, err := convert[openchoreov1alpha1.Workload, gen.Workload](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated workload", "error", err)
		return gen.UpdateWorkload500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Workload updated successfully", "namespaceName", request.NamespaceName, "workload", updated.Name)
	return gen.UpdateWorkload200JSONResponse(genWorkload), nil
}

// DeleteWorkload deletes a workload by name.
func (h *Handler) DeleteWorkload(
	ctx context.Context,
	request gen.DeleteWorkloadRequestObject,
) (gen.DeleteWorkloadResponseObject, error) {
	h.logger.Info("DeleteWorkload called", "namespaceName", request.NamespaceName, "workloadName", request.WorkloadName)

	err := h.services.WorkloadService.DeleteWorkload(ctx, request.NamespaceName, request.WorkloadName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteWorkload403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, workloadsvc.ErrWorkloadNotFound) {
			return gen.DeleteWorkload404JSONResponse{NotFoundJSONResponse: notFound("Workload")}, nil
		}
		h.logger.Error("Failed to delete workload", "error", err)
		return gen.DeleteWorkload500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Workload deleted successfully", "namespaceName", request.NamespaceName, "workload", request.WorkloadName)
	return gen.DeleteWorkload204Response{}, nil
}
