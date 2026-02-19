// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import (
	"context"
	"log/slog"
	"time"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authz "github.com/openchoreo/openchoreo/internal/authz/core"
)

type DisabledAuthorizer struct {
	logger *slog.Logger
}

// NewDisabledAuthorizer creates a new disabled authorization implementation
func NewDisabledAuthorizer(logger *slog.Logger) *DisabledAuthorizer {
	return &DisabledAuthorizer{
		logger: logger.With("module", "authz.passthrough"),
	}
}

// ============================================================================
// PDP Implementation - All authorization checks pass
// ============================================================================

// Evaluate always returns a decision allowing access
func (da *DisabledAuthorizer) Evaluate(ctx context.Context, request *authz.EvaluateRequest) (*authz.Decision, error) {
	da.logger.Debug("disabled authorizer: evaluate called (authorization disabled)",
		"subject", request.SubjectContext,
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
func (da *DisabledAuthorizer) GetSubjectProfile(ctx context.Context, request *authz.ProfileRequest) (*authz.UserCapabilitiesResponse, error) {
	da.logger.Debug("disabled authorizer: get subject profile called (authorization disabled)",
		"subject", request.SubjectContext)

	return &authz.UserCapabilitiesResponse{
		User: request.SubjectContext,
		Capabilities: map[string]*authz.ActionCapability{
			"*": { // Wildcard action - all actions allowed
				Allowed: []*authz.CapabilityResource{
					{
						Path:        "*", // Wildcard path - all resources
						Constraints: nil, // No constraints
					},
				},
				Denied: []*authz.CapabilityResource{}, // Empty - no denials
			},
		},
		GeneratedAt: time.Now(),
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
func (da *DisabledAuthorizer) RemoveRole(ctx context.Context, roleRef *authz.RoleRef) error {
	return authz.ErrAuthzDisabled
}

// GetRole fails with error
func (da *DisabledAuthorizer) GetRole(ctx context.Context, roleRef *authz.RoleRef) (*authz.Role, error) {
	return nil, authz.ErrAuthzDisabled
}

// UpdateRole fails with error
func (da *DisabledAuthorizer) UpdateRole(ctx context.Context, role *authz.Role) error {
	return authz.ErrAuthzDisabled
}

// ListRoles fails with error
func (da *DisabledAuthorizer) ListRoles(ctx context.Context, filter *authz.RoleFilter) ([]*authz.Role, error) {
	return nil, authz.ErrAuthzDisabled
}

// AddRoleEntitlementMapping fails with error
func (da *DisabledAuthorizer) AddRoleEntitlementMapping(ctx context.Context, mapping *authz.RoleEntitlementMapping) error {
	return authz.ErrAuthzDisabled
}

// UpdateRoleEntitlementMapping fails with error
func (da *DisabledAuthorizer) UpdateRoleEntitlementMapping(ctx context.Context, mapping *authz.RoleEntitlementMapping) error {
	return authz.ErrAuthzDisabled
}

// RemoveRoleEntitlementMapping fails with error
func (da *DisabledAuthorizer) RemoveRoleEntitlementMapping(ctx context.Context, mappingRef *authz.MappingRef) error {
	return authz.ErrAuthzDisabled
}

// ListRoleEntitlementMappings fails with error
func (da *DisabledAuthorizer) ListRoleEntitlementMappings(ctx context.Context, filter *authz.RoleEntitlementMappingFilter) ([]*authz.RoleEntitlementMapping, error) {
	return nil, authz.ErrAuthzDisabled
}

// ListActions fails with error
func (da *DisabledAuthorizer) ListActions(ctx context.Context) ([]string, error) {
	return nil, authz.ErrAuthzDisabled
}

// GetRoleEntitlementMapping fails with error
func (da *DisabledAuthorizer) GetRoleEntitlementMapping(ctx context.Context, mappingRef *authz.MappingRef) (*authz.RoleEntitlementMapping, error) {
	return nil, authz.ErrAuthzDisabled
}

func (da *DisabledAuthorizer) CreateClusterRole(ctx context.Context, role *openchoreov1alpha1.AuthzClusterRole) (*openchoreov1alpha1.AuthzClusterRole, error) {
	return nil, authz.ErrAuthzDisabled
}

func (da *DisabledAuthorizer) GetClusterRole(ctx context.Context, name string) (*openchoreov1alpha1.AuthzClusterRole, error) {
	return nil, authz.ErrAuthzDisabled
}

func (da *DisabledAuthorizer) ListClusterRoles(ctx context.Context) (*openchoreov1alpha1.AuthzClusterRoleList, error) {
	return nil, authz.ErrAuthzDisabled
}

func (da *DisabledAuthorizer) UpdateClusterRole(ctx context.Context, role *openchoreov1alpha1.AuthzClusterRole) (*openchoreov1alpha1.AuthzClusterRole, error) {
	return nil, authz.ErrAuthzDisabled
}

func (da *DisabledAuthorizer) CreateNamespacedRole(ctx context.Context, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error) {
	return nil, authz.ErrAuthzDisabled
}

func (da *DisabledAuthorizer) GetNamespacedRole(ctx context.Context, name string, namespace string) (*openchoreov1alpha1.AuthzRole, error) {
	return nil, authz.ErrAuthzDisabled
}

func (da *DisabledAuthorizer) ListNamespacedRoles(ctx context.Context, namespace string) (*openchoreov1alpha1.AuthzRoleList, error) {
	return nil, authz.ErrAuthzDisabled
}

func (da *DisabledAuthorizer) UpdateNamespacedRole(ctx context.Context, role *openchoreov1alpha1.AuthzRole) (*openchoreov1alpha1.AuthzRole, error) {
	return nil, authz.ErrAuthzDisabled
}

func (da *DisabledAuthorizer) CreateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzClusterRoleBinding) (*openchoreov1alpha1.AuthzClusterRoleBinding, error) {
	return nil, authz.ErrAuthzDisabled
}

func (da *DisabledAuthorizer) GetClusterRoleBinding(ctx context.Context, name string) (*openchoreov1alpha1.AuthzClusterRoleBinding, error) {
	return nil, authz.ErrAuthzDisabled
}

func (da *DisabledAuthorizer) ListClusterRoleBindings(ctx context.Context) (*openchoreov1alpha1.AuthzClusterRoleBindingList, error) {
	return nil, authz.ErrAuthzDisabled
}

func (da *DisabledAuthorizer) UpdateClusterRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzClusterRoleBinding) (*openchoreov1alpha1.AuthzClusterRoleBinding, error) {
	return nil, authz.ErrAuthzDisabled
}

func (da *DisabledAuthorizer) CreateNamespacedRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	return nil, authz.ErrAuthzDisabled
}

func (da *DisabledAuthorizer) GetNamespacedRoleBinding(ctx context.Context, name string, namespace string) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	return nil, authz.ErrAuthzDisabled
}

func (da *DisabledAuthorizer) ListNamespacedRoleBindings(ctx context.Context, namespace string) (*openchoreov1alpha1.AuthzRoleBindingList, error) {
	return nil, authz.ErrAuthzDisabled
}

func (da *DisabledAuthorizer) UpdateNamespacedRoleBinding(ctx context.Context, binding *openchoreov1alpha1.AuthzRoleBinding) (*openchoreov1alpha1.AuthzRoleBinding, error) {
	return nil, authz.ErrAuthzDisabled
}

// These var declarations enforce at compile-time that DisabledAuthorizer
// implements the PDP and PAP interfaces correctly.
var (
	_ authz.PDP = (*DisabledAuthorizer)(nil)
	_ authz.PAP = (*DisabledAuthorizer)(nil)
)
