// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TargetPlane constants define which plane resources should be deployed to.
const (
	// TargetPlaneDataPlane indicates resources should be deployed to the data plane.
	TargetPlaneDataPlane = "dataplane"

	// TargetPlaneObservabilityPlane indicates resources should be deployed to the observability plane.
	TargetPlaneObservabilityPlane = "observabilityplane"
)

// ValidationRule defines a CEL-based validation rule evaluated during rendering.
type ValidationRule struct {
	// Rule is a CEL expression wrapped in ${...} that must evaluate to true.
	// Uses the same syntax as includeWhen and where fields.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^\$\{[\s\S]+\}\s*$`
	Rule string `json:"rule"`

	// Message is the error message shown when the rule evaluates to false.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Message string `json:"message"`
}

// ComponentTypeSpec defines the desired state of ComponentType.
// +kubebuilder:validation:XValidation:rule="self.workloadType == 'proxy' || self.resources.exists(r, r.id == self.workloadType)",message="resources must contain a primary resource with id matching workloadType (unless workloadType is 'proxy')"
type ComponentTypeSpec struct {
	// WorkloadType must be one of: deployment, statefulset, cronjob, job, proxy
	// This determines the primary workload resource type for this component type
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=deployment;statefulset;cronjob;job;proxy
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.workloadType cannot be changed after creation"
	WorkloadType string `json:"workloadType"`

	// AllowedWorkflows restricts which ComponentWorkflow CRs developers can use
	// for building components of this type. If empty, no ComponentWorkflows are allowed.
	// References must point to ComponentWorkflow resources, not generic Workflow resources.
	// +optional
	AllowedWorkflows []string `json:"allowedWorkflows,omitempty"`

	// Schema defines what developers can configure when creating components of this type
	// +optional
	Schema ComponentTypeSchema `json:"schema,omitempty"`

	// Traits are pre-configured trait instances embedded in the ComponentType.
	// The PE binds trait parameters using concrete values or CEL expressions
	// referencing the ComponentType schema (e.g., "${parameters.storage.mountPath}").
	// These traits are automatically applied to all Components of this type.
	// +optional
	Traits []ComponentTypeTrait `json:"traits,omitempty"`

	// AllowedTraits restricts which Trait CRs developers can attach to Components of this type.
	// Trait names listed here must not overlap with traits already embedded in spec.traits.
	// +optional
	AllowedTraits []string `json:"allowedTraits,omitempty"`

	// Validations are CEL-based rules evaluated during rendering.
	// All rules must evaluate to true for rendering to proceed.
	// +optional
	Validations []ValidationRule `json:"validations,omitempty"`

	// Resources are templates that generate Kubernetes resources dynamically.
	// At least one resource template is required. For non-proxy workload types,
	// one resource must have an id matching the workloadType. When workloadType
	// is "proxy", a matching resource id is not required.
	// +kubebuilder:validation:MinItems=1
	Resources []ResourceTemplate `json:"resources"`
}

