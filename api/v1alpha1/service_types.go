// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ServiceSpec defines the desired state of Service.
type ServiceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Owner ServiceOwner `json:"owner"`

	// WorkloadName is the name of the workload that this service is referencing.
	WorkloadName string `json:"workloadName"`
	// ClassName is the name of the service class that provides the service-specific deployment configuration.
	// +kubebuilder:default=default
	ClassName string `json:"className"`

	Overrides map[string]bool `json:"overrides,omitempty"` // TODO: Think about how to structure this

	APIs map[string]*ServiceAPI `json:"apis,omitempty"`
}

type ServiceOwner struct {
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`
	// +kubebuilder:validation:MinLength=1
	ComponentName string `json:"componentName"`
}

type ServiceAPI struct {
	EndpointTemplateSpec `json:",inline"`
}

// ServiceStatus defines the observed state of Service.
type ServiceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Service is the Schema for the services API.
type Service struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceSpec   `json:"spec,omitempty"`
	Status ServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceList contains a list of Service.
type ServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Service `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Service{}, &ServiceList{})
}
