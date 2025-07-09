// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// APIClassSpec defines the desired state of APIClass.
type APIClassSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	RESTPolicy *RESTAPIPolicy `json:"restPolicy,omitempty"`
	GRPCPolicy *GRPCAPIPolicy `json:"grpcPolicy,omitempty"`
}

// RESTAPIPolicy defines REST-specific API policies
type RESTAPIPolicy struct {
	// Default policies that apply to all expose levels
	Defaults *RESTPolicyWithConditionals `json:"defaults,omitempty"`
	// Override policies for public expose level
	Public *RESTPolicyWithConditionals `json:"public,omitempty"`
	// Override policies for organization expose level
	Organization *RESTPolicyWithConditionals `json:"organization,omitempty"`
}

// GRPCAPIPolicy defines gRPC-specific API policies (placeholder for future implementation)
type GRPCAPIPolicy struct {
	// TODO: Implement gRPC-specific policies
}

// APIClassStatus defines the observed state of APIClass.
type APIClassStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// APIClass is the Schema for the apiclasses API.
type APIClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   APIClassSpec   `json:"spec,omitempty"`
	Status APIClassStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// APIClassList contains a list of APIClass.
type APIClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []APIClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&APIClass{}, &APIClassList{})
}
