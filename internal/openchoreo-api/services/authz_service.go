// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

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

// ListRoles lists all authorization roles
func (s *AuthzService) ListRoles(ctx context.Context) ([]*authz.Role, error) {
	s.logger.Debug("Listing authorization roles")

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

// GetRole retrieves a specific role by name
func (s *AuthzService) GetRole(ctx context.Context, roleName string) (*authz.Role, error) {
	s.logger.Debug("Getting authorization role", "role", roleName)

	if err := checkAuthorization(ctx, s.logger, s.pdp, SystemActionViewRole, ResourceTypeRole, roleName,
		authz.ResourceHierarchy{}); err != nil {
		return nil, err
	}

	role, err := s.pap.GetRole(ctx, &authz.RoleRef{Name: roleName})
	if err != nil {
		return nil, fmt.Errorf("failed to get role: %w", err)
	}

	return role, nil
}

// AddRole creates a new authorization role
func (s *AuthzService) AddRole(ctx context.Context, role *authz.Role) error {
	s.logger.Debug("Adding authorization role", "role", role.Name, "actions", role.Actions)

	if err := checkAuthorization(ctx, s.logger, s.pdp, SystemActionCreateRole, ResourceTypeRole, role.Name,
		authz.ResourceHierarchy{}); err != nil {
		return err
	}

	if err := s.pap.AddRole(ctx, role); err != nil {
		return fmt.Errorf("failed to add role: %w", err)
	}

	s.logger.Debug("Authorization role added", "role", role.Name)
	return nil
}

// RemoveRole deletes an authorization role
// If force is true, it will also remove all associated role-entitlement mappings
func (s *AuthzService) RemoveRole(ctx context.Context, roleName string, force bool) error {
	s.logger.Debug("Removing authorization role", "role", roleName, "force", force)

	if err := checkAuthorization(ctx, s.logger, s.pdp, SystemActionDeleteRole, ResourceTypeRole, roleName,
		authz.ResourceHierarchy{}); err != nil {
		return err
	}

	roleRef := &authz.RoleRef{Name: roleName}
	if force {
		if err := s.pap.ForceRemoveRole(ctx, roleRef); err != nil {
			return fmt.Errorf("failed to force remove role: %w", err)
		}
		s.logger.Debug("Authorization role and mappings removed", "role", roleName)
	} else {
		if err := s.pap.RemoveRole(ctx, roleRef); err != nil {
			return fmt.Errorf("failed to remove role: %w", err)
		}
		s.logger.Debug("Authorization role removed", "role", roleName)
	}

	return nil
}

// UpdateRole updates an existing authorization role
func (s *AuthzService) UpdateRole(ctx context.Context, role *authz.Role) error {
	s.logger.Debug("Updating authorization role", "role", role.Name, "actions", role.Actions)

	if err := checkAuthorization(ctx, s.logger, s.pdp, SystemActionUpdateRole, ResourceTypeRole, role.Name,
		authz.ResourceHierarchy{}); err != nil {
		return err
	}

	if err := s.pap.UpdateRole(ctx, role); err != nil {
		return fmt.Errorf("failed to update role: %w", err)
	}

	s.logger.Debug("Authorization role updated", "role", role.Name)
	return nil
}

// ListRoleMappings lists role-entitlement mappings with optional filters
// Supports filtering by:
//   - roleName: Filter by role name
//   - claim & value: Filter by entitlement (both must be provided together)
func (s *AuthzService) ListRoleMappings(ctx context.Context, roleName, claim, value string) ([]*authz.RoleEntitlementMapping, error) {
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
	}

	mappings, err := s.pap.ListRoleEntitlementMappings(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list role mappings: %w", err)
	}

	s.logger.Debug("Listed authorization role mappings with filters", "count", len(mappings))
	return mappings, nil
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
		authz.ResourceHierarchy{}); err != nil {
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
		"mapping_id", mapping.ID,
		"entitlement_claim", mapping.Entitlement.Claim,
		"entitlement_value", mapping.Entitlement.Value,
		"role", mapping.RoleRef.Name,
		"hierarchy", mapping.Hierarchy)

	if err := checkAuthorization(ctx, s.logger, s.pdp, SystemActionUpdateRoleMapping, ResourceTypeRoleMapping, fmt.Sprintf("%d", mapping.ID),
		authz.ResourceHierarchy{}); err != nil {
		return err
	}

	if err := s.pap.UpdateRoleEntitlementMapping(ctx, mapping); err != nil {
		return fmt.Errorf("failed to update role mapping: %w", err)
	}
	s.logger.Debug("Authorization role entitlement mapping updated", "mapping_id", mapping.ID)
	return nil
}

// RemoveRoleMappingByID removes a role-entitlement mapping by ID
func (s *AuthzService) RemoveRoleMappingByID(ctx context.Context, mappingID uint) error {
	s.logger.Debug("Removing authorization role mapping", "mapping_id", mappingID)

	if err := checkAuthorization(ctx, s.logger, s.pdp, SystemActionDeleteRoleMapping, ResourceTypeRoleMapping, fmt.Sprintf("%d", mappingID),
		authz.ResourceHierarchy{}); err != nil {
		return err
	}

	if err := s.pap.RemoveRoleEntitlementMapping(ctx, mappingID); err != nil {
		return fmt.Errorf("failed to remove role mapping: %w", err)
	}

	s.logger.Debug("Authorization role mapping removed", "mapping_id", mappingID)
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
