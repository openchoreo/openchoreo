// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/google/uuid"
	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// ListEnvironments returns a paginated list of environments
func (h *Handler) ListEnvironments(
	ctx context.Context,
	request gen.ListEnvironmentsRequestObject,
) (gen.ListEnvironmentsResponseObject, error) {
	h.logger.Debug("ListEnvironments called", "namespaceName", request.NamespaceName)

	environments, err := h.services.EnvironmentService.ListEnvironments(ctx, request.NamespaceName)
	if err != nil {
		h.logger.Error("Failed to list environments", "error", err)
		return gen.ListEnvironments500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert to generated types
	items := make([]gen.Environment, 0, len(environments))
	for _, env := range environments {
		items = append(items, toGenEnvironment(env))
	}

	// TODO: Implement proper cursor-based pagination with Kubernetes continuation tokens
	return gen.ListEnvironments200JSONResponse{
		Items:      items,
		Pagination: gen.Pagination{},
	}, nil
}

// toGenEnvironment converts models.EnvironmentResponse to gen.Environment
func toGenEnvironment(env *models.EnvironmentResponse) gen.Environment {
	uid, _ := uuid.Parse(env.UID)
	return gen.Environment{
		Uid:          uid,
		Name:         env.Name,
		Namespace:    env.Namespace,
		DisplayName:  ptr.To(env.DisplayName),
		Description:  ptr.To(env.Description),
		DataPlaneRef: ptr.To(env.DataPlaneRef),
		IsProduction: env.IsProduction,
		DnsPrefix:    ptr.To(env.DNSPrefix),
		CreatedAt:    env.CreatedAt,
		Status:       ptr.To(env.Status),
	}
}

// CreateEnvironment creates a new environment
func (h *Handler) CreateEnvironment(
	ctx context.Context,
	request gen.CreateEnvironmentRequestObject,
) (gen.CreateEnvironmentResponseObject, error) {
	return nil, errNotImplemented
}

// GetEnvironment returns details of a specific environment
func (h *Handler) GetEnvironment(
	ctx context.Context,
	request gen.GetEnvironmentRequestObject,
) (gen.GetEnvironmentResponseObject, error) {
	return nil, errNotImplemented
}

// GetEnvironmentObserverURL returns the observer URL for an environment
func (h *Handler) GetEnvironmentObserverURL(
	ctx context.Context,
	request gen.GetEnvironmentObserverURLRequestObject,
) (gen.GetEnvironmentObserverURLResponseObject, error) {
	return nil, errNotImplemented
}

// GetRCAAgentURL returns the RCA agent URL for an environment
func (h *Handler) GetRCAAgentURL(
	ctx context.Context,
	request gen.GetRCAAgentURLRequestObject,
) (gen.GetRCAAgentURLResponseObject, error) {
	return nil, errNotImplemented
}
