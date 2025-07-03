package handlers

import (
	"errors"
	"net/http"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// ListOrganizations handles GET /api/v1/orgs
func (h *Handler) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	organizations, err := h.services.OrganizationService.ListOrganizations(ctx)
	if err != nil {
		h.logger.Error("Failed to list organizations", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list organizations", services.CodeInternalError)
		return
	}

	writeListResponse(w, organizations, len(organizations), 1, len(organizations))
}

// GetOrganization handles GET /api/v1/orgs/{orgName}
func (h *Handler) GetOrganization(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgName := r.PathValue("orgName")

	if orgName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Organization name is required", services.CodeInvalidInput)
		return
	}

	organization, err := h.services.OrganizationService.GetOrganization(ctx, orgName)
	if err != nil {
		if errors.Is(err, services.ErrOrganizationNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Organization not found", services.CodeOrganizationNotFound)
			return
		}
		h.logger.Error("Failed to get organization", "error", err, "org", orgName)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get organization", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, organization)
}
