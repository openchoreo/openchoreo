// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// GetWorkloads returns the workload definition for a component
func (h *Handler) GetWorkloads(
	ctx context.Context,
	request gen.GetWorkloadsRequestObject,
) (gen.GetWorkloadsResponseObject, error) {
	return nil, errNotImplemented
}

// CreateWorkload creates or updates the workload definition for a component
func (h *Handler) CreateWorkload(
	ctx context.Context,
	request gen.CreateWorkloadRequestObject,
) (gen.CreateWorkloadResponseObject, error) {
	return nil, errNotImplemented
}
