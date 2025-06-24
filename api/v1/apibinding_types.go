// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// APIBindingSpec defines the desired state of APIBinding.
type APIBindingSpec struct {
	// APIClassName specifies the APIClass to use for this binding
	// +kubebuilder:validation:MinLength=1
	APIClassName string `json:"apiClassName"`

	// APIName specifies the API resource to bind
	// +kubebuilder:validation:MinLength=1
	APIName string `json:"apiName"`

	// Environment specifies the target environment for this binding
	// +kubebuilder:validation:MinLength=1
	EnvironmentName string `json:"environmentName"`
}

// APIBindingStatus defines the observed state of APIBinding.
type APIBindingStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// APIBinding is the Schema for the apibindings API.
type APIBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   APIBindingSpec   `json:"spec,omitempty"`
	Status APIBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// APIBindingList contains a list of APIBinding.
type APIBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []APIBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&APIBinding{}, &APIBindingList{})
}
