// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package endpointpipeline renders the inlined (Cluster)EndpointType.spec.resources
// templates for a single EndpointReleaseBinding. It is a sibling of
// internal/pipeline/project rather than a mode of internal/pipeline/component:
// an Endpoint's render scope spans every backend it references, which may
// belong to different components, so it cannot be scoped to one component
// binding. It reuses the project pipeline's dataplane and gateway CEL surface,
// depends on internal/template (the shared CEL engine), internal/schema
// (OpenAPI v3 helpers), and the control-flow helpers from
// internal/pipeline/component/renderer; it does not import controller-runtime.
//
// The CEL context exposes ${metadata.*}, ${parameters.*},
// ${environmentConfigs.*}, ${visibility}, ${backends}, ${dataplane.*},
// ${environment.*}, and the effective ${gateway.*}.
package endpointpipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/renderer"
	"github.com/openchoreo/openchoreo/internal/schema"
	"github.com/openchoreo/openchoreo/internal/template"
)

// Option is a function that configures a Pipeline.
type Option func(*Pipeline)

// WithCostLimit sets the maximum accumulated cost for a single CEL expression
// evaluated by this pipeline's template engine. Zero selects the engine's
// built-in safe default; it never means unlimited.
func WithCostLimit(limit uint64) Option {
	return func(p *Pipeline) {
		p.celCostLimit = limit
	}
}

// WithRenderTimeout bounds the wall-clock duration of each call into this
// pipeline. The deadline is derived inside every entry point rather than by the
// caller, so it covers exactly one rendering step. Zero or negative means no
// deadline; the CEL cost limits remain the primary bound.
func WithRenderTimeout(timeout time.Duration) Option {
	return func(p *Pipeline) {
		p.renderTimeout = timeout
	}
}

// NewPipeline returns a Pipeline backed by a fresh template.Engine. Reuse the
// Pipeline instance to keep the engine's CEL env and program caches warm.
func NewPipeline(opts ...Option) *Pipeline {
	p := &Pipeline{}
	for _, opt := range opts {
		opt(p)
	}
	if p.templateEngine == nil {
		p.templateEngine = template.NewEngineWithOptions(
			template.WithCostLimit(p.celCostLimit),
		)
	}
	return p
}

// Render walks EndpointTypeSpec.Resources[] and returns one RenderedEntry per
// template that passes its IncludeWhen check. ForEach templates expand into one
// entry per iteration with the ID suffixed by index.
//
// EndpointTypeSpec.Validations are evaluated against the same context before
// rendering begins; any rule that returns false aborts the call.
func (p *Pipeline) Render(ctx context.Context, input *RenderInput) (*RenderOutput, error) {
	ctx, cancel := template.WithRenderTimeout(ctx, p.renderTimeout)
	defer cancel()

	ctx = renderer.WithForEachBudget(ctx)

	if err := validateInput(input); err != nil {
		return nil, err
	}

	celContext, err := buildBaseContext(input)
	if err != nil {
		return nil, err
	}

	spec := input.EndpointTypeSpec
	if err := renderer.EvaluateValidationRules(ctx, p.templateEngine, spec.Validations, celContext); err != nil {
		return nil, fmt.Errorf("endpoint type validation failed: %w", err)
	}

	entries := make([]RenderedEntry, 0, len(spec.Resources))
	for i := range spec.Resources {
		tmpl := &spec.Resources[i]

		// The binding emits a single data-plane RenderedRelease, so a template
		// aimed at another plane has nowhere to go.
		if tmpl.TargetPlane != "" && tmpl.TargetPlane != v1alpha1.TargetPlaneDataPlane {
			return nil, fmt.Errorf("resource %q: targetPlane %q is not supported by endpoint types; only %q is",
				tmpl.ID, tmpl.TargetPlane, v1alpha1.TargetPlaneDataPlane)
		}

		include, err := renderer.ShouldInclude(ctx, p.templateEngine, tmpl.IncludeWhen, celContext)
		if err != nil {
			return nil, fmt.Errorf("evaluate includeWhen for resource %q: %w", tmpl.ID, err)
		}
		if !include {
			continue
		}

		if tmpl.ForEach != "" {
			expanded, err := p.expandForEach(ctx, tmpl, celContext)
			if err != nil {
				return nil, err
			}
			entries = append(entries, expanded...)
			continue
		}

		object, err := p.renderSingleTemplate(ctx, tmpl, celContext)
		if err != nil {
			return nil, err
		}
		entries = append(entries, RenderedEntry{ID: tmpl.ID, Object: object})
	}

	return &RenderOutput{Entries: entries}, nil
}

// expandForEach iterates ResourceTemplate.ForEach and renders the template once
// per item, tagging each entry "<id>-<index>".
func (p *Pipeline) expandForEach(
	ctx context.Context,
	tmpl *v1alpha1.ResourceTemplate,
	celContext map[string]any,
) ([]RenderedEntry, error) {
	itemContexts, err := renderer.EvalForEach(ctx, p.templateEngine, tmpl.ForEach, tmpl.Var, celContext)
	if err != nil {
		return nil, fmt.Errorf("evaluate forEach for resource %q: %w", tmpl.ID, err)
	}

	entries := make([]RenderedEntry, 0, len(itemContexts))
	for idx, itemCtx := range itemContexts {
		object, err := p.renderSingleTemplate(ctx, tmpl, itemCtx)
		if err != nil {
			return nil, err
		}
		entries = append(entries, RenderedEntry{
			ID:     fmt.Sprintf("%s-%d", tmpl.ID, idx),
			Object: object,
		})
	}
	return entries, nil
}

