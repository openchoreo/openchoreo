// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ServiceBindingSpec defines the desired state of ServiceBinding.
type ServiceBindingSpec struct {
	// Owner defines the component and project that owns this service binding
	Owner ServiceOwner `json:"owner"`

	// Environment is the target environment for this binding
	// +kubebuilder:validation:MinLength=1
	Environment string `json:"environment"`
	// ClassName is the name of the service class that provides the service-specific deployment configuration.
	ClassName string `json:"className"`

	WorkloadSpec WorkloadTemplateSpec `json:"workloadSpec"`

	APIs map[string]*ServiceAPI `json:"apis,omitempty"`
}

// ServiceBindingStatus defines the observed state of ServiceBinding.
type ServiceBindingStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ServiceBinding is the Schema for the servicebindings API.
type ServiceBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceBindingSpec   `json:"spec,omitempty"`
	Status ServiceBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceBindingList contains a list of ServiceBinding.
type ServiceBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServiceBinding{}, &ServiceBindingList{})
}
