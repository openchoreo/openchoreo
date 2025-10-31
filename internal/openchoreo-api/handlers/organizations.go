// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/middleware/logger"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// ListOrganizations handles GET /api/v1/orgs
// Supports both legacy (no params) and cursor-based (?cursor=X&limit=Y) pagination
func (h *Handler) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)

	// Check if cursor-based pagination is requested
	cursor, limit, useCursor, err := parseCursorParams(r)
	if err != nil {
		errorCode := services.GetPaginationErrorCode(err)
		writeErrorResponse(w, http.StatusBadRequest, err.Error(), errorCode)
		return
	}

	if useCursor {
		// only validate non-empty cursors
		if cursor != "" {
			if err := validateCursorWithContext(cursor); err != nil {
				logger.Warn("Invalid cursor", "error", err, "ip", r.RemoteAddr)
				writeErrorResponse(w, http.StatusBadRequest,
					"Invalid cursor parameter", services.CodeInvalidCursorFormat)
				return
			}
		}

		organizations, nextCursor, err := h.services.OrganizationService.ListOrganizationsWithCursor(
			ctx, cursor, limit)
		if err != nil {
			// Check specific errors first
			if errors.Is(err, services.ErrOrganizationNotFound) {
				writeErrorResponse(w, http.StatusNotFound,
					"Organization not found", services.CodeOrganizationNotFound)
				return
			}
			if errors.Is(err, services.ErrContinueTokenExpired) {
				writeTokenExpiredError(w)
				return
			}
			if errors.Is(err, services.ErrInvalidCursorFormat) {
				logger.Error("Invalid cursor format", "error", err, "ip", r.RemoteAddr)
				writeErrorResponse(w, http.StatusBadRequest,
					"Invalid cursor format", services.CodeInvalidCursorFormat)
				return
			}
			if strings.Contains(err.Error(), "service unavailable") {
				writeErrorResponse(w, http.StatusServiceUnavailable,
					"Service temporarily unavailable", services.CodeInternalError)
				return
			}
			logger.Error("Failed to list organizations with cursor", "error", err)
			writeErrorResponse(w, http.StatusInternalServerError,
				"Failed to list organizations", services.CodeInternalError)
			return
		}

		writeCursorListResponse(w, organizations, nextCursor)
		return
	}

	// Legacy mode: return all organizations
	organizations, err := h.services.OrganizationService.ListOrganizations(ctx)
	if err != nil {
		logger.Error("Failed to list organizations", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError,
			"Failed to list organizations", services.CodeInternalError)
		return
	}

	writeListResponse(w, organizations, len(organizations), 1, len(organizations))
}

// GetOrganization handles GET /api/v1/orgs/{orgName}
func (h *Handler) GetOrganization(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logger.GetLogger(ctx)
	orgName := r.PathValue("orgName")

	if orgName == "" {
		logger.Warn("Organization name is required")
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", services.CodeInvalidInput)
		return
	}

	organization, err := h.services.OrganizationService.GetOrganization(ctx, orgName)
	if err != nil {
		if errors.Is(err, services.ErrOrganizationNotFound) {
			logger.Warn("Organization not found", "org", orgName)
			writeErrorResponse(w, http.StatusNotFound, "Organization not found", services.CodeOrganizationNotFound)
			return
		}
		logger.Error("Failed to get organization", "error", err, "org", orgName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get organization", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, organization)
}
