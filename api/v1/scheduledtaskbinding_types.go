// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ScheduledTaskBindingSpec defines the desired state of ScheduledTaskBinding.
type ScheduledTaskBindingSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ScheduledTaskBinding. Edit scheduledtaskbinding_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// ScheduledTaskBindingStatus defines the observed state of ScheduledTaskBinding.
type ScheduledTaskBindingStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ScheduledTaskBinding is the Schema for the scheduledtaskbindings API.
type ScheduledTaskBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScheduledTaskBindingSpec   `json:"spec,omitempty"`
	Status ScheduledTaskBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ScheduledTaskBindingList contains a list of ScheduledTaskBinding.
type ScheduledTaskBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScheduledTaskBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ScheduledTaskBinding{}, &ScheduledTaskBindingList{})
}
