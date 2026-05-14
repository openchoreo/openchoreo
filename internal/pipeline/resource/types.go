// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcepipeline

import (
	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

// Pipeline orchestrates ResourceType template rendering and output resolution
// for a single ResourceReleaseBinding. The instance holds a template.Engine
// whose CEL environment and program caches are reused across calls; consumers
// (controller, webhook, future CLI) instantiate one Pipeline and reuse it.
type Pipeline struct {
	templateEngine *template.Engine
}

// RenderInput carries everything RenderManifests, ResolveOutputs, and
// EvaluateReadyWhen need. Inputs are typed CRDs (mirrors the component
// pipeline at internal/pipeline/component/types.go:21-65). The binding
// controller rehydrates ResourceType and Resource from the immutable
// ResourceRelease snapshot before calling; webhooks pass live CRDs
// directly.
type RenderInput struct {
	// ResourceType carries the ResourceType template. The pipeline reads
	// Spec.Resources, Spec.Outputs, Spec.Parameters (schema), and
	// Spec.EnvironmentConfigs (schema) from it. Required.
	ResourceType *v1alpha1.ResourceType

	// Resource is the developer-authored Resource CR. The pipeline reads
	// Spec.Parameters from it. Required.
	Resource *v1alpha1.Resource

	// ResourceReleaseBinding carries per-env overrides. The pipeline reads
	// Spec.ResourceTypeEnvironmentConfigs from it. Optional: nil means no
	// environmentConfigs were provided (defaults from the schema still
	// apply).
	ResourceReleaseBinding *v1alpha1.ResourceReleaseBinding

	// Metadata is the controller-computed metadata surface exposed to CEL
	// as ${metadata.*}.
	Metadata MetadataContext

	// DataPlane is the controller-computed dataplane surface exposed to CEL
	// as ${dataplane.*}.
	DataPlane DataPlaneContext
}

// RenderOutput carries the rendered manifests in spec order.
type RenderOutput struct {
	// Entries are the rendered resources, one per
	// ResourceTypeSpec.Resources[] that passed its IncludeWhen check. IDs
	// are preserved verbatim from the input so the binding controller can
	// correlate the observed applied status back to the originating
	// template entry when calling ResolveOutputs.
	Entries []RenderedEntry
}

// RenderedEntry is one rendered manifest. The controller marshals Object
// into runtime.RawExtension at its own boundary so the pipeline stays free
// of K8s runtime types in its public surface.
type RenderedEntry struct {
	// ID matches ResourceTypeSpec.Resources[].ID.
	ID string

	// Object is the rendered Kubernetes resource as a map. CEL substitutions
	// have been evaluated and omit-sentinel keys removed.
	Object map[string]any
}

// ResolvedOutput is one entry resolved from ResourceTypeSpec.Outputs. Exactly
// one of Value, SecretKeyRef, or ConfigMapKeyRef is populated, matching the
// source kind declared on the ResourceType output.
type ResolvedOutput struct {
	Name            string
	Value           string
	SecretKeyRef    *v1alpha1.SecretKeyRef
	ConfigMapKeyRef *v1alpha1.ConfigMapKeyRef
}

// MetadataContext is the platform-injected metadata surface exposed to CEL
// templates as ${metadata.*}. The controller computes every field before
// calling the pipeline. JSON tags determine the CEL-facing field names when
// the context is marshaled via structToMap, and are also reflected on by the
// validation package to declare the CEL env surface.
type MetadataContext struct {
	// Name is the platform-computed base name for rendered resources,
	// shaped {resource}-{env}-{hash} (mirrors component pipeline's
	// {component}-{env}-{hash} convention).
	Name string `json:"name"`

	// Namespace is the DP-side project-env mapped namespace. Exposed via
	// CEL only; the pipeline does NOT force-set metadata.namespace on
	// rendered manifests.
	Namespace string `json:"namespace"`

	// ResourceNamespace is the CP namespace where the Resource CR lives.
	ResourceNamespace string `json:"resourceNamespace"`

	ResourceName    string `json:"resourceName"`
	ResourceUID     string `json:"resourceUID"`
	ProjectName     string `json:"projectName"`
	ProjectUID      string `json:"projectUID"`
	EnvironmentName string `json:"environmentName"`
	EnvironmentUID  string `json:"environmentUID"`
	DataPlaneName   string `json:"dataPlaneName"`
	DataPlaneUID    string `json:"dataPlaneUID"`

	// Labels are platform-injected standard labels (openchoreo.dev/resource,
	// openchoreo.dev/project, openchoreo.dev/environment, plus matching
	// *-uid keys). PE templates that propagate ${metadata.labels} get
	// consistent labeling across every rendered DP-side object.
	Labels map[string]string `json:"labels"`

	// Annotations are propagated from the Resource CR.
	Annotations map[string]string `json:"annotations"`
}

// DataPlaneContext is the dataplane surface exposed to CEL templates as
// ${dataplane.*}. The surface is deliberately narrow — only the fields
// managed-infra ResourceTypes commonly need; gateway and networking surface
// is intentionally omitted. Optional fields use omitempty so templates can
// guard with has(...) — mirrors the component pipeline's contract.
type DataPlaneContext struct {
	// SecretStore is the name of the ESO ClusterSecretStore configured on
	// the DataPlane. ResourceTypes emitting ExternalSecret reference this.
	SecretStore string `json:"secretStore,omitempty"`

	// ObservabilityPlaneRef is the observability plane reference for
	// ResourceTypes that emit observability-plane-side resources (alert
	// rules, dashboards). Optional; nil when the DataPlane has no
	// observability plane configured. PE templates that reference
	// ${dataplane.observabilityPlaneRef.*} must guard with
	// has(dataplane.observabilityPlaneRef) — same convention as
	// dataplane.gateway in the component pipeline.
	ObservabilityPlaneRef *ObservabilityPlaneRefContext `json:"observabilityPlaneRef,omitempty"`
}

// ObservabilityPlaneRefContext is the {kind, name} reference exposed under
// ${dataplane.observabilityPlaneRef.*}.
type ObservabilityPlaneRefContext struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// BaseContext is the top-level CEL surface produced by buildBaseContext and
// fed into the template engine. withApplied layers applied.<id>.status.* on
// top of it for output and readyWhen evaluation. The struct exists so the
// pipeline and the validation package share a single source of truth for the
// CEL surface — validation reflects on BaseContext to declare CEL variables;
// the pipeline JSON-marshals it via structToMap for runtime evaluation.
type BaseContext struct {
	Metadata           MetadataContext  `json:"metadata"`
	Parameters         map[string]any   `json:"parameters"`
	EnvironmentConfigs map[string]any   `json:"environmentConfigs"`
	DataPlane          DataPlaneContext `json:"dataplane"`
}
