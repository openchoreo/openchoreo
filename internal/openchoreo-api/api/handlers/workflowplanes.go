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
	workflowplanesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowplane"
)

// ListWorkflowPlanes returns a paginated list of workflow planes within a namespace.
func (h *Handler) ListWorkflowPlanes(
	ctx context.Context,
	request gen.ListWorkflowPlanesRequestObject,
) (gen.ListWorkflowPlanesResponseObject, error) {
	h.logger.Debug("ListWorkflowPlanes called", "namespaceName", request.NamespaceName)

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor, request.Params.LabelSelector)

	result, err := h.services.WorkflowPlaneService.ListWorkflowPlanes(ctx, request.NamespaceName, opts)
	if err != nil {
		h.logger.Error("Failed to list workflow planes", "error", err)
		return gen.ListWorkflowPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.WorkflowPlane, gen.WorkflowPlane](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert workflow planes", "error", err)
		return gen.ListWorkflowPlanes500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListWorkflowPlanes200JSONResponse{
		Items:      items,
		Pagination: ToPagination(result),
	}, nil
}

// GetWorkflowPlane returns details of a specific workflow plane.
func (h *Handler) GetWorkflowPlane(
	ctx context.Context,
	request gen.GetWorkflowPlaneRequestObject,
) (gen.GetWorkflowPlaneResponseObject, error) {
	h.logger.Debug("GetWorkflowPlane called", "namespaceName", request.NamespaceName, "workflowPlaneName", request.WorkflowPlaneName)

	workflowPlane, err := h.services.WorkflowPlaneService.GetWorkflowPlane(ctx, request.NamespaceName, request.WorkflowPlaneName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetWorkflowPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, workflowplanesvc.ErrWorkflowPlaneNotFound) {
			return gen.GetWorkflowPlane404JSONResponse{NotFoundJSONResponse: notFound("WorkflowPlane")}, nil
		}
		h.logger.Error("Failed to get workflow plane", "error", err)
		return gen.GetWorkflowPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genWorkflowPlane, err := convert[openchoreov1alpha1.WorkflowPlane, gen.WorkflowPlane](*workflowPlane)
	if err != nil {
		h.logger.Error("Failed to convert workflow plane", "error", err)
		return gen.GetWorkflowPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetWorkflowPlane200JSONResponse(genWorkflowPlane), nil
}

// CreateWorkflowPlane creates a new workflow plane within a namespace.
func (h *Handler) CreateWorkflowPlane(
	ctx context.Context,
	request gen.CreateWorkflowPlaneRequestObject,
) (gen.CreateWorkflowPlaneResponseObject, error) {
	h.logger.Info("CreateWorkflowPlane called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateWorkflowPlane400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	bpCR, err := convert[gen.WorkflowPlane, openchoreov1alpha1.WorkflowPlane](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateWorkflowPlane400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	created, err := h.services.WorkflowPlaneService.CreateWorkflowPlane(ctx, request.NamespaceName, &bpCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateWorkflowPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, workflowplanesvc.ErrWorkflowPlaneAlreadyExists) {
			return gen.CreateWorkflowPlane409JSONResponse{ConflictJSONResponse: conflict("WorkflowPlane already exists")}, nil
		}
		if validationErr, ok := errors.AsType[*services.ValidationError](err); ok {
			if validationErr.StatusCode == http.StatusUnprocessableEntity {
				return gen.CreateWorkflowPlane422JSONResponse{UnprocessableContentJSONResponse: unprocessableContent(validationErr.Msg)}, nil
			}
			return gen.CreateWorkflowPlane400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to create workflow plane", "error", err)
		return gen.CreateWorkflowPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genBP, err := convert[openchoreov1alpha1.WorkflowPlane, gen.WorkflowPlane](*created)
	if err != nil {
		h.logger.Error("Failed to convert created workflow plane", "error", err)
		return gen.CreateWorkflowPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("WorkflowPlane created successfully", "namespaceName", request.NamespaceName, "workflowPlane", created.Name)
	return gen.CreateWorkflowPlane201JSONResponse(genBP), nil
}

// UpdateWorkflowPlane replaces an existing workflow plane.
func (h *Handler) UpdateWorkflowPlane(
	ctx context.Context,
	request gen.UpdateWorkflowPlaneRequestObject,
) (gen.UpdateWorkflowPlaneResponseObject, error) {
	h.logger.Info("UpdateWorkflowPlane called", "namespaceName", request.NamespaceName, "workflowPlaneName", request.WorkflowPlaneName)

	if request.Body == nil {
		return gen.UpdateWorkflowPlane400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	bpCR, err := convert[gen.WorkflowPlane, openchoreov1alpha1.WorkflowPlane](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateWorkflowPlane400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	// Ensure the name from the URL path is used
	bpCR.Name = request.WorkflowPlaneName

	updated, err := h.services.WorkflowPlaneService.UpdateWorkflowPlane(ctx, request.NamespaceName, &bpCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateWorkflowPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, workflowplanesvc.ErrWorkflowPlaneNotFound) {
			return gen.UpdateWorkflowPlane404JSONResponse{NotFoundJSONResponse: notFound("WorkflowPlane")}, nil
		}
		if validationErr, ok := errors.AsType[*services.ValidationError](err); ok {
			if validationErr.StatusCode == http.StatusUnprocessableEntity {
				return gen.UpdateWorkflowPlane422JSONResponse{UnprocessableContentJSONResponse: unprocessableContent(validationErr.Msg)}, nil
			}
			return gen.UpdateWorkflowPlane400JSONResponse{BadRequestJSONResponse: badRequest(validationErr.Msg)}, nil
		}
		h.logger.Error("Failed to update workflow plane", "error", err)
		return gen.UpdateWorkflowPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genBP, err := convert[openchoreov1alpha1.WorkflowPlane, gen.WorkflowPlane](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated workflow plane", "error", err)
		return gen.UpdateWorkflowPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("WorkflowPlane updated successfully", "namespaceName", request.NamespaceName, "workflowPlane", updated.Name)
	return gen.UpdateWorkflowPlane200JSONResponse(genBP), nil
}

// DeleteWorkflowPlane deletes a workflow plane by name.
func (h *Handler) DeleteWorkflowPlane(
	ctx context.Context,
	request gen.DeleteWorkflowPlaneRequestObject,
) (gen.DeleteWorkflowPlaneResponseObject, error) {
	h.logger.Info("DeleteWorkflowPlane called", "namespaceName", request.NamespaceName, "workflowPlaneName", request.WorkflowPlaneName)

	err := h.services.WorkflowPlaneService.DeleteWorkflowPlane(ctx, request.NamespaceName, request.WorkflowPlaneName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteWorkflowPlane403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, workflowplanesvc.ErrWorkflowPlaneNotFound) {
			return gen.DeleteWorkflowPlane404JSONResponse{NotFoundJSONResponse: notFound("WorkflowPlane")}, nil
		}
		h.logger.Error("Failed to delete workflow plane", "error", err)
		return gen.DeleteWorkflowPlane500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("WorkflowPlane deleted successfully", "namespaceName", request.NamespaceName, "workflowPlane", request.WorkflowPlaneName)
	return gen.DeleteWorkflowPlane204Response{}, nil
}
