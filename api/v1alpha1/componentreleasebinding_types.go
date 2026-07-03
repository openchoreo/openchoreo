// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ComponentReleaseBindingSpec defines the desired state of ComponentReleaseBinding.
type ComponentReleaseBindingSpec struct {
	// Owner identifies the component and project this ComponentReleaseBinding belongs to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.owner is immutable"
	Owner ComponentReleaseBindingOwner `json:"owner"`

	// EnvironmentName is the name of the environment this binds the ComponentRelease to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.environment is immutable"
	Environment string `json:"environment"`

	// ReleaseName is the name of the ComponentRelease to bind
	// When ComponentSpec.AutoDeploy is enabled, this field will be handled by the controller
	// +optional
	ReleaseName string `json:"releaseName,omitempty"`

	// ComponentTypeEnvironmentConfigs for ComponentType environmentConfigs parameters
	// These values override the defaults defined in the Component for this specific environment
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	ComponentTypeEnvironmentConfigs *runtime.RawExtension `json:"componentTypeEnvironmentConfigs,omitempty"`

	// TraitEnvironmentConfigs provides environment-specific overrides for trait configurations
	// Keyed by instanceName (which must be unique across all traits in the component)
	// Structure: map[instanceName]overrideValues
	// +optional
	TraitEnvironmentConfigs map[string]runtime.RawExtension `json:"traitEnvironmentConfigs,omitempty"`

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

// ComponentReleaseBindingOwner identifies the component this ComponentReleaseBinding belongs to
type ComponentReleaseBindingOwner struct {
	// ProjectName is the name of the project that owns this component
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`

	// ComponentName is the name of the component
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ComponentName string `json:"componentName"`
}

// ComponentReleaseBindingStatus defines the observed state of ComponentReleaseBinding.
type ComponentReleaseBindingStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastSpecUpdateTime is the timestamp of the last spec change observed by the controller.
	// Updated when the controller detects a new generation (i.e., spec was modified).
	// +optional
	LastSpecUpdateTime *metav1.Time `json:"lastSpecUpdateTime,omitempty"`

	// Conditions represent the latest available observations of the ComponentReleaseBinding's current state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Endpoints contains the resolved invoke URLs for each named workload endpoint,
	// keyed by endpoint name. Populated once the component is deployed and the
	// corresponding HTTPRoutes are available.
	// +optional
	Endpoints []EndpointURLStatus `json:"endpoints,omitempty"`

	// ConnectionTargets lists the connection targets derived from the workload connections.
	// Used as an index source for finding consumer ComponentReleaseBindings when a provider's endpoints change.
	// +optional
	ConnectionTargets []ConnectionTarget `json:"connectionTargets,omitempty"`

	// ResolvedConnections contains the connections that have been successfully resolved.
	// +optional
	ResolvedConnections []ResolvedConnection `json:"resolvedConnections,omitempty"`

	// PendingConnections contains the connections that could not be resolved.
	// +optional
	PendingConnections []PendingConnection `json:"pendingConnections,omitempty"`

	// ResourceDependencyTargets lists the resource dependency targets derived from the
	// workload's dependencies.resources[]. Used as an index source for finding consumer
	// ComponentReleaseBindings when a provider ResourceReleaseBinding's status.outputs change.
	// +optional
	ResourceDependencyTargets []ResourceDependencyTarget `json:"resourceDependencyTargets,omitempty"`

	// PendingResourceDependencies contains the resource dependencies that could not be resolved.
	// +optional
	PendingResourceDependencies []PendingResourceDependency `json:"pendingResourceDependencies,omitempty"`

	// SecretReferenceNames lists the names of SecretReferences used by this ComponentReleaseBinding's workload.
	// Used as an index source for finding affected ComponentReleaseBindings when a SecretReference changes.
	// +optional
	SecretReferenceNames []string `json:"secretReferenceNames,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.owner.projectName`
// +kubebuilder:printcolumn:name="Component",type=string,JSONPath=`.spec.owner.componentName`
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.spec.environment`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ComponentReleaseBinding is the Schema for the componentreleasebindings API.
// Deprecated: ComponentReleaseBinding is deprecated and will be removed in a future release. Use ComponentComponentReleaseBinding instead.
type ComponentReleaseBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComponentReleaseBindingSpec   `json:"spec,omitempty"`
	Status ComponentReleaseBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ComponentReleaseBindingList contains a list of ComponentReleaseBinding.
type ComponentReleaseBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComponentReleaseBinding `json:"items"`
}

// GetConditions returns the conditions from the status
func (r *ComponentReleaseBinding) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

// SetConditions sets the conditions in the status
func (r *ComponentReleaseBinding) SetConditions(conditions []metav1.Condition) {
	r.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&ComponentReleaseBinding{}, &ComponentReleaseBindingList{})
}
