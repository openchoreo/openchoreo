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

	roles, err := s.pap.ListRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list roles: %w", err)
	}

	s.logger.Debug("Listed authorization roles", "count", len(roles))
	return roles, nil
}

// GetRole retrieves a specific role by name
func (s *AuthzService) GetRole(ctx context.Context, roleName string) (*authz.Role, error) {
	s.logger.Debug("Getting authorization role", "role", roleName)

	role, err := s.pap.GetRole(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("failed to get role: %w", err)
	}

	return role, nil
}

// AddRole creates a new authorization role
func (s *AuthzService) AddRole(ctx context.Context, role *authz.Role) error {
	s.logger.Debug("Adding authorization role", "role", role.Name, "actions", role.Actions)

	if err := s.pap.AddRole(ctx, role); err != nil {
		return fmt.Errorf("failed to add role: %w", err)
	}

	s.logger.Info("Authorization role added", "role", role.Name)
	return nil
}

// RemoveRole deletes an authorization role
func (s *AuthzService) RemoveRole(ctx context.Context, roleName string) error {
	s.logger.Debug("Removing authorization role", "role", roleName)

	if err := s.pap.RemoveRole(ctx, roleName); err != nil {
		return fmt.Errorf("failed to remove role: %w", err)
	}

	s.logger.Info("Authorization role removed", "role", roleName)
	return nil
}

// ListRoleMappings lists all role-entitlement mappings (role mappings)
func (s *AuthzService) ListRoleMappings(ctx context.Context) ([]*authz.RoleEntitlementMapping, error) {
	s.logger.Debug("Listing authorization role mappings")

	mappings, err := s.pap.ListRoleEntitlementMappings(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list role mappings: %w", err)
	}

	s.logger.Debug("Listed authorization role mappings", "count", len(mappings))
	return mappings, nil
}

// AddRoleMapping creates a new role-entitlement mapping
func (s *AuthzService) AddRoleMapping(ctx context.Context, mapping *authz.RoleEntitlementMapping) error {
	s.logger.Debug("Adding authorization role entitlement mapping",
		"entitlement", mapping.EntitlementValue,
		"role", mapping.RoleName,
		"hierarchy", mapping.Hierarchy)

	if err := s.pap.AddRoleEntitlementMapping(ctx, mapping); err != nil {
		return fmt.Errorf("failed to add policy: %w", err)
	}

	s.logger.Info("Authorization policy added",
		"entitlement", mapping.EntitlementValue,
		"role", mapping.RoleName)
	return nil
}

// RemoveRoleMapping removes a role-entitlement mapping
func (s *AuthzService) RemoveRoleMapping(ctx context.Context, mapping *authz.RoleEntitlementMapping) error {
	s.logger.Debug("Removing authorization role mapping",
		"entitlement", mapping.EntitlementValue,
		"role", mapping.RoleName)

	if err := s.pap.RemoveRoleEntitlementMapping(ctx, mapping); err != nil {
		return fmt.Errorf("failed to remove role mapping: %w", err)
	}

	s.logger.Info("Authorization role mapping removed",
		"entitlement", mapping.EntitlementValue,
		"role", mapping.RoleName)
	return nil
}

// ListActions lists all available actions in the system
func (s *AuthzService) ListActions(ctx context.Context) ([]string, error) {
	s.logger.Debug("Listing authorization actions")

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
		"subject", request.Subject,
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
