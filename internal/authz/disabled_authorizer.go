// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"context"
	"log/slog"

	authz "github.com/openchoreo/openchoreo/internal/authz/core"
)

type DisabledAuthorizer struct {
	logger *slog.Logger
}

// NewDisabledAuthorizer creates a new disabled authorization implementation
func NewDisabledAuthorizer(logger *slog.Logger) *DisabledAuthorizer {
	return &DisabledAuthorizer{
		logger: logger,
	}
}

// ============================================================================
// PDP Implementation - All authorization checks pass
// ============================================================================

// Evaluate always returns a decision allowing access
func (da *DisabledAuthorizer) Evaluate(ctx context.Context, request *authz.EvaluateRequest) (*authz.Decision, error) {
	da.logger.Debug("disabled authorizer: evaluate called (authorization disabled)",
		"subject", request.Subject,
		"resource", request.Resource,
		"action", request.Action)

	return &authz.Decision{
		Decision: true,
		Context: &authz.DecisionContext{
			Reason: "Authorization disabled - all access granted",
		},
	}, nil
}

// BatchEvaluate always returns decisions allowing access for all requests
func (da *DisabledAuthorizer) BatchEvaluate(ctx context.Context, request *authz.BatchEvaluateRequest) (*authz.BatchEvaluateResponse, error) {
	da.logger.Debug("disabled authorizer: batch evaluate called (authorization disabled)",
		"num_requests", len(request.Requests))

	decisions := make([]authz.Decision, len(request.Requests))
	for i := range request.Requests {
		decisions[i] = authz.Decision{
			Decision: true,
			Context: &authz.DecisionContext{
				Reason: "Authorization disabled - all access granted",
			},
		}
	}

	return &authz.BatchEvaluateResponse{
		Decisions: decisions,
	}, nil
}

// GetSubjectProfile returns a profile with all actions allowed
func (da *DisabledAuthorizer) GetSubjectProfile(ctx context.Context, request *authz.ProfileRequest) (*authz.SubjectProfile, error) {
	da.logger.Debug("disabled authorizer: get subject profile called (authorization disabled)",
		"subject", request.Subject)

	return &authz.SubjectProfile{
		Hierarchy: authz.ProfileResourceNode{
			Type:     "organization",
			ID:       request.Scope.Organization,
			Actions:  []string{"*"},
			Children: []authz.ProfileResourceNode{},
		},
	}, nil
}

// ============================================================================
// PAP Implementation - All policy operations fail
// ============================================================================

// AddRole fails with error
func (da *DisabledAuthorizer) AddRole(ctx context.Context, role *authz.Role) error {
	return authz.ErrAuthzDisabled
}

// RemoveRole fails with error
func (da *DisabledAuthorizer) RemoveRole(ctx context.Context, roleName string) error {
	return authz.ErrAuthzDisabled
}

// GetRole fails with error
func (da *DisabledAuthorizer) GetRole(ctx context.Context, roleName string) (*authz.Role, error) {
	return nil, authz.ErrAuthzDisabled
}

// ListRoles fails with error
func (da *DisabledAuthorizer) ListRoles(ctx context.Context) ([]*authz.Role, error) {
	return nil, authz.ErrAuthzDisabled
}

// AddRoleEntitlementMapping fails with error
func (da *DisabledAuthorizer) AddRoleEntitlementMapping(ctx context.Context, mapping *authz.RoleEntitlementMapping) error {
	return authz.ErrAuthzDisabled
}

// RemoveRoleEntitlementMapping fails with error
func (da *DisabledAuthorizer) RemoveRoleEntitlementMapping(ctx context.Context, mapping *authz.RoleEntitlementMapping) error {
	return authz.ErrAuthzDisabled
}

// GetEntitlementMappings fails with error
func (da *DisabledAuthorizer) GetEntitlementMappings(ctx context.Context, principal string) ([]*authz.RoleEntitlementMapping, error) {
	return nil, authz.ErrAuthzDisabled
}

// GetRoleMappings fails with error
func (da *DisabledAuthorizer) GetRoleMappings(ctx context.Context, roleName string) ([]*authz.RoleEntitlementMapping, error) {
	return nil, authz.ErrAuthzDisabled
}

// ListRoleEntitlementMappings fails with error
func (da *DisabledAuthorizer) ListRoleEntitlementMappings(ctx context.Context) ([]*authz.RoleEntitlementMapping, error) {
	return nil, authz.ErrAuthzDisabled
}

// ListActions fails with error
func (da *DisabledAuthorizer) ListActions(ctx context.Context) ([]string, error) {
	return nil, authz.ErrAuthzDisabled
}

// These var declarations enforce at compile-time that DisabledAuthorizer
// implements the PDP and PAP interfaces correctly.
var (
	_ authz.PDP = (*DisabledAuthorizer)(nil)
	_ authz.PAP = (*DisabledAuthorizer)(nil)
)
