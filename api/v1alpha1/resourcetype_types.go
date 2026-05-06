// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ResourceTypeSpec defines the desired state of ResourceType.
type ResourceTypeSpec struct {
	// Parameters defines the dev-facing schema. Validates Resource.spec.parameters.
	// +optional
	Parameters *SchemaSection `json:"parameters,omitempty"`

	// EnvironmentConfigs defines the PE-facing per-env schema.
	// Validates ResourceBinding.spec.resourceTypeEnvironmentConfigs.
	// +optional
	EnvironmentConfigs *SchemaSection `json:"environmentConfigs,omitempty"`

	// RetainPolicy is the PE-defined default retention for ResourceBindings of this type.
	// Per-env override is available via ResourceBinding.spec.retainPolicy.
	// +optional
	// +kubebuilder:default=Delete
	RetainPolicy ResourceRetainPolicy `json:"retainPolicy,omitempty"`

	// Outputs declares values that workloads consume via
	// Workload.spec.dependencies.resources[].envBindings or fileBindings.
	// Each entry is identified by a unique name and picks exactly one of value,
	// secretKeyRef, or configMapKeyRef. Output value, name, and key fields support
	// ${...} CEL templating evaluated against metadata.*, parameters.*,
	// environmentConfigs.*, and applied.<id>.status.*.
	// +optional
	// +listType=map
	// +listMapKey=name
	Outputs []ResourceTypeOutput `json:"outputs,omitempty"`

	// Resources are the Kubernetes manifests the ResourceType provisioner emits
	// on the data plane. Each entry has a unique id used by readyWhen and outputs
	// CEL to reference applied.<id>.status.* fields.
	// +kubebuilder:validation:MinItems=1
	// +listType=map
	// +listMapKey=id
	Resources []ResourceTypeManifest `json:"resources"`
}

// ResourceTypeOutput defines a single output of a ResourceType.
// Exactly one of value, secretKeyRef, or configMapKeyRef must be set.
// +kubebuilder:validation:XValidation:rule="(has(self.value)?1:0) + (has(self.secretKeyRef)?1:0) + (has(self.configMapKeyRef)?1:0) == 1",message="exactly one of value, secretKeyRef, or configMapKeyRef must be set"
type ResourceTypeOutput struct {
	// Name uniquely identifies this output within the ResourceType. Referenced by
	// Workload.spec.dependencies.resources[].envBindings and fileBindings keys.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Value is a literal or ${...} CEL expression evaluating to a string.
	// Use only for non-sensitive data (host, port, region, database name); the
	// resolved value transits to the control plane.
	// +optional
	Value string `json:"value,omitempty"`

	// SecretKeyRef references a Secret on the data plane.
	// Use for sensitive credentials (passwords, tokens, private keys).
	// Only the {name, key} reference transits to the control plane; the
	// underlying value never leaves the data plane.
	// Both name and key support ${...} CEL templating.
	// +optional
	SecretKeyRef *SecretKeyRef `json:"secretKeyRef,omitempty"`

	// ConfigMapKeyRef references a ConfigMap on the data plane.
	// Both name and key support ${...} CEL templating.
	// +optional
	ConfigMapKeyRef *ConfigMapKeyRef `json:"configMapKeyRef,omitempty"`
}

// ResourceTypeManifest defines a Kubernetes resource template that the
// ResourceType provisioner emits on the data plane.
type ResourceTypeManifest struct {
	// ID uniquely identifies this entry within the ResourceType.
	// Referenced by readyWhen and outputs CEL via applied.<id>.status.*.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ID string `json:"id"`

	// Template contains the Kubernetes resource with ${...} CEL expressions.
	// Phase-1 (manifest rendering) CEL has access to metadata.*, parameters.*,
	// and environmentConfigs.*. applied.<id>.status.* is NOT available during
	// rendering.
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Template *runtime.RawExtension `json:"template"`

	// ReadyWhen is an optional CEL expression that determines whether this entry
	// contributes to ResourceBinding.status.conditions[ResourcesReady]. Evaluated
	// against applied.<id>.* once the manifest has been applied. If unset, falls
	// back to RenderedRelease per-Kind health inference.
	// +optional
	ReadyWhen string `json:"readyWhen,omitempty"`
}

// ResourceTypeStatus defines the observed state of ResourceType.
type ResourceTypeStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=rt;rts
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ResourceType is the Schema for the resourcetypes API.
// PEs publish ResourceType templates in a namespace; developers reference them
// by name from Resource.spec.type.
type ResourceType struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceTypeSpec   `json:"spec,omitempty"`
	Status ResourceTypeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ResourceTypeList contains a list of ResourceType.
type ResourceTypeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceType `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ResourceType{}, &ResourceTypeList{})
}
