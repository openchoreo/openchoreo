// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ReleaseBindingSpec defines the desired state of ReleaseBinding.
type ReleaseBindingSpec struct {
	// Owner identifies the component and project this ReleaseBinding belongs to
	// +kubebuilder:validation:Required
	Owner ReleaseBindingOwner `json:"owner"`

	// EnvironmentName is the name of the environment this binds the release to
	// +kubebuilder:validation:Required
	Environment string `json:"environment"`

	// ReleaseName is the name of the release to bind
	// +kubebuilder:validation:Required
	ReleaseName string `json:"releaseName"`

	// ComponentTypeEnvOverrides for ComponentType envOverrides parameters
	// These values override the defaults defined in the Component for this specific environment
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	ComponentTypeEnvOverrides *runtime.RawExtension `json:"componentTypeEnvOverrides,omitempty"`

	// TraitOverrides provides environment-specific overrides for trait configurations
	// Keyed by instanceName (which must be unique across all traits in the component)
	// Structure: map[instanceName]overrideValues
	// +optional
	TraitOverrides map[string]runtime.RawExtension `json:"traitOverrides,omitempty"`

	// ConfigurationOverrides provides environment-specific overrides for workload configurations
	// These values override or add to the configurations defined in the workload.yaml
	// +optional
	ConfigurationOverrides *EnvConfigurationOverrides `json:"configurationOverrides,omitempty"`
}

// ReleaseBindingOwner identifies the component this ReleaseBinding belongs to
type ReleaseBindingOwner struct {
	// ProjectName is the name of the project that owns this component
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`

	// ComponentName is the name of the component
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ComponentName string `json:"componentName"`
}

// ReleaseBindingStatus defines the observed state of ReleaseBinding.
type ReleaseBindingStatus struct {
	// Conditions represent the latest available observations of the ReleaseBinding's current state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ReleaseBinding is the Schema for the releasebindings API.
type ReleaseBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ReleaseBindingSpec   `json:"spec,omitempty"`
	Status ReleaseBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ReleaseBindingList contains a list of ReleaseBinding.
type ReleaseBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ReleaseBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ReleaseBinding{}, &ReleaseBindingList{})
}
