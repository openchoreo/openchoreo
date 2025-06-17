// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// WorkloadBindingSpec defines the desired state of WorkloadBinding.
type WorkloadBindingSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:MinLength=1
	EnvironmentName string `json:"environmentName"`

	// Spec of the workload that binds to the environment.
	WorkloadSpec WorkloadSpec `json:"workloadSpec"`
}

// WorkloadBindingStatus defines the observed state of WorkloadBinding.
type WorkloadBindingStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// WorkloadBinding is the Schema for the workloadbindings API.
type WorkloadBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkloadBindingSpec   `json:"spec,omitempty"`
	Status WorkloadBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WorkloadBindingList contains a list of WorkloadBinding.
type WorkloadBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkloadBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkloadBinding{}, &WorkloadBindingList{})
}
