// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	authz "github.com/openchoreo/openchoreo/internal/authz/core"
)

// AuthzService handles authorization-related business logic
type AuthzService struct {
	pap    authz.PAP
	pdp    authz.PDP
	logger *slog.Logger
}

// NewAuthzService creates a new authorization service
func NewAuthzService(pap authz.PAP, pdp authz.PDP, logger *slog.Logger) *AuthzService {
	return &AuthzService{
		pap:    pap,
		pdp:    pdp,
		logger: logger,
	}
}

// ListRoles lists all authorization roles (both cluster and namespace-scoped)
func (s *AuthzService) ListRoles(ctx context.Context) ([]*authz.Role, error) {
	s.logger.Debug("Listing all authorization roles")

	if err := checkAuthorization(ctx, s.logger, s.pdp, SystemActionViewRole, ResourceTypeRole, "*",
		authz.ResourceHierarchy{}); err != nil {
		return nil, err
	}

	roles, err := s.pap.ListRoles(ctx, &authz.RoleFilter{IncludeAll: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list roles: %w", err)
	}

	s.logger.Debug("Listed authorization roles", "count", len(roles))
	return roles, nil
}

// ListClusterRoles lists only cluster-scoped roles
func (s *AuthzService) ListClusterRoles(ctx context.Context) ([]*authz.Role, error) {
	s.logger.Debug("Listing cluster authorization roles")

	if err := checkAuthorization(ctx, s.logger, s.pdp, SystemActionViewRole, ResourceTypeRole, "*",
		authz.ResourceHierarchy{}); err != nil {
		return nil, err
	}

	roles, err := s.pap.ListRoles(ctx, &authz.RoleFilter{Namespace: ""})
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster roles: %w", err)
	}

	s.logger.Debug("Listed cluster authorization roles", "count", len(roles))
	return roles, nil
}

// ListNamespaceRoles lists namespace-scoped roles for a specific namespace
func (s *AuthzService) ListNamespaceRoles(ctx context.Context, namespace string) ([]*authz.Role, error) {
	s.logger.Debug("Listing namespace authorization roles", "namespace", namespace)

	if err := checkAuthorization(ctx, s.logger, s.pdp, SystemActionViewRole, ResourceTypeRole, "*",
		authz.ResourceHierarchy{Namespace: namespace}); err != nil {
		return nil, err
	}

	roles, err := s.pap.ListRoles(ctx, &authz.RoleFilter{Namespace: namespace})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespace roles: %w", err)
	}

	s.logger.Debug("Listed namespace authorization roles", "namespace", namespace, "count", len(roles))
	return roles, nil
}

// GetRoleByRef retrieves a specific role by RoleRef (name and namespace)
func (s *AuthzService) GetRoleByRef(ctx context.Context, roleRef *authz.RoleRef) (*authz.Role, error) {
	s.logger.Debug("Getting authorization role", "role", roleRef.Name, "namespace", roleRef.Namespace)

	hierarchy := authz.ResourceHierarchy{}
	if roleRef.Namespace != "" {
		hierarchy.Namespace = roleRef.Namespace
	}

	if err := checkAuthorization(ctx, s.logger, s.pdp, SystemActionViewRole, ResourceTypeRole, roleRef.Name,
		hierarchy); err != nil {
		return nil, err
	}

	role, err := s.pap.GetRole(ctx, roleRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get role: %w", err)
	}

	return role, nil
}

// AddRole creates a new authorization role
func (s *AuthzService) AddRole(ctx context.Context, role *authz.Role) error {
	s.logger.Debug("Adding authorization role", "role", role.Name, "namespace", role.Namespace, "actions", role.Actions)

	hierarchy := authz.ResourceHierarchy{}
	if role.Namespace != "" {
		hierarchy.Namespace = role.Namespace
	}

	if err := checkAuthorization(ctx, s.logger, s.pdp, SystemActionCreateRole, ResourceTypeRole, role.Name,
		hierarchy); err != nil {
		return err
	}

	if err := s.pap.AddRole(ctx, role); err != nil {
		return fmt.Errorf("failed to add role: %w", err)
	}

	s.logger.Debug("Authorization role added", "role", role.Name, "namespace", role.Namespace)
	return nil
}

