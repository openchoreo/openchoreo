// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ListDataPlanes returns a paginated list of data planes
func (h *Handler) ListDataPlanes(
	ctx context.Context,
	request gen.ListDataPlanesRequestObject,
) (gen.ListDataPlanesResponseObject, error) {
	return nil, errNotImplemented
}

// CreateDataPlane creates a new data plane
func (h *Handler) CreateDataPlane(
	ctx context.Context,
	request gen.CreateDataPlaneRequestObject,
) (gen.CreateDataPlaneResponseObject, error) {
	return nil, errNotImplemented
}

// GetDataPlane returns details of a specific data plane
func (h *Handler) GetDataPlane(
	ctx context.Context,
	request gen.GetDataPlaneRequestObject,
) (gen.GetDataPlaneResponseObject, error) {
	return nil, errNotImplemented
}
