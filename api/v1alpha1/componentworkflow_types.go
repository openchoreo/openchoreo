// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ComponentWorkflowSpec defines the desired state of ComponentWorkflow.
// ComponentWorkflow is a specialized workflow template for component builds that requires
// structured system parameters for build-specific platform features (webhooks, auto-build, UI actions).
type ComponentWorkflowSpec struct {
	// Schema defines the parameter schemas for this component workflow.
	// It includes both required system parameters (for repository information)
	// and flexible developer parameters (PE-defined).
	// +kubebuilder:validation:Required
	Schema ComponentWorkflowSchema `json:"schema"`

	// RunTemplate is the Kubernetes resource template to be rendered and applied to the build plane.
	// Template variables are substituted with context and parameter values.
	// Supported template variables:
	//   ${metadata.workflowRunName}  - ComponentWorkflowRun CR name
	//   ${metadata.componentName}    - Component name
	//   ${metadata.projectName}      - Project name
	//   ${metadata.namespaceName}    - Namespace
	//   ${systemParameters.*}        - System parameter values
	//   ${parameters.*}              - Developer parameter values
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	RunTemplate *runtime.RawExtension `json:"runTemplate"`

	// Resources are additional templates that generate Kubernetes resources dynamically
	// to be deployed alongside the workflow run (e.g., secrets, configmaps).
	// Template variables are substituted with context and parameter values.
	// +optional
	Resources []ComponentWorkflowResource `json:"resources,omitempty"`

	// TTLAfterCompletion defines the time-to-live for ComponentWorkflowRun instances after completion.
	// it will be automatically deleted after this duration.
	// Format: duration string supporting days, hours, minutes, seconds (e.g., "90d", "10d 1h 30m", "1h30m")
	// +optional
	// +kubebuilder:validation:Pattern=`^(\d+d)?(\s*\d+h)?(\s*\d+m)?(\s*\d+s)?$`
	TTLAfterCompletion string `json:"ttlAfterCompletion,omitempty"`
}

// ComponentWorkflowResource defines a template for generating Kubernetes resources
// to be deployed alongside the workflow run.
type ComponentWorkflowResource struct {
	// ID uniquely identifies this resource within the component workflow.
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

// ComponentWorkflowSchema defines the parameter schemas for component workflows.
// It separates system parameters (required structure for build features) from
// developer parameters (flexible PE-defined configuration).
type ComponentWorkflowSchema struct {
	// Types defines reusable type definitions that can be referenced in schema fields.
	// This is a nested map structure where keys are type names and values are type definitions.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Types *runtime.RawExtension `json:"types,omitempty"`

	// SystemParameters defines the required structured schema for repository information.
	// This schema must follow a specific structure to enable build-specific platform features.
	// Platform Engineers can customize defaults, enums, and descriptions within each field,
	// but must maintain the field names, nesting structure, and types (all must be string).
	// +kubebuilder:validation:Required
	SystemParameters SystemParametersSchema `json:"systemParameters"`

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

// GetTypes returns the types raw extension.
func (s *ComponentWorkflowSchema) GetTypes() *runtime.RawExtension { return s.Types }

// SystemParametersSchema defines the required schema structure for system parameters.
// This structure is enforced to enable build-specific platform features like webhooks,
// auto-build, and UI actions.
type SystemParametersSchema struct {
	// Repository contains the schema for repository-related parameters.
	// +kubebuilder:validation:Required
	Repository RepositorySchema `json:"repository"`
}

// RepositorySchema defines the schema for repository parameters.
type RepositorySchema struct {
	// URL is the schema definition for the Git repository URL field.
	// Must be a string type schema definition.
	// Supports HTTP/HTTPS and SSH URL formats in default values.
	// Format: 'string | default=value description=... enum=val1,val2'
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=6
	// +kubebuilder:validation:Pattern=`^string(\s*\|.*)?$`
	URL string `json:"url"`

	// SecretRef is the schema definition for the Git credentials secret reference name.
	// Must be a string type schema definition.
	// Format: 'string | default=value description=...'
	// Example: 'string | description="Secret reference name for Git credentials"'
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^string(\s*\|.*)?$`
	SecretRef string `json:"secretRef,omitempty"`

	// Revision contains the schema for revision-related parameters (branch, commit).
	// All fields have defaults, so this struct will be auto-populated if not provided.
	// +kubebuilder:validation:Required
	Revision RepositoryRevisionSchema `json:"revision"`

	// AppPath is the schema definition for the application path within the repository.
	// Must be a string type schema definition.
	// Format: 'string | default=value description=...'
	// Example: 'string | default=. description="Path to the application code within the repository"'
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^string(\s*\|.*)?$`
	AppPath string `json:"appPath"`
}

// RepositoryRevisionSchema defines the schema for repository revision parameters.
type RepositoryRevisionSchema struct {
	// Branch is the schema definition for the Git branch field.
	// Must be a string type schema definition.
	// Format: 'string | default=value description=...'
	// Example: 'string | default=main description="Git branch to build from"'
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^string(\s*\|.*)?$`
	Branch string `json:"branch"`

	// Commit is the schema definition for the Git commit SHA field.
	// Must be a string type schema definition.
	// Format: 'string | default=value description=...'
	// Example: 'string | description="Specific commit SHA to build (optional)"'
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^string(\s*\|.*)?$`
	Commit string `json:"commit"`
}

// ComponentWorkflowStatus defines the observed state of ComponentWorkflow.
type ComponentWorkflowStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the ComponentWorkflow resource.
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

// ComponentWorkflow is the Schema for the componentworkflows API
type ComponentWorkflow struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of ComponentWorkflow
	// +required
	Spec ComponentWorkflowSpec `json:"spec"`

	// status defines the observed state of ComponentWorkflow
	// +optional
	Status ComponentWorkflowStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// ComponentWorkflowList contains a list of ComponentWorkflow
type ComponentWorkflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComponentWorkflow `json:"items"`
}

// GetConditions returns the conditions from the ComponentWorkflow status.
func (c *ComponentWorkflow) GetConditions() []metav1.Condition {
	return c.Status.Conditions
}

// SetConditions sets the conditions in the ComponentWorkflow status.
func (c *ComponentWorkflow) SetConditions(conditions []metav1.Condition) {
	c.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&ComponentWorkflow{}, &ComponentWorkflowList{})
}
