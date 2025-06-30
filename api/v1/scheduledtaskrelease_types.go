// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ScheduledTaskReleaseSpec defines the desired state of ScheduledTaskRelease.
type ScheduledTaskReleaseSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ScheduledTaskRelease. Edit scheduledtaskrelease_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// ScheduledTaskReleaseStatus defines the observed state of ScheduledTaskRelease.
type ScheduledTaskReleaseStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ScheduledTaskRelease is the Schema for the scheduledtaskreleases API.
type ScheduledTaskRelease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScheduledTaskReleaseSpec   `json:"spec,omitempty"`
	Status ScheduledTaskReleaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ScheduledTaskReleaseList contains a list of ScheduledTaskRelease.
type ScheduledTaskReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScheduledTaskRelease `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ScheduledTaskRelease{}, &ScheduledTaskReleaseList{})
}
