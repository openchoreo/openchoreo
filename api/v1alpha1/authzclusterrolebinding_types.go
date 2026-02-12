// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AuthzClusterRoleBindingSpec defines the desired state of AuthzClusterRoleBinding
// +kubebuilder:validation:XValidation:rule="self.roleRef.kind == 'AuthzClusterRole'",message="AuthzClusterRoleBinding can only reference AuthzClusterRole"
type AuthzClusterRoleBindingSpec struct {
	// Entitlement defines the subject (from JWT claims) to grant the role to
	// +required
	Entitlement EntitlementClaim `json:"entitlement"`

	// RoleRef references the AuthzClusterRole to bind
	// NOTE: AuthzClusterRoleBinding can ONLY reference AuthzClusterRole
	// +required
	RoleRef RoleRef `json:"roleRef"`

	// Effect indicates whether this binding allows or denies access (default: allow)
	// +kubebuilder:default=allow
	// +optional
	Effect EffectType `json:"effect,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AuthzClusterRoleBinding is the Schema for the authzclusterrolebindings API
type AuthzClusterRoleBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AuthzClusterRoleBindingSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// AuthzClusterRoleBindingList contains a list of AuthzClusterRoleBinding
type AuthzClusterRoleBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AuthzClusterRoleBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AuthzClusterRoleBinding{}, &AuthzClusterRoleBindingList{})
}
