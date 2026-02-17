// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"

	authz "github.com/openchoreo/openchoreo/internal/authz/core"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	services "github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices"
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

// ListClusterRoles returns all cluster-scoped roles
func (h *Handler) ListClusterRoles(
	ctx context.Context,
	request gen.ListClusterRolesRequestObject,
) (gen.ListClusterRolesResponseObject, error) {
	h.logger.Debug("ListClusterRoles handler called")

	roles, err := h.services.AuthzService.ListClusterRoles(ctx)
	if err != nil {
		h.logger.Error("Failed to list cluster roles", "error", err)
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListClusterRoles403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authz.ErrAuthzDisabled) {
			return gen.ListClusterRoles403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		return gen.ListClusterRoles500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert to API response format
	apiRoles := make([]gen.Role, len(roles))
	for i, role := range roles {
		apiRoles[i] = gen.Role{
			Actions:     role.Actions,
			Description: &role.Description,
			Name:        role.Name,
			Namespace:   nil,
		}
	}

	h.logger.Debug("Listed cluster roles successfully", "count", len(roles))
	return gen.ListClusterRoles200JSONResponse(apiRoles), nil
}

// CreateClusterRole creates a new cluster-scoped role
func (h *Handler) CreateClusterRole(
	ctx context.Context,
	request gen.CreateClusterRoleRequestObject,
) (gen.CreateClusterRoleResponseObject, error) {
	h.logger.Info("CreateClusterRole handler called", "name", request.Body.Name)

	// Convert request to internal model
	description := ""
	if request.Body.Description != nil {
		description = *request.Body.Description
	}
	role := &authz.Role{
		Name:        request.Body.Name,
		Actions:     request.Body.Actions,
		Description: description,
		Namespace:   "",
	}

	err := h.services.AuthzService.AddRole(ctx, role)
	if err != nil {
		h.logger.Error("Failed to create cluster role", "error", err)
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateClusterRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authz.ErrAuthzDisabled) {
			return gen.CreateClusterRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrRoleAlreadyExists) {
			return gen.CreateClusterRole409JSONResponse{ConflictJSONResponse: conflict("Cluster role already exists")}, nil
		}
		return gen.CreateClusterRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Debug("Cluster role created successfully", "role", request.Body.Name)
	return gen.CreateClusterRole201JSONResponse{
		Actions:     role.Actions,
		Description: &role.Description,
		Name:        role.Name,
		Namespace:   nil,
	}, nil
}

// GetClusterRole returns details of a specific cluster role
func (h *Handler) GetClusterRole(
	ctx context.Context,
	request gen.GetClusterRoleRequestObject,
) (gen.GetClusterRoleResponseObject, error) {
	h.logger.Debug("GetClusterRole handler called", "name", request.Name)

	role, err := h.services.AuthzService.GetRoleByRef(ctx, &authz.RoleRef{Name: request.Name})
	if err != nil {
		h.logger.Error("Failed to get cluster role", "error", err)
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetClusterRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authz.ErrAuthzDisabled) {
			return gen.GetClusterRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrRoleNotFound) {
			return gen.GetClusterRole404JSONResponse{NotFoundJSONResponse: notFound("Cluster role")}, nil
		}
		return gen.GetClusterRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Debug("Retrieved cluster role successfully", "role", request.Name)
	return gen.GetClusterRole200JSONResponse{
		Actions:     role.Actions,
		Description: &role.Description,
		Name:        role.Name,
		Namespace:   nil,
	}, nil
}

// UpdateClusterRole updates an existing cluster role
func (h *Handler) UpdateClusterRole(
	ctx context.Context,
	request gen.UpdateClusterRoleRequestObject,
) (gen.UpdateClusterRoleResponseObject, error) {
	h.logger.Debug("UpdateClusterRole handler called", "name", request.Name)

	// Convert request to internal model
	description := ""
	if request.Body.Description != nil {
		description = *request.Body.Description
	}
	role := &authz.Role{
		Name:        request.Name,
		Actions:     request.Body.Actions,
		Description: description,
		Namespace:   "",
	}

	err := h.services.AuthzService.UpdateRole(ctx, role)
	if err != nil {
		h.logger.Error("Failed to update cluster role", "error", err)
		if errors.Is(err, authz.ErrAuthzDisabled) {
			return gen.UpdateClusterRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateClusterRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrRoleNotFound) {
			return gen.UpdateClusterRole404JSONResponse{NotFoundJSONResponse: notFound("Cluster role")}, nil
		}
		return gen.UpdateClusterRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster role updated successfully", "role", request.Name)
	return gen.UpdateClusterRole200JSONResponse{
		Actions:     role.Actions,
		Description: &role.Description,
		Name:        role.Name,
		Namespace:   nil,
	}, nil
}

// DeleteClusterRole deletes a cluster role
func (h *Handler) DeleteClusterRole(
	ctx context.Context,
	request gen.DeleteClusterRoleRequestObject,
) (gen.DeleteClusterRoleResponseObject, error) {
	h.logger.Debug("DeleteClusterRole handler called", "name", request.Name)

	err := h.services.AuthzService.RemoveRoleByRef(ctx, &authz.RoleRef{Name: request.Name})
	if err != nil {
		h.logger.Error("Failed to delete cluster role", "error", err)
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteClusterRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authz.ErrAuthzDisabled) {
			return gen.DeleteClusterRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrRoleNotFound) {
			return gen.DeleteClusterRole404JSONResponse{NotFoundJSONResponse: notFound("Cluster role")}, nil
		}
		if errors.Is(err, services.ErrRoleInUse) {
			return gen.DeleteClusterRole409JSONResponse{ConflictJSONResponse: conflict("Cluster role is in use by role bindings")}, nil
		}
		return gen.DeleteClusterRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster role deleted successfully", "role", request.Name)
	return gen.DeleteClusterRole204Response{}, nil
}

// ListClusterRoleBindings returns all cluster-scoped role bindings
func (h *Handler) ListClusterRoleBindings(
	ctx context.Context,
	request gen.ListClusterRoleBindingsRequestObject,
) (gen.ListClusterRoleBindingsResponseObject, error) {
	h.logger.Debug("ListClusterRoleBindings handler called")

	roleName := getStringValue(request.Params.RoleName)
	claim := getStringValue(request.Params.Claim)
	claimValue := getStringValue(request.Params.Value)
	effect := ""
	if request.Params.Effect != nil {
		effect = string(*request.Params.Effect)
	}

	mappings, err := h.services.AuthzService.ListClusterRoleMappings(ctx, roleName, claim, claimValue, effect)
	if err != nil {
		h.logger.Error("Failed to list cluster role bindings", "error", err)
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListClusterRoleBindings403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authz.ErrAuthzDisabled) {
			return gen.ListClusterRoleBindings403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		return gen.ListClusterRoleBindings500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert to API response format
	apiMappings := make([]gen.RoleEntitlementMapping, len(mappings))
	for i, mapping := range mappings {
		apiMappings[i] = gen.RoleEntitlementMapping{
			Name: mapping.Name,
			Role: gen.RoleRef{
				Name:      mapping.RoleRef.Name,
				Namespace: nil,
			},
			Entitlement: gen.Entitlement{
				Claim: mapping.Entitlement.Claim,
				Value: mapping.Entitlement.Value,
			},
			Hierarchy: gen.ResourceHierarchy{
				Namespace: nil,
				Project:   nil,
				Component: nil,
			},
			Effect: gen.RoleEntitlementMappingEffect(mapping.Effect),
		}
	}

	h.logger.Debug("Listed cluster role bindings successfully", "count", len(apiMappings))
	return gen.ListClusterRoleBindings200JSONResponse(apiMappings), nil
}

// CreateClusterRoleBinding creates a new cluster-scoped role binding
func (h *Handler) CreateClusterRoleBinding(
	ctx context.Context,
	request gen.CreateClusterRoleBindingRequestObject,
) (gen.CreateClusterRoleBindingResponseObject, error) {
	h.logger.Debug("CreateClusterRoleBinding handler called", "name", request.Body.Name)

	effect := authz.PolicyEffectAllow
	if request.Body.Effect != nil {
		effect = authz.PolicyEffectType(*request.Body.Effect)
	}

	mapping := &authz.RoleEntitlementMapping{
		Name: request.Body.Name,
		RoleRef: authz.RoleRef{
			Name:      request.Body.Role,
			Namespace: "",
		},
		Entitlement: authz.Entitlement{
			Claim: request.Body.Entitlement.Claim,
			Value: request.Body.Entitlement.Value,
		},
		Hierarchy: authz.ResourceHierarchy{},
		Effect:    effect,
	}

	err := h.services.AuthzService.AddRoleMapping(ctx, mapping)
	if err != nil {
		h.logger.Error("Failed to create cluster role binding", "error", err)
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateClusterRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authz.ErrAuthzDisabled) {
			return gen.CreateClusterRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrRoleBindingAlreadyExists) {
			return gen.CreateClusterRoleBinding409JSONResponse{ConflictJSONResponse: conflict("Cluster role binding already exists")}, nil
		}
		if errors.Is(err, services.ErrRoleNotFound) {
			return gen.CreateClusterRoleBinding400JSONResponse{BadRequestJSONResponse: badRequest("Referenced role not found")}, nil
		}
		return gen.CreateClusterRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Debug("Cluster role binding created successfully", "binding", request.Body.Name)
	return gen.CreateClusterRoleBinding201JSONResponse{
		Name: mapping.Name,
		Role: gen.RoleRef{
			Name:      mapping.RoleRef.Name,
			Namespace: nil,
		},
		Entitlement: gen.Entitlement{
			Claim: mapping.Entitlement.Claim,
			Value: mapping.Entitlement.Value,
		},
		Hierarchy: gen.ResourceHierarchy{
			Namespace: nil,
			Project:   nil,
			Component: nil,
		},
		Effect: gen.RoleEntitlementMappingEffect(mapping.Effect),
	}, nil
}

// GetClusterRoleBinding returns details of a specific cluster role binding
func (h *Handler) GetClusterRoleBinding(
	ctx context.Context,
	request gen.GetClusterRoleBindingRequestObject,
) (gen.GetClusterRoleBindingResponseObject, error) {
	h.logger.Debug("GetClusterRoleBinding handler called", "name", request.Name)

	mapping, err := h.services.AuthzService.GetRoleMapping(ctx, &authz.MappingRef{Name: request.Name})
	if err != nil {
		h.logger.Error("Failed to get cluster role binding", "error", err)
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetClusterRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrRoleBindingNotFound) {
			return gen.GetClusterRoleBinding404JSONResponse{NotFoundJSONResponse: notFound("Cluster role binding")}, nil
		}
		if errors.Is(err, authz.ErrAuthzDisabled) {
			return gen.GetClusterRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		return gen.GetClusterRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Debug("Retrieved cluster role binding successfully", "binding", request.Name)
	return gen.GetClusterRoleBinding200JSONResponse{
		Name: mapping.Name,
		Role: gen.RoleRef{
			Name:      mapping.RoleRef.Name,
			Namespace: nil,
		},
		Entitlement: gen.Entitlement{
			Claim: mapping.Entitlement.Claim,
			Value: mapping.Entitlement.Value,
		},
		Hierarchy: gen.ResourceHierarchy{
			Namespace: nil,
			Project:   nil,
			Component: nil,
		},
		Effect: gen.RoleEntitlementMappingEffect(mapping.Effect),
	}, nil
}

// UpdateClusterRoleBinding updates an existing cluster role binding
func (h *Handler) UpdateClusterRoleBinding(
	ctx context.Context,
	request gen.UpdateClusterRoleBindingRequestObject,
) (gen.UpdateClusterRoleBindingResponseObject, error) {
	h.logger.Debug("UpdateClusterRoleBinding handler called", "name", request.Name)
	// Convert request to internal model
	mapping := &authz.RoleEntitlementMapping{
		Name: request.Name,
		RoleRef: authz.RoleRef{
			Name:      request.Body.Role,
			Namespace: "",
		},
		Entitlement: authz.Entitlement{
			Claim: request.Body.Entitlement.Claim,
			Value: request.Body.Entitlement.Value,
		},
		Hierarchy: authz.ResourceHierarchy{},
		Effect:    authz.PolicyEffectType(request.Body.Effect),
	}

	err := h.services.AuthzService.UpdateRoleMapping(ctx, mapping)
	if err != nil {
		h.logger.Error("Failed to update cluster role binding", "error", err)
		if errors.Is(err, authz.ErrAuthzDisabled) {
			return gen.UpdateClusterRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateClusterRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrRoleBindingNotFound) {
			return gen.UpdateClusterRoleBinding404JSONResponse{NotFoundJSONResponse: notFound("Cluster role binding")}, nil
		}
		if errors.Is(err, authz.ErrCannotModifySystemMapping) {
			return gen.UpdateClusterRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		return gen.UpdateClusterRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster role binding updated successfully", "binding", request.Name)
	return gen.UpdateClusterRoleBinding200JSONResponse{
		Name: mapping.Name,
		Role: gen.RoleRef{
			Name:      mapping.RoleRef.Name,
			Namespace: nil,
		},
		Entitlement: gen.Entitlement{
			Claim: mapping.Entitlement.Claim,
			Value: mapping.Entitlement.Value,
		},
		Hierarchy: gen.ResourceHierarchy{
			Namespace: nil,
			Project:   nil,
			Component: nil,
		},
		Effect: gen.RoleEntitlementMappingEffect(mapping.Effect),
	}, nil
}

// DeleteClusterRoleBinding deletes a cluster role binding
func (h *Handler) DeleteClusterRoleBinding(
	ctx context.Context,
	request gen.DeleteClusterRoleBindingRequestObject,
) (gen.DeleteClusterRoleBindingResponseObject, error) {
	h.logger.Debug("DeleteClusterRoleBinding handler called", "name", request.Name)

	err := h.services.AuthzService.RemoveRoleMapping(ctx, &authz.MappingRef{Name: request.Name, Namespace: ""})
	if err != nil {
		h.logger.Error("Failed to delete cluster role binding", "error", err)
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteClusterRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authz.ErrAuthzDisabled) {
			return gen.DeleteClusterRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrRoleBindingNotFound) {
			return gen.DeleteClusterRoleBinding404JSONResponse{NotFoundJSONResponse: notFound("Cluster role binding")}, nil
		}
		if errors.Is(err, authz.ErrCannotDeleteSystemMapping) {
			return gen.DeleteClusterRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		return gen.DeleteClusterRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Cluster role binding deleted successfully", "binding", request.Name)
	return gen.DeleteClusterRoleBinding204Response{}, nil
}

// ListNamespaceRoles returns all namespace-scoped roles
func (h *Handler) ListNamespaceRoles(
	ctx context.Context,
	request gen.ListNamespaceRolesRequestObject,
) (gen.ListNamespaceRolesResponseObject, error) {
	h.logger.Debug("ListNamespaceRoles handler called", "namespace", request.NamespaceName)

	roles, err := h.services.AuthzService.ListNamespaceRoles(ctx, request.NamespaceName)
	if err != nil {
		h.logger.Error("Failed to list namespace roles", "error", err)
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListNamespaceRoles403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authz.ErrAuthzDisabled) {
			return gen.ListNamespaceRoles403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		return gen.ListNamespaceRoles500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert to API response format
	apiRoles := make([]gen.Role, len(roles))
	for i, role := range roles {
		namespace := role.Namespace
		apiRoles[i] = gen.Role{
			Actions:     role.Actions,
			Description: &role.Description,
			Name:        role.Name,
			Namespace:   &namespace,
		}
	}

	h.logger.Debug("Listed namespace roles successfully", "namespace", request.NamespaceName, "count", len(roles))
	return gen.ListNamespaceRoles200JSONResponse(apiRoles), nil
}

// CreateNamespaceRole creates a new namespace-scoped role
func (h *Handler) CreateNamespaceRole(
	ctx context.Context,
	request gen.CreateNamespaceRoleRequestObject,
) (gen.CreateNamespaceRoleResponseObject, error) {
	h.logger.Info("CreateNamespaceRole handler called", "name", request.Body.Name, "namespace", request.NamespaceName)

	// Convert request to internal model
	description := ""
	if request.Body.Description != nil {
		description = *request.Body.Description
	}
	role := &authz.Role{
		Name:        request.Body.Name,
		Actions:     request.Body.Actions,
		Description: description,
		Namespace:   request.NamespaceName,
	}

	err := h.services.AuthzService.AddRole(ctx, role)
	if err != nil {
		h.logger.Error("Failed to create namespace role", "error", err)
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateNamespaceRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authz.ErrAuthzDisabled) {
			return gen.CreateNamespaceRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrRoleAlreadyExists) {
			return gen.CreateNamespaceRole409JSONResponse{ConflictJSONResponse: conflict("Namespace role already exists")}, nil
		}
		return gen.CreateNamespaceRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	namespace := role.Namespace
	h.logger.Debug("Namespace role created successfully", "role", request.Body.Name, "namespace", request.NamespaceName)
	return gen.CreateNamespaceRole201JSONResponse{
		Actions:     role.Actions,
		Description: &role.Description,
		Name:        role.Name,
		Namespace:   &namespace,
	}, nil
}

// GetNamespaceRole returns details of a specific namespace role
func (h *Handler) GetNamespaceRole(
	ctx context.Context,
	request gen.GetNamespaceRoleRequestObject,
) (gen.GetNamespaceRoleResponseObject, error) {
	h.logger.Debug("GetNamespaceRole handler called", "name", request.Name, "namespace", request.NamespaceName)

	role, err := h.services.AuthzService.GetRoleByRef(ctx, &authz.RoleRef{Name: request.Name, Namespace: request.NamespaceName})
	if err != nil {
		h.logger.Error("Failed to get namespace role", "error", err)
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetNamespaceRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authz.ErrAuthzDisabled) {
			return gen.GetNamespaceRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrRoleNotFound) {
			return gen.GetNamespaceRole404JSONResponse{NotFoundJSONResponse: notFound("Namespace role")}, nil
		}
		return gen.GetNamespaceRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	namespace := role.Namespace
	h.logger.Debug("Retrieved namespace role successfully", "role", request.Name, "namespace", request.NamespaceName)
	return gen.GetNamespaceRole200JSONResponse{
		Actions:     role.Actions,
		Description: &role.Description,
		Name:        role.Name,
		Namespace:   &namespace,
	}, nil
}

// UpdateNamespaceRole updates an existing namespace role
func (h *Handler) UpdateNamespaceRole(
	ctx context.Context,
	request gen.UpdateNamespaceRoleRequestObject,
) (gen.UpdateNamespaceRoleResponseObject, error) {
	h.logger.Debug("UpdateNamespaceRole handler called", "name", request.Name, "namespace", request.NamespaceName)

	// Convert request to internal model
	description := ""
	if request.Body.Description != nil {
		description = *request.Body.Description
	}
	role := &authz.Role{
		Name:        request.Name,
		Actions:     request.Body.Actions,
		Description: description,
		Namespace:   request.NamespaceName,
	}

	err := h.services.AuthzService.UpdateRole(ctx, role)
	if err != nil {
		h.logger.Error("Failed to update namespace role", "error", err)
		if errors.Is(err, authz.ErrAuthzDisabled) {
			return gen.UpdateNamespaceRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			return gen.UpdateNamespaceRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrRoleNotFound) {
			return gen.UpdateNamespaceRole404JSONResponse{NotFoundJSONResponse: notFound("Namespace role")}, nil
		}
		return gen.UpdateNamespaceRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	namespace := role.Namespace
	h.logger.Info("Namespace role updated successfully", "role", request.Name, "namespace", request.NamespaceName)
	return gen.UpdateNamespaceRole200JSONResponse{
		Actions:     role.Actions,
		Description: &role.Description,
		Name:        role.Name,
		Namespace:   &namespace,
	}, nil
}

// DeleteNamespaceRole deletes a namespace role
func (h *Handler) DeleteNamespaceRole(
	ctx context.Context,
	request gen.DeleteNamespaceRoleRequestObject,
) (gen.DeleteNamespaceRoleResponseObject, error) {
	h.logger.Debug("DeleteNamespaceRole handler called", "name", request.Name, "namespace", request.NamespaceName)

	err := h.services.AuthzService.RemoveRoleByRef(ctx, &authz.RoleRef{Name: request.Name, Namespace: request.NamespaceName})
	if err != nil {
		h.logger.Error("Failed to delete namespace role", "error", err)
		if errors.Is(err, services.ErrForbidden) {
			return gen.DeleteNamespaceRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authz.ErrAuthzDisabled) {
			return gen.DeleteNamespaceRole403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrRoleNotFound) {
			return gen.DeleteNamespaceRole404JSONResponse{NotFoundJSONResponse: notFound("Namespace role")}, nil
		}
		if errors.Is(err, services.ErrRoleInUse) {
			return gen.DeleteNamespaceRole409JSONResponse{ConflictJSONResponse: conflict("Namespace role is in use by role bindings")}, nil
		}
		return gen.DeleteNamespaceRole500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Namespace role deleted successfully", "role", request.Name, "namespace", request.NamespaceName)
	return gen.DeleteNamespaceRole204Response{}, nil
}

// ListNamespaceRoleBindings returns all namespace-scoped role bindings
func (h *Handler) ListNamespaceRoleBindings(
	ctx context.Context,
	request gen.ListNamespaceRoleBindingsRequestObject,
) (gen.ListNamespaceRoleBindingsResponseObject, error) {
	h.logger.Debug("ListNamespaceRoleBindings handler called", "namespace", request.NamespaceName)

	roleName := getStringValue(request.Params.RoleName)
	roleNs := getStringValue(request.Params.RoleNamespace)
	claim := getStringValue(request.Params.Claim)
	claimValue := getStringValue(request.Params.Value)
	effect := ""
	if request.Params.Effect != nil {
		effect = string(*request.Params.Effect)
	}

	mappings, err := h.services.AuthzService.ListNamespacedRoleMappings(ctx,
		request.NamespaceName,
		&authz.RoleRef{
			Name:      roleName,
			Namespace: roleNs,
		}, claim, claimValue, effect)
	if err != nil {
		h.logger.Error("Failed to list namespace role bindings", "error", err)
		if errors.Is(err, services.ErrForbidden) {
			return gen.ListNamespaceRoleBindings403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authz.ErrAuthzDisabled) {
			return gen.ListNamespaceRoleBindings403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		return gen.ListNamespaceRoleBindings500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	// Convert to API response format
	apiMappings := make([]gen.RoleEntitlementMapping, len(mappings))
	for i, mapping := range mappings {
		namespace := mapping.Hierarchy.Namespace
		var roleNamespace *string
		if mapping.RoleRef.Namespace != "" {
			roleNamespace = &mapping.RoleRef.Namespace
		}
		var project *string
		if mapping.Hierarchy.Project != "" {
			project = &mapping.Hierarchy.Project
		}
		var component *string
		if mapping.Hierarchy.Component != "" {
			component = &mapping.Hierarchy.Component
		}

		apiMappings[i] = gen.RoleEntitlementMapping{
			Name: mapping.Name,
			Role: gen.RoleRef{
				Name:      mapping.RoleRef.Name,
				Namespace: roleNamespace,
			},
			Entitlement: gen.Entitlement{
				Claim: mapping.Entitlement.Claim,
				Value: mapping.Entitlement.Value,
			},
			Hierarchy: gen.ResourceHierarchy{
				Namespace: &namespace,
				Project:   project,
				Component: component,
			},
			Effect: gen.RoleEntitlementMappingEffect(mapping.Effect),
		}
	}

	h.logger.Debug("Listed namespace role bindings successfully", "namespace", request.NamespaceName, "count", len(apiMappings))
	return gen.ListNamespaceRoleBindings200JSONResponse(apiMappings), nil
}

// CreateNamespaceRoleBinding creates a new namespace-scoped role binding
func (h *Handler) CreateNamespaceRoleBinding(
	ctx context.Context,
	request gen.CreateNamespaceRoleBindingRequestObject,
) (gen.CreateNamespaceRoleBindingResponseObject, error) {
	h.logger.Info("CreateNamespaceRoleBinding handler called", "name", request.Body.Name, "namespace", request.NamespaceName)

	// Default effect is allow if not specified
	effect := authz.PolicyEffectAllow
	if request.Body.Effect != nil {
		effect = authz.PolicyEffectType(*request.Body.Effect)
	}

	// Build hierarchy from namespace and optional target path
	targetPathComponent := ""
	targetPathProject := ""
	if request.Body.TargetPath != nil {
		if request.Body.TargetPath.Project != nil {
			targetPathProject = *request.Body.TargetPath.Project
		}
		if request.Body.TargetPath.Component != nil {
			targetPathComponent = *request.Body.TargetPath.Component
		}
	}
	hierarchy := authz.ResourceHierarchy{
		Namespace: request.NamespaceName,
		Project:   targetPathProject,
		Component: targetPathComponent,
	}

	// Convert request to internal model
	mapping := &authz.RoleEntitlementMapping{
		Name: request.Body.Name,
		RoleRef: authz.RoleRef{
			Name:      request.Body.Role.Name,
			Namespace: getStringValue(request.Body.Role.Namespace),
		},
		Entitlement: authz.Entitlement{
			Claim: request.Body.Entitlement.Claim,
			Value: request.Body.Entitlement.Value,
		},
		Hierarchy: hierarchy,
		Effect:    effect,
	}

	err := h.services.AuthzService.AddRoleMapping(ctx, mapping)
	if err != nil {
		h.logger.Error("Failed to create namespace role binding", "error", err)
		if errors.Is(err, services.ErrForbidden) {
			return gen.CreateNamespaceRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authz.ErrAuthzDisabled) {
			return gen.CreateNamespaceRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrRoleBindingAlreadyExists) {
			return gen.CreateNamespaceRoleBinding409JSONResponse{ConflictJSONResponse: conflict("Namespace role binding already exists")}, nil
		}
		return gen.CreateNamespaceRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Debug("Namespace role binding created successfully", "binding", request.Body.Name, "namespace", request.NamespaceName)
	return gen.CreateNamespaceRoleBinding201JSONResponse{
		Name: mapping.Name,
		Role: gen.RoleRef{
			Name:      mapping.RoleRef.Name,
			Namespace: &mapping.RoleRef.Namespace,
		},
		Entitlement: gen.Entitlement{
			Claim: mapping.Entitlement.Claim,
			Value: mapping.Entitlement.Value,
		},
		Hierarchy: gen.ResourceHierarchy{
			Namespace: &mapping.Hierarchy.Namespace,
			Project:   &mapping.Hierarchy.Project,
			Component: &mapping.Hierarchy.Component,
		},
		Effect: gen.RoleEntitlementMappingEffect(mapping.Effect),
	}, nil
}

// GetNamespaceRoleBinding returns details of a specific namespace role binding
func (h *Handler) GetNamespaceRoleBinding(
	ctx context.Context,
	request gen.GetNamespaceRoleBindingRequestObject,
) (gen.GetNamespaceRoleBindingResponseObject, error) {
	h.logger.Debug("GetNamespaceRoleBinding handler called", "name", request.Name, "namespace", request.NamespaceName)

	mappings, err := h.services.AuthzService.GetRoleMapping(ctx,
		&authz.MappingRef{
			Name:      request.Name,
			Namespace: request.NamespaceName,
		})
	if err != nil {
		h.logger.Error("Failed to get namespace role binding", "error", err)
		if errors.Is(err, services.ErrForbidden) {
			return gen.GetNamespaceRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrRoleBindingNotFound) {
			return gen.GetNamespaceRoleBinding404JSONResponse{NotFoundJSONResponse: notFound("Namespace role binding")}, nil
		}
		if errors.Is(err, authz.ErrAuthzDisabled) {
			return gen.GetNamespaceRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		return gen.GetNamespaceRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Debug("Retrieved namespace role binding successfully", "binding", request.Name, "namespace", request.NamespaceName)
	return gen.GetNamespaceRoleBinding200JSONResponse{
		Name: mappings.Name,
		Role: gen.RoleRef{
			Name:      mappings.RoleRef.Name,
			Namespace: &mappings.RoleRef.Namespace,
		},
		Entitlement: gen.Entitlement{
			Claim: mappings.Entitlement.Claim,
			Value: mappings.Entitlement.Value,
		},
		Hierarchy: gen.ResourceHierarchy{
			Namespace: &mappings.Hierarchy.Namespace,
			Project:   &mappings.Hierarchy.Project,
			Component: &mappings.Hierarchy.Component,
		},
		Effect: gen.RoleEntitlementMappingEffect(mappings.Effect),
	}, nil
}

// UpdateNamespaceRoleBinding updates an existing namespace role binding
func (h *Handler) UpdateNamespaceRoleBinding(
	ctx context.Context,
	request gen.UpdateNamespaceRoleBindingRequestObject,
) (gen.UpdateNamespaceRoleBindingResponseObject, error) {
	h.logger.Debug("UpdateNamespaceRoleBinding handler called", "name", request.Name, "namespace", request.NamespaceName)

	targetPathComponent := ""
	targetPathProject := ""
	if request.Body.TargetPath.Project != nil {
		targetPathProject = *request.Body.TargetPath.Project
	}
	if request.Body.TargetPath.Component != nil {
		targetPathComponent = *request.Body.TargetPath.Component
	}

	hierarchy := authz.ResourceHierarchy{
		Namespace: request.NamespaceName,
		Project:   targetPathProject,
		Component: targetPathComponent,
	}

	// Convert request to internal model
	mapping := &authz.RoleEntitlementMapping{
		Name: request.Name,
		RoleRef: authz.RoleRef{
			Name:      request.Body.Role.Name,
			Namespace: getStringValue(request.Body.Role.Namespace),
		},
		Entitlement: authz.Entitlement{
			Claim: request.Body.Entitlement.Claim,
			Value: request.Body.Entitlement.Value,
		},
		Hierarchy: hierarchy,
		Effect:    authz.PolicyEffectType(request.Body.Effect),
	}

	err := h.services.AuthzService.UpdateRoleMapping(ctx, mapping)
	if err != nil {
		if errors.Is(err, authz.ErrAuthzDisabled) {
			return gen.UpdateNamespaceRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrForbidden) {
			h.logger.Warn("Unauthorized to update namespace role binding", "binding", request.Name)
			return gen.UpdateNamespaceRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrRoleBindingNotFound) {
			h.logger.Warn("Namespace role binding not found", "binding", request.Name)
			return gen.UpdateNamespaceRoleBinding404JSONResponse{NotFoundJSONResponse: notFound("Namespace role binding")}, nil
		}
		if errors.Is(err, authz.ErrCannotModifySystemMapping) {
			return gen.UpdateNamespaceRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to update namespace role binding", "error", err)
		return gen.UpdateNamespaceRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Namespace role binding updated successfully", "binding", request.Name, "namespace", request.NamespaceName)
	return gen.UpdateNamespaceRoleBinding200JSONResponse{
		Name: mapping.Name,
		Role: gen.RoleRef{
			Name:      mapping.RoleRef.Name,
			Namespace: &mapping.RoleRef.Namespace,
		},
		Entitlement: gen.Entitlement{
			Claim: mapping.Entitlement.Claim,
			Value: mapping.Entitlement.Value,
		},
		Hierarchy: gen.ResourceHierarchy{
			Namespace: &mapping.Hierarchy.Namespace,
			Project:   &mapping.Hierarchy.Project,
			Component: &mapping.Hierarchy.Component,
		},
		Effect: gen.RoleEntitlementMappingEffect(mapping.Effect),
	}, nil
}

// DeleteNamespaceRoleBinding deletes a namespace role binding
func (h *Handler) DeleteNamespaceRoleBinding(
	ctx context.Context,
	request gen.DeleteNamespaceRoleBindingRequestObject,
) (gen.DeleteNamespaceRoleBindingResponseObject, error) {
	h.logger.Debug("DeleteNamespaceRoleBinding handler called", "name", request.Name, "namespace", request.NamespaceName)

	// Delete the role mapping
	err := h.services.AuthzService.RemoveRoleMapping(ctx, &authz.MappingRef{Name: request.Name, Namespace: request.NamespaceName})
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			h.logger.Warn("Unauthorized to delete namespace role binding", "binding", request.Name)
			return gen.DeleteNamespaceRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, authz.ErrAuthzDisabled) {
			return gen.DeleteNamespaceRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		if errors.Is(err, services.ErrRoleBindingNotFound) {
			h.logger.Warn("Namespace role binding not found", "binding", request.Name)
			return gen.DeleteNamespaceRoleBinding404JSONResponse{NotFoundJSONResponse: notFound("Namespace role binding")}, nil
		}
		if errors.Is(err, authz.ErrCannotDeleteSystemMapping) {
			return gen.DeleteNamespaceRoleBinding403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
		}
		h.logger.Error("Failed to delete namespace role binding", "error", err)
		return gen.DeleteNamespaceRoleBinding500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	h.logger.Info("Namespace role binding deleted successfully", "binding", request.Name, "namespace", request.NamespaceName)
	return gen.DeleteNamespaceRoleBinding204Response{}, nil
}
