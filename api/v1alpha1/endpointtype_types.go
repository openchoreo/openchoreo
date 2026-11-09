// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EndpointTypeSpec defines the desired state of EndpointType.
type EndpointTypeSpec struct {
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
	// Route) the EndpointType emits on the data plane for each
	// EndpointReleaseBinding. Uses the same templating engine as ComponentType.
	// At render time the CEL context exposes metadata.*, parameters.*,
	// environmentConfigs.*, dataplane.*, environment.* (aliased gateway.*),
	// and backends[] resolved for the binding's environment.
	// +kubebuilder:validation:MinItems=1
	// +listType=map
	// +listMapKey=id
	Resources []ResourceTemplate `json:"resources"`
}

// EndpointTypeStatus defines the observed state of EndpointType.
type EndpointTypeStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=ept;epts
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// EndpointType is the Schema for the endpointtypes API.
// PEs publish EndpointType templates in a namespace; Endpoint authors reference
// them by name from Endpoint.spec.type.
type EndpointType struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EndpointTypeSpec   `json:"spec,omitempty"`
	Status EndpointTypeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EndpointTypeList contains a list of EndpointType.
type EndpointTypeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EndpointType `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EndpointType{}, &EndpointTypeList{})
}
