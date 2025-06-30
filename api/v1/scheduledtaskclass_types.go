// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ScheduledTaskClassSpec defines the desired state of ScheduledTaskClass.
type ScheduledTaskClassSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ScheduledTaskClass. Edit scheduledtaskclass_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// ScheduledTaskClassStatus defines the observed state of ScheduledTaskClass.
type ScheduledTaskClassStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ScheduledTaskClass is the Schema for the scheduledtaskclasses API.
type ScheduledTaskClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScheduledTaskClassSpec   `json:"spec,omitempty"`
	Status ScheduledTaskClassStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ScheduledTaskClassList contains a list of ScheduledTaskClass.
type ScheduledTaskClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScheduledTaskClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ScheduledTaskClass{}, &ScheduledTaskClassList{})
}
