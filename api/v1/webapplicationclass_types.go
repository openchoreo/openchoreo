// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// WebApplicationClassSpec defines the desired state of WebApplicationClass.
type WebApplicationClassSpec struct {
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	DeploymentTemplate appsv1.DeploymentSpec `json:"deploymentTemplate,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	ServiceTemplate corev1.ServiceSpec `json:"serviceTemplate,omitempty"`
}

// WebApplicationClassStatus defines the observed state of WebApplicationClass.
type WebApplicationClassStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// WebApplicationClass is the Schema for the webapplicationclasses API.
type WebApplicationClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WebApplicationClassSpec   `json:"spec,omitempty"`
	Status WebApplicationClassStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WebApplicationClassList contains a list of WebApplicationClass.
type WebApplicationClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WebApplicationClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WebApplicationClass{}, &WebApplicationClassList{})
}
