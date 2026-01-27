// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AuthzClusterRoleSpec defines the desired state of AuthzClusterRole
type AuthzClusterRoleSpec struct {
	// Actions is the list of actions this role can perform
	// +kubebuilder:validation:MinItems=1
	// +required
	Actions []string `json:"actions"`

	// Description is a human-readable description of the role
	// +optional
	Description string `json:"description,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Actions",type=string,JSONPath=`.spec.actions`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AuthzClusterRole is the Schema for the authzclusterroles API
type AuthzClusterRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AuthzClusterRoleSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// AuthzClusterRoleList contains a list of AuthzClusterRole
type AuthzClusterRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AuthzClusterRole `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AuthzClusterRole{}, &AuthzClusterRoleList{})
}
