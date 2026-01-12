// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ListEnvironments returns a paginated list of environments
func (h *Handler) ListEnvironments(
	ctx context.Context,
	request gen.ListEnvironmentsRequestObject,
) (gen.ListEnvironmentsResponseObject, error) {
	return nil, errNotImplemented
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
