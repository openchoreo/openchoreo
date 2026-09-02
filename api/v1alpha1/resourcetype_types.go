// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ResourceTypeSpec defines the desired state of ResourceType.
type ResourceTypeSpec struct {
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

	// Endpoints declares the dialable network endpoints of this resource type.
	// Consumers may redirect the env bindings of a declared endpoint to a different
	// address, so an address that must always be used as published -- an externally
	// served admin UI, for example -- must not be declared here.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=10
	Endpoints []ResourceTypeEndpoint `json:"endpoints,omitempty"`

	// Resources are the Kubernetes manifests the ResourceType provisioner emits
	// on the data plane. Each entry has a unique id used by readyWhen and outputs
	// CEL to reference applied.<id>.status.* fields.
	// +kubebuilder:validation:MinItems=1
	// +listType=map
	// +listMapKey=id
	Resources []ResourceTypeManifest `json:"resources"`
}

// ResourceTypeOutput defines a single output of a ResourceType.
// Exactly one of value, secretKeyRef, or configMapKeyRef must be set.
// +kubebuilder:validation:XValidation:rule="(has(self.value)?1:0) + (has(self.secretKeyRef)?1:0) + (has(self.configMapKeyRef)?1:0) == 1",message="exactly one of value, secretKeyRef, or configMapKeyRef must be set"
type ResourceTypeOutput struct {
	// Name uniquely identifies this output within the ResourceType. Referenced by
	// Workload.spec.dependencies.resources[].envBindings and fileBindings keys.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Value is a literal or ${...} CEL expression evaluating to a string.
	// Use only for non-sensitive data (host, port, region, database name); the
	// resolved value transits to the control plane.
	// +optional
	Value string `json:"value,omitempty"`

	// SecretKeyRef references a Secret on the data plane.
	// Use for sensitive credentials (passwords, tokens, private keys).
	// Only the {name, key} reference transits to the control plane; the
	// underlying value never leaves the data plane.
	// Both name and key support ${...} CEL templating.
	// +optional
	SecretKeyRef *SecretKeyRef `json:"secretKeyRef,omitempty"`

	// ConfigMapKeyRef references a ConfigMap on the data plane.
	// Both name and key support ${...} CEL templating.
	// +optional
	ConfigMapKeyRef *ConfigMapKeyRef `json:"configMapKeyRef,omitempty"`
}

// ResourceTypeEndpoint declares one dialable network endpoint of a resource type.
// The address is declared in one of two ways, and both halves must use the same one:
// as the outputs that carry them (HostFrom and PortFrom), or literally (Host and Port)
// for types that do not publish the address as outputs at all. Most types name outputs
// and need nothing else.
// +kubebuilder:validation:XValidation:rule="(has(self.hostFrom) || has(self.host)) && (has(self.portFrom) || has(self.port))",message="host must come from hostFrom or host, and port from portFrom or port"
// +kubebuilder:validation:XValidation:rule="has(self.hostFrom) == has(self.portFrom)",message="hostFrom and portFrom must be set together: name outputs for both halves of the address, or declare both inline with host and port"
type ResourceTypeEndpoint struct {
	// Name uniquely identifies this endpoint within the resource type. It becomes a
	// segment of the remote-connect target key, so it is restricted to a DNS-1123 label:
	// a name carrying a "/" would make two distinct endpoints share one key.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`

	// HostFrom is the name of the output carrying this endpoint's hostname. Naming it
	// is what lets a consumer redirect the env bindings of that output along with the
	// endpoint. Several endpoints may share one host output.
	//
	// The output may be reference-backed (a Secret or ConfigMap key), which is how a
	// provisioner-backed resource publishes an address it only learns after
	// provisioning; the endpoint then resolves without a dialable address, since the
	// control plane does not read those values. Omit HostFrom only when no output
	// carries the host on its own -- a type publishing only a composed connection URL,
	// say -- and set Host instead.
	//
	// HostFrom and PortFrom are set together or not at all: a consumer redirects an
	// endpoint by rewriting the env bindings of its named outputs, so naming one half
	// without the other would leave that other binding on the in-cluster value.
	// +optional
	// +kubebuilder:validation:MinLength=1
	HostFrom string `json:"hostFrom,omitempty"`

	// PortFrom is the name of the output carrying this endpoint's TCP port. Two
	// endpoints of the same resource type must not share a port output, because one
	// env var cannot carry a redirected address for both. Omit it -- and set Port
	// instead -- only when no output carries the port on its own, which also means
	// HostFrom must be left unset and Host set alongside.
	// +optional
	// +kubebuilder:validation:MinLength=1
	PortFrom string `json:"portFrom,omitempty"`

	// Host supplies the hostname to dial for a type that publishes no host output of
	// its own -- one whose only output is a composed connection string. A ${...} CEL
	// expression evaluated against the same context as outputs, so it only suits an
	// address knowable at render time: a service DNS name, or an applied resource's
	// status field. Takes precedence over HostFrom when both are set.
	//
	// An endpoint declared this way has no output binding to redirect, so a consumer
	// re-points it inside a composed value instead. Set Port alongside it, never
	// PortFrom.
	// +optional
	// +kubebuilder:validation:MaxLength=512
	Host string `json:"host,omitempty"`

	// Port supplies the TCP port to dial, under the same rules as Host. A string
	// rather than an integer so it can be a ${...} expression, matching every other
	// templated field on this type; it must render to a number in 1-65535, which is
	// checked at admission for a literal and at resolve time for an expression. Quote
	// it even when it is a literal -- port: "6379".
	// +optional
	// +kubebuilder:validation:MaxLength=512
	Port string `json:"port,omitempty"`
}

