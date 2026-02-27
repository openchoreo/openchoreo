// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConnectionTarget identifies a specific endpoint on a target component to resolve.
type ConnectionTarget struct {
	// Namespace is the control plane namespace of the target component.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`

	// Project is the name of the project that owns the target component.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Project string `json:"project"`

	// Component is the name of the target component.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Component string `json:"component"`

	// Endpoint is the name of the endpoint on the target component.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Endpoint string `json:"endpoint"`

	// Visibility is the desired visibility level for resolving the endpoint URL.
	// +kubebuilder:validation:Required
	Visibility EndpointVisibility `json:"visibility"`
}

// ResolvedConnection holds the resolved URL for a single connection.
type ResolvedConnection struct {
	// Namespace is the control plane namespace of the target component.
	Namespace string `json:"namespace"`

	// Project is the name of the project that owns the target component.
	Project string `json:"project"`

	// Component is the name of the target component.
	Component string `json:"component"`

	// Endpoint is the name of the endpoint on the target component.
	Endpoint string `json:"endpoint"`

	// URL is the resolved endpoint URL.
	URL EndpointURL `json:"url"`
}

// PendingConnection represents a connection that could not be resolved.
type PendingConnection struct {
	// Namespace is the control plane namespace of the target component.
	Namespace string `json:"namespace"`

	// Project is the name of the project that owns the target component.
	Project string `json:"project"`

	// Component is the name of the target component.
	Component string `json:"component"`

	// Endpoint is the name of the endpoint on the target component.
	Endpoint string `json:"endpoint"`

	// Reason describes why the connection could not be resolved.
	Reason string `json:"reason"`
}

// ConnectionBindingSpec defines the desired state of ConnectionBinding.
type ConnectionBindingSpec struct {
	// ReleaseBindingRef is the name of the ReleaseBinding this ConnectionBinding belongs to.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.releaseBindingRef is immutable"
	ReleaseBindingRef string `json:"releaseBindingRef"`

	// Environment is the name of the environment this ConnectionBinding resolves connections for.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.environment is immutable"
	Environment string `json:"environment"`

	// Connections is the list of connection targets to resolve.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Connections []ConnectionTarget `json:"connections"`
}

// ConnectionBindingStatus defines the observed state of ConnectionBinding.
type ConnectionBindingStatus struct {
	// Conditions represent the latest available observations of the ConnectionBinding's current state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Resolved contains the connections that have been successfully resolved.
	// +optional
	Resolved []ResolvedConnection `json:"resolved,omitempty"`

	// Pending contains the connections that could not be resolved.
	// +optional
	Pending []PendingConnection `json:"pending,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ReleaseBinding",type=string,JSONPath=`.spec.releaseBindingRef`
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.spec.environment`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ConnectionBinding is the Schema for the connectionbindings API.
type ConnectionBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConnectionBindingSpec   `json:"spec,omitempty"`
	Status ConnectionBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ConnectionBindingList contains a list of ConnectionBinding.
type ConnectionBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConnectionBinding `json:"items"`
}

// GetConditions returns the conditions from the status.
func (cb *ConnectionBinding) GetConditions() []metav1.Condition {
	return cb.Status.Conditions
}

// SetConditions sets the conditions in the status.
func (cb *ConnectionBinding) SetConditions(conditions []metav1.Condition) {
	cb.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&ConnectionBinding{}, &ConnectionBindingList{})
}
