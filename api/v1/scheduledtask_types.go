// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ScheduledTaskSpec defines the desired state of ScheduledTask.
type ScheduledTaskSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ScheduledTask. Edit scheduledtask_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// ScheduledTaskStatus defines the observed state of ScheduledTask.
type ScheduledTaskStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ScheduledTask is the Schema for the scheduledtasks API.
type ScheduledTask struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScheduledTaskSpec   `json:"spec,omitempty"`
	Status ScheduledTaskStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ScheduledTaskList contains a list of ScheduledTask.
type ScheduledTaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScheduledTask `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ScheduledTask{}, &ScheduledTaskList{})
}
