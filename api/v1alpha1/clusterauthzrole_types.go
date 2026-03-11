// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterAuthzRoleSpec defines the desired state of ClusterAuthzRole
type ClusterAuthzRoleSpec struct {
	// Actions is the list of actions this role can perform
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:example:={"component:create"}
	// +required
	Actions []string `json:"actions"`

	// Description is a human-readable description of the role
	// +optional
	Description string `json:"description,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ClusterAuthzRole is the Schema for the clusterauthzroles API
type ClusterAuthzRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ClusterAuthzRoleSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterAuthzRoleList contains a list of ClusterAuthzRole
type ClusterAuthzRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []ClusterAuthzRole `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterAuthzRole{}, &ClusterAuthzRoleList{})
}
