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

	// URLs declares, per visibility, where clients reach the endpoint and how
	// to call it. Each entry is optional: a type declares only the visibilities
	// it serves. Every field is a ${...} CEL template evaluated at render time
	// against the same context as resources (metadata.*, parameters.*,
	// environmentConfigs.*, visibility, backends[], dataplane.*, environment.*
	// and gateway.*); nothing is read back from the data plane. Resolved values
	// are reported on EndpointReleaseBinding.status.urls.
	// +optional
	URLs *EndpointTypeURLs `json:"urls,omitempty"`

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

// EndpointTypeURLs holds one optional URL template per Endpoint visibility.
// The keys match the values of Endpoint.spec.visibility.
type EndpointTypeURLs struct {
	// Project is the URL for callers within the Endpoint's project. Project
	// visibility is implicit for every Endpoint, so it is resolved whenever it
	// is declared; includeWhen is its only gate.
	// +optional
	Project *EndpointURLTemplate `json:"project,omitempty"`

	// Namespace is the URL for callers within the namespace. Resolved only when
	// the Endpoint requests namespace visibility.
	// +optional
	Namespace *EndpointURLTemplate `json:"namespace,omitempty"`

	// Internal is the URL for callers inside the organization's network.
	// Resolved only when the Endpoint requests internal visibility.
	// +optional
	Internal *EndpointURLTemplate `json:"internal,omitempty"`

	// External is the URL for callers on the public internet. Resolved only
	// when the Endpoint requests external visibility.
	// +optional
	External *EndpointURLTemplate `json:"external,omitempty"`
}

// EndpointURLTemplate templates one URL and the security clients must satisfy
// to call it. Every string field accepts a literal or ${...} CEL.
type EndpointURLTemplate struct {
	// IncludeWhen is an optional ${...}-wrapped CEL expression, ANDed with the
	// visibility gate, that decides whether this URL is resolved. Use it for
	// conditions of the type itself, such as the environment having a gateway
	// for this visibility. Must evaluate to a boolean.
	// Example: "${gateway.?ingress.?external.?https.hasValue()}".
	// +optional
	// +kubebuilder:validation:Pattern=`^\$\{[\s\S]+\}\s*$`
	IncludeWhen string `json:"includeWhen,omitempty"`

	// Scheme is the URL scheme. Must resolve to one of http, https, ws, wss,
	// grpc, grpcs, tcp, tls.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Scheme string `json:"scheme"`

	// Host is the hostname or IP address clients connect to.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`

	// Port is the port clients connect to, as a literal or a ${...} expression
	// resolving to an integer between 1 and 65535. Omit it for the scheme's
	// default port.
	// +optional
	Port string `json:"port,omitempty"`

	// Path is the URL path the endpoint is served under.
	// +optional
	Path string `json:"path,omitempty"`

	// Security declares what the gateway enforces at this URL, so clients know
	// which credential to present. It is a declaration by the type's author;
	// the platform does not verify it against the data plane.
	// +optional
	Security *EndpointSecurityTemplate `json:"security,omitempty"`
}

// EndpointSecurityTemplate templates the security enforced at a URL.
type EndpointSecurityTemplate struct {
	// Scheme is the authentication scheme clients must use. Must resolve to one
	// of none, apiKey, oauth2, basicAuth.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Scheme string `json:"scheme"`

	// APIKey describes how to send the key. Required when scheme resolves to
	// apiKey and ignored when a templated scheme resolves to anything else, so
	// a parameter can switch API-key auth on and off. Rejected alongside a
	// literal scheme other than apiKey.
	// +optional
	APIKey *EndpointAPIKeyTemplate `json:"apiKey,omitempty"`
}

// EndpointAPIKeyTemplate templates where an API key is sent.
type EndpointAPIKeyTemplate struct {
	// In is where the key is sent. Must resolve to header or query.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	In string `json:"in"`

	// Name is the header or query parameter name carrying the key.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
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
