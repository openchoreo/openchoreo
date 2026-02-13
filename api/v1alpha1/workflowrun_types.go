// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// WorkflowRunSpec defines the desired state of WorkflowRun.
// WorkflowRun represents a runtime execution instance of a Workflow.
type WorkflowRunSpec struct {
	// Workflow configuration referencing the Workflow CR and providing schema values.
	// +required
	Workflow WorkflowRunConfig `json:"workflow"`

	// TTLAfterCompletion defines the time-to-live for this workflow run after completion.
	// This value is copied from the Workflow template.
	// Once the workflow completes, the run will be automatically deleted after this duration.
	// Format: duration string supporting days, hours, minutes, seconds (e.g., "90d", "10d 1h 30m", "1h30m")
	// Examples: "90d", "10d", "1h30m", "30m", "1d 12h 30m 15s"
	// +optional
	// +kubebuilder:validation:Pattern=`^(\d+d)?(\s*\d+h)?(\s*\d+m)?(\s*\d+s)?$`
	TTLAfterCompletion string `json:"ttlAfterCompletion,omitempty"`
}

// WorkflowRunConfig defines the workflow configuration for execution.
type WorkflowRunConfig struct {
	// Name references the Workflow CR to use for this execution.
	// The Workflow CR contains the schema definition and resource template.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Parameters contains the developer-provided values for the flexible parameter schema
	// defined in the referenced ComponentWorkflow CR.
	//
	// These values are validated against the ComponentWorkflow's parameter schema.
	//
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
}

// WorkflowRunStatus defines the observed state of WorkflowRun.
type WorkflowRunStatus struct {
	// Conditions represent the current state of the WorkflowRun resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// RunReference contains a reference to the workflow run resource that was applied to the cluster.
	// This tracks the actual workflow execution instance (e.g., Argo Workflow) in the target cluster.
	// +optional
	RunReference *ResourceReference `json:"runReference,omitempty"`

	// Resources contains references to additional resources applied to the cluster.
	// These are tracked for cleanup when the WorkflowRun is deleted.
	// +optional
	Resources *[]ResourceReference `json:"resources,omitempty"`

	// Tasks contains the list of workflow tasks with their execution status.
	// This provides a vendor-neutral view of the workflow steps regardless of the underlying
	// workflow engine (e.g., Argo Workflows, Tekton).
	// Tasks are ordered by their execution sequence.
	// +optional
	Tasks []WorkflowTask `json:"tasks,omitempty"`

	// StartedAt is the timestamp when this workflow run started execution.
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// FinishedAt is the timestamp when this workflow run finished execution (succeeded or failed).
	// This is used together with TTLAfterCompletion to determine when to delete the workflow run.
	// +optional
	FinishedAt *metav1.Time `json:"finishedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// WorkflowRun is the Schema for the workflowruns API
// WorkflowRun represents a runtime execution instance of a Workflow.
type WorkflowRun struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of WorkflowRun
	// +required
	Spec WorkflowRunSpec `json:"spec"`

	// status defines the observed state of WorkflowRun
	// +optional
	Status WorkflowRunStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// WorkflowRunList contains a list of WorkflowRun
type WorkflowRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkflowRun `json:"items"`
}

// GetConditions returns the conditions from the workflowrun status
func (w *WorkflowRun) GetConditions() []metav1.Condition {
	return w.Status.Conditions
}

// SetConditions sets the conditions in the workflowrun status
func (w *WorkflowRun) SetConditions(conditions []metav1.Condition) {
	w.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&WorkflowRun{}, &WorkflowRunList{})
}
