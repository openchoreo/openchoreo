// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// GetMetadata returns platform metadata.
func (h *Handler) GetMetadata(
	ctx context.Context,
	request gen.GetMetadataRequestObject,
) (gen.GetMetadataResponseObject, error) {
	md, err := h.services.MetadataService.GetMetadata(ctx)
	if err != nil {
		h.logger.Error("Failed to get platform metadata", "error", err)
		return gen.GetMetadata500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	auditLogs := gen.AuditLogsFeature{Enabled: md.AuditLogs.Enabled}
	if md.AuditLogs.ObserverURL != "" {
		observerURL := md.AuditLogs.ObserverURL
		auditLogs.ObserverURL = &observerURL
	}

	return gen.GetMetadata200JSONResponse{
		Features: gen.MetadataFeatures{AuditLogs: auditLogs},
	}, nil
}
