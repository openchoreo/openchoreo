// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DeploymentSpec defines the desired state of Deployment.
type DeploymentSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Number of deployment revisions to keep for rollback.
	// +optional (default: 10)
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`

	// Reference to the deployable artifact that is being deployed.
	// +required
	DeploymentArtifactRef string `json:"deploymentArtifactRef"`

	// Environment-specific configuration overrides applied to the artifact
	// before being deployed.
	// +optional
	ConfigurationOverrides *ConfigurationOverrides `json:"configurationOverrides,omitempty"`
}

// ConfigurationOverrides holds environment-specific overrides to the artifact configuration.
type ConfigurationOverrides struct {
	// Endpoint configuration overrides for this deployment.
	// +optional
	EndpointTemplates []EndpointOverride `json:"endpointTemplates,omitempty"`

	// Dependency configuration overrides for this deployment.
	// +optional
	Dependencies *DependenciesOverride `json:"dependencies,omitempty"`

	// Application configuration overrides for this deployment.
	// +optional
	Application *Application `json:"application,omitempty"`
}

// EndpointOverride captures overrides for an existing endpoint’s configuration.
type EndpointOverride struct {
	// TODO: Define the structure of the endpoint override.
}

// DependenciesOverride captures overrides for dependencies.
type DependenciesOverride struct {
	// TODO: Define the structure of the dependencies override.
}

// DeploymentStatus defines the observed state of Deployment.
type DeploymentStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Artifact",type="string",JSONPath=".spec.deploymentArtifactRef"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Deployment is the Schema for the deployments API.
type Deployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeploymentSpec   `json:"spec,omitempty"`
	Status DeploymentStatus `json:"status,omitempty"`
}

func (d *Deployment) GetConditions() []metav1.Condition {
	return d.Status.Conditions
}

func (d *Deployment) SetConditions(conditions []metav1.Condition) {
	d.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// DeploymentList contains a list of Deployment.
type DeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Deployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Deployment{}, &DeploymentList{})
}
