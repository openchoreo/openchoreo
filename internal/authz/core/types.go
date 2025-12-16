// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"time"
)

// SubjectType defines the type of subject making the authorization request
type SubjectType string

const (
	SubjectTypeUser           SubjectType = "user"
	SubjectTypeServiceAccount SubjectType = "service_account"
)

// PolicyEffectType defines the effect of a policy: allow or deny
type PolicyEffectType string

const (
	PolicyEffectAllow PolicyEffectType = "allow"
	PolicyEffectDeny  PolicyEffectType = "deny"
)

// Subject represents the actor making the authorization request
type Subject struct {
	JwtToken string `json:"jwt_token"`
}

// SubjectContext - internal auth context for the subject
type SubjectContext struct {
	Type              SubjectType
	EntitlementClaim  string
	EntitlementValues []string
}

// ResourceHierarchy represents a single item in a resource hierarchy
type ResourceHierarchy struct {
	Organization      string   `json:"organization,omitempty"`
	OrganizationUnits []string `json:"organization_units,omitempty"`
	Project           string   `json:"project,omitempty"`
	Component         string   `json:"component,omitempty"`
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
	Subject  Subject  `json:"subject"`
	Resource Resource `json:"resource"`
	Action   string   `json:"action"`
	Context  Context  `json:"context"`
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
	// Subject is the actor whose profile is being requested
	Subject Subject `json:"subject"`

	// Scope is the resource hierarchy scope for the profile
	Scope ResourceHierarchy `json:"scope"`
}

// Role represents a role with a set of allowed actions
type Role struct {
	// Name is the unique identifier for the role
	Name string `json:"name"`

	// Actions is the list of actions this role permits
	Actions []string `json:"actions"`

	// IsInternal indicates if this role should be hidden from public listings
	IsInternal bool `json:"-"`
}

// Entitlement represents an entitlement with its claim and value
type Entitlement struct {
	// Claim is the JWT claim name (e.g., "group", "sub")
	Claim string `json:"claim" yaml:"claim"`

	// Value is the entitlement value (e.g., "admin-group", "service-123")
	Value string `json:"value" yaml:"value"`
}

// EntitlementClaimInfo represents information about an entitlement claim
type EntitlementClaimInfo struct {
	// Name is the JWT claim name (e.g., "group", "sub")
	Name string `json:"name"`

	// DisplayName is a human-readable name for the claim
	DisplayName string `json:"display_name"`
}

// UserTypeInfo represents information about a configured user type
type UserTypeInfo struct {
	// Type is the user type (e.g., "user", "service_account")
	Type SubjectType `json:"type"`

	// DisplayName is a human-readable name for the user type
	DisplayName string `json:"display_name"`

	// Priority determines the order in which user types are checked (lower = higher priority)
	Priority int `json:"priority"`

	// Entitlement contains information about the entitlement claim
	Entitlement EntitlementClaimInfo `json:"entitlement"`
}

// RoleEntitlementMapping represents the assignment of a role to an entitlement within a hierarchical scope
type RoleEntitlementMapping struct {
	// ID is the unique identifier for the mapping
	ID uint `json:"id" yaml:"id,omitempty"`

	// RoleName is the name of the role being assigned
	RoleName string `json:"role_name" yaml:"role_name"`

	// Entitlement contains the claim and value for this mapping
	Entitlement Entitlement `json:"entitlement" yaml:"entitlement"`

	// Hierarchy defines the resource hierarchy scope where this role applies
	Hierarchy ResourceHierarchy `json:"hierarchy" yaml:"hierarchy,omitempty"`

	// Effect indicates whether the mapping is to allow or deny access
	Effect PolicyEffectType `json:"effect" yaml:"effect,omitempty"`

	// Context provides optional additional context metadata for this mapping
	Context Context `json:"context" yaml:"context,omitempty"`

	// IsInternal indicates if this mapping should be hidden from public listings
	IsInternal bool `json:"-" yaml:"-"`
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
	ErrRolePolicyMappingAlreadyExists = fmt.Errorf("role policy mapping already exists")
	ErrRolePolicyMappingNotFound      = fmt.Errorf("role policy mapping not found")
	ErrInvalidRequest                 = fmt.Errorf("invalid request")
)
