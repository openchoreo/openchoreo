// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ResourceReleaseBindingSpec defines the desired state of ResourceReleaseBinding.
// Pins a ResourceRelease to an Environment and carries per-env overrides.
// The Resource controller never creates or modifies ResourceReleaseBindings;
// they are authored externally (kubectl, GitOps, API server).
// The resourceRelease pin is advanced manually via `occ resource promote` or
// kubectl edit.
type ResourceReleaseBindingSpec struct {
	// Owner identifies the resource and project this ResourceReleaseBinding belongs to.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.owner is immutable"
	Owner ResourceReleaseBindingOwner `json:"owner"`

	// Environment is the name of the Environment this binding targets.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.environment is immutable"
	Environment string `json:"environment"`

	// ResourceRelease is the name of the ResourceRelease pinned by this binding.
	// The release pin is advanced manually (e.g. via `occ resource promote` or
	// `kubectl edit`). Unset before the first ResourceRelease is cut; the
	// controller leaves the binding pending until set.
	// +optional
	ResourceRelease string `json:"resourceRelease,omitempty"`

	// RetainPolicy controls whether emitted DP-side resources survive binding
	// deletion. When unset, falls back to the ResourceType's retainPolicy (which
	// itself defaults to Delete). Per-env override.
	// +optional
	RetainPolicy ResourceRetainPolicy `json:"retainPolicy,omitempty"`

	// ResourceTypeEnvironmentConfigs provides per-environment values for the schema
	// declared on the referenced ResourceType (or ClusterResourceType). Validated
	// against ResourceType.spec.environmentConfigs by the binding controller;
	// failures surface via status.conditions.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	ResourceTypeEnvironmentConfigs *runtime.RawExtension `json:"resourceTypeEnvironmentConfigs,omitempty"`
}

// ResourceReleaseBindingOwner identifies the resource this ResourceReleaseBinding belongs to.
// Mirrors ResourceReleaseOwner for namespace-disambiguation: a namespace can host
// multiple projects, so a bare resource name is ambiguous.
type ResourceReleaseBindingOwner struct {
	// ProjectName is the name of the project that owns this resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`

	// ResourceName is the name of the Resource.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ResourceName string `json:"resourceName"`
}

// ResourceReleaseBindingStatus defines the observed state of ResourceReleaseBinding.
type ResourceReleaseBindingStatus struct {
	// Conditions represent the latest available observations of the binding's state.
	// Includes Synced, ResourcesReady, OutputsResolved, Ready (aggregate), and
	// Finalizing during deletion. observedGeneration is set per-condition (project
	// convention).
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Outputs holds resolved output values for this binding's environment, populated
	// from the underlying RenderedRelease.status by the binding controller. Each
	// entry corresponds to a declared ResourceType output. Secret/ConfigMap values
	// stay on the data plane; only the {name, key} reference transits to the
	// control plane.
	// +optional
	// +listType=map
	// +listMapKey=name
	Outputs []ResolvedResourceOutput `json:"outputs,omitempty"`
}

// ResolvedResourceOutput is a single resolved output value populated by the
// binding controller after evaluating the ResourceType output CEL against the
// applied DP-side objects.
// Exactly one of value, secretKeyRef, or configMapKeyRef must be set.
// +kubebuilder:validation:XValidation:rule="(has(self.value)?1:0) + (has(self.secretKeyRef)?1:0) + (has(self.configMapKeyRef)?1:0) == 1",message="exactly one of value, secretKeyRef, or configMapKeyRef must be set"
type ResolvedResourceOutput struct {
	// Name uniquely identifies this output within the binding. Matches the
	// declared output name on the referenced ResourceType.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Value is the resolved literal value when the ResourceType output is declared
	// with `value:`. Only used for non-sensitive data; the resolved value transits
	// to the control plane.
	// +optional
	Value string `json:"value,omitempty"`

	// SecretKeyRef is the resolved {name, key} reference to a DP-side Secret.
	// Used for sensitive credentials; the underlying value never leaves the data
	// plane.
	// +optional
	SecretKeyRef *SecretKeyRef `json:"secretKeyRef,omitempty"`

	// ConfigMapKeyRef is the resolved {name, key} reference to a DP-side ConfigMap.
	// +optional
	ConfigMapKeyRef *ConfigMapKeyRef `json:"configMapKeyRef,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=rrb;rrbs
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.owner.projectName`
// +kubebuilder:printcolumn:name="Resource",type=string,JSONPath=`.spec.owner.resourceName`
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.spec.environment`
// +kubebuilder:printcolumn:name="Release",type=string,JSONPath=`.spec.resourceRelease`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ResourceReleaseBinding is the Schema for the resourcereleasebindings API.
// Pins a ResourceRelease to an Environment and carries per-env config overrides.
// Authored externally; not managed by the Resource controller.
type ResourceReleaseBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceReleaseBindingSpec   `json:"spec,omitempty"`
	Status ResourceReleaseBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ResourceReleaseBindingList contains a list of ResourceReleaseBinding.
type ResourceReleaseBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceReleaseBinding `json:"items"`
}

// GetConditions returns the conditions from the status.
func (r *ResourceReleaseBinding) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

// SetConditions sets the conditions in the status.
func (r *ResourceReleaseBinding) SetConditions(conditions []metav1.Condition) {
	r.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&ResourceReleaseBinding{}, &ResourceReleaseBindingList{})
}