// RemoveRoleByRef deletes an authorization role by RoleRef
func (s *AuthzService) RemoveRoleByRef(ctx context.Context, roleRef *authz.RoleRef) error {
	s.logger.Debug("Removing authorization role", "role", roleRef.Name, "namespace", roleRef.Namespace)

	hierarchy := authz.ResourceHierarchy{}
	if roleRef.Namespace != "" {
		hierarchy.Namespace = roleRef.Namespace
	}

	if err := checkAuthorization(ctx, s.logger, s.pdp, SystemActionDeleteRole, ResourceTypeRole, roleRef.Name,
		hierarchy); err != nil {
		return err
	}

	if err := s.pap.RemoveRole(ctx, roleRef); err != nil {
		return fmt.Errorf("failed to remove role: %w", err)
	}

	s.logger.Debug("Authorization role removed", "role", roleRef.Name, "namespace", roleRef.Namespace)
	return nil
}

// UpdateRole updates an existing authorization role
func (s *AuthzService) UpdateRole(ctx context.Context, role *authz.Role) error {
	s.logger.Debug("Updating authorization role", "role", role.Name, "namespace", role.Namespace, "actions", role.Actions)

	hierarchy := authz.ResourceHierarchy{}
	if role.Namespace != "" {
		hierarchy.Namespace = role.Namespace
	}

	if err := checkAuthorization(ctx, s.logger, s.pdp, SystemActionUpdateRole, ResourceTypeRole, role.Name,
		hierarchy); err != nil {
		return err
	}

	if err := s.pap.UpdateRole(ctx, role); err != nil {
		return fmt.Errorf("failed to update role: %w", err)
	}

	s.logger.Debug("Authorization role updated", "role", role.Name, "namespace", role.Namespace)
	return nil
}

// ListClusterRoleMappings lists cluster role-entitlement mappings with optional filters
// Supports filtering by:
//   - roleName: Filter by role name
//   - claim & value: Filter by entitlement (both must be provided together)
func (s *AuthzService) ListClusterRoleMappings(ctx context.Context, roleName, claim, value, effect string) ([]*authz.RoleEntitlementMapping, error) {
	s.logger.Debug("Listing authorization role mappings with filters",
		"role", roleName, "claim", claim, "value", value)

	if err := checkAuthorization(ctx, s.logger, s.pdp, SystemActionViewRoleMapping, ResourceTypeRoleMapping, "*",
		authz.ResourceHierarchy{}); err != nil {
		return nil, err
	}

	var filter *authz.RoleEntitlementMappingFilter
	if roleName != "" || (claim != "" && value != "") {
		filter = &authz.RoleEntitlementMappingFilter{}

		if roleName != "" {
			filter.RoleRef = &authz.RoleRef{Name: roleName}
		}

		if claim != "" && value != "" {
			filter.Entitlement = &authz.Entitlement{
				Claim: claim,
				Value: value,
			}
		}
		if effect != "" {
			effectType := authz.PolicyEffectType(effect)
			filter.Effect = &effectType
		}
	}

	mappings, err := s.pap.ListRoleEntitlementMappings(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list role mappings: %w", err)
	}
	// Filter to only cluster-scoped bindings (those without namespace in hierarchy)
	clusterMappings := make([]*authz.RoleEntitlementMapping, 0)
	for _, mapping := range mappings {
		if mapping.Hierarchy.Namespace == "" && mapping.RoleRef.Namespace == "" {
			clusterMappings = append(clusterMappings, mapping)
		}
	}

	s.logger.Debug("Listed authorization role mappings with filters", "count", len(clusterMappings))
	return clusterMappings, nil
}

