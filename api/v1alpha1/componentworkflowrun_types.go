// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ComponentWorkflowRunSpec defines the desired state of ComponentWorkflowRun.
// ComponentWorkflowRun represents a runtime execution instance of a ComponentWorkflow.
type ComponentWorkflowRunSpec struct {
	// Owner identifies the Component that owns this ComponentWorkflowRun.
	// This is required for component workflow executions.
	// +kubebuilder:validation:Required
	Owner ComponentWorkflowOwner `json:"owner"`

	// Workflow configuration referencing the ComponentWorkflow CR and providing parameter values.
	// +kubebuilder:validation:Required
	Workflow ComponentWorkflowRunConfig `json:"workflow"`

	// TTLAfterCompletion defines the time-to-live for this workflow run after completion.
	// This value is copied from the ComponentWorkflow template.
	// Format: duration string supporting days, hours, minutes, seconds without spaces (e.g., "90d", "10d1h30m", "1h30m")
	// Examples: "90d", "10d", "1h30m", "30m", "1d12h30m15s"
	// +optional
	// +kubebuilder:validation:Pattern=`^(\d+d)?(\d+h)?(\d+m)?(\d+s)?$`
	TTLAfterCompletion string `json:"ttlAfterCompletion,omitempty"`
}

// ComponentWorkflowOwner identifies the Component that owns a ComponentWorkflowRun execution.
type ComponentWorkflowOwner struct {
	// ProjectName is the name of the owning Project
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProjectName string `json:"projectName"`

	// ComponentName is the name of the owning Component
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ComponentName string `json:"componentName"`
}

// ComponentWorkflowRunConfig defines the workflow configuration for execution.
type ComponentWorkflowRunConfig struct {
	// Name references the ComponentWorkflow CR to use for this execution.
	// The ComponentWorkflow CR contains the schema definition and resource template.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// SystemParameters contains the actual repository information values.
	// This must conform to the required system parameters structure:
	//   - repository.url
	//   - repository.revision.branch
	//   - repository.revision.commit
	//   - repository.appPath
	// +kubebuilder:validation:Required
	SystemParameters SystemParametersValues `json:"systemParameters"`

	// Parameters contains the developer-provided values for the flexible parameter schema
	// defined in the referenced ComponentWorkflow CR.
	//
	// These values are validated against the ComponentWorkflow's parameter schema.
	//
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
}

// SystemParametersValues contains the actual values for system parameters.
// This structure matches the required schema for build-specific features.
type SystemParametersValues struct {
	// Repository contains the actual repository information values.
	// +kubebuilder:validation:Required
	Repository RepositoryValues `json:"repository"`
}

// RepositoryValues contains the actual repository parameter values.
type RepositoryValues struct {
	// URL is the Git repository URL for the component source code.
	// Supports HTTP/HTTPS and SSH formats.
	// Examples:
	//   - "https://github.com/openchoreo/sample-workloads"
	//   - "git@github.com:openchoreo/sample-workloads.git"
	//   - "ssh://git@github.com/openchoreo/sample-workloads.git"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^(https?://|ssh://|git@[^:]+:)`
	URL string `json:"url"`

	// SecretRef is the name of the SecretReference for Git credentials.
	// This should reference a SecretReference CR in the same namespace.
	// Example: "my-git-credentials"
	// +optional
	SecretRef string `json:"secretRef,omitempty"`

	// Revision contains the revision-related parameter values (branch, commit).
	// +kubebuilder:validation:Required
	Revision RepositoryRevisionValues `json:"revision"`

	// AppPath is the path to the application code within the repository.
	// Example: "/service-go-reading-list" or "."
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	AppPath string `json:"appPath"`
}

// RepositoryRevisionValues contains the actual repository revision values.
type RepositoryRevisionValues struct {
	// Branch is the Git branch to build from.
	// Example: "main"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Branch string `json:"branch"`

	// Commit is the specific commit SHA to build.
	// Can be empty to use the latest commit.
	// Example: "a1b2c3d4e5f6" or ""
	// +optional
	// +kubebuilder:validation:Pattern=`^[0-9a-fA-F]{7,40}$`
	Commit string `json:"commit,omitempty"`
}

