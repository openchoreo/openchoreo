// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointpipeline

import (
	"time"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	projectpipeline "github.com/openchoreo/openchoreo/internal/pipeline/project"
	"github.com/openchoreo/openchoreo/internal/template"
)

// Pipeline renders the inlined (Cluster)EndpointType.spec.resources templates
// for a single EndpointReleaseBinding. The instance holds a template.Engine
// whose CEL environment and program caches are reused across calls; consumers
// instantiate one Pipeline and reuse it.
type Pipeline struct {
	templateEngine *template.Engine

	// celCostLimit bounds the accumulated cost of a single CEL expression.
	// Zero selects the template engine's built-in default.
	celCostLimit uint64

	// renderTimeout bounds the wall-clock duration of each Render call. Zero or
	// negative means no deadline.
	renderTimeout time.Duration
}

// DataPlaneContext is the dataplane surface exposed to CEL as ${dataplane.*}.
// Shared with the project pipeline so both template families see one shape.
type DataPlaneContext = projectpipeline.DataPlaneContext

// EnvironmentContext is the per-environment surface exposed to CEL as
// ${environment.*}, carrying the env-or-dataplane merged gateway.
type EnvironmentContext = projectpipeline.EnvironmentContext

// GatewayData provides gateway configuration in templates.
type GatewayData = projectpipeline.GatewayData

// BuildDataPlaneContext extracts the dataplane CEL surface from a DataPlane.
var BuildDataPlaneContext = projectpipeline.BuildDataPlaneContext

// BuildEnvironmentContext extracts the per-environment CEL surface, merging
// environment-level gateway overrides with dataplane-level fallbacks.
var BuildEnvironmentContext = projectpipeline.BuildEnvironmentContext

// RenderInput carries everything Render needs. The binding controller
// rehydrates EndpointTypeSpec, Visibility and Parameters from the immutable
// EndpointRelease snapshot, resolves Backends for the binding's environment,
// and supplies EnvironmentConfigs from the binding.
type RenderInput struct {
	// EndpointTypeSpec is the inlined (Cluster)EndpointType spec snapshot from
	// the EndpointRelease. Required.
	EndpointTypeSpec *v1alpha1.EndpointTypeSpec

	// Parameters is the snapshot of Endpoint.spec.parameters, exposed to CEL as
	// ${parameters.*} with EndpointTypeSpec.Parameters schema defaults applied.
	Parameters *runtime.RawExtension

	// EnvironmentConfigs is EndpointReleaseBinding.spec.endpointTypeEnvironmentConfigs,
	// exposed to CEL as ${environmentConfigs.*} with schema defaults applied.
	EnvironmentConfigs *runtime.RawExtension

	// Visibility is the snapshot of Endpoint.spec.visibility, exposed to CEL as
	// ${visibility} so a type can render one route per requested visibility
	// (e.g. includeWhen: ${"external" in visibility}).
	Visibility []v1alpha1.EndpointVisibility

	// Backends are the resolved traffic backends for this environment, exposed
	// to CEL as ${backends}. Required to be non-empty: the binding controller
	// does not render when nothing can receive traffic.
	Backends []Backend

	// Rules are the routing rules for this environment, exposed to CEL as
	// ${rules}: one rule per backend with a match, in backendRefs order, then
	// the default rule that splits every other request by weight. Optional:
	// types that only route by weight keep using ${backends}.
	Rules []Rule

	// Metadata is the controller-computed metadata surface exposed to CEL as
	// ${metadata.*}.
	Metadata MetadataContext

	// DataPlane is exposed to CEL as ${dataplane.*}.
	DataPlane DataPlaneContext

	// Environment is exposed to CEL as ${environment.*}; its gateway is also
	// aliased at the top level as ${gateway.*}.
	Environment EnvironmentContext
}

// Backend is one resolved traffic backend, normalized so templates read the
// same fields whatever kind of backend it came from.
type Backend struct {
	// Name is the backendRef's name: the stable handle a template uses to
	// single out one backend.
	Name string `json:"name"`

	// Kind is the backendRef's kind (Component).
	Kind string `json:"kind"`

	// ComponentName is the backend Component. Set for kind Component.
	ComponentName string `json:"componentName,omitempty"`

	// Endpoint is the workload endpoint name on the Component. Set for kind
	// Component.
	Endpoint string `json:"endpoint,omitempty"`

	// Protocol is the backend's protocol (HTTP, gRPC, ...), so a type can
	// restrict what it routes through its validations.
	Protocol string `json:"protocol"`

	// ServiceName and Namespace locate the backend's Kubernetes Service on the
	// data plane. Gateway API backendRefs use {name: serviceName, port: port}.
	ServiceName string `json:"serviceName"`
	Namespace   string `json:"namespace"`
	Port        int32  `json:"port"`

	// Weight is the relative traffic weight from the backendRef.
	Weight int32 `json:"weight"`

	// Percent is the backend's share of its rule's traffic, from 0 to 100.
	// The shares of a rule's backends add up to exactly 100, for data planes
	// that take percentages instead of relative weights.
	Percent int32 `json:"percent"`

	// ServiceURL is the in-cluster URL of the backend, e.g.
	// http://greeter-dev-1a2b3c.dp-default-shop-dev-4d5e6f.svc.cluster.local:8080/api.
	ServiceURL string `json:"serviceURL"`

	// BasePath is the path the backend serves under ("/" when unset). Templates
	// rewrite their public path prefix to it.
	BasePath string `json:"basePath"`
}

