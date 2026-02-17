// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacy_services"
)

// ListNamespaces returns a paginated list of namespaces
func (h *Handler) ListNamespaces(
	ctx context.Context,
	request gen.ListNamespacesRequestObject,
) (gen.ListNamespacesResponseObject, error) {
	h.logger.Debug("ListNamespaces called")

	namespaces, err := h.services.NamespaceService.ListNamespaces(ctx)
	if err != nil {
		h.logger.Error("Failed to list namespaces", "error", err)
		return gen.ListNamespaces500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert to generated types
	items := make([]gen.Namespace, 0, len(namespaces))
	for _, ns := range namespaces {
		items = append(items, toGenNamespace(ns))
	}

	// TODO: Implement proper cursor-based pagination with Kubernetes continuation tokens
	return gen.ListNamespaces200JSONResponse{
		Items:      items,
		Pagination: gen.Pagination{},
	}, nil
}

// GetNamespace returns details of a specific namespace
func (h *Handler) GetNamespace(
	ctx context.Context,
	request gen.GetNamespaceRequestObject,
) (gen.GetNamespaceResponseObject, error) {
	h.logger.Debug("GetNamespace called", "namespaceName", request.NamespaceName)

	namespace, err := h.services.NamespaceService.GetNamespace(ctx, request.NamespaceName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetNamespace403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrNamespaceNotFound) {
			return gen.GetNamespace404JSONResponse{NotFoundJSONResponse: notFound("Namespace")}, nil
		}
		h.logger.Error("Failed to get namespace", "error", err)
		return gen.GetNamespace500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetNamespace200JSONResponse(toGenNamespace(namespace)), nil
}

// CreateNamespace creates a new namespace
func (h *Handler) CreateNamespace(
	ctx context.Context,
	request gen.CreateNamespaceRequestObject,
) (gen.CreateNamespaceResponseObject, error) {
	h.logger.Debug("CreateNamespace called")

	// Validate request body
	if request.Body == nil {
		return gen.CreateNamespace400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	// Validate required fields
	if request.Body.Name == "" {
		return gen.CreateNamespace400JSONResponse{BadRequestJSONResponse: badRequest("Namespace name is required")}, nil
	}

	// Convert gen.CreateNamespaceRequest to models.CreateNamespaceRequest
	req := &models.CreateNamespaceRequest{
		Name: request.Body.Name,
	}
	if request.Body.DisplayName != nil {
		req.DisplayName = *request.Body.DisplayName
	}
	if request.Body.Description != nil {
		req.Description = *request.Body.Description
	}

	// Create namespace
	namespace, err := h.services.NamespaceService.CreateNamespace(ctx, req)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateNamespace403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrNamespaceAlreadyExists) {
			return gen.CreateNamespace409JSONResponse(conflict("Namespace already exists")), nil
		}
		h.logger.Error("Failed to create namespace", "error", err, "namespace", req.Name)
		return gen.CreateNamespace500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Namespace created successfully", "namespace", req.Name)
	return gen.CreateNamespace201JSONResponse(toGenNamespace(namespace)), nil
}

// toGenNamespace converts models.NamespaceResponse to gen.Namespace
func toGenNamespace(namespace *models.NamespaceResponse) gen.Namespace {
	return gen.Namespace{
		Name:        namespace.Name,
		DisplayName: ptr.To(namespace.DisplayName),
		Description: ptr.To(namespace.Description),
		CreatedAt:   namespace.CreatedAt,
		Status:      ptr.To(namespace.Status),
	}
}
