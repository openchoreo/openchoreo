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
// calling the pipeline.
type MetadataContext struct {
	// Name is the platform-computed base name for rendered resources,
	// shaped {resource}-{env}-{hash} (mirrors component pipeline's
	// {component}-{env}-{hash} convention).
	Name string

	// Namespace is the DP-side project-env mapped namespace. Exposed via
	// CEL only; the pipeline does NOT force-set metadata.namespace on
	// rendered manifests.
	Namespace string

	// ResourceNamespace is the CP namespace where the Resource CR lives.
	ResourceNamespace string

	ResourceName    string
	ResourceUID     string
	ProjectName     string
	ProjectUID      string
	EnvironmentName string
	EnvironmentUID  string
	DataPlaneName   string
	DataPlaneUID    string

	// Labels are platform-injected standard labels (openchoreo.dev/resource,
	// openchoreo.dev/project, openchoreo.dev/environment, plus matching
	// *-uid keys). PE templates that propagate ${metadata.labels} get
	// consistent labeling across every rendered DP-side object.
	Labels map[string]string

	// Annotations are propagated from the Resource CR.
	Annotations map[string]string
}

// DataPlaneContext is the dataplane surface exposed to CEL templates as
// ${dataplane.*}. The surface is deliberately narrow — only the fields
// managed-infra ResourceTypes commonly need; gateway and networking surface
// is intentionally omitted.
type DataPlaneContext struct {
	// SecretStore is the name of the ESO ClusterSecretStore configured on
	// the DataPlane. ResourceTypes emitting ExternalSecret reference this.
	SecretStore string

	// ObservabilityPlaneRef is the observability plane reference for
	// ResourceTypes that emit observability-plane-side resources (alert
	// rules, dashboards). Optional; nil when the DataPlane has no
	// observability plane configured.
	ObservabilityPlaneRef *ObservabilityPlaneRefContext
}

// ObservabilityPlaneRefContext is the {kind, name} reference exposed under
// ${dataplane.observabilityPlaneRef.*}.
type ObservabilityPlaneRefContext struct {
	Kind string
	Name string
}
