// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ClusterWorkflowSpec defines the desired state of ClusterWorkflow.
// ClusterWorkflow is a cluster-scoped version of Workflow that can be
// referenced by Components across all namespaces via ClusterComponentType.
type ClusterWorkflowSpec struct {
	// BuildPlaneRef references the ClusterBuildPlane for this workflow's build operations.
	// If not specified, the controller resolves to the ClusterBuildPlane named "default".
	// +optional
	BuildPlaneRef *ClusterBuildPlaneRef `json:"buildPlaneRef,omitempty"`

	// Schema defines the developer-facing parameters that can be configured
	// when creating a WorkflowRun instance.
	// +optional
	Schema *WorkflowSchema `json:"schema,omitempty"`

	// RunTemplate is the Kubernetes resource template to be rendered and applied to the cluster.
	// +required
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	RunTemplate *runtime.RawExtension `json:"runTemplate"`

	// Resources are additional templates that generate Kubernetes resources dynamically.
	// +optional
	Resources []WorkflowResource `json:"resources,omitempty"`

	// ExternalRefs declares references to external CRs that are resolved at runtime.
	// +optional
	// +listType=map
	// +listMapKey=id
	ExternalRefs []ExternalRef `json:"externalRefs,omitempty"`

	// TTLAfterCompletion defines the time-to-live for WorkflowRun instances after completion.
	// +optional
	// +kubebuilder:validation:Pattern=`^(\d+d)?(\d+h)?(\d+m)?(\d+s)?$`
	TTLAfterCompletion string `json:"ttlAfterCompletion,omitempty"`
}

// ClusterWorkflowStatus defines the observed state of ClusterWorkflow.
type ClusterWorkflowStatus struct {
	// conditions represent the current state of the ClusterWorkflow resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=cwf;cwfs
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ClusterWorkflow is the Schema for the clusterworkflows API.
// ClusterWorkflow is a cluster-scoped version of Workflow that can be
// referenced by Components across all namespaces.
type ClusterWorkflow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterWorkflowSpec   `json:"spec,omitempty"`
	Status ClusterWorkflowStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterWorkflowList contains a list of ClusterWorkflow.
type ClusterWorkflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterWorkflow `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterWorkflow{}, &ClusterWorkflowList{})
}
