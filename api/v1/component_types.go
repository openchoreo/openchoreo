/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&Component{}, &ComponentList{})
}

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
type ComponentSpec struct {
	// Type of the component that indicates how the component deployed.
	Type ComponentType `json:"type,omitempty"`
	// Source the source information of the component where the code or image is retrieved.
	Source ComponentSource `json:"source,omitempty"`
}

// ComponentStatus defines the observed state of Component.
type ComponentStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// ComponentType defines how the component is deployed.
type ComponentType string

const (
	ComponentTypeService        ComponentType = "Service"
	ComponentTypeManualTask     ComponentType = "ManualTask"
	ComponentTypeScheduledTask  ComponentType = "ScheduledTask"
	ComponentTypeWebApplication ComponentType = "WebApplication"
	ComponentTypeWebhook        ComponentType = "Webhook"
	ComponentTypeAPIProxy       ComponentType = "APIProxy"
	ComponentTypeTestRunner     ComponentType = "TestRunner"
	ComponentTypeEventHandler   ComponentType = "EventHandler"
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
