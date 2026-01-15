// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"time"
)

// PolicyEffectType defines the effect of a policy: allow or deny
type PolicyEffectType string

const (
	PolicyEffectAllow PolicyEffectType = "allow"
	PolicyEffectDeny  PolicyEffectType = "deny"
)

// SubjectContext represents the authenticated subject making the authorization request
type SubjectContext struct {
	Type              string   `json:"type"`
	EntitlementClaim  string   `json:"entitlement_claim"`
	EntitlementValues []string `json:"entitlement_values"`
}

// ResourceHierarchy represents a single item in a resource hierarchy
type ResourceHierarchy struct {
	Namespace string `json:"namespace,omitempty"`
	Project   string `json:"project,omitempty"`
	Component string `json:"component,omitempty"`
}

// Resource represents a resource in the authorization request
type Resource struct {
	Type      string            `json:"type"`
	ID        string            `json:"id,omitempty"`
	Hierarchy ResourceHierarchy `json:"hierarchy"`
}

// Context additional resource instance level context
type Context struct {
	// This field is used for storing arbitrary key-value pairs that can be used for policy evaluation
	// TODO: Define specific context fields as needed
}

// Decision represents the authorization decision response
type Decision struct {
	Decision bool             `json:"decision"`
	Context  *DecisionContext `json:"context,omitempty"`
}

// DecisionContext contains additional context about the decision
type DecisionContext struct {
	Reason string `json:"reason,omitempty"`
}

// EvaluateRequest represents a single authorization request
type EvaluateRequest struct {
	SubjectContext *SubjectContext `json:"subject_context"`
	Resource       Resource        `json:"resource"`
	Action         string          `json:"action"`
	Context        Context         `json:"context"`
}

// BatchEvaluateRequest represents a batch of authorization requests
type BatchEvaluateRequest struct {
	Requests []EvaluateRequest `json:"requests"`
}

// BatchEvaluateResponse represents a batch of authorization decisions
type BatchEvaluateResponse struct {
	Decisions []Decision `json:"decisions"`
}

// ProfileRequest represents a request to retrieve a subject's authorization profile
type ProfileRequest struct {
	// SubjectContext is the authenticated subject whose profile is being requested
	SubjectContext *SubjectContext `json:"subject_context"`

	// Scope is the resource hierarchy scope for the profile
	Scope ResourceHierarchy `json:"scope"`
}

// Role represents a role with a set of allowed actions
// If Namespace is empty, it's a cluster-scoped role; otherwise it's namespace-scoped
type Role struct {
	// Name is the unique identifier for the role
	Name string `json:"name"`

	// Actions is the list of actions this role permits
	Actions []string `json:"actions"`

	// Namespace is the namespace for namespace-scoped roles, empty for cluster roles
	Namespace string `json:"namespace,omitempty"`

	// IsInternal indicates if this role should be hidden from public listings
	IsInternal bool `json:"-"`
}

// RoleRef uniquely identifies a role by name and namespace
type RoleRef struct {
	// Name is the role name
	Name string `json:"name" yaml:"name"`

	// Namespace identifies the role scope:
	// - Empty string ("") = cluster-scoped role
	// - Non-empty = namespace-scoped role in the specified namespace
	Namespace string `json:"namespace,omitempty" yaml:"namespace"`
}

// Entitlement represents an entitlement with its claim and value
type Entitlement struct {
	// Claim is the JWT claim name (e.g., "group", "sub")
	Claim string `json:"claim" yaml:"claim"`

	// Value is the entitlement value (e.g., "admin-group", "service-123")
	Value string `json:"value" yaml:"value"`
}

// RoleEntitlementMapping represents the assignment of a role to an entitlement within a hierarchical scope
type RoleEntitlementMapping struct {
	// ID is the unique identifier for the mapping
	ID uint `json:"id" yaml:"id,omitempty"`

	// RoleRef identifies the role being assigned
	RoleRef RoleRef `json:"role" yaml:"role"`

	// Entitlement contains the claim and value for this mapping
	Entitlement Entitlement `json:"entitlement" yaml:"entitlement"`

	// Hierarchy defines the resource hierarchy scope where this role applies
	// Empty hierarchy means global scope (*)
	Hierarchy ResourceHierarchy `json:"hierarchy" yaml:"hierarchy,omitempty"`

	// Effect indicates whether the mapping is to allow or deny access
	Effect PolicyEffectType `json:"effect" yaml:"effect"`

	// Context provides optional additional context metadata for this mapping
	Context Context `json:"context" yaml:"context,omitempty"`

	// IsInternal indicates if this mapping should be hidden from public listings
	IsInternal bool `json:"-" yaml:"-"`
}

// RoleEntitlementMappingFilter provides filters for listing role-entitlement mappings
type RoleEntitlementMappingFilter struct {
	// RoleRef filters mappings by role reference (name and namespace)
	RoleRef *RoleRef

	// Entitlement filters mappings by entitlement claim and value
	Entitlement *Entitlement
}

// RoleFilter provides filters for listing roles
type RoleFilter struct {
	// Namespace filters roles by namespace scope:
	// - "" (empty) = cluster-scoped roles only
	// - "ns1" = namespace-scoped roles in "ns1" only
	// Ignored when IncludeAll is true
	Namespace string

	// IncludeAll when true returns all roles (cluster + all namespaces)
	IncludeAll bool
}

// ActionCapability represents capabilities for a specific action
type ActionCapability struct {
	Allowed []*CapabilityResource `json:"allowed"`
	Denied  []*CapabilityResource `json:"denied"`
}

// CapabilityResource represents a resource with permission details (SIMPLIFIED)
type CapabilityResource struct {
	Path        string       `json:"path"`        // Full resource path: "org/acme/project/payment"
	Constraints *interface{} `json:"constraints"` // represents additional instance level restrictions

}

// UserCapabilitiesResponse represents the complete capabilities response
type UserCapabilitiesResponse struct {
	User         *SubjectContext              `json:"user"`
	Capabilities map[string]*ActionCapability `json:"capabilities"`
	GeneratedAt  time.Time                    `json:"evaluatedAt"`
}

var (
	ErrAuthzDisabled                  = fmt.Errorf("authorization is disabled - policy management operations are not available")
	ErrRoleAlreadyExists              = fmt.Errorf("role already exists")
	ErrRoleNotFound                   = fmt.Errorf("role not found")
	ErrRoleInUse                      = fmt.Errorf("role is in use and cannot be deleted")
	ErrRolePolicyMappingAlreadyExists = fmt.Errorf("role policy mapping already exists")
	ErrRolePolicyMappingNotFound      = fmt.Errorf("role policy mapping not found")
	ErrCannotDeleteSystemMapping      = fmt.Errorf("cannot delete system mapping")
	ErrCannotModifySystemMapping      = fmt.Errorf("cannot modify system mapping")
	ErrInvalidRequest                 = fmt.Errorf("invalid request")
)
