// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/jwt"
)

// UpdateRoleRequest represents the request body for updating a role
type UpdateRoleRequest struct {
	Actions []string `json:"actions"`
}

// UpdateRoleMappingRequest represents the request body for updating a role-entitlement mapping
type UpdateRoleMappingRequest struct {
	RoleName    string                  `json:"role_name"`
	Entitlement authz.Entitlement       `json:"entitlement"`
	Hierarchy   authz.ResourceHierarchy `json:"hierarchy"`
	Effect      authz.PolicyEffectType  `json:"effect"`
	Context     authz.Context           `json:"context"`
}

// ListRoles handles GET /api/v1/authz/roles
func (h *Handler) ListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.services.AuthzService.ListRoles(r.Context())
	if err != nil {
		h.logger.Error("Failed to list roles", "error", err)
		if handleAuthzDisabledError(w, err) {
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list roles", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, roles)
}

// GetRole handles GET /api/v1/authz/roles/{roleName}
func (h *Handler) GetRole(w http.ResponseWriter, r *http.Request) {
	roleName := r.PathValue("roleName")
	if roleName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Role name is required", services.CodeInvalidInput)
		return
	}

	role, err := h.services.AuthzService.GetRole(r.Context(), roleName)
	if err != nil {
		h.logger.Error("Failed to get role", "error", err, "role", roleName)
		if handleAuthzDisabledError(w, err) {
			return
		}
		writeErrorResponse(w, http.StatusNotFound, "Role not found", services.CodeNotFound)
		return
	}

	writeSuccessResponse(w, http.StatusOK, role)
}

