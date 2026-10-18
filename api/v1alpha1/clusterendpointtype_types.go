// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterEndpointTypeSpec defines the desired state of ClusterEndpointType.
// Currently mirrors EndpointTypeSpec; may diverge as cluster-scoped concerns emerge.
type ClusterEndpointTypeSpec struct {
	// Parameters is the schema for Endpoint.spec.parameters values supplied by
	// Endpoint authors (e.g. hostnames, path prefixes). Validated against this
	// schema.
	// +optional
	Parameters *SchemaSection `json:"parameters,omitempty"`

	// EnvironmentConfigs defines the per-env schema.
	// Validates EndpointReleaseBinding.spec.endpointTypeEnvironmentConfigs.
	// +optional
	EnvironmentConfigs *SchemaSection `json:"environmentConfigs,omitempty"`

	// Validations are CEL-based rules evaluated before rendering. All rules must
	// evaluate to true for rendering to proceed.
	// +optional
	Validations []ValidationRule `json:"validations,omitempty"`

	// Resources are the routing manifests (e.g. HTTPRoute, Ingress, OpenShift
	// Route) the ClusterEndpointType emits on the data plane for each
	// EndpointReleaseBinding. Uses the same templating engine as ComponentType.
	// At render time the CEL context exposes metadata.*, parameters.*,
	// environmentConfigs.*, dataplane.*, environment.* (aliased gateway.*),
	// and backends[] resolved for the binding's environment.
	// +kubebuilder:validation:MinItems=1
	// +listType=map
	// +listMapKey=id
	Resources []ResourceTemplate `json:"resources"`
}

// ClusterEndpointTypeStatus defines the observed state of ClusterEndpointType.
type ClusterEndpointTypeStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=cept;cepts
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ClusterEndpointType is the Schema for the clusterendpointtypes API.
// ClusterEndpointType is the cluster-scoped sibling of EndpointType. Endpoints
// in any namespace can reference a ClusterEndpointType via Endpoint.spec.type.
type ClusterEndpointType struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterEndpointTypeSpec   `json:"spec,omitempty"`
	Status ClusterEndpointTypeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterEndpointTypeList contains a list of ClusterEndpointType.
type ClusterEndpointTypeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterEndpointType `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterEndpointType{}, &ClusterEndpointTypeList{})
}
