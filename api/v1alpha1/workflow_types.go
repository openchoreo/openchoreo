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
	Schema *WorkflowSchema `json:"schema,omitempty"`

	// RunTemplate is the Kubernetes resource template to be rendered and applied to the cluster.
	// Template variables are substituted with context and parameter values.
	// Supported template variables:
	//   - ${metadata.workflowRunName} - WorkflowRun name (the execution instance)
	//   - ${metadata.namespaceName} - Namespace name
	//   - ${parameters.*} - Developer-provided parameter values
	//
	// Note: PE-controlled parameters should be hardcoded directly in the template.
	//
	// +required
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	RunTemplate *runtime.RawExtension `json:"runTemplate"`

	// Resources are additional templates that generate Kubernetes resources dynamically
	// to be deployed alongside the workflow run (e.g., secrets, configmaps).
	// Template variables are substituted with context and parameter values using CEL expressions.
	// +optional
	Resources []WorkflowResource `json:"resources,omitempty"`

	// TTLAfterCompletion defines the time-to-live for WorkflowRun instances after completion.
	// Once a WorkflowRun completes, it will be automatically deleted after this duration.
	// Format: duration string supporting days, hours, minutes, seconds without spaces (e.g., "90d", "10d1h30m", "1h30m")
	// Examples: "90d", "10d", "1h30m", "30m", "1d12h30m15s"
	// If empty, workflow runs are not automatically deleted.
	// +optional
	// +kubebuilder:validation:Pattern=`^(\d+d)?(\d+h)?(\d+m)?(\d+s)?$`
	TTLAfterCompletion string `json:"ttlAfterCompletion,omitempty"`
}

// WorkflowSchema defines the parameter schemas for workflows.
type WorkflowSchema struct {
	// Types defines reusable type definitions that can be referenced in schema fields.
	// This is a nested map structure where keys are type names and values are type definitions.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Types *runtime.RawExtension `json:"types,omitempty"`

	// Parameters defines the flexible PE-defined schema for additional build configuration.
	// Platform Engineers have complete freedom to define any structure, types, and validation rules.
	//
	// This is a nested map structure where keys are field names and values are type definitions.
	// Type definition format: "type | default=value enum=val1,val2 minimum=N maximum=N"
	//
	// Supported types: string, integer, boolean, array<type>, object
	//
	// Example:
	//   parameters:
	//     version: 'integer | default=1 description="Build version"'
	//     testMode: "string | enum=unit,integration,none default=unit"
	//     resources:
	//       cpuCores: "integer | default=1 minimum=1 maximum=8"
	//       memoryGb: "integer | default=2 minimum=1 maximum=32"
	//     cache:
	//       enabled: "boolean | default=true"
	//       paths: "array<string> | default=["/root/.cache"]"
	//
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
}

// WorkflowResource defines a template for generating Kubernetes resources
// to be deployed alongside the workflow run.
type WorkflowResource struct {
	// ID uniquely identifies this resource within the workflow.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ID string `json:"id"`

	// IncludeWhen is a CEL expression that determines whether this resource should be rendered.
	// If the expression evaluates to false, the resource is skipped.
	// If empty, the resource is always included.
	// Example: has(systemParameters.secretRef)
	// +optional
	IncludeWhen string `json:"includeWhen,omitempty"`

	// Template contains the Kubernetes resource with CEL expressions.
	// CEL expressions are enclosed in ${...} and will be evaluated at runtime.
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Template *runtime.RawExtension `json:"template"`
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
