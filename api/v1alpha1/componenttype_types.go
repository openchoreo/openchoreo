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

// SchemaSection holds one schema in either ocSchema or openAPIV3Schema format.
// The two formats are mutually exclusive within a section.
// +kubebuilder:validation:XValidation:rule="!(has(self.ocSchema) && has(self.openAPIV3Schema))",message="ocSchema and openAPIV3Schema are mutually exclusive"
type SchemaSection struct {
	// OCSchema defines the schema using OpenChoreo's simple schema format.
	// The blob may contain a "$types" key for reusable type definitions scoped to this section.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	OCSchema *runtime.RawExtension `json:"ocSchema,omitempty"`

	// OpenAPIV3Schema defines the schema using standard OpenAPI V3 / JSON Schema format.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	OpenAPIV3Schema *runtime.RawExtension `json:"openAPIV3Schema,omitempty"`
}

// GetRaw returns the raw extension for whichever format is set.
// Returns OpenAPIV3Schema if set, otherwise OCSchema.
func (s *SchemaSection) GetRaw() *runtime.RawExtension {
	if s == nil {
		return nil
	}
	if s.OpenAPIV3Schema != nil {
		return s.OpenAPIV3Schema
	}
	return s.OCSchema
}

// IsOpenAPIV3 returns true if OpenAPIV3Schema is set.
func (s *SchemaSection) IsOpenAPIV3() bool {
	return s != nil && s.OpenAPIV3Schema != nil
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

	// AllowedWorkflows restricts which workflow CRs developers can use
	// for building components of this type. If empty, no workflows are allowed.
	// Each entry is a WorkflowRef whose Kind defaults to ClusterWorkflow and
	// may be either Workflow (namespace-scoped) or ClusterWorkflow (cluster-scoped).
	// +optional
	AllowedWorkflows []WorkflowRef `json:"allowedWorkflows,omitempty"`

	// Parameters defines what developers can configure when creating components of this type.
	// +optional
	Parameters *SchemaSection `json:"parameters,omitempty"`

	// EnvironmentConfigs defines per-environment configs developers can set via ReleaseBinding.
	// +optional
	EnvironmentConfigs *SchemaSection `json:"environmentConfigs,omitempty"`

	// Traits are pre-configured trait instances embedded in the ComponentType.
	// The PE binds trait parameters using concrete values or CEL expressions
	// referencing the ComponentType schema (e.g., "${parameters.storage.mountPath}").
	// These traits are automatically applied to all Components of this type.
	// +optional
	Traits []ComponentTypeTrait `json:"traits,omitempty"`

	// AllowedTraits restricts which Trait or ClusterTrait CRs developers can attach to Components of this type.
	// When specified, only traits listed here (matched by kind and name) may be attached beyond those already embedded in spec.traits.
	// Trait references listed here must not overlap with traits already embedded in spec.traits.
	// If empty or omitted, no additional component-level traits are allowed.
	// +optional
	AllowedTraits []TraitRef `json:"allowedTraits,omitempty"`

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
	// Kind is the kind of trait (Trait or ClusterTrait)
	// +optional
	// +kubebuilder:default=Trait
	Kind TraitRefKind `json:"kind,omitempty"`

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

	// EnvironmentConfigs contains trait environment config bindings.
	// Values can be concrete (locked by PE) or CEL expressions referencing
	// the ComponentType schema using ${...} syntax.
	// Example: "${environmentConfigs.storage.size}" or "local-path" (locked)
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	EnvironmentConfigs *runtime.RawExtension `json:"environmentConfigs,omitempty"`
}

// ClusterComponentTypeTrait represents a pre-configured trait instance embedded in a ClusterComponentType.
// Only ClusterTrait references are allowed since ClusterComponentType is cluster-scoped.
type ClusterComponentTypeTrait struct {
	// Kind is the kind of trait. Must be ClusterTrait.
	// +kubebuilder:default=ClusterTrait
	Kind ClusterTraitRefKind `json:"kind,omitempty"`

	// Name is the name of the ClusterTrait resource to use.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// InstanceName uniquely identifies this trait instance.
	// Must be unique across all embedded traits in the ClusterComponentType
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

	// EnvironmentConfigs contains trait environment config bindings.
	// Values can be concrete (locked by PE) or CEL expressions referencing
	// the ComponentType schema using ${...} syntax.
	// Example: "${environmentConfigs.storage.size}" or "local-path" (locked)
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	EnvironmentConfigs *runtime.RawExtension `json:"environmentConfigs,omitempty"`
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
