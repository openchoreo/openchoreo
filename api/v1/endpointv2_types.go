// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// EndpointV2Spec defines the desired state of EndpointV2.
type EndpointV2Spec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Owner EndpointOwner `json:"owner"`

	// +kubebuilder:validation:MinLength=1
	EnvironmentName string `json:"environmentName"`

	EndpointTemplateSpec `json:",inline"`
}

type EndpointTemplateSpec struct {
	ClassName    string        `json:"className"`
	Type         EndpointType  `json:"type"`
	RESTEndpoint *RESTEndpoint `json:"rest,omitempty"`
	//GRPCEndpointSpec GRPCEndpointSpec `json:"grpc,omitempty"`
	//TCPEndpointSpec  TCPEndpointSpec  `json:"tcp,omitempty"`
}

type RESTEndpoint struct {
	Backend    HTTPBackend             `json:"backend,omitempty"`
	Operations []RESTEndpointOperation `json:"operations,omitempty"`
}

type RESTEndpointOperation struct {
	Method       string                     `json:"method"`
	Path         string                     `json:"path"`
	Description  string                     `json:"description,omitempty"`
	Scopes       []string                   `json:"scopes,omitempty"`
	ExposeLevels []RESTOperationExposeLevel `json:"exposeLevels,omitempty"`
}

type RESTOperationExposeLevel string

const (
	ExposeLevelProject      RESTOperationExposeLevel = "Project"
	ExposeLevelOrganization RESTOperationExposeLevel = "Organization"
	ExposeLevelPublic       RESTOperationExposeLevel = "Public"
)

type HTTPBackend struct {
	Port     int32  `json:"port"`
	BasePath string `json:"basePath,omitempty"`
}

type EndpointOwner struct {
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`
	// +kubebuilder:validation:MinLength=1
	ComponentName string `json:"componentName"`
}

// EndpointV2Status defines the observed state of EndpointV2.
type EndpointV2Status struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// EndpointV2 is the Schema for the endpointv2s API.
type EndpointV2 struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EndpointV2Spec   `json:"spec,omitempty"`
	Status EndpointV2Status `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EndpointV2List contains a list of EndpointV2.
type EndpointV2List struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EndpointV2 `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EndpointV2{}, &EndpointV2List{})
}
