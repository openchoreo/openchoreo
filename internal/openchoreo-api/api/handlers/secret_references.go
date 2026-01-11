// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ListSecretReferences returns a list of secret references
func (h *Handler) ListSecretReferences(
	ctx context.Context,
	request gen.ListSecretReferencesRequestObject,
) (gen.ListSecretReferencesResponseObject, error) {
	return nil, errNotImplemented
}
