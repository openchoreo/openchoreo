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
// Workflow provides a schema-driven template for workflow execution
// with developer-facing schemas and CEL-based resource rendering.
// PE-controlled parameters should be hardcoded directly in the template.
type WorkflowSpec struct {
	// Schema defines the developer-facing parameters that can be configured
	// when creating a WorkflowRun instance. Uses the same shorthand schema syntax
	// as ComponentType.
	//
	// Schema format: nested maps where keys are field names and values are either
	// nested maps or type definition strings.
	// Type definition format: "type | default=value minimum=2 enum=val1,val2"
	//
	// Example:
	//   repository:
	//     url: string | description="Git repository URL"
	//     revision:
	//       branch: string | default=main description="Git branch to checkout"
	//       commit: string | default=HEAD description="Git commit SHA or reference"
	//     appPath: string | default=. description="Path to the application directory"
	//     credentialsRef: string | enum=checkout-repo-credentials-dev,payments-repo-credentials-dev description="Repository credentials secret reference"
	//   version: integer | default=1 description="Build version number"
	//   testMode: string | enum=unit,integration,none default=unit description="Test mode to execute"
	//
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Schema *runtime.RawExtension `json:"schema,omitempty"`

	// Secrets lists the secret references that should be synchronized to the build plane.
	// Can reference fields from the schema using CEL expressions like ${schema.repository.credentialsRef}.
	//
	// +optional
	Secrets []string `json:"secrets,omitempty"`

	// Resource contains the Kubernetes resource (typically an Argo Workflow)
	// with CEL expressions enclosed in ${...} that will be evaluated at runtime.
	//
	// Available CEL variables:
	//   - ${ctx.workflowRunName} - WorkflowRun name (the execution instance)
	//   - ${ctx.componentName} - Component name (if workflow is component-bound)
	//   - ${ctx.projectName} - Project name (if workflow is component-bound)
	//   - ${ctx.orgName} - Organization name
	//   - ${ctx.timestamp} - Unix timestamp
	//   - ${ctx.uuid} - Short UUID (8 chars)
	//   - ${schema.*} - Developer-provided values from schema
	//
	// Note: PE-controlled parameters should be hardcoded directly in the resource.
	//
	// +required
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Resource *runtime.RawExtension `json:"resource"`
}

// WorkflowStatus defines the observed state of Workflow.
type WorkflowStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the Workflow resource.
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

// Workflow is the Schema for the workflows API
// Workflow provides a template definition for workflow execution with schema and resource templates.
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
