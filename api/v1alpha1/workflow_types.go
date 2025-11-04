// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// WorkflowSpec defines the desired state of Workflow.
type WorkflowSpec struct {
	// Owner identifies the Component that owns this Workflow.
	// This is used for tracking and reporting purposes.
	// +required
	Owner WorkflowOwner `json:"owner"`

	// WorkflowDefinitionRef references the WorkflowDefinition to use for this execution.
	// The WorkflowDefinition contains the schema, fixed parameters, and resource template.
	// +required
	WorkflowDefinitionRef string `json:"workflowDefinitionRef"`

	// Parameters contains the developer-provided values that conform to the schema
	// defined in the referenced WorkflowDefinition.
	//
	// These parameters are merged with fixed parameters and context variables
	// when rendering the final workflow resource.
	//
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Schema *runtime.RawExtension `json:"schema,omitempty"`
}

// WorkflowOwner identifies the Component that owns a Workflow execution.
type WorkflowOwner struct {
	// ProjectName is the name of the owning Project
	// +required
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`

	// ComponentName is the name of the owning Component
	// +required
	// +kubebuilder:validation:MinLength=1
	ComponentName string `json:"componentName"`
}

// WorkflowStatus defines the observed state of Workflow.
type WorkflowStatus struct {
	// Phase represents the current phase of the workflow execution.
	// Possible values: Pending, Running, Succeeded, Failed, Error
	// +optional
	Phase string `json:"phase,omitempty"`

	// Message provides a human-readable message about the workflow status
	// +optional
	Message string `json:"message,omitempty"`

	// StartTime is the time when the workflow started execution
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is the time when the workflow completed
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// conditions represent the current state of the Workflow resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Workflow is the Schema for the workflows API
// Workflow represents a runtime execution instance of a WorkflowDefinition.
type Workflow struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of Workflow
	// +required
	Spec WorkflowSpec `json:"spec"`

	// status defines the observed state of Workflow
	// +optional
	Status WorkflowStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// WorkflowList contains a list of Workflow
type WorkflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workflow `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Workflow{}, &WorkflowList{})
}
