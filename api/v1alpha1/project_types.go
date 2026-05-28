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
	DeploymentPipelineRef DeploymentPipelineRef `json:"deploymentPipelineRef"`

	// ExternalCI optionally configures external CI platform integrations that are
	// trusted to create Workloads in this Project.
	// +optional
	ExternalCI *ProjectExternalCI `json:"externalCI,omitempty"`
}

// ProjectExternalCI configures external CI platform integrations trusted to
// create Workloads in a Project.
type ProjectExternalCI struct {
	// GitHubActions configures the trust boundary for GitHub Actions workflows
	// that authenticate to openchoreo-api via GitHub's OIDC token.
	// +optional
	GitHubActions *ProjectGitHubActions `json:"githubActions,omitempty"`
}

// ProjectGitHubActions defines which GitHub Actions workflows are trusted to
// register Workloads against Components owned by this Project. The values are
// matched against the corresponding claims in the GitHub-issued OIDC token.
// Empty slices mean "no restriction" for that dimension.
type ProjectGitHubActions struct {
	// AllowedRepositories is an allow-list of "owner/repo" entries matched
	// against the `repository` claim. At least one repository MUST be listed
	// for OIDC-authenticated requests to be accepted for this Project.
	// +optional
	// +listType=set
	AllowedRepositories []string `json:"allowedRepositories,omitempty"`

	// AllowedRefs is an optional allow-list of git refs (e.g.
	// "refs/heads/main") matched against the `ref` claim. Empty means any ref
	// from an allowed repository is accepted.
	// +optional
	// +listType=set
	AllowedRefs []string `json:"allowedRefs,omitempty"`

	// AllowedJobWorkflowRefs is an optional allow-list of immutable
	// `job_workflow_ref` values (e.g.
	// "octo-org/octo-repo/.github/workflows/release.yml@refs/heads/main")
	// matched against the `job_workflow_ref` claim. This provides the
	// strongest trust boundary because `job_workflow_ref` is immutable for
	// the duration of a workflow run. Empty means any workflow from an
	// allowed repository and ref is accepted.
	// +optional
	// +listType=set
	AllowedJobWorkflowRefs []string `json:"allowedJobWorkflowRefs,omitempty"`
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
