// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// WorkflowRunSpec defines the desired state of WorkflowRun.
// WorkflowRun represents a runtime execution instance of a Workflow.
type WorkflowRunSpec struct {
	// Workflow configuration referencing the Workflow CR and providing schema values.
	// +required
	Workflow WorkflowRunConfig `json:"workflow"`

	// InputArtifacts are immutable, content-addressed inputs made available to the
	// workflow runner as read-only files. They are deliberately distinct from
	// workflow.parameters: parameter values are small control-plane data, while
	// artifact contents never transit or persist in this API object.
	//
	// The only accepted URI form is
	// s3://openchoreo-workflow-inputs/sha256/<sha256>. The artifact publisher is
	// responsible for writing that content-addressed object once and for granting
	// the existing workflow workload identity read-only access to the bucket.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="inputArtifacts are immutable"
	InputArtifacts []WorkflowRunInputArtifact `json:"inputArtifacts,omitempty"`

	// TTLAfterCompletion defines the time-to-live for this workflow run after completion.
	// This value is copied from the Workflow template.
	// Once the workflow completes, the run will be automatically deleted after this duration.
	// Format: duration string supporting days, hours, minutes, seconds without spaces (e.g., "90d", "10d1h30m", "1h30m")
	// Examples: "90d", "10d", "1h30m", "30m", "1d12h30m15s"
	// +optional
	// +kubebuilder:validation:Pattern=`^(\d+d)?(\d+h)?(\d+m)?(\d+s)?$`
	TTLAfterCompletion string `json:"ttlAfterCompletion,omitempty"`
}

const (
	// WorkflowRunInputArtifactBucket is the sole trusted artifact store accepted
	// by the v1 input-artifact contract.
	WorkflowRunInputArtifactBucket = "openchoreo-workflow-inputs"
	// WorkflowRunInputArtifactMaxSizeBytes prevents a WorkflowRun from becoming
	// a large-payload transport. Larger inputs require a later contract version.
	WorkflowRunInputArtifactMaxSizeBytes int64 = 10 * 1024 * 1024
)

// WorkflowRunInputArtifact describes an immutable workflow input. It contains
// metadata and a trusted, content-addressed reference only; it never contains
// bytes, credentials, signed URLs, or other secrets.
type WorkflowRunInputArtifact struct {
	// Name identifies the Argo input artifact expected by the workflow template.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`

	// URI is a trusted content-addressed S3 reference. It must be exactly
	// s3://openchoreo-workflow-inputs/sha256/<sha256>, where <sha256> equals sha256.
	// +kubebuilder:validation:Pattern=`^s3://openchoreo-workflow-inputs/sha256/[a-f0-9]{64}$`
	URI string `json:"uri"`

	// MediaType identifies the artifact representation, for example text/x-diff.
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=127
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9!#$&^_.+-]+/[A-Za-z0-9!#$&^_.+-]+$`
	MediaType string `json:"mediaType"`

	// SizeBytes is the exact uncompressed payload size.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10485760
	SizeBytes int64 `json:"sizeBytes"`

	// SHA256 is the lowercase SHA-256 digest of the exact bytes the runner reads.
	// +kubebuilder:validation:Pattern=`^[a-f0-9]{64}$`
	SHA256 string `json:"sha256"`

	// ExpiresAt is the hard expiry for fetching the input. It must be in the
	// future when submitted and no more than 24 hours away.
	ExpiresAt metav1.Time `json:"expiresAt"`
}

// WorkflowRunInputArtifactStatus exposes non-sensitive, auditable metadata for
// an accepted input. The URI, credentials, and artifact bytes are never copied
// to status.
type WorkflowRunInputArtifactStatus struct {
	Name      string      `json:"name"`
	MediaType string      `json:"mediaType"`
	SizeBytes int64       `json:"sizeBytes"`
	SHA256    string      `json:"sha256"`
	ExpiresAt metav1.Time `json:"expiresAt"`
}

// WorkflowRunConfig defines the workflow configuration for execution.
type WorkflowRunConfig struct {
	// Kind is the kind of workflow (Workflow or ClusterWorkflow).
	// +optional
	// +kubebuilder:default=ClusterWorkflow
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="kind is immutable"
	Kind WorkflowRefKind `json:"kind,omitempty"`

	// Name references the Workflow or ClusterWorkflow CR to use for this execution.
	// The Workflow CR contains the schema definition and resource template.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// Parameters contains the developer-provided values for the flexible parameter schema
	// defined in the referenced Workflow CR.
	//
	// These values are validated against the Workflow's parameter schema.
	//
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
}

// ResourceReference tracks a resource applied to the workflow plane cluster for cleanup purposes.
type ResourceReference struct {
	// APIVersion is the API version of the resource (e.g., "v1", "apps/v1").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	APIVersion string `json:"apiVersion"`

	// Kind is the type of the resource (e.g., "Secret", "ConfigMap").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Kind string `json:"kind"`

	// Name is the name of the resource in the workflow plane cluster.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace is the namespace of the resource in the workflow plane cluster.
	// Empty for cluster-scoped resources.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// WorkflowTask represents a single task/step in a workflow execution.
// This provides a vendor-neutral abstraction over workflow engine-specific steps
// (e.g., Argo Workflow nodes, Tekton TaskRuns).
type WorkflowTask struct {
	// Name is the name of the task/step.
	// For Argo Workflows, this corresponds to the node's displayName (or parsed node name from the
	// node name pattern "workflow-name[N].step-name"), not the templateName, since workflows now use
	// ClusterWorkflowTemplates with templateRef instead of inline templates.
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

// WorkflowRunStatus defines the observed state of WorkflowRun.
type WorkflowRunStatus struct {
	// Conditions represent the current state of the WorkflowRun resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// InputArtifacts contains non-sensitive metadata for the immutable inputs
	// accepted by the runner. It intentionally excludes URI and all content.
	// +listType=map
	// +listMapKey=name
	// +optional
	InputArtifacts []WorkflowRunInputArtifactStatus `json:"inputArtifacts,omitempty"`

	// RunReference contains a reference to the workflow run resource that was applied to the cluster.
	// This tracks the actual workflow execution instance (e.g., Argo Workflow) in the target cluster.
	// +optional
	RunReference *ResourceReference `json:"runReference,omitempty"`

	// Resources contains references to additional resources applied to the cluster.
	// These are tracked for cleanup when the WorkflowRun is deleted.
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

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// WorkflowRun is the Schema for the workflowruns API
// WorkflowRun represents a runtime execution instance of a Workflow.
type WorkflowRun struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of WorkflowRun
	// +required
	Spec WorkflowRunSpec `json:"spec"`

	// status defines the observed state of WorkflowRun
	// +optional
	Status WorkflowRunStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// WorkflowRunList contains a list of WorkflowRun
type WorkflowRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkflowRun `json:"items"`
}

// GetConditions returns the conditions from the workflowrun status
func (w *WorkflowRun) GetConditions() []metav1.Condition {
	return w.Status.Conditions
}

// SetConditions sets the conditions in the workflowrun status
func (w *WorkflowRun) SetConditions(conditions []metav1.Condition) {
	w.Status.Conditions = conditions
}

func init() {
	SchemeBuilder.Register(&WorkflowRun{}, &WorkflowRunList{})
}
