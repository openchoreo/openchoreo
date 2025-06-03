// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type ComponentOwner struct {
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`
}

// ComponentV2Spec defines the desired state of ComponentV2.
type ComponentV2Spec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Owner ComponentOwner `json:"owner"`

	Workload WorkloadTemplateSpec `json:"workload,omitempty"`

	Endpoints []ComponentEndpointSpec `json:"endpoints,omitempty"`
}

type ComponentEndpointSpec struct {
	Name         string                 `json:"name"`
	ClassName    string                 `json:"className"`
	Type         EndpointType           `json:"type"`
	RESTEndpoint *ComponentRESTEndpoint `json:"rest,omitempty"`
}

type ComponentRESTEndpoint struct {
	Backend    ComponentHTTPBackend    `json:"backend,omitempty"`
	Operations []RESTEndpointOperation `json:"operations,omitempty"`
}

type ComponentHTTPBackend struct {
	Port     int32  `json:"port"`
	BasePath string `json:"basePath,omitempty"`

	// Other backend configurations can be added here. E.g., Retry policies, timeouts, etc.
}

// ComponentV2Status defines the observed state of ComponentV2.
type ComponentV2Status struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ComponentV2 is the Schema for the componentv2s API.
type ComponentV2 struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComponentV2Spec   `json:"spec,omitempty"`
	Status ComponentV2Status `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ComponentV2List contains a list of ComponentV2.
type ComponentV2List struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComponentV2 `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ComponentV2{}, &ComponentV2List{})
}
