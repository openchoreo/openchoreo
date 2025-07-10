// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// EndpointClassSpec defines the desired state of EndpointClass.
type EndpointClassSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	RESTPolicy *RESTEndpointPolicy `json:"restPolicy,omitempty"`
	GRPCPolicy *GRPCEndpointPolicy `json:"grpcPolicy,omitempty"`
}

// RESTEndpointPolicy defines REST-specific endpoint policies
type RESTEndpointPolicy struct {
	//// Default policies that apply to all expose levels
	// Defaults *RESTPolicyWithConditionals `json:"defaults,omitempty"`
	//// Override policies for public expose level
	//Public *RESTPolicyWithConditionals `json:"public,omitempty"`
	//// Override policies for organization expose level
	//Organization *RESTPolicyWithConditionals `json:"organization,omitempty"`
}

// GRPCEndpointPolicy defines gRPC-specific endpoint policies (placeholder for future implementation)
type GRPCEndpointPolicy struct {
	// TODO: Implement gRPC-specific policies
}

// RESTPolicyCondition defines conditions for applying conditional policies
type RESTPolicyCondition struct {
	// HTTP method to match
	Method *HTTPMethod `json:"method,omitempty"`
	// Paths to match
	Paths []string `json:"paths,omitempty"`
}

// TransformationRule defines a single transformation rule
type TransformationRule struct {
	Type        string            `json:"type"`   // json | xml
	Action      string            `json:"action"` // addFields | addHeader | removeHeaders | removeFields
	Fields      map[string]string `json:"fields,omitempty"`
	Headers     []string          `json:"headers,omitempty"`
	HeaderName  *string           `json:"headerName,omitempty"`
	HeaderValue *string           `json:"headerValue,omitempty"`
}

// EndpointClassStatus defines the observed state of EndpointClass.
type EndpointClassStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// EndpointClass is the Schema for the endpointclasses API.
type EndpointClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EndpointClassSpec   `json:"spec,omitempty"`
	Status EndpointClassStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EndpointClassList contains a list of EndpointClass.
type EndpointClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EndpointClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EndpointClass{}, &EndpointClassList{})
}
