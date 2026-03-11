// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterTargetScope defines which resources this cluster binding applies to within the ownership hierarchy.
// All fields are optional - omitted fields mean "all" at that level.
type ClusterTargetScope struct {
	// Namespace scopes to a specific namespace (optional)
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Project scopes to a specific project (optional, requires namespace)
	// +optional
	Project string `json:"project,omitempty"`

	// Component scopes to a specific component (optional, requires project)
	// +optional
	Component string `json:"component,omitempty"`
}

// ClusterRoleMapping pairs a role reference with an optional scope for cluster-scoped bindings
// +kubebuilder:validation:XValidation:rule="!has(self.scope) || (!has(self.scope.project) || has(self.scope.namespace)) && (!has(self.scope.component) || has(self.scope.project))",message="scope.project requires scope.namespace, and scope.component requires scope.project"
type ClusterRoleMapping struct {
	// RoleRef references the AuthzClusterRole to bind
	RoleRef RoleRef `json:"roleRef"`

	// Scope defines the target scope within the ownership hierarchy
	// +optional
	Scope ClusterTargetScope `json:"scope,omitempty"`
}

// AuthzClusterRoleBindingSpec defines the desired state of AuthzClusterRoleBinding
// +kubebuilder:validation:XValidation:rule="self.roleMappings.all(m, m.roleRef.kind == 'AuthzClusterRole')",message="AuthzClusterRoleBinding can only reference AuthzClusterRole"
type AuthzClusterRoleBindingSpec struct {
	// Entitlement defines the subject (from JWT claims) to grant the role to
	// +required
	Entitlement EntitlementClaim `json:"entitlement"`

	// RoleMappings is the list of cluster roles this binding grants
	// +required
	// +kubebuilder:validation:MinItems=1
	RoleMappings []ClusterRoleMapping `json:"roleMappings"`

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
