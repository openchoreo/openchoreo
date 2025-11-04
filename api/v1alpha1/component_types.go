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
// +kubebuilder:validation:XValidation:rule="has(self.type) || has(self.componentType)",message="Component must have either spec.type or spec.componentType set"
type ComponentSpec struct {
	// Owner defines the ownership information for the component
	// +kubebuilder:validation:Required
	Owner ComponentOwner `json:"owner"`

	// Type specifies the component type (e.g., Service, WebApplication, etc.)
	// LEGACY FIELD: Use componentType instead for new components with ComponentTypeDefinitions
	// +optional
	Type CompType `json:"type,omitempty"`

	// ComponentType specifies the component type in the format: {workloadType}/{componentTypeDefinitionName}
	// Example: "deployment/web-app", "cronjob/scheduled-task"
	// This field is used with ComponentTypeDefinitions (new model)
	// +optional
	// +kubebuilder:validation:Pattern=`^(deployment|statefulset|cronjob|job)/[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.componentType cannot be changed after creation"
	ComponentType string `json:"componentType,omitempty"`

	// Parameters from ComponentTypeDefinition (oneOf schema based on componentType)
	// This is the merged schema of parameters + envOverrides from the ComponentTypeDefinition
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`

	// Addons to compose into this component
	// Each addon can be instantiated multiple times with different instanceNames
	// +optional
	Addons []ComponentAddon `json:"addons,omitempty"`

	// Build defines the build configuration for the component
	// +optional
	Build BuildSpecInComponent `json:"build,omitempty"`
}

// ComponentAddon represents an addon instance attached to a component
type ComponentAddon struct {
	// Name is the name of the Addon resource to use
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// InstanceName uniquely identifies this addon instance within the component
	// Allows the same addon to be used multiple times with different configurations
	// Must be unique across all addons in the component
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	InstanceName string `json:"instanceName"`

	// Config contains the addon parameter values
	// The schema for this config is defined in the Addon's schema.parameters and schema.envOverrides
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Config *runtime.RawExtension `json:"config,omitempty"`
}

type ComponentOwner struct {
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`
}

// BuildSpecInComponent defines the build configuration for a component
// This specification is used to create Build resources when builds are triggered
type BuildSpecInComponent struct {
	// WorkflowTemplate specifies the WorkflowDefinition to use for building this component
	// This references a WorkflowDefinition resource that defines the build workflow template
	// and schema for developer-provided parameters
	// +required
	WorkflowTemplate string `json:"workflowTemplate,omitempty"`

	// Parameters contains developer-provided build parameters that conform to the schema
	// defined in the referenced WorkflowDefinition
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Schema *runtime.RawExtension `json:"schema,omitempty"`
}

// BuildRepository defines the source repository configuration for component builds
type BuildRepository struct {
	// URL is the repository URL where the component source code is located
	// Example: "https://github.com/org/repo" or "git@github.com:org/repo.git"
	URL string `json:"url"`

	// Revision specifies the default revision configuration for builds
	// This can be overridden when triggering specific builds
	Revision BuildRevision `json:"revision"`

	// AppPath is the path to the application within the repository
	// This is relative to the repository root. Default is "." for root directory
	AppPath string `json:"appPath"`
}

// BuildRevision defines the revision specification for component builds
type BuildRevision struct {
	// Branch specifies the default branch to build from
	// This will be used when no specific commit is provided for a build
	Branch string `json:"branch"`
}

// ComponentStatus defines the observed state of Component.
type ComponentStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// CompType defines how the component is deployed.
type CompType string

const (
	ComponentTypeService        CompType = "Service"
	ComponentTypeManualTask     CompType = "ManualTask"
	ComponentTypeScheduledTask  CompType = "ScheduledTask"
	ComponentTypeWebApplication CompType = "WebApplication"
	ComponentTypeWebhook        CompType = "Webhook"
	ComponentTypeAPIProxy       CompType = "APIProxy"
	ComponentTypeTestRunner     CompType = "TestRunner"
	ComponentTypeEventHandler   CompType = "EventHandler"
)

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
