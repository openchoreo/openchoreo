// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
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

// writeListResponse writes a list API response
func writeListResponse[T any](w http.ResponseWriter, items []T, resourceVersion, continueToken string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := models.ListSuccessResponse(items, resourceVersion, continueToken)
	_ = json.NewEncoder(w).Encode(response) // Ignore encoding errors for response
}
// extractListParams parses and validates query parameters for list operations
func extractListParams(query url.Values) (*models.ListOptions, error) {
	opts := &models.ListOptions{
		Limit:    models.DefaultPageLimit,
		Continue: "",
	}

	if continueToken := query.Get("continue"); continueToken != "" {
		opts.Continue = continueToken
	}

	if limitStr := query.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return nil, fmt.Errorf("limit must be a valid integer")
		}
		if limit < models.MinPageLimit {
			return nil, fmt.Errorf("limit %d out of range [%d, %d]", limit, models.MinPageLimit, models.MaxPageLimit)
		}
		if limit > models.MaxPageLimit {
			limit = models.MaxPageLimit
		}
		opts.Limit = limit
	}

	return opts, nil
}

// handlePaginationError handles pagination-related errors and returns true if the error was handled
func handlePaginationError(w http.ResponseWriter, err error, log *slog.Logger) bool {
	if errors.Is(err, services.ErrContinueTokenExpired) {
		log.Warn("Continue token expired")
		writeErrorResponse(w, http.StatusGone,
			"Continue token has expired. Please restart listing from the beginning.",
			services.CodeContinueTokenExpired)
		return true
	}
	if errors.Is(err, services.ErrInvalidContinueToken) {
		log.Warn("Invalid continue token provided")
		writeErrorResponse(w, http.StatusBadRequest,
			"Invalid continue token provided",
			services.CodeInvalidContinueToken)
		return true
	}
	return false
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
