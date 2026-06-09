// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectpipeline

import (
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

// Pipeline renders the inlined (Cluster)ProjectType.spec.resources templates
// for a single ProjectReleaseBinding. The instance holds a template.Engine
// whose CEL environment and program caches are reused across calls; consumers
// (binding controller, future CLI) instantiate one Pipeline and reuse it.
type Pipeline struct {
	templateEngine *template.Engine
}

// RenderInput carries everything Render needs. The binding controller
// rehydrates ProjectTypeSpec and ProjectParameters from the immutable
// ProjectRelease snapshot and supplies EnvironmentConfigs from the binding.
type RenderInput struct {
	// ProjectTypeSpec is the inlined (Cluster)ProjectType spec snapshot from
	// the ProjectRelease (release.spec.projectType.spec). The pipeline reads
	// Validations, Parameters (schema), EnvironmentConfigs (schema), and
	// Resources from it. Required.
	ProjectTypeSpec *v1alpha1.ProjectTypeSpec

	// ProjectParameters is the snapshot of Project.spec.parameters from the
	// ProjectRelease (release.spec.parameters). Validated against
	// ProjectTypeSpec.Parameters and exposed to CEL as ${parameters.*}.
	// Optional: nil yields an empty parameter map before schema defaults are
	// applied.
	ProjectParameters *runtime.RawExtension

	// EnvironmentConfigs is the per-env override supplied via
	// ProjectReleaseBinding.spec.environmentConfigs. Validated against
	// ProjectTypeSpec.EnvironmentConfigs and exposed to CEL as
	// ${environmentConfigs.*}. Optional.
	EnvironmentConfigs *runtime.RawExtension

	// Metadata is the controller-computed metadata surface exposed to CEL as
	// ${metadata.*}. Must include CellNamespace; the mandated Namespace
	// template renders against it as metadata.name = ${metadata.cellNamespace}.
	Metadata MetadataContext
}

// RenderOutput carries the rendered manifests in spec order. ForEach
// templates contribute one entry per iteration, with ID suffixed by the
// iteration index.
type RenderOutput struct {
	// Entries are the rendered resources, one per
	// ProjectTypeSpec.Resources[] that passed its IncludeWhen check, plus
	// one extra entry per forEach iteration.
	Entries []RenderedEntry
}

// RenderedEntry is one rendered manifest. The controller marshals Object
// into runtime.RawExtension at its own boundary so the pipeline stays free
// of K8s runtime types in its public surface.
type RenderedEntry struct {
	// ID identifies the rendered entry. For non-forEach templates it equals
	// the source ResourceTemplate.ID verbatim; for forEach iterations it is
	// suffixed with "-<index>" so the binding controller can correlate
	// observed status back to a specific iteration.
	ID string

	// Object is the rendered Kubernetes resource as a map. CEL substitutions
	// have been evaluated and omit-sentinel keys removed.
	Object map[string]any
}

// MetadataContext is the platform-injected metadata surface exposed to CEL
// templates as ${metadata.*}. The controller computes every field before
// calling the pipeline. JSON tags determine the CEL-facing field names when
// the context is marshaled via structToMap.
type MetadataContext struct {
	// CellNamespace is the platform-computed dp-{ns}-{project}-{env}-{hash}
	// name. The mandated Namespace template references it as
	// metadata.name = ${metadata.cellNamespace}; other templates set
	// metadata.namespace from it.
	CellNamespace string `json:"cellNamespace"`

	// ProjectNamespace is the control-plane namespace where the
	// ProjectReleaseBinding lives.
	ProjectNamespace string `json:"projectNamespace"`

	ProjectName     string `json:"projectName"`
	ProjectUID      string `json:"projectUID"`
	EnvironmentName string `json:"environmentName"`
	EnvironmentUID  string `json:"environmentUID"`
	DataPlaneName   string `json:"dataPlaneName"`
	DataPlaneUID    string `json:"dataPlaneUID"`

	// Labels are platform-injected standard labels. PE templates that
	// propagate ${metadata.labels} get consistent labeling across every
	// rendered DP-side object.
	Labels map[string]string `json:"labels"`

	// Annotations are propagated from the ProjectReleaseBinding CR.
	Annotations map[string]string `json:"annotations"`
}

// BaseContext is the top-level CEL surface produced by buildBaseContext and
// fed into the template engine.
type BaseContext struct {
	Metadata           MetadataContext `json:"metadata"`
	Parameters         map[string]any  `json:"parameters"`
	EnvironmentConfigs map[string]any  `json:"environmentConfigs"`
}
