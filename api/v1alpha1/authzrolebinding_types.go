// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TargetScope defines which resources this binding applies to within the ownership hierarchy
// All fields are optional - omitted fields mean "all" at that level
type TargetScope struct {
	// Project scopes to a specific project (optional)
	// +optional
	Project string `json:"project,omitempty"`

	// Component scopes to a specific component (optional)
	// +optional
	Component string `json:"component,omitempty"`
}

// RoleMapping pairs a role reference with an optional scope
// +kubebuilder:validation:XValidation:rule="!has(self.scope) || !has(self.scope.component) || has(self.scope.project)",message="scope.component requires scope.project"
type RoleMapping struct {
	// RoleRef references the role to bind
	RoleRef RoleRef `json:"roleRef"`

	// Scope defines the target scope within the ownership hierarchy
	// +optional
	Scope TargetScope `json:"scope,omitempty"`
}

// AuthzRoleBindingSpec defines the desired state of AuthzRoleBinding
type AuthzRoleBindingSpec struct {
	// Entitlement defines the subject (from JWT claims) to grant the role to
	// +required
	Entitlement EntitlementClaim `json:"entitlement"`

	// RoleMappings is the list of role-scope pairs this binding grants
	// +required
	// +kubebuilder:validation:MinItems=1
	RoleMappings []RoleMapping `json:"roleMappings"`

	// Effect indicates whether this binding allows or denies access (default: allow)
	// +kubebuilder:default=allow
	// +optional
	Effect EffectType `json:"effect,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AuthzRoleBinding is the Schema for the authzrolebindings API
type AuthzRoleBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AuthzRoleBindingSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// AuthzRoleBindingList contains a list of AuthzRoleBinding
type AuthzRoleBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AuthzRoleBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AuthzRoleBinding{}, &AuthzRoleBindingList{})
}
