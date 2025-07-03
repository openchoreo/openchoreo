// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ServiceReleaseSpec defines the desired state of ServiceRelease.
type ServiceReleaseSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Owner ServiceOwner `json:"owner"`
	// +kubebuilder:validation:MinLength=1
	EnvironmentName string `json:"environmentName"`

	// Scalable resource template approach (KRO-inspired)
	// Supports any Kubernetes resource type including HPA, PDB, NetworkPolicy, CRDs, etc. that can
	// be applied to the data plane.
	// +kubebuilder:validation:Optional
	Resources []Resource `json:"resources,omitempty"`
}

// ServiceReleaseStatus defines the observed state of ServiceRelease.
type ServiceReleaseStatus struct {
	// Resources contain the list of resources that have been successfully applied to the data plane
	// +optional
	Resources []ResourceStatus `json:"resources,omitempty"`

	// Conditions represent the latest available observations of the ServiceRelease's current state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ServiceRelease is the Schema for the servicereleases API.
type ServiceRelease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceReleaseSpec   `json:"spec,omitempty"`
	Status ServiceReleaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceReleaseList contains a list of ServiceRelease.
type ServiceReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceRelease `json:"items"`
}

// GetConditions returns the conditions from the status
func (in *ServiceRelease) GetConditions() []metav1.Condition {
	return in.Status.Conditions
}

// SetConditions sets the conditions in the status
func (in *ServiceRelease) SetConditions(conditions []metav1.Condition) {
	in.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&ServiceRelease{}, &ServiceReleaseList{})
}
