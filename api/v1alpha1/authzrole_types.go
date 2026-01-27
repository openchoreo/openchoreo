// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AuthzRoleSpec defines the desired state of AuthzRole
type AuthzRoleSpec struct {
	// Actions is the list of actions this role can perform
	// +kubebuilder:validation:MinItems=1
	// +required
	Actions []string `json:"actions"`

	// Description is a human-readable description of the role
	// +optional
	Description string `json:"description,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Actions",type=string,JSONPath=`.spec.actions`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AuthzRole is the Schema for the authzroles API
type AuthzRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AuthzRoleSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// AuthzRoleList contains a list of AuthzRole
type AuthzRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AuthzRole `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AuthzRole{}, &AuthzRoleList{})
}
