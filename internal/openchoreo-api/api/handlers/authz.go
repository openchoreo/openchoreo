// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	authzsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/authz"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// Helper functions to safely dereference pointers
func getStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ListActions returns all defined authorization actions
func (h *Handler) ListActions(
	ctx context.Context,
	request gen.ListActionsRequestObject,
) (gen.ListActionsResponseObject, error) {
	h.logger.Debug("ListActions handler called")

	actions, err := h.services.AuthzService.ListActions(ctx)
	if err != nil {
		h.logger.Error("Failed to list actions", "error", err)
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListActions403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authz.ErrAuthzDisabled) {
			return gen.ListActions403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		return gen.ListActions500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Debug("Listed actions successfully", "count", len(actions))
	return gen.ListActions200JSONResponse(actions), nil
}

// Evaluate evaluates a single authorization request
func (h *Handler) Evaluate(
	ctx context.Context,
	request gen.EvaluateRequestObject,
) (gen.EvaluateResponseObject, error) {
	h.logger.Debug("Evaluate handler called", "action", request.Body.Action)

	// Convert API request to internal model
	evalReq := &authz.EvaluateRequest{
		Action: request.Body.Action,
		Resource: authz.Resource{
			Type: request.Body.Resource.Type,
			ID:   getStringValue(request.Body.Resource.Id),
			Hierarchy: authz.ResourceHierarchy{
				Namespace: getStringValue(request.Body.Resource.Hierarchy.Namespace),
				Project:   getStringValue(request.Body.Resource.Hierarchy.Project),
				Component: getStringValue(request.Body.Resource.Hierarchy.Component),
			},
		},
		SubjectContext: &authz.SubjectContext{
			Type:              string(request.Body.SubjectContext.Type),
			EntitlementClaim:  request.Body.SubjectContext.EntitlementClaim,
			EntitlementValues: request.Body.SubjectContext.EntitlementValues,
		},
	}

	decision, err := h.services.AuthzService.Evaluate(ctx, evalReq)
	if err != nil {
		h.logger.Error("Failed to evaluate", "error", err)
		if errors.Is(err, authz.ErrInvalidRequest) {
			return gen.Evaluate400JSONResponse{BadRequestJSONResponse: badRequest(err.Error())}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.Evaluate400JSONResponse{BadRequestJSONResponse: badRequest(services.ErrForbidden.Error())}, nil
		}
		return gen.Evaluate500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert internal decision to API response
	response := gen.Decision{
		Decision: decision.Decision,
	}
	if decision.Context != nil && decision.Context.Reason != "" {
		response.Context = &struct {
			Reason *string `json:"reason,omitempty"`
		}{
			Reason: &decision.Context.Reason,
		}
	}

	h.logger.Debug("Evaluation completed", "decision", decision.Decision)
	return gen.Evaluate200JSONResponse(response), nil
}

// BatchEvaluate evaluates multiple authorization requests
func (h *Handler) BatchEvaluate(
	ctx context.Context,
	request gen.BatchEvaluateRequestObject,
) (gen.BatchEvaluateResponseObject, error) {
	h.logger.Debug("BatchEvaluate handler called", "count", len(request.Body.Requests))

	// Convert API requests to internal model
	internalRequests := make([]authz.EvaluateRequest, len(request.Body.Requests))
	for i, req := range request.Body.Requests {
		internalRequests[i] = authz.EvaluateRequest{
			Action: req.Action,
			Resource: authz.Resource{
				Type: req.Resource.Type,
				ID:   getStringValue(req.Resource.Id),
				Hierarchy: authz.ResourceHierarchy{
					Namespace: getStringValue(req.Resource.Hierarchy.Namespace),
					Project:   getStringValue(req.Resource.Hierarchy.Project),
					Component: getStringValue(req.Resource.Hierarchy.Component),
				},
			},
			SubjectContext: &authz.SubjectContext{
				Type:              string(req.SubjectContext.Type),
				EntitlementClaim:  req.SubjectContext.EntitlementClaim,
				EntitlementValues: req.SubjectContext.EntitlementValues,
			},
		}
	}

	batchReq := &authz.BatchEvaluateRequest{
		Requests: internalRequests,
	}

	batchResp, err := h.services.AuthzService.BatchEvaluate(ctx, batchReq)
	if err != nil {
		h.logger.Error("Failed to batch evaluate", "error", err)
		if errors.Is(err, authz.ErrInvalidRequest) {
			return gen.BatchEvaluate400JSONResponse{BadRequestJSONResponse: badRequest(err.Error())}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.BatchEvaluate400JSONResponse{BadRequestJSONResponse: badRequest(services.ErrForbidden.Error())}, nil
		}
		return gen.BatchEvaluate500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert internal decisions to API response
	decisions := make([]gen.Decision, len(batchResp.Decisions))
	for i, decision := range batchResp.Decisions {
		decisions[i] = gen.Decision{
			Decision: decision.Decision,
		}
		if decision.Context != nil && decision.Context.Reason != "" {
			decisions[i].Context = &struct {
				Reason *string `json:"reason,omitempty"`
			}{
				Reason: &decision.Context.Reason,
			}
		}
	}

	h.logger.Debug("Batch evaluation completed", "count", len(decisions))
	return gen.BatchEvaluate200JSONResponse{Decisions: decisions}, nil
}

// GetSubjectProfile returns the authorization profile for the authenticated subject
func (h *Handler) GetSubjectProfile(
	ctx context.Context,
	request gen.GetSubjectProfileRequestObject,
) (gen.GetSubjectProfileResponseObject, error) {
	h.logger.Debug("GetSubjectProfile handler called")

	// Extract subject context from the request context
	subjectCtx, ok := auth.GetSubjectContextFromContext(ctx)
	if !ok || subjectCtx == nil {
		return gen.GetSubjectProfile403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	}

	// Convert API request to internal model
	profileReq := &authz.ProfileRequest{
		SubjectContext: authz.GetAuthzSubjectContext(subjectCtx),
		Scope: authz.ResourceHierarchy{
			Namespace: getStringValue(request.Params.Namespace),
			Project:   getStringValue(request.Params.Project),
			Component: getStringValue(request.Params.Component),
		},
	}

	profile, err := h.services.AuthzService.GetSubjectProfile(ctx, profileReq)
	if err != nil {
		h.logger.Error("Failed to get subject profile", "error", err)
		if errors.Is(err, authz.ErrInvalidRequest) {
			return gen.GetSubjectProfile400JSONResponse{BadRequestJSONResponse: badRequest(err.Error())}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetSubjectProfile403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		return gen.GetSubjectProfile500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert internal profile to API response
	response := gen.UserCapabilitiesResponse{
		EvaluatedAt: &profile.GeneratedAt,
	}

	if profile.User != nil {
		response.User = &gen.SubjectContext{
			Type:              gen.SubjectContextType(profile.User.Type),
			EntitlementClaim:  profile.User.EntitlementClaim,
			EntitlementValues: profile.User.EntitlementValues,
		}
	}

	if profile.Capabilities != nil {
		caps := make(map[string]gen.ActionCapability)
		for action, capability := range profile.Capabilities {
			// Convert CapabilityResource slices
			var allowed *[]gen.CapabilityResource
			var denied *[]gen.CapabilityResource

			if capability.Allowed != nil {
				allowedResources := make([]gen.CapabilityResource, len(capability.Allowed))
				for i, res := range capability.Allowed {
					// Convert constraints type
					var constraints *map[string]interface{}
					if res.Constraints != nil {
						constraintsMap := (*res.Constraints).(map[string]interface{})
						constraints = &constraintsMap
					}
					path := res.Path
					allowedResources[i] = gen.CapabilityResource{
						Constraints: constraints,
						Path:        &path,
					}
				}
				allowed = &allowedResources
			}

			if capability.Denied != nil {
				deniedResources := make([]gen.CapabilityResource, len(capability.Denied))
				for i, res := range capability.Denied {
					// Convert constraints type
					var constraints *map[string]interface{}
					if res.Constraints != nil {
						constraintsMap := (*res.Constraints).(map[string]interface{})
						constraints = &constraintsMap
					}
					path := res.Path
					deniedResources[i] = gen.CapabilityResource{
						Constraints: constraints,
						Path:        &path,
					}
				}
				denied = &deniedResources
			}

			caps[action] = gen.ActionCapability{
				Allowed: allowed,
				Denied:  denied,
			}
		}
		response.Capabilities = &caps
	}

	h.logger.Debug("Retrieved subject profile successfully")
	return gen.GetSubjectProfile200JSONResponse(response), nil
}

// ListUserTypes returns the configured user types
func (h *Handler) ListUserTypes(
	ctx context.Context,
	request gen.ListUserTypesRequestObject,
) (gen.ListUserTypesResponseObject, error) {
	h.logger.Debug("ListUserTypes handler called")

	userTypes := h.Config.Security.ToSubjectUserTypeConfigs()

	// Convert subject.UserTypeConfig to gen.UserTypeConfig
	genUserTypes := make([]gen.UserTypeConfig, len(userTypes))
	for i, ut := range userTypes {
		authMechanisms := make([]gen.AuthMechanismConfig, len(ut.AuthMechanisms))
		for j, am := range ut.AuthMechanisms {
			authMechanisms[j] = gen.AuthMechanismConfig{
				Type: am.Type,
				Entitlement: gen.EntitlementConfig{
					Claim:       am.Entitlement.Claim,
					DisplayName: am.Entitlement.DisplayName,
				},
			}
		}

		genUserTypes[i] = gen.UserTypeConfig{
			Type:           ut.Type,
			DisplayName:    ut.DisplayName,
			Priority:       ut.Priority,
			AuthMechanisms: authMechanisms,
		}
	}

	h.logger.Debug("Listed user types successfully", "count", len(genUserTypes))
	return gen.ListUserTypes200JSONResponse(genUserTypes), nil
}

// --- Cluster Roles ---

// ListClusterRoles returns all cluster-scoped roles.
func (h *Handler) ListClusterRoles(
	ctx context.Context,
	request gen.ListClusterRolesRequestObject,
) (gen.ListClusterRolesResponseObject, error) {
	h.logger.Debug("ListClusterRoles called")

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)
	result, err := h.authzService.ListClusterRoles(ctx, opts)
	if err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			return gen.ListClusterRoles403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to list cluster roles", "error", err)
		return gen.ListClusterRoles500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.AuthzClusterRole, gen.AuthzClusterRole](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert cluster roles", "error", err)
		return gen.ListClusterRoles500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListClusterRoles200JSONResponse{Items: items, Pagination: ToPaginationPtr(result)}, nil
}

// CreateClusterRole creates a new cluster-scoped role.
func (h *Handler) CreateClusterRole(
	ctx context.Context,
	request gen.CreateClusterRoleRequestObject,
) (gen.CreateClusterRoleResponseObject, error) {
	h.logger.Info("CreateClusterRole called")

	if request.Body == nil {
		return gen.CreateClusterRole400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	roleCR, err := convert[gen.AuthzClusterRole, openchoreov1alpha1.AuthzClusterRole](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateClusterRole400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	created, err := h.authzService.CreateClusterRole(ctx, &roleCR)
	if err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			return gen.CreateClusterRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authzsvc.ErrRoleAlreadyExists) {
			return gen.CreateClusterRole409JSONResponse{ConflictJSONResponse: conflict("Cluster role already exists")}, nil
		}
		h.logger.Error("Failed to create cluster role", "error", err)
		return gen.CreateClusterRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRole, err := convert[openchoreov1alpha1.AuthzClusterRole, gen.AuthzClusterRole](*created)
	if err != nil {
		h.logger.Error("Failed to convert created cluster role", "error", err)
		return gen.CreateClusterRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster role created successfully", "name", created.Name)
	return gen.CreateClusterRole201JSONResponse(genRole), nil
}

// GetClusterRole returns details of a specific cluster role.
func (h *Handler) GetClusterRole(
	ctx context.Context,
	request gen.GetClusterRoleRequestObject,
) (gen.GetClusterRoleResponseObject, error) {
	h.logger.Debug("GetClusterRole called", "name", request.Name)

	role, err := h.authzService.GetClusterRole(ctx, request.Name)
	if err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			return gen.GetClusterRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authzsvc.ErrRoleNotFound) {
			return gen.GetClusterRole404JSONResponse{NotFoundJSONResponse: notFound("Cluster role")}, nil
		}
		h.logger.Error("Failed to get cluster role", "error", err)
		return gen.GetClusterRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRole, err := convert[openchoreov1alpha1.AuthzClusterRole, gen.AuthzClusterRole](*role)
	if err != nil {
		h.logger.Error("Failed to convert cluster role", "error", err)
		return gen.GetClusterRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetClusterRole200JSONResponse(genRole), nil
}

// UpdateClusterRole updates an existing cluster role.
func (h *Handler) UpdateClusterRole(
	ctx context.Context,
	request gen.UpdateClusterRoleRequestObject,
) (gen.UpdateClusterRoleResponseObject, error) {
	h.logger.Info("UpdateClusterRole called", "name", request.Name)

	if request.Body == nil {
		return gen.UpdateClusterRole400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	roleCR, err := convert[gen.AuthzClusterRole, openchoreov1alpha1.AuthzClusterRole](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateClusterRole400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	roleCR.Name = request.Name

	updated, err := h.authzService.UpdateClusterRole(ctx, &roleCR)
	if err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			return gen.UpdateClusterRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authzsvc.ErrRoleNotFound) {
			return gen.UpdateClusterRole404JSONResponse{NotFoundJSONResponse: notFound("Cluster role")}, nil
		}
		h.logger.Error("Failed to update cluster role", "error", err)
		return gen.UpdateClusterRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRole, err := convert[openchoreov1alpha1.AuthzClusterRole, gen.AuthzClusterRole](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated cluster role", "error", err)
		return gen.UpdateClusterRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster role updated successfully", "name", updated.Name)
	return gen.UpdateClusterRole200JSONResponse(genRole), nil
}

// DeleteClusterRole deletes a cluster role.
func (h *Handler) DeleteClusterRole(
	ctx context.Context,
	request gen.DeleteClusterRoleRequestObject,
) (gen.DeleteClusterRoleResponseObject, error) {
	h.logger.Info("DeleteClusterRole called", "name", request.Name)

	err := h.authzService.DeleteClusterRole(ctx, request.Name)
	if err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			return gen.DeleteClusterRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authzsvc.ErrRoleNotFound) {
			return gen.DeleteClusterRole404JSONResponse{NotFoundJSONResponse: notFound("Cluster role")}, nil
		}
		if errors.Is(err, authzsvc.ErrRoleInUse) {
			return gen.DeleteClusterRole409JSONResponse{ConflictJSONResponse: conflict("Cluster role is in use by role bindings")}, nil
		}
		h.logger.Error("Failed to delete cluster role", "error", err)
		return gen.DeleteClusterRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster role deleted successfully", "name", request.Name)
	return gen.DeleteClusterRole204Response{}, nil
}

// --- Cluster Role Bindings ---

// ListClusterRoleBindings returns all cluster-scoped role bindings.
func (h *Handler) ListClusterRoleBindings(
	ctx context.Context,
	request gen.ListClusterRoleBindingsRequestObject,
) (gen.ListClusterRoleBindingsResponseObject, error) {
	h.logger.Debug("ListClusterRoleBindings called")

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)
	result, err := h.authzService.ListClusterRoleBindings(ctx, opts)
	if err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			return gen.ListClusterRoleBindings403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to list cluster role bindings", "error", err)
		return gen.ListClusterRoleBindings500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.AuthzClusterRoleBinding, gen.AuthzClusterRoleBinding](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert cluster role bindings", "error", err)
		return gen.ListClusterRoleBindings500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListClusterRoleBindings200JSONResponse{Items: items, Pagination: ToPaginationPtr(result)}, nil
}

// CreateClusterRoleBinding creates a new cluster-scoped role binding.
func (h *Handler) CreateClusterRoleBinding(
	ctx context.Context,
	request gen.CreateClusterRoleBindingRequestObject,
) (gen.CreateClusterRoleBindingResponseObject, error) {
	h.logger.Info("CreateClusterRoleBinding called")

	if request.Body == nil {
		return gen.CreateClusterRoleBinding400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	bindingCR, err := convert[gen.AuthzClusterRoleBinding, openchoreov1alpha1.AuthzClusterRoleBinding](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateClusterRoleBinding400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	created, err := h.authzService.CreateClusterRoleBinding(ctx, &bindingCR)
	if err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			return gen.CreateClusterRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authzsvc.ErrRoleBindingAlreadyExists) {
			return gen.CreateClusterRoleBinding409JSONResponse{ConflictJSONResponse: conflict("Cluster role binding already exists")}, nil
		}
		if errors.Is(err, authzsvc.ErrRoleNotFound) {
			return gen.CreateClusterRoleBinding400JSONResponse{BadRequestJSONResponse: badRequest("Referenced role not found")}, nil
		}
		h.logger.Error("Failed to create cluster role binding", "error", err)
		return gen.CreateClusterRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genBinding, err := convert[openchoreov1alpha1.AuthzClusterRoleBinding, gen.AuthzClusterRoleBinding](*created)
	if err != nil {
		h.logger.Error("Failed to convert created cluster role binding", "error", err)
		return gen.CreateClusterRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster role binding created successfully", "name", created.Name)
	return gen.CreateClusterRoleBinding201JSONResponse(genBinding), nil
}

// GetClusterRoleBinding returns details of a specific cluster role binding.
func (h *Handler) GetClusterRoleBinding(
	ctx context.Context,
	request gen.GetClusterRoleBindingRequestObject,
) (gen.GetClusterRoleBindingResponseObject, error) {
	h.logger.Debug("GetClusterRoleBinding called", "name", request.Name)

	binding, err := h.authzService.GetClusterRoleBinding(ctx, request.Name)
	if err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			return gen.GetClusterRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authzsvc.ErrRoleBindingNotFound) {
			return gen.GetClusterRoleBinding404JSONResponse{NotFoundJSONResponse: notFound("Cluster role binding")}, nil
		}
		h.logger.Error("Failed to get cluster role binding", "error", err)
		return gen.GetClusterRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genBinding, err := convert[openchoreov1alpha1.AuthzClusterRoleBinding, gen.AuthzClusterRoleBinding](*binding)
	if err != nil {
		h.logger.Error("Failed to convert cluster role binding", "error", err)
		return gen.GetClusterRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetClusterRoleBinding200JSONResponse(genBinding), nil
}

// UpdateClusterRoleBinding updates an existing cluster role binding.
func (h *Handler) UpdateClusterRoleBinding(
	ctx context.Context,
	request gen.UpdateClusterRoleBindingRequestObject,
) (gen.UpdateClusterRoleBindingResponseObject, error) {
	h.logger.Info("UpdateClusterRoleBinding called", "name", request.Name)

	if request.Body == nil {
		return gen.UpdateClusterRoleBinding400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	bindingCR, err := convert[gen.AuthzClusterRoleBinding, openchoreov1alpha1.AuthzClusterRoleBinding](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateClusterRoleBinding400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	bindingCR.Name = request.Name

	updated, err := h.authzService.UpdateClusterRoleBinding(ctx, &bindingCR)
	if err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			return gen.UpdateClusterRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authzsvc.ErrRoleBindingNotFound) {
			return gen.UpdateClusterRoleBinding404JSONResponse{NotFoundJSONResponse: notFound("Cluster role binding")}, nil
		}
		h.logger.Error("Failed to update cluster role binding", "error", err)
		return gen.UpdateClusterRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genBinding, err := convert[openchoreov1alpha1.AuthzClusterRoleBinding, gen.AuthzClusterRoleBinding](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated cluster role binding", "error", err)
		return gen.UpdateClusterRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster role binding updated successfully", "name", updated.Name)
	return gen.UpdateClusterRoleBinding200JSONResponse(genBinding), nil
}

// DeleteClusterRoleBinding deletes a cluster role binding.
func (h *Handler) DeleteClusterRoleBinding(
	ctx context.Context,
	request gen.DeleteClusterRoleBindingRequestObject,
) (gen.DeleteClusterRoleBindingResponseObject, error) {
	h.logger.Info("DeleteClusterRoleBinding called", "name", request.Name)

	err := h.authzService.DeleteClusterRoleBinding(ctx, request.Name)
	if err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			return gen.DeleteClusterRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authzsvc.ErrRoleBindingNotFound) {
			return gen.DeleteClusterRoleBinding404JSONResponse{NotFoundJSONResponse: notFound("Cluster role binding")}, nil
		}
		h.logger.Error("Failed to delete cluster role binding", "error", err)
		return gen.DeleteClusterRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster role binding deleted successfully", "name", request.Name)
	return gen.DeleteClusterRoleBinding204Response{}, nil
}

// --- Namespace Roles ---

// ListNamespaceRoles returns all namespace-scoped roles.
func (h *Handler) ListNamespaceRoles(
	ctx context.Context,
	request gen.ListNamespaceRolesRequestObject,
) (gen.ListNamespaceRolesResponseObject, error) {
	h.logger.Debug("ListNamespaceRoles called", "namespace", request.NamespaceName)

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)
	result, err := h.authzService.ListNamespaceRoles(ctx, request.NamespaceName, opts)
	if err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			return gen.ListNamespaceRoles403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to list namespace roles", "error", err)
		return gen.ListNamespaceRoles500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.AuthzRole, gen.AuthzRole](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert namespace roles", "error", err)
		return gen.ListNamespaceRoles500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListNamespaceRoles200JSONResponse{Items: items, Pagination: ToPaginationPtr(result)}, nil
}

// CreateNamespaceRole creates a new namespace-scoped role.
func (h *Handler) CreateNamespaceRole(
	ctx context.Context,
	request gen.CreateNamespaceRoleRequestObject,
) (gen.CreateNamespaceRoleResponseObject, error) {
	h.logger.Info("CreateNamespaceRole called", "namespace", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateNamespaceRole400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	roleCR, err := convert[gen.AuthzRole, openchoreov1alpha1.AuthzRole](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateNamespaceRole400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	created, err := h.authzService.CreateNamespaceRole(ctx, request.NamespaceName, &roleCR)
	if err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			return gen.CreateNamespaceRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authzsvc.ErrRoleAlreadyExists) {
			return gen.CreateNamespaceRole409JSONResponse{ConflictJSONResponse: conflict("Namespace role already exists")}, nil
		}
		h.logger.Error("Failed to create namespace role", "error", err)
		return gen.CreateNamespaceRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRole, err := convert[openchoreov1alpha1.AuthzRole, gen.AuthzRole](*created)
	if err != nil {
		h.logger.Error("Failed to convert created namespace role", "error", err)
		return gen.CreateNamespaceRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Namespace role created successfully", "namespace", request.NamespaceName, "name", created.Name)
	return gen.CreateNamespaceRole201JSONResponse(genRole), nil
}

// GetNamespaceRole returns details of a specific namespace role.
func (h *Handler) GetNamespaceRole(
	ctx context.Context,
	request gen.GetNamespaceRoleRequestObject,
) (gen.GetNamespaceRoleResponseObject, error) {
	h.logger.Debug("GetNamespaceRole called", "namespace", request.NamespaceName, "name", request.Name)

	role, err := h.authzService.GetNamespaceRole(ctx, request.NamespaceName, request.Name)
	if err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			return gen.GetNamespaceRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authzsvc.ErrRoleNotFound) {
			return gen.GetNamespaceRole404JSONResponse{NotFoundJSONResponse: notFound("Namespace role")}, nil
		}
		h.logger.Error("Failed to get namespace role", "error", err)
		return gen.GetNamespaceRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRole, err := convert[openchoreov1alpha1.AuthzRole, gen.AuthzRole](*role)
	if err != nil {
		h.logger.Error("Failed to convert namespace role", "error", err)
		return gen.GetNamespaceRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetNamespaceRole200JSONResponse(genRole), nil
}

// UpdateNamespaceRole updates an existing namespace role.
func (h *Handler) UpdateNamespaceRole(
	ctx context.Context,
	request gen.UpdateNamespaceRoleRequestObject,
) (gen.UpdateNamespaceRoleResponseObject, error) {
	h.logger.Info("UpdateNamespaceRole called", "namespace", request.NamespaceName, "name", request.Name)

	if request.Body == nil {
		return gen.UpdateNamespaceRole400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	roleCR, err := convert[gen.AuthzRole, openchoreov1alpha1.AuthzRole](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateNamespaceRole400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	roleCR.Name = request.Name

	updated, err := h.authzService.UpdateNamespaceRole(ctx, request.NamespaceName, &roleCR)
	if err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			return gen.UpdateNamespaceRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authzsvc.ErrRoleNotFound) {
			return gen.UpdateNamespaceRole404JSONResponse{NotFoundJSONResponse: notFound("Namespace role")}, nil
		}
		h.logger.Error("Failed to update namespace role", "error", err)
		return gen.UpdateNamespaceRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genRole, err := convert[openchoreov1alpha1.AuthzRole, gen.AuthzRole](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated namespace role", "error", err)
		return gen.UpdateNamespaceRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Namespace role updated successfully", "namespace", request.NamespaceName, "name", updated.Name)
	return gen.UpdateNamespaceRole200JSONResponse(genRole), nil
}

// DeleteNamespaceRole deletes a namespace role.
func (h *Handler) DeleteNamespaceRole(
	ctx context.Context,
	request gen.DeleteNamespaceRoleRequestObject,
) (gen.DeleteNamespaceRoleResponseObject, error) {
	h.logger.Info("DeleteNamespaceRole called", "namespace", request.NamespaceName, "name", request.Name)

	err := h.authzService.DeleteNamespaceRole(ctx, request.NamespaceName, request.Name)
	if err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			return gen.DeleteNamespaceRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authzsvc.ErrRoleNotFound) {
			return gen.DeleteNamespaceRole404JSONResponse{NotFoundJSONResponse: notFound("Namespace role")}, nil
		}
		if errors.Is(err, authzsvc.ErrRoleInUse) {
			return gen.DeleteNamespaceRole409JSONResponse{ConflictJSONResponse: conflict("Namespace role is in use by role bindings")}, nil
		}
		h.logger.Error("Failed to delete namespace role", "error", err)
		return gen.DeleteNamespaceRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Namespace role deleted successfully", "namespace", request.NamespaceName, "name", request.Name)
	return gen.DeleteNamespaceRole204Response{}, nil
}

// --- Namespace Role Bindings ---

// ListNamespaceRoleBindings returns all namespace-scoped role bindings.
func (h *Handler) ListNamespaceRoleBindings(
	ctx context.Context,
	request gen.ListNamespaceRoleBindingsRequestObject,
) (gen.ListNamespaceRoleBindingsResponseObject, error) {
	h.logger.Debug("ListNamespaceRoleBindings called", "namespace", request.NamespaceName)

	opts := NormalizeListOptions(request.Params.Limit, request.Params.Cursor)
	result, err := h.authzService.ListNamespaceRoleBindings(ctx, request.NamespaceName, opts)
	if err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			return gen.ListNamespaceRoleBindings403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to list namespace role bindings", "error", err)
		return gen.ListNamespaceRoleBindings500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	items, err := convertList[openchoreov1alpha1.AuthzRoleBinding, gen.AuthzRoleBinding](result.Items)
	if err != nil {
		h.logger.Error("Failed to convert namespace role bindings", "error", err)
		return gen.ListNamespaceRoleBindings500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.ListNamespaceRoleBindings200JSONResponse{Items: items, Pagination: ToPaginationPtr(result)}, nil
}

// CreateNamespaceRoleBinding creates a new namespace-scoped role binding.
func (h *Handler) CreateNamespaceRoleBinding(
	ctx context.Context,
	request gen.CreateNamespaceRoleBindingRequestObject,
) (gen.CreateNamespaceRoleBindingResponseObject, error) {
	h.logger.Info("CreateNamespaceRoleBinding called", "namespace", request.NamespaceName)

	if request.Body == nil {
		return gen.CreateNamespaceRoleBinding400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	bindingCR, err := convert[gen.AuthzRoleBinding, openchoreov1alpha1.AuthzRoleBinding](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert create request", "error", err)
		return gen.CreateNamespaceRoleBinding400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	created, err := h.authzService.CreateNamespaceRoleBinding(ctx, request.NamespaceName, &bindingCR)
	if err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			return gen.CreateNamespaceRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authzsvc.ErrRoleBindingAlreadyExists) {
			return gen.CreateNamespaceRoleBinding409JSONResponse{ConflictJSONResponse: conflict("Namespace role binding already exists")}, nil
		}
		h.logger.Error("Failed to create namespace role binding", "error", err)
		return gen.CreateNamespaceRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genBinding, err := convert[openchoreov1alpha1.AuthzRoleBinding, gen.AuthzRoleBinding](*created)
	if err != nil {
		h.logger.Error("Failed to convert created namespace role binding", "error", err)
		return gen.CreateNamespaceRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Namespace role binding created successfully", "namespace", request.NamespaceName, "name", created.Name)
	return gen.CreateNamespaceRoleBinding201JSONResponse(genBinding), nil
}

// GetNamespaceRoleBinding returns details of a specific namespace role binding.
func (h *Handler) GetNamespaceRoleBinding(
	ctx context.Context,
	request gen.GetNamespaceRoleBindingRequestObject,
) (gen.GetNamespaceRoleBindingResponseObject, error) {
	h.logger.Debug("GetNamespaceRoleBinding called", "namespace", request.NamespaceName, "name", request.Name)

	binding, err := h.authzService.GetNamespaceRoleBinding(ctx, request.NamespaceName, request.Name)
	if err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			return gen.GetNamespaceRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authzsvc.ErrRoleBindingNotFound) {
			return gen.GetNamespaceRoleBinding404JSONResponse{NotFoundJSONResponse: notFound("Namespace role binding")}, nil
		}
		h.logger.Error("Failed to get namespace role binding", "error", err)
		return gen.GetNamespaceRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genBinding, err := convert[openchoreov1alpha1.AuthzRoleBinding, gen.AuthzRoleBinding](*binding)
	if err != nil {
		h.logger.Error("Failed to convert namespace role binding", "error", err)
		return gen.GetNamespaceRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetNamespaceRoleBinding200JSONResponse(genBinding), nil
}

// UpdateNamespaceRoleBinding updates an existing namespace role binding.
func (h *Handler) UpdateNamespaceRoleBinding(
	ctx context.Context,
	request gen.UpdateNamespaceRoleBindingRequestObject,
) (gen.UpdateNamespaceRoleBindingResponseObject, error) {
	h.logger.Info("UpdateNamespaceRoleBinding called", "namespace", request.NamespaceName, "name", request.Name)

	if request.Body == nil {
		return gen.UpdateNamespaceRoleBinding400JSONResponse{BadRequestJSONResponse: badRequest("Request body is required")}, nil
	}

	bindingCR, err := convert[gen.AuthzRoleBinding, openchoreov1alpha1.AuthzRoleBinding](*request.Body)
	if err != nil {
		h.logger.Error("Failed to convert update request", "error", err)
		return gen.UpdateNamespaceRoleBinding400JSONResponse{BadRequestJSONResponse: badRequest("Invalid request body")}, nil
	}

	bindingCR.Name = request.Name

	updated, err := h.authzService.UpdateNamespaceRoleBinding(ctx, request.NamespaceName, &bindingCR)
	if err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			return gen.UpdateNamespaceRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authzsvc.ErrRoleBindingNotFound) {
			return gen.UpdateNamespaceRoleBinding404JSONResponse{NotFoundJSONResponse: notFound("Namespace role binding")}, nil
		}
		h.logger.Error("Failed to update namespace role binding", "error", err)
		return gen.UpdateNamespaceRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	genBinding, err := convert[openchoreov1alpha1.AuthzRoleBinding, gen.AuthzRoleBinding](*updated)
	if err != nil {
		h.logger.Error("Failed to convert updated namespace role binding", "error", err)
		return gen.UpdateNamespaceRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Namespace role binding updated successfully", "namespace", request.NamespaceName, "name", updated.Name)
	return gen.UpdateNamespaceRoleBinding200JSONResponse(genBinding), nil
}

// DeleteNamespaceRoleBinding deletes a namespace role binding.
func (h *Handler) DeleteNamespaceRoleBinding(
	ctx context.Context,
	request gen.DeleteNamespaceRoleBindingRequestObject,
) (gen.DeleteNamespaceRoleBindingResponseObject, error) {
	h.logger.Info("DeleteNamespaceRoleBinding called", "namespace", request.NamespaceName, "name", request.Name)

	err := h.authzService.DeleteNamespaceRoleBinding(ctx, request.NamespaceName, request.Name)
	if err != nil {
		if errors.Is(err, svcpkg.ErrForbidden) {
			return gen.DeleteNamespaceRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authzsvc.ErrRoleBindingNotFound) {
			return gen.DeleteNamespaceRoleBinding404JSONResponse{NotFoundJSONResponse: notFound("Namespace role binding")}, nil
		}
		h.logger.Error("Failed to delete namespace role binding", "error", err)
		return gen.DeleteNamespaceRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Namespace role binding deleted successfully", "namespace", request.NamespaceName, "name", request.Name)
	return gen.DeleteNamespaceRoleBinding204Response{}, nil
}
