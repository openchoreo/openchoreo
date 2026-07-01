// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package projectpipeline renders the inlined (Cluster)ProjectType.spec.resources
// templates for a single ProjectReleaseBinding. It depends on
// internal/template (the shared CEL engine), internal/schema (OpenAPI v3
// helpers), and the exported control-flow helpers from
// internal/pipeline/component/renderer (ShouldInclude, EvalForEach,
// EvaluateValidationRules); it does not import controller-runtime.
//
// The pipeline exposes one method:
//   - Render walks ProjectTypeSpec.Resources[] and returns the rendered
//     entries. ForEach templates contribute one entry per iteration with
//     ID suffixed by the iteration index. CEL context exposes ${metadata.*}
//     (including namespace), ${parameters.*}, ${environmentConfigs.*},
//     ${dataplane.*}, ${environment.*}, and the effective ${gateway.*}.
package projectpipeline

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/renderer"
	"github.com/openchoreo/openchoreo/internal/schema"
	"github.com/openchoreo/openchoreo/internal/template"
)

// NewPipeline returns a Pipeline backed by a fresh template.Engine. The
// engine's CEL env and program caches accumulate across calls; reuse the
// Pipeline instance to keep them warm.
func NewPipeline() *Pipeline {
	return &Pipeline{
		templateEngine: template.NewEngine(),
	}
}

// Render walks ProjectTypeSpec.Resources[] and returns one RenderedEntry per
// template that passes its IncludeWhen check. ForEach templates expand into
// one entry per iteration with the ID suffixed by index.
//
// The CEL context built once per call exposes:
//   - ${metadata.*} (namespace, projectName/UID, environmentName/UID, ...)
//   - ${parameters.*} (Project.spec.parameters with schema defaults applied)
//   - ${environmentConfigs.*} (binding overrides with schema defaults applied)
//   - ${dataplane.*} (secretStore, gateway, observabilityPlaneRef)
//   - ${environment.*} (env-or-dataplane merged gateway)
//   - ${gateway.*} (top-level alias of the effective environment gateway)
//
// ProjectTypeSpec.Validations are evaluated against the same context before
// rendering begins; any rule that returns false aborts the call.
func (p *Pipeline) Render(input *RenderInput) (*RenderOutput, error) {
	if err := validateInput(input); err != nil {
		return nil, err
	}

	ctx, err := buildBaseContext(input)
	if err != nil {
		return nil, err
	}

	spec := input.ProjectTypeSpec
	if err := renderer.EvaluateValidationRules(p.templateEngine, spec.Validations, ctx); err != nil {
		return nil, fmt.Errorf("project type validation failed: %w", err)
	}

	entries := make([]RenderedEntry, 0, len(spec.Resources))
	for i := range spec.Resources {
		tmpl := &spec.Resources[i]

		include, err := renderer.ShouldInclude(p.templateEngine, tmpl.IncludeWhen, ctx)
		if err != nil {
			return nil, fmt.Errorf("evaluate includeWhen for resource %q: %w", tmpl.ID, err)
		}
		if !include {
			continue
		}

		if tmpl.ForEach != "" {
			expanded, err := p.expandForEach(tmpl, ctx)
			if err != nil {
				return nil, err
			}
			entries = append(entries, expanded...)
			continue
		}

		object, err := p.renderSingleTemplate(tmpl, ctx)
		if err != nil {
			return nil, err
		}
		entries = append(entries, RenderedEntry{ID: tmpl.ID, Object: object})
	}

	return &RenderOutput{Entries: entries}, nil
}

