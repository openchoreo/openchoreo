// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TargetPath defines which resources this binding applies to within the ownership hierarchy
// All fields are optional - omitted fields mean "all" at that level
type TargetPath struct {
	// Project scopes to a specific project (optional)
	// +optional
	Project string `json:"project,omitempty"`

	// Component scopes to a specific component (optional)
	// +optional
	Component string `json:"component,omitempty"`
}

// AuthzRoleBindingSpec defines the desired state of AuthzRoleBinding
type AuthzRoleBindingSpec struct {
	// Entitlement defines the subject (from JWT claims) to grant the role to
	// +required
	Entitlement EntitlementClaim `json:"entitlement"`

	// RoleRef references the role to bind
	// Can reference AuthzClusterRole (platform-wide) or AuthzRole (same namespace only)
	// +required
	RoleRef RoleRef `json:"roleRef"`

	// TargetPath defines which resources this binding applies to within the ownership hierarchy
	// All fields are optional - omitted fields mean "all" at that level
	// +optional
	TargetPath TargetPath `json:"targetPath,omitempty"`

	// Effect indicates whether this binding allows or denies access (default: allow)
	// +kubebuilder:default=allow
	// +optional
	Effect EffectType `json:"effect,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Subject",type=string,JSONPath=`.spec.entitlement.value`
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.spec.roleRef.name`
// +kubebuilder:printcolumn:name="Effect",type=string,JSONPath=`.spec.effect`
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
