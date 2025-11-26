// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ComponentWorkflowRunSpec defines the desired state of ComponentWorkflowRun
type ComponentWorkflowRunSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// foo is an example field of ComponentWorkflowRun. Edit componentworkflowrun_types.go to remove/update
	// +optional
	Foo *string `json:"foo,omitempty"`
}

// ComponentWorkflowRunStatus defines the observed state of ComponentWorkflowRun.
type ComponentWorkflowRunStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the ComponentWorkflowRun resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ComponentWorkflowRun is the Schema for the componentworkflowruns API
type ComponentWorkflowRun struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of ComponentWorkflowRun
	// +required
	Spec ComponentWorkflowRunSpec `json:"spec"`

	// status defines the observed state of ComponentWorkflowRun
	// +optional
	Status ComponentWorkflowRunStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// ComponentWorkflowRunList contains a list of ComponentWorkflowRun
type ComponentWorkflowRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComponentWorkflowRun `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ComponentWorkflowRun{}, &ComponentWorkflowRunList{})
}
