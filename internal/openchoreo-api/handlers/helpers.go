// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

const (
	DefaultLimit           = 16   // If limit not specified, use 16 as default
	MaxLimit               = 1024 // Maximum items per page
	MaxCursorLength        = 512  // To prevent hitting kubernetes API Server's maximum URL length 8KB
	MaxDecodedCursorLength = 512  // Maximum decoded cursor content size
)

// Base64 encoding variants we support for cursor validation.
// Kubernetes continue tokens use RFC 4648 base64 encoding, often without padding.
// Reference: https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/list-meta/
const (
	// Standard base64 encoding with padding (RFC 4648 Section 4)
	base64VariantStd = "StdEncoding"
	// Standard base64 encoding without padding (RFC 4648 Section 3.2)
	base64VariantRawStd = "RawStdEncoding"
	// URL-safe base64 encoding with padding
	base64VariantURL = "URLEncoding"
	// URL-safe base64 encoding without padding (RFC 4648 Section 5)
	base64VariantRawURL = "RawURLEncoding"
)

// writeSuccessResponse writes a successful API response
func writeSuccessResponse[T any](w http.ResponseWriter, statusCode int, data T) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := models.SuccessResponse(data)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Can't change status code (already sent), write error to response
		fmt.Fprintf(w, `{"error":{"message":"Internal server error","code":"ENCODING_ERROR"}}`)
	}
}

// writeErrorResponse writes an error API response
func writeErrorResponse(w http.ResponseWriter, statusCode int, message, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := models.ErrorResponse(message, code)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Can't change status code (already sent), but log the error
		fmt.Fprintf(w, `{"error":{"message":"Internal server error","code":"ENCODING_ERROR"}}`)
	}
}

// writeListResponse writes a paginated list response
func writeTokenExpiredError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusGone)

	metadata := map[string]any{
		"retryable": true,
		"code":      "CONTINUE_TOKEN_EXPIRED",
	}

	response := models.ErrorResponseWithMetadata(
		"Continue token has expired",
		"CONTINUE_TOKEN_EXPIRED",
		metadata,
	)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		fmt.Fprintf(w, `{"error":{"message":"Internal server error","code":"ENCODING_ERROR"}}`)
	}
}

func writeListResponse[T any](w http.ResponseWriter, items []T, total, page, pageSize int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := models.ListSuccessResponse(items, total, page, pageSize)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		fmt.Fprintf(w, `{"error":{"message":"Internal server error","code":"ENCODING_ERROR"}}`)
	}
}

// parseCursorParams parses cursor and limit parameters
//
// Pagination Mode Precedence (highest to lowest):
// 1. Explicit ?pagination=cursor or ?pagination=legacy parameter
// 2. Presence of ?cursor parameter (enables cursor mode)
// 3. Feature flag config.GetCursorPaginationEnabled()
// 4. Default: legacy mode (useCursor=false)
func parseCursorParams(r *http.Request) (cursor string, limit int64, useCursor bool, err error) {
	query := r.URL.Query()

	cursor = query.Get("cursor")
	limitStr := query.Get("limit")
	mode := query.Get("pagination")

	// Determine pagination mode using precedence rules
	// Precedence 1: Explicit mode parameter overrides everything
	if mode == "cursor" {
		useCursor = true
	} else if mode == "legacy" {
		useCursor = false
	} else if mode != "" {
		// Invalid pagination mode specified
		return "", 0, false, fmt.Errorf("invalid pagination mode: %s. Valid values are 'cursor' or 'legacy'", mode)
	} else if cursor != "" {
		// Precedence 2: Presence of cursor parameter enables cursor mode
		useCursor = true
	} else {
		// Precedence 3: Feature flag determines default behavior
		useCursor = config.GetCursorPaginationEnabled()
	}

	// Validate cursor if we're using cursor mode
	if useCursor {
		if err := validateCursorModeParams(cursor); err != nil {
			return "", 0, false, err
		}
	}

	// Enforce limits
	limit = DefaultLimit

	if limitStr != "" {
		if parsedLimit, parseErr := strconv.ParseInt(limitStr, 10, 64); parseErr != nil {
			return "", 0, false, fmt.Errorf("invalid limit format: %w", parseErr)
		} else if parsedLimit <= 0 {
			return "", 0, false, fmt.Errorf("limit must be positive, got: %d", parsedLimit)
		} else if parsedLimit > MaxLimit {
			limit = MaxLimit
		} else {
			limit = parsedLimit
		}
	}

	return cursor, limit, useCursor, nil
}

