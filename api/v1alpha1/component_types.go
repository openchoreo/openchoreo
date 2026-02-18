// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=comp;comps
// +kubebuilder:printcolumn:name="Project",type=string,JSONPath=`.spec.owner.projectName`
// +kubebuilder:printcolumn:name="ComponentType",type=string,JSONPath=`.spec.componentType.name`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Component is the Schema for the components API.
type Component struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComponentSpec   `json:"spec,omitempty"`
	Status ComponentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ComponentList contains a list of Component.
type ComponentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Component `json:"items"`
}

// ComponentSpec defines the desired state of Component.
type ComponentSpec struct {
	// Owner defines the ownership information for the component
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.owner is immutable"
	Owner ComponentOwner `json:"owner"`

	// ComponentType specifies the component type reference with kind and name.
	// Name is in the format: {workloadType}/{componentTypeName}
	// Example: kind=ComponentType, name="deployment/web-app"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.componentType cannot be changed after creation"
	ComponentType ComponentTypeRef `json:"componentType"`

	// AutoDeploy indicates whether the component should be deployed automatically when created
	// When not specified, defaults to false (zero value)
	// +optional
	AutoDeploy bool `json:"autoDeploy,omitempty"`

	// AutoBuild enables automatic builds when code is pushed to the repository
	// When enabled, webhook events will trigger builds automatically
	// Users must manually configure webhooks in their Git provider
	// +optional
	AutoBuild *bool `json:"autoBuild,omitempty"`

	// Parameters from ComponentType (oneOf schema based on componentType)
	// This is the merged schema of parameters + envOverrides from the ComponentType
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`

	// Traits to compose into this component
	// Each trait can be instantiated multiple times with different instanceNames
	// +optional
	Traits []ComponentTrait `json:"traits,omitempty"`

	// Workflow defines the component workflow configuration for building the component.
	// This references a ComponentWorkflow CR and provides both system parameters (repository info)
	// and developer-configured parameter values.
	// The ComponentWorkflow must be in the allowedWorkflows list of the ComponentType.
	// +optional
	Workflow *ComponentWorkflowRunConfig `json:"workflow,omitempty"`
}

// ComponentTrait represents an trait instance attached to a component
type ComponentTrait struct {
	// Name is the name of the Trait resource to use
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// InstanceName uniquely identifies this trait instance within the component
	// Allows the same trait to be used multiple times with different configurations
	// Must be unique across all traits in the component
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	InstanceName string `json:"instanceName"`

	// Parameters contains the trait parameter values
	// The schema for this config is defined in the Trait's schema.parameters and schema.envOverrides
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
}

type ComponentOwner struct {
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`
}

// ComponentStatus defines the observed state of Component.
type ComponentStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	// LatestRelease keeps the information of the latest ComponentRelease created for this component
	// This is used to make sure the component's latest ComponentRelease is always
	// deployed to the first environment, if the autoDeploy flag is set to true
	// +optional
	LatestRelease *LatestRelease `json:"latestRelease,omitempty"`
}

// LatestRelease has name and generated hash of the latest ComponentRelease spec
type LatestRelease struct {
	// Name of the ComponentRelease resource
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// ReleaseHash record the hash value of the spec of ComponentRelease.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Required
	ReleaseHash string `json:"releaseHash,omitempty"`
}

// ComponentSource defines the source information of the component where the code or image is retrieved.
type ComponentSource struct {
	// GitRepository specifies the configuration for the component source to be a Git repository indicating
	// that the component should be built from the source code.
	// This field is mutually exclusive with the other source types.
	GitRepository *GitRepository `json:"gitRepository,omitempty"`

	// ContainerRegistry specifies the configuration for the component source to be a container image indicating
	// that the component should be deployed using the provided image.
	// This field is mutually exclusive with the other source types.
	ContainerRegistry *ContainerRegistry `json:"containerRegistry,omitempty"`
}

// GitRepository defines the Git repository configuration
type GitRepository struct {
	// URL the Git repository URL
	// Examples:
	// - https://github.com/jhonb2077/customer-service
	// - https://gitlab.com/jhonb2077/customer-service
	URL string `json:"url"`

	// Authentication the authentication information to access the Git repository
	// If not provided, the Git repository should be public
	Authentication GitAuthentication `json:"authentication,omitempty"`
}

// GitAuthentication defines the authentication configuration for Git
type GitAuthentication struct {
	// SecretRef is a reference to the secret containing Git credentials
	SecretRef string `json:"secretRef"`
}

// ContainerRegistry defines the container registry configuration.
type ContainerRegistry struct {
	// Image name of the container image. Format: <registry>/<image> without the tag.
	// Example: docker.io/library/nginx
	ImageName string `json:"imageName,omitempty"`
	// Authentication information to access the container registry.
	Authentication *RegistryAuthentication `json:"authentication,omitempty"`
}

// RegistryAuthentication defines the authentication configuration for container registry
type RegistryAuthentication struct {
	// Reference to the secret that contains the container registry authentication info.
	SecretRef string `json:"secretRef,omitempty"`
}

func (p *Component) GetConditions() []metav1.Condition {
	return p.Status.Conditions
}

func (p *Component) SetConditions(conditions []metav1.Condition) {
	p.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&Component{}, &ComponentList{})
}
