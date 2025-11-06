// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ComponentTypeDefinitionSpec defines the desired state of ComponentTypeDefinition.
// +kubebuilder:validation:XValidation:rule="self.resources.exists(r, r.id == self.workloadType)",message="resources must contain a primary resource with id matching workloadType"
type ComponentTypeDefinitionSpec struct {
	// WorkloadType must be one of: deployment, statefulset, cronjob, job
	// This determines the primary workload resource type for this component type
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=deployment;statefulset;cronjob;job
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.workloadType cannot be changed after creation"
	WorkloadType string `json:"workloadType"`

	// Schema defines what developers can configure when creating components of this type
	// +optional
	Schema ComponentTypeSchema `json:"schema,omitempty"`

	// Resources are templates that generate Kubernetes resources dynamically
	// At least one resource must be defined with an id matching the workloadType
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

	// EnvOverrides can be overridden per environment via ComponentDeployment by platform engineers.
	// These are also exposed to developers but can be changed per environment.
	// Same nested map structure and type definition format as Parameters.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	EnvOverrides *runtime.RawExtension `json:"envOverrides,omitempty"`
}

// ResourceTemplate defines a template for generating Kubernetes resources
// +kubebuilder:validation:XValidation:rule="!has(self.forEach) || has(self.var)",message="var is required when forEach is specified"
type ResourceTemplate struct {
	// ID uniquely identifies this resource within the component type
	// For the primary workload resource, this must match the workloadType
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ID string `json:"id"`

	// IncludeWhen is a CEL expression that determines if this resource should be created
	// If not specified, the resource is always created
	// Example: "${spec.autoscaling.enabled}"
	// +optional
	IncludeWhen string `json:"includeWhen,omitempty"`

	// ForEach enables generating multiple resources from a list using CEL expression
	// Example: "${spec.configurations}" to iterate over a list
	// +optional
	// +kubebuilder:validation:Pattern=`^\$\{.+\}$`
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

// ComponentTypeDefinitionStatus defines the observed state of ComponentTypeDefinition.
type ComponentTypeDefinitionStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=ctd;ctds
// +kubebuilder:printcolumn:name="WorkloadType",type=string,JSONPath=`.spec.workloadType`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ComponentTypeDefinition is the Schema for the componenttypedefinitions API.
type ComponentTypeDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComponentTypeDefinitionSpec   `json:"spec,omitempty"`
	Status ComponentTypeDefinitionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ComponentTypeDefinitionList contains a list of ComponentTypeDefinition.
type ComponentTypeDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComponentTypeDefinition `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ComponentTypeDefinition{}, &ComponentTypeDefinitionList{})
}
