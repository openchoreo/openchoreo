// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// WorkflowDefinitionSpec defines the desired state of WorkflowDefinition.
// WorkflowDefinition provides a schema-driven template for workflow execution
// with developer-facing schemas, PE-controlled fixed parameters, and CEL-based
// resource rendering.
type WorkflowDefinitionSpec struct {
	// Schema defines the developer-facing parameters that can be configured
	// when creating a Workflow instance. Uses the same shorthand schema syntax
	// as ComponentTypeDefinition.
	//
	// Schema format: nested maps where keys are field names and values are either
	// nested maps or type definition strings.
	// Type definition format: "type | default=value minimum=2 enum=[val1,val2]"
	//
	// Example:
	//   repository:
	//     url: "string"
	//     revision:
	//       branch: "string | default=main"
	//       commit: "string | default=HEAD"
	//     appPath: "string | default=."
	//     credentialsRef: "string | enum=checkout-repo-credentials-dev,payments-repo-credentials-dev"
	//   version: "integer | default=1"
	//   testMode: "string | enum=unit,integration,none default=unit"
	//
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Schema *runtime.RawExtension `json:"schema,omitempty"`

	// FixedParameters are static, PE-controlled parameters that are hidden from developers.
	// These parameters are used for security policies, compliance requirements, and
	// platform-level configuration that should not be modified by developers.
	//
	// Can be overridden per ComponentTypeDefinition to allow component-type-specific policies.
	//
	// +optional
	FixedParameters []WorkflowParameter `json:"fixedParameters,omitempty"`

	// Secrets lists the secret references that should be synchronized to the build plane.
	// Can reference fields from the schema using CEL expressions like ${schema.repository.credentialsRef}.
	//
	// +optional
	Secrets []string `json:"secrets,omitempty"`

	// Resource contains the template for the workflow resource to be rendered.
	// The template can contain CEL expressions that will be evaluated with:
	//   - ${ctx.*} - Context variables (componentName, projectName, orgName, timestamp, uuid)
	//   - ${schema.*} - Developer-provided values from the schema
	//   - ${fixedParameters.*} - PE-controlled fixed parameters
	//
	// +required
	// +kubebuilder:pruning:PreserveUnknownFields
	Resource WorkflowResource `json:"resource"`
}

// WorkflowParameter represents a name-value parameter pair for PE-controlled configuration.
type WorkflowParameter struct {
	// Name is the parameter name
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Value is the parameter value
	// +required
	Value string `json:"value"`
}

// WorkflowResource defines the resource template to be rendered.
type WorkflowResource struct {
	// Template contains the Kubernetes resource (typically an Argo Workflow)
	// with CEL expressions enclosed in ${...} that will be evaluated at runtime.
	//
	// Available CEL variables:
	//   - ${ctx.workflowName} - Workflow name
	//   - ${ctx.componentName} - Component name
	//   - ${ctx.projectName} - Project name
	//   - ${ctx.orgName} - Organization name
	//   - ${ctx.timestamp} - Unix timestamp
	//   - ${ctx.uuid} - Short UUID (8 chars)
	//   - ${schema.*} - Developer-provided values from schema
	//   - ${fixedParameters.*} - PE-controlled fixed parameters
	//
	// +required
	// +kubebuilder:pruning:PreserveUnknownFields
	Template *runtime.RawExtension `json:"template"`
}

// WorkflowDefinitionStatus defines the observed state of WorkflowDefinition.
type WorkflowDefinitionStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the WorkflowDefinition resource.
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

// WorkflowDefinition is the Schema for the workflowdefinitions API
type WorkflowDefinition struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of WorkflowDefinition
	// +required
	Spec WorkflowDefinitionSpec `json:"spec"`

	// status defines the observed state of WorkflowDefinition
	// +optional
	Status WorkflowDefinitionStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// WorkflowDefinitionList contains a list of WorkflowDefinition
type WorkflowDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkflowDefinition `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkflowDefinition{}, &WorkflowDefinitionList{})
}