// renderSingleTemplate JSON-decodes a ResourceTemplate body, evaluates CEL
// expressions, strips omit-sentinel keys, and asserts the minimum
// {apiVersion, kind, metadata.name} surface.
func (p *Pipeline) renderSingleTemplate(
	ctx context.Context,
	tmpl *v1alpha1.ResourceTemplate,
	celContext map[string]any,
) (map[string]any, error) {
	if tmpl.Template == nil || len(tmpl.Template.Raw) == 0 {
		return nil, fmt.Errorf("template is empty for resource %q", tmpl.ID)
	}

	var data any
	if err := json.Unmarshal(tmpl.Template.Raw, &data); err != nil {
		return nil, fmt.Errorf("unmarshal template for resource %q: %w", tmpl.ID, err)
	}

	rendered, err := p.templateEngine.Render(ctx, data, celContext)
	if err != nil {
		return nil, fmt.Errorf("render template for resource %q: %w", tmpl.ID, err)
	}

	cleaned := template.RemoveOmittedFields(rendered)
	object, ok := cleaned.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("rendered template for resource %q is not a map (got %T)", tmpl.ID, cleaned)
	}

	if err := validateRenderedManifest(object, tmpl.ID); err != nil {
		return nil, err
	}
	return object, nil
}

// validateRenderedManifest asserts the basic Kubernetes object shape every
// rendered entry must satisfy.
func validateRenderedManifest(resource map[string]any, id string) error {
	if apiVersion, _ := resource["apiVersion"].(string); apiVersion == "" {
		return fmt.Errorf("resource %q missing apiVersion", id)
	}
	if kind, _ := resource["kind"].(string); kind == "" {
		return fmt.Errorf("resource %q missing kind", id)
	}
	metadata, ok := resource["metadata"].(map[string]any)
	if !ok {
		return fmt.Errorf("resource %q missing metadata", id)
	}
	if name, _ := metadata["name"].(string); name == "" {
		return fmt.Errorf("resource %q missing metadata.name", id)
	}
	return nil
}

// validateInput checks the minimum invariants Render needs.
func validateInput(input *RenderInput) error {
	if input == nil {
		return fmt.Errorf("input is nil")
	}
	if input.EndpointTypeSpec == nil {
		return fmt.Errorf("input.EndpointTypeSpec is nil")
	}
	if input.Metadata.Namespace == "" {
		return fmt.Errorf("input.Metadata.Namespace is empty")
	}
	if len(input.Backends) == 0 {
		return fmt.Errorf("input.Backends is empty")
	}
	return nil
}

// buildBaseContext returns the CEL context fed into the template engine.
// Parameters and EnvironmentConfigs are overlaid with the schema defaults the
// EndpointTypeSpec declares for them.
func buildBaseContext(input *RenderInput) (map[string]any, error) {
	spec := input.EndpointTypeSpec

	parameters, err := extractAndDefault(input.Parameters, spec.Parameters)
	if err != nil {
		return nil, fmt.Errorf("resolve parameters: %w", err)
	}
	envConfigs, err := extractAndDefault(input.EnvironmentConfigs, spec.EnvironmentConfigs)
	if err != nil {
		return nil, fmt.Errorf("resolve environmentConfigs: %w", err)
	}

	// Coerce nil maps and lists to empty ones so the JSON round-trip emits {} or
	// [] instead of null, keeping CEL indexing and `in` safe when callers leave
	// them unset.
	md := input.Metadata
	if md.Labels == nil {
		md.Labels = map[string]string{}
	}
	if md.Annotations == nil {
		md.Annotations = map[string]string{}
	}
	visibility := make([]string, 0, len(input.Visibility))
	for _, v := range input.Visibility {
		visibility = append(visibility, string(v))
	}

	return structToMap(BaseContext{
		Metadata:           md,
		Parameters:         parameters,
		EnvironmentConfigs: envConfigs,
		Visibility:         visibility,
		Backends:           input.Backends,
		DataPlane:          input.DataPlane,
		Environment:        input.Environment,
		Gateway:            input.Environment.Gateway,
	})
}

// extractAndDefault unmarshals raw into a map and overlays schema defaults.
// Both inputs are optional: nil raw yields an empty target before defaults;
// nil section yields the unmodified target.
func extractAndDefault(raw *runtime.RawExtension, section *v1alpha1.SchemaSection) (map[string]any, error) {
	target := map[string]any{}
	if raw != nil && len(raw.Raw) > 0 {
		if err := json.Unmarshal(raw.Raw, &target); err != nil {
			return nil, fmt.Errorf("unmarshal: %w", err)
		}
		if target == nil {
			target = map[string]any{}
		}
	}

	structural, err := schema.ResolveSectionToStructural(section)
	if err != nil {
		return nil, err
	}
	if structural == nil {
		return target, nil
	}
	return schema.ApplyDefaults(target, structural), nil
}

// structToMap converts typed Go structs to map[string]any for CEL evaluation
// via JSON round-trip.
func structToMap(v any) (map[string]any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}
