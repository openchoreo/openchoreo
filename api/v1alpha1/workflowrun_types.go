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
	// Owner identifies the Component that owns this WorkflowRun.
	// This is optional for generic workflows not bound to components.
	// +optional
	Owner WorkflowOwner `json:"owner,omitempty"`

	// Workflow configuration referencing the Workflow CR and providing schema values.
	// +required
	Workflow WorkflowConfig `json:"workflow"`
}

// WorkflowConfig defines the workflow configuration for execution.
type WorkflowConfig struct {
	// Name references the Workflow CR to use for this execution.
	// The Workflow CR contains the schema definition and resource template.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Schema contains the developer-provided values that conform to the schema
	// defined in the referenced Workflow CR.
	//
	// These values are merged with context variables when rendering the final workflow resource.
	//
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Schema *runtime.RawExtension `json:"schema,omitempty"`
}

// WorkflowOwner identifies the Component that owns a WorkflowRun execution.
type WorkflowOwner struct {
	// +required
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`

	// ComponentName is the name of the owning Component
	// +required
	// +kubebuilder:validation:MinLength=1
	ComponentName string `json:"componentName"`
}

// WorkflowRunStatus defines the observed state of WorkflowRun.
type WorkflowRunStatus struct {
	// Conditions represent the current state of the WorkflowRun resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ImageStatus contains information about the built container image from the workflow execution.
	// This is populated when the workflow produces a container image.
	// +optional
	ImageStatus WorkflowImage `json:"imageStatus,omitempty"`

	// RunReference contains a reference to the workflow run resource that was applied to the cluster.
	// This tracks the actual workflow execution instance (e.g., Argo Workflow) in the target cluster.
	// +optional
	RunReference *ResourceReference `json:"runReference,omitempty"`

	// Resources contains references to additional resources applied to the cluster.
	// These are tracked for cleanup when the WorkflowRun is deleted.
	// +optional
	Resources *[]ResourceReference `json:"resources,omitempty"`
}

// WorkflowImage contains information about a container image produced by a workflow execution.
type WorkflowImage struct {
	// Image is the fully qualified image name (e.g., registry.example.com/myapp:v1.0.0)
	// +optional
	Image string `json:"image,omitempty"`
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
