// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// APISpec defines the desired state of API.
type APISpec struct {
	Owner EndpointOwner `json:"owner"`

	// +kubebuilder:validation:MinLength=1
	EnvironmentName string `json:"environmentName"`

	EndpointTemplateSpec `json:",inline"`
}

// APIStatus defines the observed state of API.
type APIStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
	Address    string             `json:"address,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// API is the Schema for the apis API.
type API struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   APISpec   `json:"spec,omitempty"`
	Status APIStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// APIList contains a list of API.
type APIList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []API `json:"items"`
}

func init() {
	SchemeBuilder.Register(&API{}, &APIList{})
}
