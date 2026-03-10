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
	// WorkflowPlaneRef references the WorkflowPlane or ClusterWorkflowPlane for this workflow's operations.
	// If not specified, the controller resolves the workflow plane in the following order:
	// 1. WorkflowPlane named "default" in the same namespace
	// 2. ClusterWorkflowPlane named "default" (cluster-scoped fallback)
	// +optional
	WorkflowPlaneRef *WorkflowPlaneRef `json:"workflowPlaneRef,omitempty"`

	// Parameters defines the developer-facing parameters that can be configured
	// when creating a WorkflowRun instance.
	// +optional
	Parameters *SchemaSection `json:"parameters,omitempty"`

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

	// ExternalRefs declares references to external CRs that are resolved at runtime
	// and injected into the CEL context under their id.
	// If a reference's name evaluates to empty, it is silently skipped.
	// +optional
	// +listType=map
	// +listMapKey=id
	ExternalRefs []ExternalRef `json:"externalRefs,omitempty"`

	// TTLAfterCompletion defines the time-to-live for WorkflowRun instances after completion.
	// Once a WorkflowRun completes, it will be automatically deleted after this duration.
	// Format: duration string supporting days, hours, minutes, seconds without spaces (e.g., "90d", "10d1h30m", "1h30m")
	// Examples: "90d", "10d", "1h30m", "30m", "1d12h30m15s"
	// If empty, workflow runs are not automatically deleted.
	// +optional
	// +kubebuilder:validation:Pattern=`^(\d+d)?(\d+h)?(\d+m)?(\d+s)?$`
	TTLAfterCompletion string `json:"ttlAfterCompletion,omitempty"`
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
	// Example: ${parameters.enableMetrics}
	// +optional
	// +kubebuilder:validation:Pattern=`^\$\{[\s\S]+\}\s*$`
	IncludeWhen string `json:"includeWhen,omitempty"`

	// Template contains the Kubernetes resource with CEL expressions.
	// CEL expressions are enclosed in ${...} and will be evaluated at runtime.
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Template *runtime.RawExtension `json:"template"`
}

// ExternalRef declares a reference to an external CR whose spec is resolved
// and injected into the CEL context under the given id.
type ExternalRef struct {
	// ID uniquely identifies this external reference within the workflow.
	// The resolved CR's spec is injected into the CEL context under this name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=2
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z][a-z0-9-]*[a-z0-9]$`
	ID string `json:"id"`

	// APIVersion is the API version of the referenced resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	APIVersion string `json:"apiVersion"`

	// Kind is the kind of the referenced resource.
	// Currently only SecretReference is supported.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=SecretReference
	Kind string `json:"kind"`

	// Name is the name of the referenced resource.
	// Supports CEL expressions (e.g., ${parameters.repository.secretRef}).
	// If the name evaluates to empty, the reference is silently skipped.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
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