// ListNamespacedRoleMappings lists namespaced role-entitlement mappings with optional filters
// Supports filtering by:
//   - roleName: Filter by role name
//   - claim & value: Filter by entitlement (both must be provided together)
func (s *AuthzService) ListNamespacedRoleMappings(ctx context.Context, namespace string, roleRef *authz.RoleRef, claim, value, effect string) ([]*authz.RoleEntitlementMapping, error) {
	s.logger.Debug("Listing authorization role mappings with filters",
		"role", roleRef.Name, "claim", claim, "value", value)

	if err := checkAuthorization(ctx, s.logger, s.pdp, SystemActionViewRoleMapping, ResourceTypeRoleMapping, "*",
		authz.ResourceHierarchy{
			Namespace: namespace,
		}); err != nil {
		return nil, err
	}

	var filter *authz.RoleEntitlementMappingFilter
	if roleRef.Name != "" || roleRef.Namespace != "" || (claim != "" && value != "") {
		filter = &authz.RoleEntitlementMappingFilter{}

		filter.RoleRef = &authz.RoleRef{Name: roleRef.Name, Namespace: roleRef.Namespace}

		if claim != "" && value != "" {
			filter.Entitlement = &authz.Entitlement{
				Claim: claim,
				Value: value,
			}
		}
		if effect != "" {
			effectType := authz.PolicyEffectType(effect)
			filter.Effect = &effectType
		}
	}

	mappings, err := s.pap.ListRoleEntitlementMappings(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list role mappings: %w", err)
	}
	// Filter to only namespaced bindings (those with namespace in hierarchy)
	namespacedMappings := make([]*authz.RoleEntitlementMapping, 0)
	for _, mapping := range mappings {
		if mapping.Hierarchy.Namespace == namespace {
			namespacedMappings = append(namespacedMappings, mapping)
		}
	}

	s.logger.Debug("Listed authorization role mappings with filters", "count", len(namespacedMappings))
	return namespacedMappings, nil
}

func (s *AuthzService) GetRoleMapping(ctx context.Context, mappingRef *authz.MappingRef) (*authz.RoleEntitlementMapping, error) {
	s.logger.Debug("Getting authorization role mapping", "mapping_name", mappingRef.Name, "mapping_namespace", mappingRef.Namespace)

	if err := checkAuthorization(ctx, s.logger, s.pdp, SystemActionViewRoleMapping, ResourceTypeRoleMapping, mappingRef.Name,
		authz.ResourceHierarchy{
			Namespace: mappingRef.Namespace,
		}); err != nil {
		return nil, err
	}
	mapping, err := s.pap.GetRoleEntitlementMapping(ctx, mappingRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get role mapping: %w", err)
	}
	s.logger.Debug("Got authorization role mapping", "mapping_name", mappingRef.Name, "mapping_namespace", mappingRef.Namespace)
	return mapping, nil
}

// AddRoleMapping creates a new role-entitlement mapping
func (s *AuthzService) AddRoleMapping(ctx context.Context, mapping *authz.RoleEntitlementMapping) error {
	s.logger.Debug("Adding authorization role entitlement mapping",
		"entitlement_claim", mapping.Entitlement.Claim,
		"entitlement_value", mapping.Entitlement.Value,
		"role", mapping.RoleRef.Name,
		"role namespace", mapping.RoleRef.Namespace,
		"hierarchy", mapping.Hierarchy)

	if err := checkAuthorization(ctx, s.logger, s.pdp, SystemActionCreateRoleMapping, ResourceTypeRoleMapping, mapping.RoleRef.Name,
		authz.ResourceHierarchy{
			Namespace: mapping.Hierarchy.Namespace,
		}); err != nil {
		return err
	}

	if err := s.pap.AddRoleEntitlementMapping(ctx, mapping); err != nil {
		return fmt.Errorf("failed to add policy: %w", err)
	}
	return nil
}

