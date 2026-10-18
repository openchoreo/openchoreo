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

	// ServiceURL is the in-cluster URL of the backend, e.g.
	// http://greeter-dev-1a2b3c.dp-default-shop-dev-4d5e6f.svc.cluster.local:8080/api.
	ServiceURL string `json:"serviceURL"`

	// BasePath is the path the backend serves under ("/" when unset). Templates
	// rewrite their public path prefix to it.
	BasePath string `json:"basePath"`
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
	DataPlane          DataPlaneContext   `json:"dataplane"`
	Environment        EnvironmentContext `json:"environment"`
	// Gateway is the effective gateway (Environment.Gateway) exposed at the top
	// level for terseness. Omitted when no gateway is configured, so templates
	// that may run without one must guard with has(environment.gateway).
	Gateway *GatewayData `json:"gateway,omitempty"`
}
