// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authz

import "context"

// PDP (Policy Decision Point) interface defines the contract for authorization evaluation
type PDP interface {
	// Evaluate evaluates a single authorization request and returns a decision
	Evaluate(ctx context.Context, request *EvaluateRequest) (Decision, error)

	// BatchEvaluate evaluates multiple authorization requests and returns corresponding decisions
	BatchEvaluate(ctx context.Context, request *BatchEvaluateRequest) (BatchEvaluateResponse, error)

	// GetSubjectProfile retrieves the authorization profile for a given subject
	GetSubjectProfile(ctx context.Context, request *ProfileRequest) (SubjectProfile, error)
}

// PAP (Policy Administration Point) interface defines the contract for policy management
type PAP interface {
	// AddRole creates a new role with the specified name and actions
	AddRole(ctx context.Context, role Role) error

	// RemoveRole deletes a role by name
	RemoveRole(ctx context.Context, roleName string) error

	// GetRole retrieves a role by name
	GetRole(ctx context.Context, roleName string) (Role, error)

	// ListRoles returns all defined roles
	ListRoles(ctx context.Context) ([]Role, error)

	// AddRolePrincipalMapping creates a new role-principal mapping with optional conditions
	AddRolePrincipalMapping(ctx context.Context, mapping *PolicyMapping) error

	// RemoveRolePrincipalMapping removes a role-principal mapping
	RemoveRolePrincipalMapping(ctx context.Context, mapping *PolicyMapping) error

	// GetPrincipalMappings retrieves all role mappings for a specific principal
	GetPrincipalMappings(ctx context.Context, principal string) ([]PolicyMapping, error)

	// GetRoleMappings retrieves all principal mappings for a specific role
	GetRoleMappings(ctx context.Context, roleName string) ([]PolicyMapping, error)

	// ListRolePrincipalMappings lists all role-principal mappings
	ListRolePrincipalMappings(ctx context.Context) ([]PolicyMapping, error)

	// AddAction registers a new action in the system
	AddAction(ctx context.Context, action string) error

	// GetAction retrieves an action by name
	GetAction(ctx context.Context, action string) (string, error)

	// ListActions lists all defined actions in the system
	ListActions(ctx context.Context) ([]string, error)

	// DeleteAction removes an action from the system
	DeleteAction(ctx context.Context, action string) error
}
