// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterResourceTypeSpec defines the desired state of ClusterResourceType.
// Currently mirrors ResourceTypeSpec; may diverge as cluster-scoped concerns emerge.
type ClusterResourceTypeSpec struct {
	// Parameters is the schema for Resource.spec.parameters values supplied by
	// Resource authors. Validated against this schema.
	// +optional
	Parameters *SchemaSection `json:"parameters,omitempty"`

	// EnvironmentConfigs defines the per-env schema.
	// Validates ResourceBinding.spec.resourceTypeEnvironmentConfigs.
	// +optional
	EnvironmentConfigs *SchemaSection `json:"environmentConfigs,omitempty"`

	// RetainPolicy is the default retention for ResourceBindings of this type.
	// Per-env override is available via ResourceBinding.spec.retainPolicy.
	// +optional
	// +kubebuilder:default=Delete
	RetainPolicy ResourceRetainPolicy `json:"retainPolicy,omitempty"`

	// Outputs declares values that workloads consume via
	// Workload.spec.dependencies.resources[].envBindings or fileBindings.
	// Each entry is identified by a unique name and picks exactly one of value,
	// secretKeyRef, or configMapKeyRef. Output value, name, and key fields support
	// ${...} CEL templating evaluated against metadata.*, parameters.*,
	// environmentConfigs.*, and applied.<id>.status.*.
	// +optional
	// +listType=map
	// +listMapKey=name
	Outputs []ResourceTypeOutput `json:"outputs,omitempty"`

	// Resources are the Kubernetes manifests the ClusterResourceType provisioner
	// emits on the data plane. Each entry has a unique id used by readyWhen and
	// outputs CEL to reference applied.<id>.status.* fields.
	// +kubebuilder:validation:MinItems=1
	// +listType=map
	// +listMapKey=id
	Resources []ResourceTypeManifest `json:"resources"`
}

// ClusterResourceTypeStatus defines the observed state of ClusterResourceType.
type ClusterResourceTypeStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=crt;crts
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ClusterResourceType is the Schema for the clusterresourcetypes API.
// ClusterResourceType is the cluster-scoped sibling of ResourceType. Resources
// in any namespace can reference a ClusterResourceType via Resource.spec.type.
type ClusterResourceType struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterResourceTypeSpec   `json:"spec,omitempty"`
	Status ClusterResourceTypeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterResourceTypeList contains a list of ClusterResourceType.
type ClusterResourceTypeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterResourceType `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterResourceType{}, &ClusterResourceTypeList{})
}
