// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// WebApplicationSpec defines the desired state of WebApplication.
type WebApplicationSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Owner WebApplicationOwner `json:"owner"`

	// WorkloadName is the name of the workload that this web application is referencing.
	WorkloadName string `json:"workloadName"`
	// ClassName is the name of the web application class that provides the web application-specific deployment configuration.
	// +kubebuilder:default=default
	ClassName string `json:"className"`

	Overrides map[string]bool `json:"overrides,omitempty"` // TODO: Think about how to structure this

}

type WebApplicationOwner struct {
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`
	// +kubebuilder:validation:MinLength=1
	ComponentName string `json:"componentName"`
}

// WebApplicationStatus defines the observed state of WebApplication.
type WebApplicationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// WebApplication is the Schema for the webapplications API.
type WebApplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WebApplicationSpec   `json:"spec,omitempty"`
	Status WebApplicationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WebApplicationList contains a list of WebApplication.
type WebApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WebApplication `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WebApplication{}, &WebApplicationList{})
}
