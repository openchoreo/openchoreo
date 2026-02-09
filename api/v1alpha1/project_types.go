// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProjectSpec defines the desired state of Project.
type ProjectSpec struct {
	// DeploymentPipelineRef references the DeploymentPipeline that defines the environments
	// and deployment progression for components in this project.
	DeploymentPipelineRef string `json:"deploymentPipelineRef"`

	// BuildPlaneRef references the BuildPlane or ClusterBuildPlane for this project's build operations.
	// If not specified, the controller resolves the build plane in the following order:
	// 1. BuildPlane named "default" in the same namespace
	// 2. ClusterBuildPlane named "default" (cluster-scoped fallback)
	// 3. First available BuildPlane in the namespace
	// +optional
	BuildPlaneRef *BuildPlaneRef `json:"buildPlaneRef,omitempty"`
}

// ProjectStatus defines the observed state of Project.
type ProjectStatus struct {
	// ObservedGeneration reflects the generation of the most recently observed Project.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the current state of the Project resource.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=proj;projs

// Project is the Schema for the projects API.
type Project struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectSpec   `json:"spec,omitempty"`
	Status ProjectStatus `json:"status,omitempty"`
}

func (p *Project) GetConditions() []metav1.Condition {
	return p.Status.Conditions
}

func (p *Project) SetConditions(conditions []metav1.Condition) {
	p.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// ProjectList contains a list of Project.
type ProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Project `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Project{}, &ProjectList{})
}
