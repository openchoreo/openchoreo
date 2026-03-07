// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	clusterworkflowsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterworkflow"
)

// ListClusterWorkflows returns a paginated list of cluster-scoped workflows.
func (h *Handler) ListClusterWorkflows(
	ctx context.Context,
	request gen.ListClusterWorkflowsRequestObject,
) (gen.ListClusterWorkflowsResponseObject, error) {
	h.logger.Debug("ListClusterWorkflows called")

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.services.ClusterWorkflowService.ListClusterWorkflows(ctx, opts)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListClusterWorkflows403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to list cluster workflows", "error", err)
		return gen.ListClusterWorkflows500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.ClusterWorkflow, gen.ClusterWorkflow](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert cluster workflows", "error", err)
		return gen.ListClusterWorkflows500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListClusterWorkflows200JSONResponse{
		Items:      items,
		Pagination: ToPagination(result),
	}, nil
}

// CreateClusterWorkflow creates a new cluster-scoped workflow.
func (h *Handler) CreateClusterWorkflow(
	ctx context.Context,
	request gen.CreateClusterWorkflowRequestObject,
) (gen.CreateClusterWorkflowResponseObject, error) {
	h.logger.Info("CreateClusterWorkflow called")

	if request.Body == nil {
		return gen.CreateClusterWorkflow400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	cwfCR, err := convert[gen.ClusterWorkflow, openchoreov1alpha1.ClusterWorkflow](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateClusterWorkflow400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	created, err := h.services.ClusterWorkflowService.CreateClusterWorkflow(ctx, &cwfCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateClusterWorkflow403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterworkflowsvc.ErrClusterWorkflowAlreadyExists) {
			return gen.CreateClusterWorkflow409JSONResponse{ConflictJSONResponse: conflict("Cluster workflow already exists")}, nil
		}
		h.logger.Error("Failed to create cluster workflow", "error", err)
		return gen.CreateClusterWorkflow500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCWF, err := convert[openchoreov1alpha1.ClusterWorkflow, gen.ClusterWorkflow](*created)
	if err != nil {
		h.logger.Error("Failed to convert created cluster workflow", "error", err)
		return gen.CreateClusterWorkflow500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster workflow created successfully", "clusterWorkflow", created.Name)
	return gen.CreateClusterWorkflow201JSONResponse(genCWF), nil
}

// UpdateClusterWorkflow replaces an existing cluster-scoped workflow (full update).
func (h *Handler) UpdateClusterWorkflow(
	ctx context.Context,
	request gen.UpdateClusterWorkflowRequestObject,
) (gen.UpdateClusterWorkflowResponseObject, error) {
	h.logger.Info("UpdateClusterWorkflow called", "clusterWorkflowName", request.ClusterWorkflowName)

	if request.Body == nil {
		return gen.UpdateClusterWorkflow400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	cwfCR, err := convert[gen.ClusterWorkflow, openchoreov1alpha1.ClusterWorkflow](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateClusterWorkflow400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}
	// Ensure the name from the URL path is used
	cwfCR.Name = request.ClusterWorkflowName

	updated, err := h.services.ClusterWorkflowService.UpdateClusterWorkflow(ctx, &cwfCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateClusterWorkflow403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterworkflowsvc.ErrClusterWorkflowNotFound) {
			return gen.UpdateClusterWorkflow404JSONResponse{NotFoundJSONResponse: notFound("ClusterWorkflow")}, nil
		}
		h.logger.Error("Failed to update cluster workflow", "error", err)
		return gen.UpdateClusterWorkflow500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCWF, err := convert[openchoreov1alpha1.ClusterWorkflow, gen.ClusterWorkflow](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated cluster workflow", "error", err)
		return gen.UpdateClusterWorkflow500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster workflow updated successfully", "clusterWorkflow", updated.Name)
	return gen.UpdateClusterWorkflow200JSONResponse(genCWF), nil
}

// GetClusterWorkflow returns details of a specific cluster-scoped workflow.
func (h *Handler) GetClusterWorkflow(
	ctx context.Context,
	request gen.GetClusterWorkflowRequestObject,
) (gen.GetClusterWorkflowResponseObject, error) {
	h.logger.Debug("GetClusterWorkflow called", "clusterWorkflowName", request.ClusterWorkflowName)

	cwf, err := h.services.ClusterWorkflowService.GetClusterWorkflow(ctx, request.ClusterWorkflowName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetClusterWorkflow403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterworkflowsvc.ErrClusterWorkflowNotFound) {
			return gen.GetClusterWorkflow404JSONResponse{NotFoundJSONResponse: notFound("ClusterWorkflow")}, nil
		}
		h.logger.Error("Failed to get cluster workflow", "error", err)
		return gen.GetClusterWorkflow500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genCWF, err := convert[openchoreov1alpha1.ClusterWorkflow, gen.ClusterWorkflow](*cwf)
	if err != nil {
		h.logger.Error("Failed to convert cluster workflow", "error", err)
		return gen.GetClusterWorkflow500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetClusterWorkflow200JSONResponse(genCWF), nil
}

// DeleteClusterWorkflow deletes a cluster-scoped workflow by name.
func (h *Handler) DeleteClusterWorkflow(
	ctx context.Context,
	request gen.DeleteClusterWorkflowRequestObject,
) (gen.DeleteClusterWorkflowResponseObject, error) {
	h.logger.Info("DeleteClusterWorkflow called", "clusterWorkflowName", request.ClusterWorkflowName)

	err := h.services.ClusterWorkflowService.DeleteClusterWorkflow(ctx, request.ClusterWorkflowName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteClusterWorkflow403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, clusterworkflowsvc.ErrClusterWorkflowNotFound) {
			return gen.DeleteClusterWorkflow404JSONResponse{NotFoundJSONResponse: notFound("ClusterWorkflow")}, nil
		}
		h.logger.Error("Failed to delete cluster workflow", "error", err)
		return gen.DeleteClusterWorkflow500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("ClusterWorkflow deleted successfully", "clusterWorkflow", request.ClusterWorkflowName)
	return gen.DeleteClusterWorkflow204Response{}, nil
}

// GetClusterWorkflowSchema returns the parameter schema for a cluster-scoped workflow.
func (h *Handler) GetClusterWorkflowSchema(
	ctx context.Context,
	request gen.GetClusterWorkflowSchemaRequestObject,
) (gen.GetClusterWorkflowSchemaResponseObject, error) {
	h.logger.Debug("GetClusterWorkflowSchema called", "name", request.ClusterWorkflowName)

	jsonSchema, err := h.services.ClusterWorkflowService.GetClusterWorkflowSchema(ctx, request.ClusterWorkflowName)
	if err != nil {
		if errors.Is(err, clusterworkflowsvc.ErrClusterWorkflowNotFound) {
			return gen.GetClusterWorkflowSchema404JSONResponse{NotFoundJSONResponse: notFound("ClusterWorkflow")}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetClusterWorkflowSchema403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to get cluster workflow schema", "error", err)
		return gen.GetClusterWorkflowSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert JSONSchemaProps to SchemaResponse (map[string]interface{})
	data, err := json.Marshal(jsonSchema)
	if err != nil {
		h.logger.Error("Failed to marshal schema", "error", err)
		return gen.GetClusterWorkflowSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	var schemaResp gen.SchemaResponse
	if err := json.Unmarshal(data, &schemaResp); err != nil {
		h.logger.Error("Failed to unmarshal schema response", "error", err)
		return gen.GetClusterWorkflowSchema500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetClusterWorkflowSchema200JSONResponse(schemaResp), nil
}