// ResourceTypeManifest defines a Kubernetes resource template that the
// ResourceType provisioner emits on the data plane.
type ResourceTypeManifest struct {
	// ID uniquely identifies this entry within the ResourceType.
	// Referenced by readyWhen and outputs CEL via applied.<id>.status.*.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ID string `json:"id"`

	// IncludeWhen is an optional CEL expression that determines whether this
	// entry is rendered. Evaluated against metadata.*, parameters.*,
	// environmentConfigs.*, and dataplane.*; applied.<id>.* is NOT available
	// because the rendered objects haven't been applied yet. Must be
	// ${...}-wrapped and must evaluate to a boolean. If unset, the entry is
	// always rendered.
	// Example: "${parameters.tlsEnabled}".
	// +optional
	// +kubebuilder:validation:Pattern=`^\$\{[\s\S]+\}\s*$`
	IncludeWhen string `json:"includeWhen,omitempty"`

	// Template contains the Kubernetes resource with ${...} CEL expressions.
	// At render time the CEL context exposes metadata.*, parameters.*,
	// environmentConfigs.*, and dataplane.*. applied.<id>.status.* is NOT
	// available during rendering because the rendered objects haven't been
	// applied yet.
	// +kubebuilder:validation:Required
	// +kubebuilder:pruning:PreserveUnknownFields
	Template *runtime.RawExtension `json:"template"`

	// ReadyWhen is an optional ${...}-wrapped CEL expression that determines
	// whether this entry contributes to
	// ResourceBinding.status.conditions[ResourcesReady]. Evaluated against
	// metadata.*, parameters.*, environmentConfigs.*, dataplane.*, and
	// applied.<id>.* once the manifest has been applied. If unset, falls back
	// to RenderedRelease per-Kind health inference. Must evaluate to a boolean.
	// Example: "${applied.claim.status.conditions.exists(c, c.type == 'Ready' && c.status == 'True')}".
	// +optional
	// +kubebuilder:validation:Pattern=`^\$\{[\s\S]+\}\s*$`
	ReadyWhen string `json:"readyWhen,omitempty"`
}

// ResourceTypeStatus defines the observed state of ResourceType.
type ResourceTypeStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=rt;rts
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ResourceType is the Schema for the resourcetypes API.
// PEs publish ResourceType templates in a namespace; developers reference them
// by name from Resource.spec.type.
type ResourceType struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ResourceTypeSpec   `json:"spec,omitempty"`
	Status ResourceTypeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ResourceTypeList contains a list of ResourceType.
type ResourceTypeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceType `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ResourceType{}, &ResourceTypeList{})
}