// ComponentTypeSchema defines the configurable parameters for a component type
// Parameters and EnvOverrides are nested map structures where keys are field names
// and values define the type and validation rules using inline schema syntax.
//
// Example:
//
//	parameters:
//	  runtime:
//	    command: "array<string> | default=[]"
//	    args: "array<string> | default=[]"
//	  lifecycle:
//	    terminationGracePeriodSeconds: "integer | default=30"
//	    imagePullPolicy: "string | default=IfNotPresent | enum=Always,IfNotPresent,Never"
type ComponentTypeSchema struct {
	// Types defines reusable type definitions that can be referenced in schema fields
	// This is a nested map structure where keys are type names and values are type definitions
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Types *runtime.RawExtension `json:"types,omitempty"`

	// Parameters are static across environments and exposed as inputs to developers
	// when creating a Component of this type. This is a nested map structure where
	// keys are field names and values are either nested maps or type definition strings.
	// Type definition format: "type | default=value | required=true | enum=val1,val2"
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`

	// EnvOverrides can be overridden per environment via ReleaseBinding by platform engineers.
	// Same nested map structure and type definition format as Parameters.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	EnvOverrides *runtime.RawExtension `json:"envOverrides,omitempty"`
}

// GetTypes returns the types raw extension.
func (s *ComponentTypeSchema) GetTypes() *runtime.RawExtension { return s.Types }

// GetParameters returns the parameters raw extension.
func (s *ComponentTypeSchema) GetParameters() *runtime.RawExtension { return s.Parameters }

// GetEnvOverrides returns the envOverrides raw extension.
func (s *ComponentTypeSchema) GetEnvOverrides() *runtime.RawExtension { return s.EnvOverrides }

// ResourceTemplate defines a template for generating Kubernetes resources
// +kubebuilder:validation:XValidation:rule="!has(self.forEach) || has(self.var)",message="var is required when forEach is specified"
type ResourceTemplate struct {
	// ID uniquely identifies this resource within the component type
	// For the primary workload resource, this must match the workloadType
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ID string `json:"id"`

	// TargetPlane specifies which plane this resource should be deployed to
	// Defaults to "dataplane" if not specified
	// +optional
	// +kubebuilder:validation:Enum=dataplane;observabilityplane
	// +kubebuilder:default=dataplane
	TargetPlane string `json:"targetPlane,omitempty"`

	// IncludeWhen is a CEL expression that determines if this resource should be created
	// If not specified, the resource is always created
	// Example: "${spec.autoscaling.enabled}"
	// +optional
	// +kubebuilder:validation:Pattern=`^\$\{[\s\S]+\}\s*$`
	IncludeWhen string `json:"includeWhen,omitempty"`

	// ForEach enables generating multiple resources from a list using CEL expression
	// Example: "${spec.configurations}" to iterate over a list
	// +optional
	// +kubebuilder:validation:Pattern=`^\$\{[\s\S]+\}\s*$`
	ForEach string `json:"forEach,omitempty"`

	// Var is the loop variable name when using forEach
	// Example: "config" will make each item available as ${config} in templates
	// +optional
	// +kubebuilder:validation:Pattern=`^[a-zA-Z_][a-zA-Z0-9_]*$`
	Var string `json:"var,omitempty"`

	// Template contains the Kubernetes resource with CEL expressions
	// CEL expressions are enclosed in ${...} and will be evaluated at runtime
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Template *runtime.RawExtension `json:"template"`
}

// ComponentTypeTrait represents a pre-configured trait instance embedded in a ComponentType.
// The PE binds trait parameters using concrete values (locked) or CEL expressions
// referencing the ComponentType schema (wired to developer-configurable fields).
type ComponentTypeTrait struct {
	// Name is the name of the Trait resource to use.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// InstanceName uniquely identifies this trait instance.
	// Must be unique across all embedded traits in the ComponentType
	// and must not collide with any component-level trait instance names.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	InstanceName string `json:"instanceName"`

	// Parameters contains trait parameter bindings.
	// Values can be concrete (locked by PE) or CEL expressions referencing
	// the ComponentType schema using ${...} syntax.
	// Example: "${parameters.storage.mountPath}" or "app-data" (locked)
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`

	// EnvOverrides contains trait environment override bindings.
	// Values can be concrete (locked by PE) or CEL expressions referencing
	// the ComponentType schema using ${...} syntax.
	// Example: "${envOverrides.storage.size}" or "local-path" (locked)
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	EnvOverrides *runtime.RawExtension `json:"envOverrides,omitempty"`
}

// ComponentTypeStatus defines the observed state of ComponentType.
type ComponentTypeStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=ct;cts
// +kubebuilder:printcolumn:name="WorkloadType",type=string,JSONPath=`.spec.workloadType`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ComponentType is the Schema for the componenttypes API.
type ComponentType struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComponentTypeSpec   `json:"spec,omitempty"`
	Status ComponentTypeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ComponentTypeList contains a list of ComponentType.
type ComponentTypeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComponentType `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ComponentType{}, &ComponentTypeList{})
}
