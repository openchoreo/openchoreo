// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// GetOAuthProtectedResourceMetadata returns OAuth 2.0 protected resource metadata
func (h *Handler) GetOAuthProtectedResourceMetadata(
	ctx context.Context,
	request gen.GetOAuthProtectedResourceMetadataRequestObject,
) (gen.GetOAuthProtectedResourceMetadataResponseObject, error) {
	return nil, errNotImplemented
}