// validateCursor() validates Kubernetes continue tokens according to API specifications.
//
// Kubernetes continue tokens are opaque base64-encoded strings returned by the API server
// to support pagination of large result sets. These tokens are used with the ?continue
// query parameter to retrieve the next page of results.
func validateCursor(cursor string) error {
	if cursor == "" {
		// Allow empty cursor for first page requests
		return nil
	}

	// 1. Length check (encoded)
	if len(cursor) > MaxCursorLength {
		return fmt.Errorf("cursor exceeds maximum allowed length of %d characters", MaxCursorLength)
	}

	// 2. Base64 decode validation with multiple encoding variants
	// Kubernetes continue tokens may use any of these RFC 4648 variants:
	// - StdEncoding: Standard base64 with padding (legacy)
	// - RawStdEncoding: Standard base64 WITHOUT padding (common in K8s >= v1.15)
	// - URLEncoding: URL-safe base64 with padding
	// - RawURLEncoding: URL-safe base64 WITHOUT padding
	decoded, variant, err := tryDecodeBase64(cursor)
	if err != nil {
		return fmt.Errorf("cursor format is invalid: malformed base64 encoding (tried all RFC 4648 variants)")
	}

	// Log which variant was successful for debugging (only in development)
	_ = variant // Use in debug logging if needed

	// 3. Validate decoded content length
	if len(decoded) > MaxDecodedCursorLength {
		return fmt.Errorf("cursor exceeds maximum decoded size of %d bytes", MaxDecodedCursorLength)
	}

	// 4. Check for null bytes
	// Kubernetes continue tokens are JSON-like structures and should not contain null bytes
	for _, b := range decoded {
		if b == 0x00 {
			return fmt.Errorf("cursor format is invalid: contains null bytes")
		}
	}

	// 5. Validate UTF-8 encoding
	// Kubernetes continue tokens contain JSON structures which must be valid UTF-8
	if len(decoded) > 0 && !utf8.Valid(decoded) {
		return fmt.Errorf("cursor format is invalid: not valid UTF-8")
	}

	return nil
}

// tryDecodeBase64 attempts to decode a base64 string using all RFC 4648 encoding variants.
// Returns the decoded bytes, the encoding variant that succeeded, and any error.
//
// This is necessary because Kubernetes continue tokens may use different base64 variants:
// - Older versions: Standard base64 WITH padding (StdEncoding)
// - Newer versions: Standard base64 WITHOUT padding (RawStdEncoding)
// - Edge cases: URL-safe variants for special scenarios
//
// Reference: RFC 4648 - https://tools.ietf.org/html/rfc4648
func tryDecodeBase64(cursor string) ([]byte, string, error) {
	// Try in order of most common to least common for Kubernetes tokens

	// 1. Standard base64 without padding
	if decoded, err := base64.RawStdEncoding.DecodeString(cursor); err == nil {
		return decoded, base64VariantRawStd, nil
	}

	// 2. Standard base64 with padding
	if decoded, err := base64.StdEncoding.DecodeString(cursor); err == nil {
		return decoded, base64VariantStd, nil
	}

	// 3. URL-safe base64 without padding
	if decoded, err := base64.RawURLEncoding.DecodeString(cursor); err == nil {
		return decoded, base64VariantRawURL, nil
	}

	// 4. URL-safe base64 with padding
	if decoded, err := base64.URLEncoding.DecodeString(cursor); err == nil {
		return decoded, base64VariantURL, nil
	}

	// All variants failed
	return nil, "", fmt.Errorf("not a valid RFC 4648 base64 string")
}

// validateCursorModeParams validates cursor-specific parameters
func validateCursorModeParams(cursor string) error {
	return validateCursor(cursor)
}

// validateCursorWithContext validates the cursor
func validateCursorWithContext(cursor string) error {
	return validateCursor(cursor)
}

func writeCursorListResponse[T any](w http.ResponseWriter, items []T, nextCursor string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	var nextCursorPtr *string

	if nextCursor != "" {
		// State 1: More pages available - return the token
		nextCursorPtr = &nextCursor
	} else {
		// State 2: Pagination complete - always return empty string for consistency
		// This tells clients "pagination is complete"
		emptyCursor := ""
		nextCursorPtr = &emptyCursor
	}
	// State 3: nil case - handled automatically by var declaration
	// This occurs when no results and no cursor needed

	response := models.CursorListSuccessResponse(items, nextCursorPtr)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		fmt.Fprintf(w, `{"error":{"message":"Internal server error","code":"ENCODING_ERROR"}}`)
	}
}