// AddRole handles POST /api/v1/authz/roles
func (h *Handler) AddRole(w http.ResponseWriter, r *http.Request) {
	var role *authz.Role
	if err := json.NewDecoder(r.Body).Decode(&role); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	if err := h.services.AuthzService.AddRole(r.Context(), role); err != nil {
		h.logger.Error("Failed to add role", "error", err, "role", role.Name)
		if handleAuthzDisabledError(w, err) {
			return
		}
		if errors.Is(err, authz.ErrRoleAlreadyExists) {
			writeErrorResponse(w, http.StatusConflict, authz.ErrRoleAlreadyExists.Error(), services.CodeConflict)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to add role", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusCreated, map[string]string{"message": "Role added successfully"})
}

// UpdateRole handles PUT /api/v1/authz/roles/{roleName}
func (h *Handler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	roleName := r.PathValue("roleName")
	if roleName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Role name is required", services.CodeInvalidInput)
		return
	}

	var req UpdateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	role := &authz.Role{
		Name:    roleName,
		Actions: req.Actions,
	}

	if err := h.services.AuthzService.UpdateRole(r.Context(), role); err != nil {
		h.logger.Error("Failed to update role", "error", err, "role", roleName)
		if handleAuthzDisabledError(w, err) {
			return
		}
		if errors.Is(err, authz.ErrRoleNotFound) {
			writeErrorResponse(w, http.StatusNotFound, err.Error(), services.CodeNotFound)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to update role", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, map[string]string{"message": "Role updated successfully"})
}

// RemoveRole handles DELETE /api/v1/authz/roles/{roleName}
// Supports optional query parameter: force=true to remove role and all associated mappings
func (h *Handler) RemoveRole(w http.ResponseWriter, r *http.Request) {
	roleName := r.PathValue("roleName")
	if roleName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Role name is required", services.CodeInvalidInput)
		return
	}

	force := r.URL.Query().Get("force") == "true"

	if err := h.services.AuthzService.RemoveRole(r.Context(), roleName, force); err != nil {
		h.logger.Error("Failed to remove role", "error", err, "role", roleName, "force", force)
		if handleAuthzDisabledError(w, err) {
			return
		}
		if errors.Is(err, authz.ErrRoleNotFound) {
			writeErrorResponse(w, http.StatusNotFound, err.Error(), services.CodeNotFound)
			return
		}
		if errors.Is(err, authz.ErrRoleInUse) {
			writeErrorResponse(w, http.StatusConflict, err.Error(), services.CodeConflict)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to remove role", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusNoContent, "")
}

// ListRoleMappings handles GET /api/v1/authz/role-mappings
// Supports optional query parameters:
//   - role: Filter by role name
//   - claim & value: Filter by entitlement (both required together)
func (h *Handler) ListRoleMappings(w http.ResponseWriter, r *http.Request) {
	roleName := r.URL.Query().Get("role")
	claim := r.URL.Query().Get("claim")
	value := r.URL.Query().Get("value")

	// Validate claim/value pair - both must be present or both absent
	if (claim != "" && value == "") || (claim == "" && value != "") {
		writeErrorResponse(w, http.StatusBadRequest, "Both 'claim' and 'value' query parameters must be provided together", services.CodeInvalidInput)
		return
	}

	mappings, err := h.services.AuthzService.ListRoleMappings(r.Context(), roleName, claim, value)
	if err != nil {
		h.logger.Error("Failed to list role mappings", "error", err,
			"role", roleName, "claim", claim, "value", value)
		if handleAuthzDisabledError(w, err) {
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list role mappings", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, mappings)
}

// AddRoleMapping handles POST /api/v1/authz/role-mappings
func (h *Handler) AddRoleMapping(w http.ResponseWriter, r *http.Request) {
	var mapping *authz.RoleEntitlementMapping
	if err := json.NewDecoder(r.Body).Decode(&mapping); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	if err := h.services.AuthzService.AddRoleMapping(r.Context(), mapping); err != nil {
		h.logger.Error("Failed to add role mapping", "error", err)
		if handleAuthzDisabledError(w, err) {
			return
		}
		if errors.Is(err, authz.ErrRolePolicyMappingAlreadyExists) {
			writeErrorResponse(w, http.StatusConflict, authz.ErrRolePolicyMappingAlreadyExists.Error(), services.CodeConflict)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to add role mapping", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusCreated, map[string]string{"message": "Role mapping added successfully"})
}

// UpdateRoleMapping handles PUT /api/v1/authz/role-mappings/{mappingId}
func (h *Handler) UpdateRoleMapping(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("mappingId")
	if idStr == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Mapping ID is required", services.CodeInvalidInput)
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid mapping ID", services.CodeInvalidInput)
		return
	}

	var req UpdateRoleMappingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	// Map DTO to domain model
	mapping := &authz.RoleEntitlementMapping{
		ID:          uint(id),
		RoleName:    req.RoleName,
		Entitlement: req.Entitlement,
		Hierarchy:   req.Hierarchy,
		Effect:      req.Effect,
		Context:     req.Context,
	}

	if err := h.services.AuthzService.UpdateRoleMapping(r.Context(), mapping); err != nil {
		h.logger.Error("Failed to update role mapping", "error", err, "id", id)
		if handleAuthzDisabledError(w, err) {
			return
		}
		if errors.Is(err, authz.ErrRolePolicyMappingNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Role mapping not found", services.CodeNotFound)
			return
		}
		if errors.Is(err, authz.ErrCannotDeleteSystemMapping) {
			writeErrorResponse(w, http.StatusForbidden, authz.ErrCannotDeleteSystemMapping.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, authz.ErrRolePolicyMappingAlreadyExists) {
			writeErrorResponse(w, http.StatusConflict, "A mapping with these parameters already exists", services.CodeConflict)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to update role mapping", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, map[string]string{"message": "Role mapping updated successfully"})
}

// RemoveRoleMapping handles DELETE /api/v1/authz/role-mappings/{mappingId}
func (h *Handler) RemoveRoleMapping(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("mappingId")
	if idStr == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Mapping ID is required", services.CodeInvalidInput)
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid mapping ID", services.CodeInvalidInput)
		return
	}

	if err := h.services.AuthzService.RemoveRoleMappingByID(r.Context(), uint(id)); err != nil {
		h.logger.Error("Failed to remove role mapping", "error", err, "id", id)
		if handleAuthzDisabledError(w, err) {
			return
		}
		if errors.Is(err, authz.ErrRolePolicyMappingNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Role mapping not found", services.CodeNotFound)
			return
		}
		if errors.Is(err, authz.ErrCannotDeleteSystemMapping) {
			writeErrorResponse(w, http.StatusForbidden, authz.ErrCannotDeleteSystemMapping.Error(), services.CodeForbidden)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to remove role mapping", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusNoContent, "")
}

// ListActions handles GET /api/v1/authz/actions
func (h *Handler) ListActions(w http.ResponseWriter, r *http.Request) {
	actions, err := h.services.AuthzService.ListActions(r.Context())
	if err != nil {
		h.logger.Error("Failed to list actions", "error", err)
		if handleAuthzDisabledError(w, err) {
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list actions", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, actions)
}

// ListUserTypes handles GET /api/v1/authz/user-types
func (h *Handler) ListUserTypes(w http.ResponseWriter, r *http.Request) {
	userTypes, err := h.services.AuthzService.ListUserTypes(r.Context())
	if err != nil {
		h.logger.Error("Failed to list user types", "error", err)
		if handleAuthzDisabledError(w, err) {
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list user types", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, userTypes)
}

// Evaluate handles POST /api/v1/authz/evaluate
func (h *Handler) Evaluate(w http.ResponseWriter, r *http.Request) {
	var request authz.EvaluateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	decision, err := h.services.AuthzService.Evaluate(r.Context(), &request)
	if err != nil {
		if errors.Is(err, authz.ErrInvalidRequest) {
			writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to evaluate request", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, decision)
}

// BatchEvaluate handles POST /api/v1/authz/batch-evaluate
func (h *Handler) BatchEvaluate(w http.ResponseWriter, r *http.Request) {
	var request authz.BatchEvaluateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	response, err := h.services.AuthzService.BatchEvaluate(r.Context(), &request)
	if err != nil {
		if errors.Is(err, authz.ErrInvalidRequest) {
			writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to batch evaluate requests", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, response)
}

// GetSubjectProfile handles GET /api/v1/authz/profile
func (h *Handler) GetSubjectProfile(w http.ResponseWriter, r *http.Request) {
	// Extract token from context
	token := jwt.GetTokenFromContext(r.Context())
	if token == "" {
		writeErrorResponse(w, http.StatusUnauthorized, "Missing subject token", services.CodeInvalidInput)
		return
	}

	// Extract query parameters
	org := r.URL.Query().Get("org")
	if org == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Organization (org) query parameter is required", services.CodeInvalidInput)
		return
	}
	project := r.URL.Query().Get("project")
	component := r.URL.Query().Get("component")
	orgUnits := r.URL.Query()["ou"]

	request := &authz.ProfileRequest{
		Subject: authz.Subject{
			JwtToken: token,
		},
		Scope: authz.ResourceHierarchy{
			Organization:      org,
			OrganizationUnits: orgUnits,
			Project:           project,
			Component:         component,
		},
	}

	resp, err := h.services.AuthzService.GetSubjectProfile(r.Context(), request)
	if err != nil {
		if errors.Is(err, authz.ErrInvalidRequest) {
			writeErrorResponse(w, http.StatusBadRequest, err.Error(), services.CodeInvalidInput)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get subject profile", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, resp)
}

// handleAuthzDisabledError checks if the error indicates that authorization is disabled and returns a standardized error
func handleAuthzDisabledError(w http.ResponseWriter, err error) bool {
	if err != nil && errors.Is(err, authz.ErrAuthzDisabled) {
		writeErrorResponse(w, http.StatusForbidden, authz.ErrAuthzDisabled.Error(), services.CodeForbidden)
		return true
	}
	return false
}
