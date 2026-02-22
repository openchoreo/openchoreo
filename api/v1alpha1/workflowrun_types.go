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
	// Format: duration string supporting days, hours, minutes, seconds without spaces (e.g., "90d", "10d1h30m", "1h30m")
	// Examples: "90d", "10d", "1h30m", "30m", "1d12h30m15s"
	// +optional
	// +kubebuilder:validation:Pattern=`^(\d+d)?(\d+h)?(\d+m)?(\d+s)?$`
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
	// defined in the referenced Workflow CR.
	//
	// These values are validated against the Workflow's parameter schema.
	//
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
}

// ResourceReference tracks a resource applied to the build plane cluster for cleanup purposes.
type ResourceReference struct {
	// APIVersion is the API version of the resource (e.g., "v1", "apps/v1").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	APIVersion string `json:"apiVersion"`

	// Kind is the type of the resource (e.g., "Secret", "ConfigMap").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Kind string `json:"kind"`

	// Name is the name of the resource in the build plane cluster.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace is the namespace of the resource in the build plane cluster.
	// Empty for cluster-scoped resources.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// WorkflowTask represents a single task/step in a workflow execution.
// This provides a vendor-neutral abstraction over workflow engine-specific steps
// (e.g., Argo Workflow nodes, Tekton TaskRuns).
type WorkflowTask struct {
	// Name is the name of the task/step.
	// For Argo Workflows, this corresponds to the templateName.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Phase represents the current execution phase of the task.
	// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed;Skipped;Error
	// +optional
	Phase string `json:"phase,omitempty"`

	// StartedAt is the timestamp when the task started execution.
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// CompletedAt is the timestamp when the task finished execution.
	// +optional
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`

	// Message provides additional details about the task status.
	// This is typically populated when the task fails or errors.
	// +optional
	Message string `json:"message,omitempty"`
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

	// CompletedAt is the timestamp when this workflow run finished execution (succeeded or failed).
	// This is used together with TTLAfterCompletion to determine when to delete the workflow run.
	// +optional
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`
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
