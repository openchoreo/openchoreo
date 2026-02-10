// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ContainerOverride represents a single container in the workload.
type ContainerOverride struct {
	// Explicit environment variables.
	// +optional
	Env []EnvVar `json:"env,omitempty"`

	// File configurations.
	// +optional
	Files []FileVar `json:"files,omitempty"`
}

// WorkloadOverrideTemplateSpec defines the desired state of Workload.
type WorkloadOverrideTemplateSpec struct {
	// Containers define the container specifications for this workload.
	// The key is the container name, and the value is the container specification.
	// +optional
	Containers map[string]ContainerOverride `json:"containers,omitempty"`
}

// ReleaseBindingSpec defines the desired state of ReleaseBinding.
type ReleaseBindingSpec struct {
	// Owner identifies the component and project this ReleaseBinding belongs to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.owner is immutable"
	Owner ReleaseBindingOwner `json:"owner"`

	// EnvironmentName is the name of the environment this binds the ComponentRelease to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.environment is immutable"
	Environment string `json:"environment"`

	// ReleaseName is the name of the ComponentRelease to bind
	// When ComponentSpec.AutoDeploy is enabled, this field will be handled by the controller
	// +optional
	ReleaseName string `json:"releaseName,omitempty"`

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

	// WorkloadOverrides provides environment-specific overrides for the entire workload spec
	// These values override the workload specification for this specific environment
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	WorkloadOverrides *WorkloadOverrideTemplateSpec `json:"workloadOverrides,omitempty"`

	// State controls the state of the Release created by this binding.
	// Active: Resources are deployed normally
	// Undeploy: Resources are removed from the data plane
	// +kubebuilder:default=Active
	// +kubebuilder:validation:Enum=Active;Undeploy
	// +optional
	State ReleaseState `json:"state,omitempty"`
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

	// InvokeURL is the resolved public URL derived from the HTTPRoute rendered for this binding.
	// It is populated once the component is deployed and an HTTPRoute with a public hostname is available.
	// +optional
	InvokeURL string `json:"invokeURL,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.owner.projectName`
// +kubebuilder:printcolumn:name="Component",type=string,JSONPath=`.spec.owner.componentName`
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.spec.environment`
// +kubebuilder:printcolumn:name="InvokeURL",type=string,JSONPath=`.status.invokeURL`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

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

// GetConditions returns the conditions from the status
func (r *ReleaseBinding) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

// SetConditions sets the conditions in the status
func (r *ReleaseBinding) SetConditions(conditions []metav1.Condition) {
	r.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&ReleaseBinding{}, &ReleaseBindingList{})
}
