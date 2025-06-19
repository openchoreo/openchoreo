// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type APIReleaseOwner struct {
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`
	// +kubebuilder:validation:MinLength=1
	ComponentName string `json:"componentName"`
}

// APIReleaseSpec defines the desired state of APIRelease.
type APIReleaseSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Owner APIReleaseOwner `json:"owner"`
	// +kubebuilder:validation:MinLength=1
	EnvironmentName string `json:"environmentName"`

	Type EndpointType `json:"type"`

	// Scalable resource template approach (KRO-inspired)
	// Supports any Kubernetes resource type including HTTPRoute, Gateway, Ingress, Service, NetworkPolicy, etc. that can
	// be applied to the data plane for API configuration.
	// +kubebuilder:validation:Optional
	Resources []Resource `json:"resources,omitempty"`
}

// APIReleaseStatus defines the observed state of APIRelease.
type APIReleaseStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// APIRelease is the Schema for the apireleases API.
type APIRelease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   APIReleaseSpec   `json:"spec,omitempty"`
	Status APIReleaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// APIReleaseList contains a list of APIRelease.
type APIReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []APIRelease `json:"items"`
}

func init() {
	SchemeBuilder.Register(&APIRelease{}, &APIReleaseList{})
}
