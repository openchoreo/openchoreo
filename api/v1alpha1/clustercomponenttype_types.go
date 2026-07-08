// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterComponentTypeSpec defines the desired state of ClusterComponentType.
// +kubebuilder:validation:XValidation:rule="self.workloadType == 'proxy' || self.resources.exists(r, r.id == self.workloadType)",message="resources must contain a primary resource with id matching workloadType (unless workloadType is 'proxy')"
// +kubebuilder:validation:XValidation:rule="!(has(self.validations) && size(self.validations) > 0 && has(self.preRenderValidations) && size(self.preRenderValidations) > 0)",message="set only one of spec.validations or spec.preRenderValidations; validations is deprecated, use preRenderValidations"
type ClusterComponentTypeSpec struct {
	// WorkloadType must be one of: deployment, statefulset, cronjob, job, proxy
	// This determines the primary workload resource type for this component type
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=deployment;statefulset;cronjob;job;proxy
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.workloadType cannot be changed after creation"
	WorkloadType string `json:"workloadType"`

	// DisplayName is a human-friendly name for this component type shown in the UI.
	// +optional
	DisplayName string `json:"displayName,omitempty"`

	// AllowedWorkflows restricts which ClusterWorkflow CRs developers can use
	// for building components of this type. If empty, no workflows are allowed.
	// References must point to ClusterWorkflow resources.
	// +optional
	AllowedWorkflows []ClusterWorkflowRef `json:"allowedWorkflows,omitempty"`

	// Parameters defines what developers can configure when creating components of this type.
	// +optional
	Parameters *SchemaSection `json:"parameters,omitempty"`

	// EnvironmentConfigs defines per-environment configurations developers can set via ReleaseBinding.
	// +optional
	EnvironmentConfigs *SchemaSection `json:"environmentConfigs,omitempty"`

	// Traits are pre-configured trait instances embedded in the ClusterComponentType.
	// Only ClusterTrait references are allowed since ClusterComponentType is cluster-scoped.
	// The PE binds trait parameters using concrete values or CEL expressions
	// referencing the ComponentType schema (e.g., "${parameters.storage.mountPath}").
	// These traits are automatically applied to all Components of this type.
	// +optional
	Traits []ClusterComponentTypeTrait `json:"traits,omitempty"`

	// AllowedTraits restricts which ClusterTrait CRs developers can attach to Components of this type.
	// When specified, only ClusterTraits listed here (matched by name) may be attached beyond those already embedded in spec.traits.
	// ClusterTrait references listed here must not overlap with traits already embedded in spec.traits.
	// If empty or omitted, no additional component-level traits are allowed.
	// +optional
	AllowedTraits []ClusterTraitRef `json:"allowedTraits,omitempty"`

	// Validations are CEL-based rules evaluated during rendering.
	//
	// Deprecated: use PreRenderValidations. Retained for backward compatibility;
	// it is mutually exclusive with PreRenderValidations and has identical semantics.
	// +optional
	Validations []ValidationRule `json:"validations,omitempty"`

	// PreRenderValidations are CEL-based rules evaluated before rendering, against the
	// component context (parameters/environmentConfigs/workload/metadata). All rules must
	// evaluate to true for rendering to proceed. Replaces Validations.
	// +optional
	PreRenderValidations []ValidationRule `json:"preRenderValidations,omitempty"`

	// PostRenderValidations are CEL-based rules evaluated after all traits are applied,
	// against the final rendered Kubernetes resources.
	// +optional
	PostRenderValidations []PostRenderValidation `json:"postRenderValidations,omitempty"`

	// Resources are templates that generate Kubernetes resources dynamically.
	// At least one resource template is required. For non-proxy workload types,
	// one resource must have an id matching the workloadType. When workloadType
	// is "proxy", a matching resource id is not required.
	// +kubebuilder:validation:MinItems=1
	Resources []ResourceTemplate `json:"resources"`
}

// EffectivePreRenderValidations returns the pre-render validation rules to apply.
// PreRenderValidations takes precedence; Validations is the deprecated fallback.
// The two are mutually exclusive (enforced by a CRD XValidation rule), so at most
// one is non-empty in practice.
func (s *ClusterComponentTypeSpec) EffectivePreRenderValidations() []ValidationRule {
	if len(s.PreRenderValidations) > 0 {
		return s.PreRenderValidations
	}
	//nolint:staticcheck // deprecated field still supported for backward compatibility
	return s.Validations
}

// ToComponentTypeSpec converts a ClusterComponentTypeSpec into the equivalent
// ComponentTypeSpec, mapping the cluster-scoped ref/trait element types to their
// namespace-scoped equivalents. The two specs are not directly convertible (their
// AllowedWorkflows/Traits/AllowedTraits element types differ), so both the component
// controller (release freeze) and the openchoreo-api service call this single method
// to keep the field mapping in one place.
func (s *ClusterComponentTypeSpec) ToComponentTypeSpec() ComponentTypeSpec {
	allowedTraits := make([]TraitRef, len(s.AllowedTraits))
	for i, ref := range s.AllowedTraits {
		allowedTraits[i] = TraitRef{Kind: TraitRefKind(ref.Kind), Name: ref.Name}
	}
	traits := make([]ComponentTypeTrait, len(s.Traits))
	for i, t := range s.Traits {
		traits[i] = ComponentTypeTrait{
			Kind:               TraitRefKind(t.Kind),
			Name:               t.Name,
			InstanceName:       t.InstanceName,
			Parameters:         t.Parameters,
			EnvironmentConfigs: t.EnvironmentConfigs,
		}
	}
	allowedWorkflows := make([]WorkflowRef, len(s.AllowedWorkflows))
	for i, ref := range s.AllowedWorkflows {
		allowedWorkflows[i] = WorkflowRef{Kind: WorkflowRefKind(ref.Kind), Name: ref.Name}
	}
	return ComponentTypeSpec{
		WorkloadType:       s.WorkloadType,
		AllowedWorkflows:   allowedWorkflows,
		Parameters:         s.Parameters,
		EnvironmentConfigs: s.EnvironmentConfigs,
		Traits:             traits,
		AllowedTraits:      allowedTraits,
		//nolint:staticcheck // deprecated field still copied for backward compatibility
		Validations:           s.Validations,
		PreRenderValidations:  s.PreRenderValidations,
		PostRenderValidations: s.PostRenderValidations,
		Resources:             s.Resources,
	}
}

// ClusterComponentTypeStatus defines the observed state of ClusterComponentType.
type ClusterComponentTypeStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=cct;ccts
// +kubebuilder:printcolumn:name="WorkloadType",type=string,JSONPath=`.spec.workloadType`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ClusterComponentType is the Schema for the clustercomponenttypes API.
// ClusterComponentType is a cluster-scoped version of ComponentType that can be
// referenced by Components across all namespaces.
type ClusterComponentType struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterComponentTypeSpec   `json:"spec,omitempty"`
	Status ClusterComponentTypeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterComponentTypeList contains a list of ClusterComponentType.
type ClusterComponentTypeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterComponentType `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterComponentType{}, &ClusterComponentTypeList{})
}
