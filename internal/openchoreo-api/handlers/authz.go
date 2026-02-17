// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// ListActions handles GET /api/v1/authz/actions
func (h *Handler) ListActions(w http.ResponseWriter, r *http.Request) {
	actions, err := h.services.AuthzService.ListActions(r.Context())
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			h.logger.Warn("Unauthorized to list actions")
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		h.logger.Error("Failed to list actions", "error", err)
		if handleAuthzDisabledError(w, err) {
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list actions", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, actions)
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
	// Extract query parameters
	namespace := r.URL.Query().Get("namespace")
	project := r.URL.Query().Get("project")
	component := r.URL.Query().Get("component")

	subjectCtx, ok := auth.GetSubjectContextFromContext(r.Context())
	if !ok || subjectCtx == nil {
		writeErrorResponse(w, http.StatusForbidden, "Subject context not found", services.CodeForbidden)
		return
	}

	authzSubjectCtx := authz.GetAuthzSubjectContext(subjectCtx)

	request := &authz.ProfileRequest{
		SubjectContext: authzSubjectCtx,
		Scope: authz.ResourceHierarchy{
			Namespace: namespace,
			Project:   project,
			Component: component,
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

// ListClusterRoles handles GET /api/v1/authz/clusterroles
func (h *Handler) ListClusterRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.services.AuthzService.ListClusterRoles(r.Context())
	if err != nil {
		h.logger.Debug("Failed to list cluster roles", "error", err)
		if errors.Is(err, services.ErrForbidden) {
			h.logger.Warn("Unauthorized to list cluster roles")
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if handleAuthzDisabledError(w, err) {
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list cluster roles", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, roles)
}

// GetClusterRole handles GET /api/v1/authz/clusterroles/{name}
func (h *Handler) GetClusterRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Cluster role name is required", services.CodeInvalidInput)
		return
	}

	roleRef := &authz.RoleRef{Name: name, Namespace: ""}
	role, err := h.services.AuthzService.GetRoleByRef(r.Context(), roleRef)
	if err != nil {
		h.logger.Debug("Failed to get cluster role", "error", err, "role", name)
		if errors.Is(err, services.ErrForbidden) {
			h.logger.Warn("Unauthorized to view cluster role", "role", name)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if handleAuthzDisabledError(w, err) {
			return
		}
		if errors.Is(err, authz.ErrRoleNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Cluster role not found", services.CodeNotFound)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get cluster role", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, role)
}

// CreateClusterRole handles POST /api/v1/authz/clusterroles
func (h *Handler) CreateClusterRole(w http.ResponseWriter, r *http.Request) {
	var req gen.CreateClusterRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	if req.Name == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Cluster role name is required", services.CodeInvalidInput)
		return
	}

	ctx := r.Context()
	setAuditResource(ctx, "cluster_role", req.Name, req.Name)

	role := &authz.Role{
		Name:        req.Name,
		Actions:     req.Actions,
		Namespace:   "",
		Description: getStringValue(req.Description),
	}

	if err := h.services.AuthzService.AddRole(ctx, role); err != nil {
		h.logger.Debug("Failed to create cluster role", "error", err, "role", req.Name)
		if errors.Is(err, services.ErrForbidden) {
			h.logger.Warn("Unauthorized to create cluster role", "role", req.Name)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if handleAuthzDisabledError(w, err) {
			return
		}
		if errors.Is(err, authz.ErrRoleAlreadyExists) {
			writeErrorResponse(w, http.StatusConflict, authz.ErrRoleAlreadyExists.Error(), services.CodeConflict)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to create cluster role", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusCreated, role)
}

// UpdateClusterRole handles PUT /api/v1/authz/clusterroles/{name}
func (h *Handler) UpdateClusterRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Cluster role name is required", services.CodeInvalidInput)
		return
	}

	var req gen.UpdateClusterRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	ctx := r.Context()
	setAuditResource(ctx, "cluster_role", name, name)

	role := &authz.Role{
		Name:        name,
		Actions:     req.Actions,
		Namespace:   "",
		Description: getStringValue(req.Description),
	}

	if err := h.services.AuthzService.UpdateRole(ctx, role); err != nil {
		h.logger.Debug("Failed to update cluster role", "error", err, "role", name)
		if handleAuthzDisabledError(w, err) {
			return
		}
		if errors.Is(err, services.ErrForbidden) {
			h.logger.Warn("Unauthorized to update cluster role", "role", name)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, authz.ErrRoleNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Cluster role not found", services.CodeNotFound)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to update cluster role", services.CodeInternalError)
		return
	}

	addAuditMetadata(ctx, "actions", req.Actions)
	writeSuccessResponse(w, http.StatusOK, role)
}

// DeleteClusterRole handles DELETE /api/v1/authz/clusterroles/{name}
func (h *Handler) DeleteClusterRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Cluster role name is required", services.CodeInvalidInput)
		return
	}

	ctx := r.Context()
	setAuditResource(ctx, "cluster_role", name, name)

	roleRef := &authz.RoleRef{Name: name, Namespace: ""}

	if err := h.services.AuthzService.RemoveRoleByRef(ctx, roleRef); err != nil {
		h.logger.Debug("Failed to delete cluster role", "error", err, "role", name)
		if errors.Is(err, services.ErrForbidden) {
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if handleAuthzDisabledError(w, err) {
			return
		}
		if errors.Is(err, authz.ErrRoleNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Cluster role not found", services.CodeNotFound)
			return
		}
		if errors.Is(err, authz.ErrRoleInUse) {
			writeErrorResponse(w, http.StatusConflict, err.Error(), services.CodeConflict)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to delete cluster role", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusNoContent, "")
}

// ListClusterRoleBindings handles GET /api/v1/authz/clusterrolebindings
func (h *Handler) ListClusterRoleBindings(w http.ResponseWriter, r *http.Request) {
	// Extract query parameters
	roleName := r.URL.Query().Get("roleName")
	claim := r.URL.Query().Get("claim")
	value := r.URL.Query().Get("value")
	effect := r.URL.Query().Get("effect")

	// For cluster role bindings, we list mappings where the binding itself is cluster-scoped
	// This means mappings with no namespace in their hierarchy
	mappings, err := h.services.AuthzService.ListClusterRoleMappings(r.Context(), roleName, claim, value, effect)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			h.logger.Warn("Unauthorized to list cluster role bindings")
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		h.logger.Error("Failed to list cluster role bindings", "error", err)
		if handleAuthzDisabledError(w, err) {
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list cluster role bindings", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, mappings)
}

// GetClusterRoleBinding handles GET /api/v1/authz/clusterrolebindings/{name}
func (h *Handler) GetClusterRoleBinding(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Cluster role binding name is required", services.CodeInvalidInput)
		return
	}

	// List all mappings and find by name
	mapping, err := h.services.AuthzService.GetRoleMapping(r.Context(),
		&authz.MappingRef{
			Name:      name,
			Namespace: "",
		})

	if err != nil {
		h.logger.Debug("Failed to get cluster role binding", "error", err, "name", name)
		if errors.Is(err, services.ErrForbidden) {
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, authz.ErrRoleMappingNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Cluster role binding not found", services.CodeNotFound)
			return
		}
		if handleAuthzDisabledError(w, err) {
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get cluster role binding", services.CodeInternalError)
		return
	}
	writeSuccessResponse(w, http.StatusOK, mapping)
}

// CreateClusterRoleBinding handles POST /api/v1/authz/clusterrolebindings
func (h *Handler) CreateClusterRoleBinding(w http.ResponseWriter, r *http.Request) {
	var req gen.CreateClusterRoleBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	if req.Name == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Cluster role binding name is required", services.CodeInvalidInput)
		return
	}

	ctx := r.Context()
	setAuditResource(ctx, "cluster_role_binding", req.Name, req.Name)

	effect := authz.PolicyEffectAllow
	if req.Effect != nil {
		effect = authz.PolicyEffectType(*req.Effect)
	}

	mapping := &authz.RoleEntitlementMapping{
		Name:    req.Name,
		RoleRef: authz.RoleRef{Name: req.Role, Namespace: ""},
		Entitlement: authz.Entitlement{
			Claim: req.Entitlement.Claim,
			Value: req.Entitlement.Value,
		},
		Hierarchy: authz.ResourceHierarchy{},
		Effect:    effect,
	}

	if err := h.services.AuthzService.AddRoleMapping(ctx, mapping); err != nil {
		h.logger.Debug("Failed to create cluster role binding", "error", err, "name", req.Name)
		if errors.Is(err, services.ErrForbidden) {
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if handleAuthzDisabledError(w, err) {
			return
		}
		if errors.Is(err, authz.ErrRoleMappingAlreadyExists) {
			writeErrorResponse(w, http.StatusConflict, authz.ErrRoleMappingAlreadyExists.Error(), services.CodeConflict)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to create cluster role binding", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusCreated, mapping)
}

// UpdateClusterRoleBinding handles PUT /api/v1/authz/clusterrolebindings/{name}
func (h *Handler) UpdateClusterRoleBinding(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Cluster role binding name is required", services.CodeInvalidInput)
		return
	}

	var req gen.UpdateClusterRoleBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	ctx := r.Context()
	setAuditResource(ctx, "cluster_role_binding", name, name)

	mapping := &authz.RoleEntitlementMapping{
		Name:    name,
		RoleRef: authz.RoleRef{Name: req.Role, Namespace: ""},
		Entitlement: authz.Entitlement{
			Claim: req.Entitlement.Claim,
			Value: req.Entitlement.Value,
		},
		Hierarchy: authz.ResourceHierarchy{},
		Effect:    authz.PolicyEffectType(req.Effect),
	}

	if err := h.services.AuthzService.UpdateRoleMapping(ctx, mapping); err != nil {
		h.logger.Error("Failed to update cluster role binding", "error", err, "name", name)
		if handleAuthzDisabledError(w, err) {
			return
		}
		if errors.Is(err, services.ErrForbidden) {
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, authz.ErrRoleMappingNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Cluster role binding not found", services.CodeNotFound)
			return
		}
		if errors.Is(err, authz.ErrCannotModifySystemMapping) {
			writeErrorResponse(w, http.StatusForbidden, authz.ErrCannotModifySystemMapping.Error(), services.CodeForbidden)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to update cluster role binding", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, mapping)
}

// DeleteClusterRoleBinding handles DELETE /api/v1/authz/clusterrolebindings/{name}
func (h *Handler) DeleteClusterRoleBinding(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Cluster role binding name is required", services.CodeInvalidInput)
		return
	}

	ctx := r.Context()
	setAuditResource(ctx, "cluster_role_binding", name, name)

	mappingRef := &authz.MappingRef{Name: name, Namespace: ""}
	if err := h.services.AuthzService.RemoveRoleMapping(ctx, mappingRef); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			h.logger.Debug("Failed to delete cluster role binding", "error", err, "name", name)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if handleAuthzDisabledError(w, err) {
			return
		}
		if errors.Is(err, authz.ErrRoleMappingNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Cluster role binding not found", services.CodeNotFound)
			return
		}
		if errors.Is(err, authz.ErrCannotDeleteSystemMapping) {
			writeErrorResponse(w, http.StatusForbidden, authz.ErrCannotDeleteSystemMapping.Error(), services.CodeForbidden)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to delete cluster role binding", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusNoContent, "")
}

// ListNamespaceRoles handles GET /api/v1/namespaces/{namespaceName}/authz/roles
func (h *Handler) ListNamespaceRoles(w http.ResponseWriter, r *http.Request) {
	namespaceName := r.PathValue("namespaceName")
	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	roles, err := h.services.AuthzService.ListNamespaceRoles(r.Context(), namespaceName)
	if err != nil {
		h.logger.Debug("Failed to list namespace roles", "error", err, "namespace", namespaceName)
		if errors.Is(err, services.ErrForbidden) {
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if handleAuthzDisabledError(w, err) {
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list namespace roles", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, roles)
}

// GetNamespaceRole handles GET /api/v1/namespaces/{namespaceName}/authz/roles/{name}
func (h *Handler) GetNamespaceRole(w http.ResponseWriter, r *http.Request) {
	namespaceName := r.PathValue("namespaceName")
	name := r.PathValue("name")
	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}
	if name == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Role name is required", services.CodeInvalidInput)
		return
	}

	roleRef := &authz.RoleRef{Name: name, Namespace: namespaceName}
	role, err := h.services.AuthzService.GetRoleByRef(r.Context(), roleRef)
	if err != nil {
		h.logger.Debug("Failed to get namespace role", "error", err, "namespace", namespaceName, "role", name)
		if errors.Is(err, services.ErrForbidden) {
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if handleAuthzDisabledError(w, err) {
			return
		}
		if errors.Is(err, authz.ErrRoleNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Namespace role not found", services.CodeNotFound)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get namespace role", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, role)
}

// CreateNamespaceRole handles POST /api/v1/namespaces/{namespaceName}/authz/roles
func (h *Handler) CreateNamespaceRole(w http.ResponseWriter, r *http.Request) {
	namespaceName := r.PathValue("namespaceName")
	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	var req gen.CreateNamespaceRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	if req.Name == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Role name is required", services.CodeInvalidInput)
		return
	}

	ctx := r.Context()
	setAuditResource(ctx, "namespace_role", req.Name, req.Name)

	role := &authz.Role{
		Name:        req.Name,
		Actions:     req.Actions,
		Namespace:   namespaceName,
		Description: getStringValue(req.Description),
	}

	if err := h.services.AuthzService.AddRole(ctx, role); err != nil {
		h.logger.Debug("Failed to create namespace role", "error", err, "namespace", namespaceName, "role", req.Name)
		if errors.Is(err, services.ErrForbidden) {
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if handleAuthzDisabledError(w, err) {
			return
		}
		if errors.Is(err, authz.ErrRoleAlreadyExists) {
			writeErrorResponse(w, http.StatusConflict, authz.ErrRoleAlreadyExists.Error(), services.CodeConflict)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to create namespace role", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusCreated, role)
}

// UpdateNamespaceRole handles PUT /api/v1/namespaces/{namespaceName}/authz/roles/{name}
func (h *Handler) UpdateNamespaceRole(w http.ResponseWriter, r *http.Request) {
	namespaceName := r.PathValue("namespaceName")
	name := r.PathValue("name")
	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}
	if name == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Role name is required", services.CodeInvalidInput)
		return
	}

	var req gen.UpdateNamespaceRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	ctx := r.Context()
	setAuditResource(ctx, "namespace_role", name, name)

	role := &authz.Role{
		Name:        name,
		Actions:     req.Actions,
		Namespace:   namespaceName,
		Description: getStringValue(req.Description),
	}

	if err := h.services.AuthzService.UpdateRole(ctx, role); err != nil {
		h.logger.Error("Failed to update namespace role", "error", err, "namespace", namespaceName, "role", name)
		if handleAuthzDisabledError(w, err) {
			return
		}
		if errors.Is(err, services.ErrForbidden) {
			h.logger.Warn("Unauthorized to update namespace role", "namespace", namespaceName, "role", name)
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, authz.ErrRoleNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Namespace role not found", services.CodeNotFound)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to update namespace role", services.CodeInternalError)
		return
	}

	addAuditMetadata(ctx, "actions", req.Actions)
	writeSuccessResponse(w, http.StatusOK, role)
}

// DeleteNamespaceRole handles DELETE /api/v1/namespaces/{namespaceName}/authz/roles/{name}
func (h *Handler) DeleteNamespaceRole(w http.ResponseWriter, r *http.Request) {
	namespaceName := r.PathValue("namespaceName")
	name := r.PathValue("name")

	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}
	if name == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Role name is required", services.CodeInvalidInput)
		return
	}

	ctx := r.Context()
	setAuditResource(ctx, "namespace_role", name, name)

	roleRef := &authz.RoleRef{Name: name, Namespace: namespaceName}

	if err := h.services.AuthzService.RemoveRoleByRef(ctx, roleRef); err != nil {
		h.logger.Debug("Failed to delete namespace role", "error", err, "namespace", namespaceName, "role", name)
		if errors.Is(err, services.ErrForbidden) {
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if handleAuthzDisabledError(w, err) {
			return
		}
		if errors.Is(err, authz.ErrRoleNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Namespace role not found", services.CodeNotFound)
			return
		}
		if errors.Is(err, authz.ErrRoleInUse) {
			writeErrorResponse(w, http.StatusConflict, err.Error(), services.CodeConflict)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to delete namespace role", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusNoContent, "")
}

// ListNamespaceRoleBindings handles GET /api/v1/namespaces/{namespaceName}/authz/rolebindings
func (h *Handler) ListNamespaceRoleBindings(w http.ResponseWriter, r *http.Request) {
	namespaceName := r.PathValue("namespaceName")
	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	roleName := r.URL.Query().Get("roleName")
	roleNamespace := r.URL.Query().Get("roleNamespace")
	claim := r.URL.Query().Get("claim")
	value := r.URL.Query().Get("value")
	effect := r.URL.Query().Get("effect")

	// List all mappings and filter by namespace
	mappings, err := h.services.AuthzService.ListNamespacedRoleMappings(r.Context(),
		namespaceName,
		&authz.RoleRef{Name: roleName, Namespace: roleNamespace},
		claim,
		value,
		effect)
	if err != nil {
		h.logger.Debug("Failed to list namespace role bindings", "error", err, "namespace", namespaceName)
		if errors.Is(err, services.ErrForbidden) {
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if handleAuthzDisabledError(w, err) {
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to list namespace role bindings", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, mappings)
}

// GetNamespaceRoleBinding handles GET /api/v1/namespaces/{namespaceName}/authz/rolebindings/{name}
func (h *Handler) GetNamespaceRoleBinding(w http.ResponseWriter, r *http.Request) {
	namespaceName := r.PathValue("namespaceName")
	name := r.PathValue("name")
	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}
	if name == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Role binding name is required", services.CodeInvalidInput)
		return
	}

	// List all mappings and find by name and namespace
	mappings, err := h.services.AuthzService.GetRoleMapping(r.Context(),
		&authz.MappingRef{
			Name:      name,
			Namespace: namespaceName,
		})
	if err != nil {
		h.logger.Debug("Failed to get namespace role binding", "error", err, "namespace", namespaceName, "name", name)
		if errors.Is(err, services.ErrForbidden) {
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, authz.ErrRoleMappingNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Namespace role binding not found", services.CodeNotFound)
			return
		}
		if handleAuthzDisabledError(w, err) {
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get namespace role binding", services.CodeInternalError)
		return
	}
	writeSuccessResponse(w, http.StatusOK, mappings)
}

// CreateNamespaceRoleBinding handles POST /api/v1/namespaces/{namespaceName}/authz/rolebindings
func (h *Handler) CreateNamespaceRoleBinding(w http.ResponseWriter, r *http.Request) {
	namespaceName := r.PathValue("namespaceName")
	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}

	var req gen.CreateNamespaceRoleBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	if req.Name == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Role binding name is required", services.CodeInvalidInput)
		return
	}
	roleNs := getStringValue(req.Role.Namespace)
	if roleNs != "" && roleNs != namespaceName {
		writeErrorResponse(w, http.StatusBadRequest, "Role reference namespace does not match the target namespace", services.CodeInvalidInput)
		return
	}

	ctx := r.Context()
	setAuditResource(ctx, "namespace_role_binding", req.Name, req.Name)

	effect := authz.PolicyEffectAllow
	if req.Effect != nil {
		effect = authz.PolicyEffectType(*req.Effect)
	}

	targetPathComponent := ""
	targetPathProject := ""
	if req.TargetPath != nil {
		if req.TargetPath.Project != nil {
			targetPathProject = *req.TargetPath.Project
		}
		if req.TargetPath.Component != nil {
			targetPathComponent = *req.TargetPath.Component
		}
	}
	hierarchy := authz.ResourceHierarchy{
		Namespace: namespaceName,
		Project:   targetPathProject,
		Component: targetPathComponent,
	}
	mapping := &authz.RoleEntitlementMapping{
		Name: req.Name,
		RoleRef: authz.RoleRef{
			Name:      req.Role.Name,
			Namespace: getStringValue(req.Role.Namespace),
		},
		Entitlement: authz.Entitlement{
			Claim: req.Entitlement.Claim,
			Value: req.Entitlement.Value,
		},
		Hierarchy: hierarchy,
		Effect:    effect,
	}

	if err := h.services.AuthzService.AddRoleMapping(ctx, mapping); err != nil {
		h.logger.Error("Failed to create namespace role binding", "error", err, "namespace", namespaceName, "name", req.Name)
		if errors.Is(err, services.ErrForbidden) {
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if handleAuthzDisabledError(w, err) {
			return
		}
		if errors.Is(err, authz.ErrRoleMappingAlreadyExists) {
			writeErrorResponse(w, http.StatusConflict, authz.ErrRoleMappingAlreadyExists.Error(), services.CodeConflict)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to create namespace role binding", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusCreated, mapping)
}

// UpdateNamespaceRoleBinding handles PUT /api/v1/namespaces/{namespaceName}/authz/rolebindings/{name}
func (h *Handler) UpdateNamespaceRoleBinding(w http.ResponseWriter, r *http.Request) {
	namespaceName := r.PathValue("namespaceName")
	name := r.PathValue("name")
	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}
	if name == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Role binding name is required", services.CodeInvalidInput)
		return
	}

	var req gen.UpdateNamespaceRoleBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body", services.CodeInvalidInput)
		return
	}

	ctx := r.Context()
	setAuditResource(ctx, "namespace_role_binding", name, name)

	targetPathComponent := ""
	targetPathProject := ""
	if req.TargetPath.Project != nil {
		targetPathProject = *req.TargetPath.Project
	}
	if req.TargetPath.Component != nil {
		targetPathComponent = *req.TargetPath.Component
	}

	hierarchy := authz.ResourceHierarchy{
		Namespace: namespaceName,
		Project:   targetPathProject,
		Component: targetPathComponent,
	}
	mapping := &authz.RoleEntitlementMapping{
		Name: name,
		RoleRef: authz.RoleRef{
			Name:      req.Role.Name,
			Namespace: getStringValue(req.Role.Namespace),
		},
		Entitlement: authz.Entitlement{
			Claim: req.Entitlement.Claim,
			Value: req.Entitlement.Value,
		},
		Hierarchy: hierarchy,
		Effect:    authz.PolicyEffectType(req.Effect),
	}

	if err := h.services.AuthzService.UpdateRoleMapping(ctx, mapping); err != nil {
		h.logger.Debug("Failed to update namespace role binding", "error", err, "namespace", namespaceName, "name", name)
		if handleAuthzDisabledError(w, err) {
			return
		}
		if errors.Is(err, services.ErrForbidden) {
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if errors.Is(err, authz.ErrRoleMappingNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Namespace role binding not found", services.CodeNotFound)
			return
		}
		if errors.Is(err, authz.ErrCannotModifySystemMapping) {
			writeErrorResponse(w, http.StatusForbidden, authz.ErrCannotModifySystemMapping.Error(), services.CodeForbidden)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to update namespace role binding", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusOK, mapping)
}

// DeleteNamespaceRoleBinding handles DELETE /api/v1/namespaces/{namespaceName}/authz/rolebindings/{name}
func (h *Handler) DeleteNamespaceRoleBinding(w http.ResponseWriter, r *http.Request) {
	namespaceName := r.PathValue("namespaceName")
	name := r.PathValue("name")
	if namespaceName == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Namespace name is required", services.CodeInvalidInput)
		return
	}
	if name == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Role binding name is required", services.CodeInvalidInput)
		return
	}

	ctx := r.Context()
	setAuditResource(ctx, "namespace_role_binding", name, name)

	mappingRef := &authz.MappingRef{Name: name, Namespace: namespaceName}
	if err := h.services.AuthzService.RemoveRoleMapping(ctx, mappingRef); err != nil {
		h.logger.Debug("Failed to delete namespace role binding", "error", err, "namespace", namespaceName, "name", name)
		if errors.Is(err, services.ErrForbidden) {
			writeErrorResponse(w, http.StatusForbidden, services.ErrForbidden.Error(), services.CodeForbidden)
			return
		}
		if handleAuthzDisabledError(w, err) {
			return
		}
		if errors.Is(err, authz.ErrRoleMappingNotFound) {
			writeErrorResponse(w, http.StatusNotFound, "Namespace role binding not found", services.CodeNotFound)
			return
		}
		if errors.Is(err, authz.ErrCannotDeleteSystemMapping) {
			writeErrorResponse(w, http.StatusForbidden, authz.ErrCannotDeleteSystemMapping.Error(), services.CodeForbidden)
			return
		}
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to delete namespace role binding", services.CodeInternalError)
		return
	}

	writeSuccessResponse(w, http.StatusNoContent, "")
}
