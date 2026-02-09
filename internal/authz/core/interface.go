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

	// RemoveRole deletes a role identified by RoleRef
	RemoveRole(ctx context.Context, roleRef *RoleRef) error

	// GetRole retrieves a role identified by RoleRef
	GetRole(ctx context.Context, roleRef *RoleRef) (*Role, error)

	// UpdateRole updates an existing role's actions
	UpdateRole(ctx context.Context, role *Role) error

	// ListRoles returns roles based on the provided filter
	ListRoles(ctx context.Context, filter *RoleFilter) ([]*Role, error)

	GetRoleEntitlementMapping(ctx context.Context, mappingRef *MappingRef) (*RoleEntitlementMapping, error)

	// AddRoleEntitlementMapping creates a new role-entitlement mapping with optional conditions
	AddRoleEntitlementMapping(ctx context.Context, mapping *RoleEntitlementMapping) error

	// UpdateRoleEntitlementMapping updates an existing role-entitlement mapping
	UpdateRoleEntitlementMapping(ctx context.Context, mapping *RoleEntitlementMapping) error

	// RemoveRoleEntitlementMapping removes a role-entitlement mapping
	RemoveRoleEntitlementMapping(ctx context.Context, mappingRef *MappingRef) error

	// ListRoleEntitlementMappings lists role-entitlement mappings with optional filters
	ListRoleEntitlementMappings(ctx context.Context, filter *RoleEntitlementMappingFilter) ([]*RoleEntitlementMapping, error)

	// ListActions lists all defined actions in the system
	ListActions(ctx context.Context) ([]string, error)
}