// Rule is one routing rule with its backends resolved.
type Rule struct {
	// Name identifies the rule: the matched backend's name, or "default" for
	// the rule that splits by weight.
	Name string `json:"name"`

	// Matches select the requests the rule applies to; empty for the default
	// rule.
	Matches []Match `json:"matches"`

	// Backends are the rule's backends with their weight in this environment.
	// A match rule has its one backend, at weight 1.
	Backends []Backend `json:"backends"`
}

// Match is one set of conditions a request must all meet. Lists are never
// nil, so templates can use them without has().
type Match struct {
	// Path is the path condition, relative to where the Endpoint is served;
	// nil when the match has none. Templates join it to the prefix they serve
	// the Endpoint at.
	Path *PathMatch `json:"path,omitempty"`

	// Conditions are the match's header conditions ready to render on any
	// routing API: header names lowercased, and the cookie as a regular
	// expression on the cookie header with its name and value escaped.
	// Templates should render these rather than Headers and Cookies, so
	// every data plane matches the same requests.
	Conditions []HeaderMatch `json:"conditions"`

	// Headers are the match's header conditions, names lowercased.
	Headers []HeaderMatch `json:"headers"`
	// Cookies are the match's cookie conditions as written; see Conditions.
	Cookies []CookieMatch `json:"cookies"`
}

// PathMatch is a path condition; Type is PathPrefix or Exact.
type PathMatch struct {
	Type  string `json:"type"`
	Value string `json:"value"`

	// Regex is an RE2 pattern for the whole path that matches the same
	// requests, for routing APIs whose prefix match is a plain string prefix
	// (Istio, Envoy) rather than path segments as in Gateway API.
	Regex string `json:"regex"`
}

// HeaderMatch is a header condition; Type is Exact or RegularExpression.
type HeaderMatch struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// CookieMatch is an exact cookie condition.
type CookieMatch struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// RenderOutput carries the rendered manifests in spec order. ForEach
// templates contribute one entry per iteration, with ID suffixed by the
// iteration index.
type RenderOutput struct {
	Entries []RenderedEntry
}

// RenderedEntry is one rendered manifest. For non-forEach templates ID equals
// the source ResourceTemplate.ID; forEach iterations are suffixed "-<index>".
type RenderedEntry struct {
	ID     string
	Object map[string]any
}

// MetadataContext is the platform-injected metadata surface exposed to CEL
// templates as ${metadata.*}. The controller computes every field before
// calling the pipeline.
type MetadataContext struct {
	// Name is a platform-computed, collision-safe base name for the objects
	// this (endpoint, environment) renders. Templates rendering more than one
	// object derive distinct names from it.
	Name string `json:"name"`

	// Namespace is the dp-{ns}-{project}-{env}-{hash} data-plane namespace.
	// It is shared with the project's components, so routes reference backend
	// Services in their own namespace.
	Namespace string `json:"namespace"`

	// EndpointNamespace is the control-plane namespace of the Endpoint.
	EndpointNamespace string `json:"endpointNamespace"`

	EndpointName    string `json:"endpointName"`
	EndpointUID     string `json:"endpointUID"`
	ProjectName     string `json:"projectName"`
	ProjectUID      string `json:"projectUID"`
	EnvironmentName string `json:"environmentName"`
	EnvironmentUID  string `json:"environmentUID"`
	DataPlaneName   string `json:"dataPlaneName"`
	DataPlaneUID    string `json:"dataPlaneUID"`

	// Labels are platform-injected standard labels. Templates that propagate
	// ${metadata.labels} mark every rendered object with its owning Endpoint.
	Labels map[string]string `json:"labels"`

	// Annotations are propagated from the EndpointReleaseBinding CR.
	Annotations map[string]string `json:"annotations"`
}

// BaseContext is the top-level CEL surface fed into the template engine.
type BaseContext struct {
	Metadata           MetadataContext    `json:"metadata"`
	Parameters         map[string]any     `json:"parameters"`
	EnvironmentConfigs map[string]any     `json:"environmentConfigs"`
	Visibility         []string           `json:"visibility"`
	Backends           []Backend          `json:"backends"`
	Rules              []Rule             `json:"rules"`
	DataPlane          DataPlaneContext   `json:"dataplane"`
	Environment        EnvironmentContext `json:"environment"`
	// Gateway is the effective gateway (Environment.Gateway) exposed at the top
	// level for terseness. Omitted when no gateway is configured, so templates
	// that may run without one must guard with has(environment.gateway).
	Gateway *GatewayData `json:"gateway,omitempty"`
}
