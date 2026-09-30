// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// HookType identifies how a hook executes.
// +kubebuilder:validation:Enum=Workflow
type HookType string

const (
	// HookTypeWorkflow runs the hook as a WorkflowRun on the workflow plane.
	HookTypeWorkflow HookType = "Workflow"
)

// HookParameter maps one input of the hook's workflow to a value source.
// Exactly one of value, from, default or required must be set, except that
// from may be combined with overridable.
type HookParameter struct {
	// Name is the workflow input this parameter feeds.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-zA-Z][a-zA-Z0-9_-]*$`
	Name string `json:"name"`

	// Value fixes the parameter to a literal. A binding cannot override it.
	// +optional
	Value *string `json:"value,omitempty"`

	// From is a CEL expression (${...}) evaluated against the deployment context.
	// +optional
	From string `json:"from,omitempty"`

	// Default is the value used when the binding does not supply one.
	// +optional
	Default *string `json:"default,omitempty"`

	// Overridable lets a binding replace the value computed by From.
	// +optional
	Overridable bool `json:"overridable,omitempty"`

	// Required marks a parameter that every binding must supply.
	// +optional
	Required bool `json:"required,omitempty"`

	// Schema is an optional OpenAPI v3 fragment describing the parameter value.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Schema *runtime.RawExtension `json:"schema,omitempty"`
}

// HookSubjectRefKind is the kind of type a hook is enabled for.
// +kubebuilder:validation:Enum=ComponentType;ClusterComponentType
type HookSubjectRefKind string

const (
	HookSubjectRefKindComponentType        HookSubjectRefKind = "ComponentType"
	HookSubjectRefKindClusterComponentType HookSubjectRefKind = "ClusterComponentType"
)

// HookSubjectRef names a component type the hook is enabled for.
type HookSubjectRef struct {
	// Kind is the type kind (ComponentType or ClusterComponentType).
	// +required
	Kind HookSubjectRefKind `json:"kind"`

	// Name is the name of the type resource.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`
}

// HookSpec defines the desired state of Hook.
type HookSpec struct {
	// Type selects the executor. Only Workflow is supported.
	// +optional
	// +kubebuilder:default=Workflow
	Type HookType `json:"type,omitempty"`

	// WorkflowRef is the Workflow or ClusterWorkflow the hook runs.
	// A ClusterHook may only reference a ClusterWorkflow.
	// +required
	WorkflowRef *WorkflowRef `json:"workflowRef"`

	// Parameters maps the workflow's inputs to value sources.
	// +optional
	// +listType=map
	// +listMapKey=name
	Parameters []HookParameter `json:"parameters,omitempty"`

	// EnabledTo restricts the hook to components of the listed types. Empty
	// enables it for every component. A binding's appliesTo can narrow this
	// further but never widen it.
	// +optional
	// +kubebuilder:validation:MaxItems=50
	EnabledTo []HookSubjectRef `json:"enabledTo,omitempty"`
}

// HookStatus defines the observed state of Hook.
type HookStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the Hook's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=hk;hks
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type"
// +kubebuilder:printcolumn:name="Workflow",type="string",JSONPath=".spec.workflowRef.name"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Hook is the Schema for the hooks API.
// A Hook is a platform-engineer-defined action that an Environment binds as
// a pre-deploy or post-deploy step.
type Hook struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HookSpec   `json:"spec,omitempty"`
	Status HookStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HookList contains a list of Hook.
type HookList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Hook `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Hook{}, &HookList{})
}