// expandForEach iterates ResourceTemplate.ForEach and renders the template
// once per item. The returned entries are tagged with "<id>-<index>" IDs so
// the binding controller can correlate observed status to a specific
// iteration without inspecting the rendered object.
func (p *Pipeline) expandForEach(tmpl *v1alpha1.ResourceTemplate, ctx map[string]any) ([]RenderedEntry, error) {
	itemContexts, err := renderer.EvalForEach(p.templateEngine, tmpl.ForEach, tmpl.Var, ctx)
	if err != nil {
		return nil, fmt.Errorf("evaluate forEach for resource %q: %w", tmpl.ID, err)
	}

	entries := make([]RenderedEntry, 0, len(itemContexts))
	for idx, itemCtx := range itemContexts {
		object, err := p.renderSingleTemplate(tmpl, itemCtx)
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
// expressions against ctx, strips omit-sentinel keys, and asserts the
// minimum {apiVersion, kind, metadata.name} surface.
func (p *Pipeline) renderSingleTemplate(tmpl *v1alpha1.ResourceTemplate, ctx map[string]any) (map[string]any, error) {
	if tmpl.Template == nil || len(tmpl.Template.Raw) == 0 {
		return nil, fmt.Errorf("template is empty for resource %q", tmpl.ID)
	}

	var data any
	if err := json.Unmarshal(tmpl.Template.Raw, &data); err != nil {
		return nil, fmt.Errorf("unmarshal template for resource %q: %w", tmpl.ID, err)
	}

	rendered, err := p.templateEngine.Render(data, ctx)
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
// rendered entry must satisfy. Mirrors the resource pipeline's validation:
// apiVersion, kind, and metadata.name must be non-empty.
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
	if input.ProjectTypeSpec == nil {
		return fmt.Errorf("input.ProjectTypeSpec is nil")
	}
	if input.Metadata.Namespace == "" {
		return fmt.Errorf("input.Metadata.Namespace is empty")
	}
	return nil
}

// buildBaseContext returns the CEL context fed into the template engine.
// Parameters and EnvironmentConfigs are extracted from their respective raw
// RawExtensions and overlaid with OpenAPI v3 schema defaults declared on
// ProjectTypeSpec.Parameters and ProjectTypeSpec.EnvironmentConfigs.
func buildBaseContext(input *RenderInput) (map[string]any, error) {
	spec := input.ProjectTypeSpec

	parameters, err := extractAndDefault(input.ProjectParameters, spec.Parameters)
	if err != nil {
		return nil, fmt.Errorf("resolve parameters: %w", err)
	}
	envConfigs, err := extractAndDefault(input.EnvironmentConfigs, spec.EnvironmentConfigs)
	if err != nil {
		return nil, fmt.Errorf("resolve environmentConfigs: %w", err)
	}

	// Coerce nil Labels/Annotations to empty maps so the JSON round-trip in
	// structToMap emits {} instead of null, keeping CEL map indexing
	// (${metadata.labels["k"]}) safe even when callers leave them unset.
	md := input.Metadata
	if md.Labels == nil {
		md.Labels = map[string]string{}
	}
	if md.Annotations == nil {
		md.Annotations = map[string]string{}
	}

	return structToMap(BaseContext{
		Metadata:           md,
		Parameters:         parameters,
		EnvironmentConfigs: envConfigs,
		DataPlane:          input.DataPlane,
		Environment:        input.Environment,
		Gateway:            input.Environment.Gateway,
	})
}

// extractAndDefault unmarshals raw into a map and overlays schema defaults.
// Both inputs are optional: nil raw yields an empty target before defaults;
// nil section yields the unmodified target.
func extractAndDefault(raw *runtime.RawExtension, section *v1alpha1.SchemaSection) (map[string]any, error) {
	target, err := unmarshalRaw(raw)
	if err != nil {
		return nil, err
	}
	return applySchemaDefaults(target, section)
}

// unmarshalRaw decodes a RawExtension into a map. Empty input yields an
// empty map.
func unmarshalRaw(raw *runtime.RawExtension) (map[string]any, error) {
	if raw == nil || len(raw.Raw) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw.Raw, &out); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

// applySchemaDefaults overlays schema defaults onto target.
func applySchemaDefaults(target map[string]any, section *v1alpha1.SchemaSection) (map[string]any, error) {
	if target == nil {
		target = map[string]any{}
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
// via JSON round-trip. CEL expressions access maps and primitives, not
// arbitrary Go structs, so this round-trip is the conversion mechanism.
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