// ComponentWorkflowRunStatus defines the observed state of ComponentWorkflowRun.
type ComponentWorkflowRunStatus struct {
	// Conditions represent the current state of the ComponentWorkflowRun resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ImageStatus contains information about the built container image from the workflow execution.
	// This is populated when the workflow produces a container image.
	// +optional
	ImageStatus ComponentWorkflowImage `json:"imageStatus,omitempty"`

	// RunReference contains a reference to the workflow run resource that was applied to the build plane cluster.
	// This tracks the actual workflow execution instance (e.g., Argo Workflow) in the target cluster.
	// +optional
	RunReference *ResourceReference `json:"runReference,omitempty"`

	// Resources contains references to additional resources applied to the build plane cluster.
	// These are tracked for cleanup when the ComponentWorkflowRun is deleted.
	// +optional
	Resources *[]ResourceReference `json:"resources,omitempty"`

	// Tasks contains the list of workflow tasks with their execution status.
	// This provides a vendor-neutral view of the workflow steps regardless of the underlying
	// workflow engine (e.g., Argo Workflows, Tekton).
	// Tasks are ordered by their execution sequence.
	// +optional
	Tasks []WorkflowTask `json:"tasks,omitempty"`

	// StartedAt is the timestamp when this workflow run started execution.
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// CompletedAt is the timestamp when this workflow run finished execution (succeeded or failed).
	// This is used together with TTLAfterCompletion to determine when to delete the workflow run.
	// +optional
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`
}

// ComponentWorkflowImage contains information about a container image produced by a component workflow execution.
type ComponentWorkflowImage struct {
	// Image is the fully qualified image name (e.g., registry.example.com/myapp:v1.0.0)
	// +optional
	Image string `json:"image,omitempty"`
}

// WorkflowTask represents a single task/step in a workflow execution.
// This provides a vendor-neutral abstraction over workflow engine-specific steps
// (e.g., Argo Workflow nodes, Tekton TaskRuns).
type WorkflowTask struct {
	// Name is the name of the task/step.
	// For Argo Workflows, this corresponds to the templateName.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Phase represents the current execution phase of the task.
	// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed;Skipped;Error
	// +optional
	Phase string `json:"phase,omitempty"`

	// StartedAt is the timestamp when the task started execution.
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// CompletedAt is the timestamp when the task finished execution.
	// +optional
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`

	// Message provides additional details about the task status.
	// This is typically populated when the task fails or errors.
	// +optional
	Message string `json:"message,omitempty"`
}

// ResourceReference tracks a resource applied to the build plane cluster for cleanup purposes.
type ResourceReference struct {
	// APIVersion is the API version of the resource (e.g., "v1", "apps/v1").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	APIVersion string `json:"apiVersion"`

	// Kind is the type of the resource (e.g., "Secret", "ConfigMap").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Kind string `json:"kind"`

	// Name is the name of the resource in the build plane cluster.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace is the namespace of the resource in the build plane cluster.
	// Empty for cluster-scoped resources.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ComponentWorkflowRun is the Schema for the componentworkflowruns API
type ComponentWorkflowRun struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of ComponentWorkflowRun
	// +required
	Spec ComponentWorkflowRunSpec `json:"spec"`

	// status defines the observed state of ComponentWorkflowRun
	// +optional
	Status ComponentWorkflowRunStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// ComponentWorkflowRunList contains a list of ComponentWorkflowRun
type ComponentWorkflowRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComponentWorkflowRun `json:"items"`
}

// GetConditions returns the conditions from the ComponentWorkflowRun status.
func (c *ComponentWorkflowRun) GetConditions() []metav1.Condition {
	return c.Status.Conditions
}

// SetConditions sets the conditions in the ComponentWorkflowRun status.
func (c *ComponentWorkflowRun) SetConditions(conditions []metav1.Condition) {
	c.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&ComponentWorkflowRun{}, &ComponentWorkflowRunList{})
}