// UpdateRoleMapping updates an existing role-entitlement mapping
func (s *AuthzService) UpdateRoleMapping(ctx context.Context, mapping *authz.RoleEntitlementMapping) error {
	s.logger.Debug("Updating authorization role entitlement mapping",
		"mapping_name", mapping.Name,
		"entitlement_claim", mapping.Entitlement.Claim,
		"entitlement_value", mapping.Entitlement.Value,
		"role", mapping.RoleRef.Name,
		"hierarchy", mapping.Hierarchy)

	if err := checkAuthorization(ctx, s.logger, s.pdp, SystemActionUpdateRoleMapping, ResourceTypeRoleMapping, mapping.Name,
		authz.ResourceHierarchy{
			Namespace: mapping.Hierarchy.Namespace,
		}); err != nil {
		return err
	}

	if err := s.pap.UpdateRoleEntitlementMapping(ctx, mapping); err != nil {
		return fmt.Errorf("failed to update role mapping: %w", err)
	}
	s.logger.Debug("Authorization role entitlement mapping updated", "mapping_name", mapping.Name)
	return nil
}

// RemoveRoleMapping removes a role-entitlement mapping by MappingRef (name and namespace)
func (s *AuthzService) RemoveRoleMapping(ctx context.Context, mappingRef *authz.MappingRef) error {
	s.logger.Debug("Removing authorization role mapping", "mapping_name", mappingRef.Name, "mapping_namespace", mappingRef.Namespace)

	if err := checkAuthorization(ctx, s.logger, s.pdp, SystemActionDeleteRoleMapping, ResourceTypeRoleMapping, mappingRef.Name,
		authz.ResourceHierarchy{
			Namespace: mappingRef.Namespace,
		}); err != nil {
		return err
	}

	if err := s.pap.RemoveRoleEntitlementMapping(ctx, mappingRef); err != nil {
		return fmt.Errorf("failed to remove role mapping: %w", err)
	}

	s.logger.Debug("Authorization role mapping removed", "mapping_name", mappingRef.Name)
	return nil
}

// ListActions lists all available actions in the system
func (s *AuthzService) ListActions(ctx context.Context) ([]string, error) {
	s.logger.Debug("Listing authorization actions")

	if err := checkAuthorization(ctx, s.logger, s.pdp, SystemActionViewRole, ResourceTypeRole, "*",
		authz.ResourceHierarchy{}); err != nil {
		return nil, err
	}

	actions, err := s.pap.ListActions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list actions: %w", err)
	}

	s.logger.Debug("Listed authorization actions", "count", len(actions))
	return actions, nil
}

// Evaluate evaluates an authorization request using the PDP
func (s *AuthzService) Evaluate(ctx context.Context, request *authz.EvaluateRequest) (*authz.Decision, error) {
	s.logger.Debug("Evaluating authorization request",
		"subject", request.SubjectContext,
		"resource", request.Resource,
		"action", request.Action)

	decision, err := s.pdp.Evaluate(ctx, request)
	if err != nil {
		s.logger.Error("Failed to evaluate authorization request", "error", err)
		if errors.Is(err, authz.ErrInvalidRequest) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to evaluate request: %w", err)
	}

	s.logger.Debug("Authorization decision made",
		"allowed", decision.Decision,
		"reason", decision.Context.Reason)
	return decision, nil
}

// BatchEvaluate evaluates multiple authorization requests using the PDP
func (s *AuthzService) BatchEvaluate(ctx context.Context, request *authz.BatchEvaluateRequest) (*authz.BatchEvaluateResponse, error) {
	s.logger.Debug("Batch evaluating authorization requests", "count", len(request.Requests))

	response, err := s.pdp.BatchEvaluate(ctx, request)
	if err != nil {
		s.logger.Error("Failed to batch evaluate authorization requests", "error", err)
		if errors.Is(err, authz.ErrInvalidRequest) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to batch evaluate requests: %w", err)
	}

	s.logger.Debug("Batch authorization decisions made", "count", len(response.Decisions))
	return response, nil
}

func (s *AuthzService) GetSubjectProfile(ctx context.Context, request *authz.ProfileRequest) (*authz.UserCapabilitiesResponse, error) {
	s.logger.Debug("Retrieving subject profile", "subject", request.SubjectContext, "scope", request.Scope)

	profile, err := s.pdp.GetSubjectProfile(ctx, request)
	if err != nil {
		if errors.Is(err, authz.ErrInvalidRequest) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get subject profile: %w", err)
	}
	s.logger.Debug("Retrieved subject profile", "subject", profile.User.EntitlementValues, "result", profile.Capabilities)

	return profile, nil
}
