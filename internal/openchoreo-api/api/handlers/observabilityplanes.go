// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ListObservabilityPlanes returns a list of observability planes
func (h *Handler) ListObservabilityPlanes(
	ctx context.Context,
	request gen.ListObservabilityPlanesRequestObject,
) (gen.ListObservabilityPlanesResponseObject, error) {
	return nil, errNotImplemented
}
