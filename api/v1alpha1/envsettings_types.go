// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EnvSettingsSpec defines the desired state of EnvSettings.
type EnvSettingsSpec struct {
	// Owner identifies the component this settings applies to
	// +kubebuilder:validation:Required
	Owner EnvSettingsOwner `json:"owner"`

	// Environment is the name of the environment these settings apply to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Environment string `json:"environment"`

	// Overrides for ComponentTypeDefinition envOverrides parameters
	// These values override the defaults defined in the Component for this specific environment
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Overrides *runtime.RawExtension `json:"overrides,omitempty"`

	// AddonOverrides provides environment-specific overrides for addon configurations
	// Structure: map[addonName]map[instanceId]overrideValues
	// +optional
	AddonOverrides map[string]map[string]runtime.RawExtension `json:"addonOverrides,omitempty"`
}

// EnvSettingsOwner identifies the component this EnvSettings applies to
type EnvSettingsOwner struct {
	// ProjectName is the name of the project that owns this component
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`

	// ComponentName is the name of the component these settings apply to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ComponentName string `json:"componentName"`
}

// EnvSettingsStatus defines the observed state of EnvSettings.
type EnvSettingsStatus struct {
	// ObservedGeneration reflects the generation of the most recently observed EnvSettings
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the EnvSettings's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=envsetting;envsettings
// +kubebuilder:printcolumn:name="Component",type=string,JSONPath=`.spec.owner.componentName`
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.spec.environment`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// EnvSettings is the Schema for the envsettings API.
type EnvSettings struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EnvSettingsSpec   `json:"spec,omitempty"`
	Status EnvSettingsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EnvSettingsList contains a list of EnvSettings.
type EnvSettingsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EnvSettings `json:"items"`
}

func (e *EnvSettings) GetConditions() []metav1.Condition {
	return e.Status.Conditions
}

func (e *EnvSettings) SetConditions(conditions []metav1.Condition) {
	e.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&EnvSettings{}, &EnvSettingsList{})
}
