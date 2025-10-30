// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ConfigurationOverrides defines environment-specific configuration overrides.
type EnvConfigurationOverrides struct {
	// Environment variable overrides
	// +optional
	Env []EnvVar `json:"env,omitempty"`

	// File configuration overrides
	// +optional
	Files []FileVar `json:"file,omitempty"`
}

// ComponentDeploymentSpec defines the desired state of ComponentDeployment.
type ComponentDeploymentSpec struct {
	// Owner identifies the component this deployment applies to
	// +kubebuilder:validation:Required
	Owner ComponentDeploymentOwner `json:"owner"`

	// Environment is the name of the environment this deployment applies to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Environment string `json:"environment"`

	// Overrides for ComponentTypeDefinition envOverrides parameters
	// These values override the defaults defined in the Component for this specific environment
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Overrides *runtime.RawExtension `json:"overrides,omitempty"`

	// AddonOverrides provides environment-specific overrides for addon configurations
	// Keyed by instanceName (which must be unique across all addons in the component)
	// Structure: map[instanceName]overrideValues
	// +optional
	AddonOverrides map[string]runtime.RawExtension `json:"addonOverrides,omitempty"`

	// ConfigurationOverrides provides environment-specific overrides for workload configurations
	// These values override or add to the configurations defined in the workload.yaml
	// +optional
	ConfigurationOverrides *EnvConfigurationOverrides `json:"configurationOverrides,omitempty"`
}

// ComponentDeploymentOwner identifies the component this ComponentDeployment applies to
type ComponentDeploymentOwner struct {
	// ProjectName is the name of the project that owns this component
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`

	// ComponentName is the name of the component this deployment applies to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ComponentName string `json:"componentName"`
}

// ComponentDeploymentStatus defines the observed state of ComponentDeployment.
type ComponentDeploymentStatus struct {
	// ObservedGeneration reflects the generation of the most recently observed ComponentDeployment
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the ComponentDeployment's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=compdeployment;compdeployments
// +kubebuilder:printcolumn:name="Component",type=string,JSONPath=`.spec.owner.componentName`
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.spec.environment`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ComponentDeployment is the Schema for the componentdeployments API.
type ComponentDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComponentDeploymentSpec   `json:"spec,omitempty"`
	Status ComponentDeploymentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ComponentDeploymentList contains a list of ComponentDeployment.
type ComponentDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComponentDeployment `json:"items"`
}

func (c *ComponentDeployment) GetConditions() []metav1.Condition {
	return c.Status.Conditions
}

func (c *ComponentDeployment) SetConditions(conditions []metav1.Condition) {
	c.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&ComponentDeployment{}, &ComponentDeploymentList{})
}
