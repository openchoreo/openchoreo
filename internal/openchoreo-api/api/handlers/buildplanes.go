// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ListBuildPlanes returns a list of build planes
func (h *Handler) ListBuildPlanes(
	ctx context.Context,
	request gen.ListBuildPlanesRequestObject,
) (gen.ListBuildPlanesResponseObject, error) {
	return nil, errNotImplemented
}
