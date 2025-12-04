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
	//   ${ctx.componentWorkflowRunName} - ComponentWorkflowRun CR name
	//   ${ctx.componentName}             - Component name
	//   ${ctx.projectName}               - Project name
	//   ${ctx.orgName}                   - Organization name (namespace)
	//   ${systemParameters.*}            - System parameter values
	//   ${parameters.*}                  - Developer parameter values
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	RunTemplate *runtime.RawExtension `json:"runTemplate"`
}

// ComponentWorkflowSchema defines the parameter schemas for component workflows.
// It separates system parameters (required structure for build features) from
// developer parameters (flexible PE-defined configuration).
type ComponentWorkflowSchema struct {
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
	// Format: 'string | default=value description=... enum=val1,val2'
	// Example: 'string | description="Git repository URL for the component source code"'
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=6
	// +kubebuilder:validation:Pattern=`^string(\s*\|.*)?$`
	URL string `json:"url"`

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

	// SecretName is the schema definition for the Kubernetes secret name containing credentials for private repositories.
	// Must be a string type schema definition.
	// Format: 'string | description=...'
	// Example: 'string | description="Kubernetes secret name for private repository authentication"'
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^string(\s*\|.*)?$`
	SecretName string `json:"secretName"`
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
