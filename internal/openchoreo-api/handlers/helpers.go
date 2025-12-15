// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
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
		Limit:    models.DefaultPageLimit, // Default to 100
		Continue: "",
	}

	// Validate and set continue token if provided
	if continueToken := query.Get("continue"); continueToken != "" {
		opts.Continue = continueToken
	}

	// Parse limit if provided
	if limitStr := query.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return nil, fmt.Errorf("limit must be a valid integer")
		}
		if limit != 0 && limit < models.MinPageLimit {
			return nil, fmt.Errorf("limit %d out of range [%d, %d]", limit, models.MinPageLimit, models.MaxPageLimit)
		}
		if limit != 0 && limit > models.MaxPageLimit {
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
