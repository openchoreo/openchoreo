// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	corev1 "k8s.io/api/core/v1"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	namespacesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/namespace"
)

// ListNamespaces returns a paginated list of namespaces.
func (h *Handler) ListNamespaces(
	ctx context.Context,
	request gen.ListNamespacesRequestObject,
) (gen.ListNamespacesResponseObject, error) {
	h.logger.Debug("ListNamespaces called")

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)

	result, err := h.namespaceService.ListNamespaces(ctx, opts)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListNamespaces403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to list namespaces", "error", err)
		return gen.ListNamespaces500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[corev1.Namespace, gen.Namespace](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert namespaces", "error", err)
		return gen.ListNamespaces500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListNamespaces200JSONResponse{
		Items:      items,
		Pagination: ToPaginationPtr(result),
	}, nil
}

// GetNamespace returns details of a specific namespace.
func (h *Handler) GetNamespace(
	ctx context.Context,
	request gen.GetNamespaceRequestObject,
) (gen.GetNamespaceResponseObject, error) {
	h.logger.Debug("GetNamespace called", "namespaceName", request.NamespaceName)

	namespace, err := h.namespaceService.GetNamespace(ctx, request.NamespaceName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetNamespace403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, namespacesvc.ErrNamespaceNotFound) {
			return gen.GetNamespace404JSONResponse{NotFoundJSONResponse: notFound("Namespace")}, nil
		}
		h.logger.Error("Failed to get namespace", "error", err)
		return gen.GetNamespace500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genNS, err := convert[corev1.Namespace, gen.Namespace](*namespace)
	if err != nil {
		h.logger.Error("Failed to convert namespace", "error", err)
		return gen.GetNamespace500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetNamespace200JSONResponse(genNS), nil
}

// CreateNamespace creates a new namespace.
func (h *Handler) CreateNamespace(
	ctx context.Context,
	request gen.CreateNamespaceRequestObject,
) (gen.CreateNamespaceResponseObject, error) {
	h.logger.Info("CreateNamespace called")

	if request.Body == nil {
		return gen.CreateNamespace400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	nsCR, err := convert[gen.Namespace, corev1.Namespace](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateNamespace400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	created, err := h.namespaceService.CreateNamespace(ctx, &nsCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateNamespace403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, namespacesvc.ErrNamespaceAlreadyExists) {
			return gen.CreateNamespace409JSONResponse{ConflictJSONResponse: conflict("Namespace already exists")}, nil
		}
		h.logger.Error("Failed to create namespace", "error", err)
		return gen.CreateNamespace500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genNS, err := convert[corev1.Namespace, gen.Namespace](*created)
	if err != nil {
		h.logger.Error("Failed to convert created namespace", "error", err)
		return gen.CreateNamespace500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Namespace created successfully", "namespace", created.Name)
	return gen.CreateNamespace201JSONResponse(genNS), nil
}

// UpdateNamespace replaces an existing namespace (full update).
func (h *Handler) UpdateNamespace(
	ctx context.Context,
	request gen.UpdateNamespaceRequestObject,
) (gen.UpdateNamespaceResponseObject, error) {
	h.logger.Info("UpdateNamespace called", "namespaceName", request.NamespaceName)

	if request.Body == nil {
		return gen.UpdateNamespace400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	nsCR, err := convert[gen.Namespace, corev1.Namespace](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateNamespace400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	// Ensure the name from the URL path is used
	nsCR.Name = request.NamespaceName

	updated, err := h.namespaceService.UpdateNamespace(ctx, &nsCR)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateNamespace403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, namespacesvc.ErrNamespaceNotFound) {
			return gen.UpdateNamespace404JSONResponse{NotFoundJSONResponse: notFound("Namespace")}, nil
		}
		h.logger.Error("Failed to update namespace", "error", err)
		return gen.UpdateNamespace500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genNS, err := convert[corev1.Namespace, gen.Namespace](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated namespace", "error", err)
		return gen.UpdateNamespace500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Namespace updated successfully", "namespace", updated.Name)
	return gen.UpdateNamespace200JSONResponse(genNS), nil
}

// DeleteNamespace deletes a namespace by name.
func (h *Handler) DeleteNamespace(
	ctx context.Context,
	request gen.DeleteNamespaceRequestObject,
) (gen.DeleteNamespaceResponseObject, error) {
	h.logger.Info("DeleteNamespace called", "namespaceName", request.NamespaceName)

	err := h.namespaceService.DeleteNamespace(ctx, request.NamespaceName)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteNamespace403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, namespacesvc.ErrNamespaceNotFound) {
			return gen.DeleteNamespace404JSONResponse{NotFoundJSONResponse: notFound("Namespace")}, nil
		}
		h.logger.Error("Failed to delete namespace", "error", err)
		return gen.DeleteNamespace500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Namespace deleted successfully", "namespace", request.NamespaceName)
	return gen.DeleteNamespace204Response{}, nil
}
