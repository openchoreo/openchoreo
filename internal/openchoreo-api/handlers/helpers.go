// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// writeSuccessResponse writes a successful API response
func writeSuccessResponse[T any](w http.ResponseWriter, statusCode int, data T) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := models.SuccessResponse(data)
	_ = json.NewEncoder(w).Encode(response) // Ignore encoding errors for response
}

// writeErrorResponse writes an error API response
func writeErrorResponse(w http.ResponseWriter, statusCode int, message, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := models.ErrorResponse(message, code)
	_ = json.NewEncoder(w).Encode(response) // Ignore encoding errors for response
}

// writeListResponse writes a paginated list response
func writeListResponse[T any](w http.ResponseWriter, items []T, total, page, pageSize int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := models.ListSuccessResponse(items, total, page, pageSize)
	_ = json.NewEncoder(w).Encode(response) // Ignore encoding errors for response
}

// writeCursorListResponse writes a cursor-paginated list response
func writeCursorListResponse[T any](w http.ResponseWriter, items []T, nextCursor string, remainingCount *int64) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := models.CursorListSuccessResponse(items, nextCursor, remainingCount)
	_ = json.NewEncoder(w).Encode(response) // Ignore encoding errors for response
}

// setAuditResource sets resource information for audit logging
func setAuditResource(ctx context.Context, resourceType, resourceID, resourceName string) {
	audit.SetResource(ctx, &audit.Resource{
		Type: resourceType,
		ID:   resourceID,
		Name: resourceName,
	})
}

// addAuditMetadata adds a single metadata key-value pair for audit logging
func addAuditMetadata(ctx context.Context, key string, value any) {
	audit.AddMetadata(ctx, key, value)
}

// addAuditMetadataBatch adds multiple metadata key-value pairs for audit logging
func addAuditMetadataBatch(ctx context.Context, metadata map[string]any) {
	audit.AddMetadataBatch(ctx, metadata)
}

// Helper functions to safely dereference pointers
func getStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
