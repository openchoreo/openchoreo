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

// Context carries root-namespaced ABAC attributes available to CEL condition expressions.
// Each root corresponds to a CEL variable (e.g. resource.environment) and maps to a Go field here.
// Adding new roots (principal, request) is a non-breaking additive change.
type Context struct {
	// Resource holds attributes of the target resource instance.
	Resource ResourceAttribute `json:"resource,omitempty"`
}

// ResourceAttribute holds target-resource attributes exposed to CEL under the "resource" root.
type ResourceAttribute struct {
	// Environment is the target environment (e.g. "dev", "staging", "prod").
	Environment string `json:"environment,omitempty"`
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

// ActionCapability represents capabilities for a specific action
type ActionCapability struct {
	Allowed []*CapabilityResource `json:"allowed"`
	Denied  []*CapabilityResource `json:"denied"`
}

// CapabilityResource represents a resource with permission details (SIMPLIFIED)
type CapabilityResource struct {
	Path        string       `json:"path"`        // Full resource path: "namespace/acme/project/payment"
	Constraints *interface{} `json:"constraints"` // represents additional instance level restrictions

}

// UserCapabilitiesResponse represents the complete capabilities response
type UserCapabilitiesResponse struct {
	User         *SubjectContext              `json:"user"`
	Capabilities map[string]*ActionCapability `json:"capabilities"`
	GeneratedAt  time.Time                    `json:"evaluatedAt"`
}

var (
	ErrAuthzDisabled             = fmt.Errorf("authorization is disabled - policy management operations are not available")
	ErrRoleAlreadyExists         = fmt.Errorf("role already exists")
	ErrRoleNotFound              = fmt.Errorf("role not found")
	ErrRoleInUse                 = fmt.Errorf("role is in use and cannot be deleted")
	ErrRoleMappingAlreadyExists  = fmt.Errorf("role mapping already exists")
	ErrRoleMappingNotFound       = fmt.Errorf("role mapping not found")
	ErrCannotDeleteSystemMapping = fmt.Errorf("cannot delete system mapping")
	ErrCannotModifySystemMapping = fmt.Errorf("cannot modify system mapping")
	ErrInvalidRequest            = fmt.Errorf("invalid request")
)
