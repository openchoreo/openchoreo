// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// WebApplicationBindingSpec defines the desired state of WebApplicationBinding.
type WebApplicationBindingSpec struct {
	// Owner defines the component and project that owns this web application binding
	Owner WebApplicationOwner `json:"owner"`

	// Environment is the target environment for this binding
	// +kubebuilder:validation:MinLength=1
	Environment string `json:"environment"`
	
	// ClassName is the name of the web application class that provides the web application-specific deployment configuration.
	ClassName string `json:"className"`

	// WorkloadSpec contains the copied workload specification for this environment-specific binding
	WorkloadSpec WorkloadTemplateSpec `json:"workloadSpec"`

	// Overrides contains web application-specific overrides for this binding
	Overrides map[string]bool `json:"overrides,omitempty"`
}

// WebApplicationBindingStatus defines the observed state of WebApplicationBinding.
type WebApplicationBindingStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// WebApplicationBinding is the Schema for the webapplicationbindings API.
type WebApplicationBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WebApplicationBindingSpec   `json:"spec,omitempty"`
	Status WebApplicationBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WebApplicationBindingList contains a list of WebApplicationBinding.
type WebApplicationBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WebApplicationBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WebApplicationBinding{}, &WebApplicationBindingList{})
}
