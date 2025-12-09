// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package core

import "context"

// PDP (Policy Decision Point) interface defines the contract for authorization evaluation
type PDP interface {
	// Evaluate evaluates a single authorization request and returns a decision
	Evaluate(ctx context.Context, request *EvaluateRequest) (*Decision, error)

	// BatchEvaluate evaluates multiple authorization requests and returns corresponding decisions
	BatchEvaluate(ctx context.Context, request *BatchEvaluateRequest) (*BatchEvaluateResponse, error)

	// GetSubjectProfile retrieves the authorization profile for a given subject
	GetSubjectProfile(ctx context.Context, request *ProfileRequest) (*UserCapabilitiesResponse, error)
}

// PAP (Policy Administration Point) interface defines the contract for policy management
type PAP interface {
	// AddRole creates a new role with the specified name and actions
	AddRole(ctx context.Context, role *Role) error

	// RemoveRole deletes a role by name
	RemoveRole(ctx context.Context, roleName string) error

	// GetRole retrieves a role by name
	GetRole(ctx context.Context, roleName string) (*Role, error)

	// ListRoles returns all defined roles
	ListRoles(ctx context.Context) ([]*Role, error)

	// AddRoleEntitlementMapping creates a new role-entitlement mapping with optional conditions
	AddRoleEntitlementMapping(ctx context.Context, mapping *RoleEntitlementMapping) error

	// RemoveRoleEntitlementMapping removes a role-entitlement mapping
	RemoveRoleEntitlementMapping(ctx context.Context, mapping *RoleEntitlementMapping) error

	// GetRoleMappings retrieves all entitlement mappings for a specific role
	GetRoleMappings(ctx context.Context, roleName string) ([]*RoleEntitlementMapping, error)

	// ListRoleEntitlementMappings lists all role-entitlement mappings
	ListRoleEntitlementMappings(ctx context.Context) ([]*RoleEntitlementMapping, error)

	// ListActions lists all defined actions in the system
	ListActions(ctx context.Context) ([]string, error)

	// ListUserTypes returns all configured user types in the system
	ListUserTypes(ctx context.Context) ([]UserTypeInfo, error)
}
