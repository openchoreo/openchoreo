// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ApplyResource applies a Kubernetes resource (like kubectl apply)
func (h *Handler) ApplyResource(
	ctx context.Context,
	request gen.ApplyResourceRequestObject,
) (gen.ApplyResourceResponseObject, error) {
	return nil, errNotImplemented
}

// DeleteResource deletes a Kubernetes resource (like kubectl delete)
func (h *Handler) DeleteResource(
	ctx context.Context,
	request gen.DeleteResourceRequestObject,
) (gen.DeleteResourceResponseObject, error) {
	return nil, errNotImplemented
}
