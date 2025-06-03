// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// WorkloadClassSpec defines the desired state of WorkloadClass.
type WorkloadClassSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	ServiceWorkload        ServiceWorkload        `json:"service,omitempty"`
	ManualTaskWorkload     ManualTaskWorkload     `json:"manualTask,omitempty"`
	ScheduledTaskWorkload  ScheduledTaskWorkload  `json:"scheduledTask,omitempty"`
	WebApplicationWorkload WebApplicationWorkload `json:"webApplication,omitempty"`
}

type ServiceWorkload struct {
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	DeploymentTemplate appsv1.DeploymentSpec `json:"deploymentTemplate,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	ServiceTemplate corev1.ServiceSpec `json:"serviceTemplate,omitempty"`
}

type ManualTaskWorkload struct {
}

type ScheduledTaskWorkload struct {
}

type WebApplicationWorkload struct {
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	DeploymentTemplate appsv1.DeploymentSpec `json:"deploymentTemplate,omitempty"`
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	ServiceTemplate corev1.ServiceSpec `json:"serviceTemplate,omitempty"`
}

// WorkloadClassStatus defines the observed state of WorkloadClass.
type WorkloadClassStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// WorkloadClass is the Schema for the workloadclasses API.
type WorkloadClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkloadClassSpec   `json:"spec,omitempty"`
	Status WorkloadClassStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WorkloadClassList contains a list of WorkloadClass.
type WorkloadClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkloadClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkloadClass{}, &WorkloadClassList{})
}
