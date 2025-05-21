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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TargetEnvironmentRef defines a reference to a target environment with approval settings
type TargetEnvironmentRef struct {
	// Name of the target environment
	Name string `json:"name"`
	// RequiresApproval indicates if promotion to this environment requires approval
	// +optional
	RequiresApproval bool `json:"requiresApproval,omitempty"`
	// IsManualApprovalRequired indicates if manual approval is needed for promotion
	// +optional
	IsManualApprovalRequired bool `json:"isManualApprovalRequired,omitempty"`
}

// PromotionPath defines a path for promoting between environments
type PromotionPath struct {
	// SourceEnvironmentRef is the reference to the source environment
	SourceEnvironmentRef string `json:"sourceEnvironmentRef"`
	// TargetEnvironmentRefs is the list of target environments and their approval requirements
	TargetEnvironmentRefs []TargetEnvironmentRef `json:"targetEnvironmentRefs"`
}

// DeploymentPipelineSpec defines the desired state of DeploymentPipeline.
type DeploymentPipelineSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// PromotionPaths defines the available paths for promotion between environments
	PromotionPaths []PromotionPath `json:"promotionPaths,omitempty"`
}

// DeploymentPipelineStatus defines the observed state of DeploymentPipeline.
type DeploymentPipelineStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ObservedGeneration represents the .metadata.generation that the condition was set based upon
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions represent the latest available observations of an object's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=deppipe;deppipes

// DeploymentPipeline is the Schema for the deploymentpipelines API.
type DeploymentPipeline struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeploymentPipelineSpec   `json:"spec,omitempty"`
	Status DeploymentPipelineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DeploymentPipelineList contains a list of DeploymentPipeline.
type DeploymentPipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DeploymentPipeline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DeploymentPipeline{}, &DeploymentPipelineList{})
}
