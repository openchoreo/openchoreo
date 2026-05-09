// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package resourcepipeline renders ResourceType templates and resolves
// ResourceType outputs for a single ResourceReleaseBinding. It depends on
// internal/template (the shared CEL engine) and internal/schema (OpenAPI v3
// helpers); it does not import controller-runtime.
//
// Two CEL contexts are used: a base context (metadata, parameters,
// environmentConfigs, dataplane) for manifest rendering, and the same base
// extended with applied.<id> for output resolution and readyWhen checks.
// The base context is computed once per call by buildBaseContext;
// withApplied layers applied.<id> on top.
//
// The pipeline exposes three methods:
//   - RenderManifests walks ResourceTypeSpec.Resources[] and returns the
//     rendered entries. Runs against the base context only — applied.<id>
//     is not yet available because the rendered objects haven't been
//     applied to the data plane.
//   - ResolveOutputs walks ResourceTypeSpec.Outputs[] and evaluates each
//     output's CEL against the base context plus the observed applied
//     status, which the controller passes in.
//   - EvaluateReadyWhen evaluates a per-entry readyWhen CEL expression
//     against the same context as ResolveOutputs.
package resourcepipeline

import (
	"encoding/json"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
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

// RenderManifests walks ResourceTypeSpec.Resources[] and returns one
// RenderedEntry per template that passes its IncludeWhen check, in spec
// order. The output's ID matches the input ResourceTypeSpec.Resources[].ID
// verbatim so the binding controller can correlate the observed applied
// status back to the originating template entry when calling ResolveOutputs.
// CEL evaluation errors abort the call and return a nil RenderOutput.
func (p *Pipeline) RenderManifests(input *RenderInput) (*RenderOutput, error) {
	if err := validateInput(input); err != nil {
		return nil, err
	}

	spec := resourceTypeSpec(input)
	ctx, err := buildBaseContext(input)
	if err != nil {
		return nil, err
	}

	entries := make([]RenderedEntry, 0, len(spec.Resources))
	for i := range spec.Resources {
		entry := &spec.Resources[i]

		include, err := p.shouldInclude(entry.IncludeWhen, ctx)
		if err != nil {
			return nil, fmt.Errorf("evaluate includeWhen for resource %q: %w", entry.ID, err)
		}
		if !include {
			continue
		}

		obj, err := p.renderTemplate(entry.Template, ctx)
		if err != nil {
			return nil, fmt.Errorf("render resource %q: %w", entry.ID, err)
		}

		entries = append(entries, RenderedEntry{
			ID:     entry.ID,
			Object: obj,
		})
	}

	return &RenderOutput{Entries: entries}, nil
}

// shouldInclude evaluates an optional includeWhen expression. Empty
// expression means "always include". The expression is required to be
// ${...}-wrapped at the CRD level; here we just delegate to the template
// engine and assert the result is a bool.
func (p *Pipeline) shouldInclude(expr string, ctx map[string]any) (bool, error) {
	if expr == "" {
		return true, nil
	}
	result, err := p.templateEngine.Render(expr, ctx)
	if err != nil {
		return false, err
	}
	b, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("includeWhen must evaluate to bool, got %T", result)
	}
	return b, nil
}

// validateInput checks the minimum invariants every public method needs.
// ResourceType and Resource are load-bearing; metadata/dataplane validation
// is the controller's responsibility.
func validateInput(input *RenderInput) error {
	if input == nil {
		return fmt.Errorf("input is nil")
	}
	if input.ResourceType == nil {
		return fmt.Errorf("input.ResourceType is nil")
	}
	if input.Resource == nil {
		return fmt.Errorf("input.Resource is nil")
	}
	return nil
}

// resourceTypeSpec returns the ResourceTypeSpec from the input ResourceType
// for inline use. Callers must have validated input.
func resourceTypeSpec(input *RenderInput) *v1alpha1.ResourceTypeSpec {
	return &input.ResourceType.Spec
}

// ResolveOutputs evaluates ResourceTypeSpec.Outputs[] against the base CEL
// context plus applied.<id>, and returns one ResolvedOutput per entry.
// Per-output errors are collected and returned as a joined error;
// successfully-resolved outputs are still returned in the slice so the
// controller can write the partial result into status.outputs.
//
// The observed argument maps each ResourceType.spec.resources[].id to its
// .status content (the controller decodes this from the RawExtension at
// RenderedRelease.status.resources[<id>].status before calling).
func (p *Pipeline) ResolveOutputs(input *RenderInput, observed map[string]map[string]any) ([]ResolvedOutput, error) {
	if err := validateInput(input); err != nil {
		return nil, err
	}

	spec := resourceTypeSpec(input)
	base, err := buildBaseContext(input)
	if err != nil {
		return nil, err
	}
	ctx := withApplied(base, observed)

	resolved := make([]ResolvedOutput, 0, len(spec.Outputs))
	var errs []error
	for i := range spec.Outputs {
		out := &spec.Outputs[i]
		ro, err := p.resolveOutput(out, ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("output %q: %w", out.Name, err))
			continue
		}
		resolved = append(resolved, ro)
	}

	return resolved, errors.Join(errs...)
}

// resolveOutput dispatches on the source kind declared on a single
// ResourceTypeOutput and renders the relevant CEL expressions.
func (p *Pipeline) resolveOutput(out *v1alpha1.ResourceTypeOutput, ctx map[string]any) (ResolvedOutput, error) {
	res := ResolvedOutput{Name: out.Name}

	switch {
	case out.Value != "":
		v, err := p.renderStringValue(out.Value, ctx)
		if err != nil {
			return res, err
		}
		res.Value = v
	case out.SecretKeyRef != nil:
		ref, err := p.renderKeyRef(out.SecretKeyRef.Name, out.SecretKeyRef.Key, ctx)
		if err != nil {
			return res, err
		}
		res.SecretKeyRef = &v1alpha1.SecretKeyRef{Name: ref.name, Key: ref.key}
	case out.ConfigMapKeyRef != nil:
		ref, err := p.renderKeyRef(out.ConfigMapKeyRef.Name, out.ConfigMapKeyRef.Key, ctx)
		if err != nil {
			return res, err
		}
		res.ConfigMapKeyRef = &v1alpha1.ConfigMapKeyRef{Name: ref.name, Key: ref.key}
	default:
		return res, fmt.Errorf("no source kind set (value, secretKeyRef, or configMapKeyRef)")
	}
	return res, nil
}

type renderedKeyRef struct {
	name string
	key  string
}

// renderKeyRef evaluates the {name, key} pair shared by SecretKeyRef and
// ConfigMapKeyRef outputs.
func (p *Pipeline) renderKeyRef(nameExpr, keyExpr string, ctx map[string]any) (renderedKeyRef, error) {
	name, err := p.renderStringValue(nameExpr, ctx)
	if err != nil {
		return renderedKeyRef{}, fmt.Errorf("name: %w", err)
	}
	key, err := p.renderStringValue(keyExpr, ctx)
	if err != nil {
		return renderedKeyRef{}, fmt.Errorf("key: %w", err)
	}
	return renderedKeyRef{name: name, key: key}, nil
}

// renderStringValue evaluates a CEL-templated string expression and asserts
// the result is a string. Used for output values, secret/configmap names
// and keys, all of which the API documents as string-typed.
func (p *Pipeline) renderStringValue(expr string, ctx map[string]any) (string, error) {
	result, err := p.templateEngine.Render(expr, ctx)
	if err != nil {
		return "", err
	}
	s, ok := result.(string)
	if !ok {
		return "", fmt.Errorf("expected string, got %T", result)
	}
	return s, nil
}

// EvaluateReadyWhen evaluates a per-entry readyWhen expression against the
// same context as ResolveOutputs (base + applied.<id>). An empty expression
// returns (true, nil); the controller then falls back to per-Kind health
// inference from RenderedRelease.status.resources[].healthStatus.
//
// readyWhen is a ${...}-wrapped CEL expression matching the IncludeWhen
// pattern; CRD validation enforces the wrapping at admission. The
// expression must evaluate to a boolean.
func (p *Pipeline) EvaluateReadyWhen(input *RenderInput, observed map[string]map[string]any, expr string) (bool, error) {
	if expr == "" {
		return true, nil
	}
	if err := validateInput(input); err != nil {
		return false, err
	}

	base, err := buildBaseContext(input)
	if err != nil {
		return false, err
	}
	ctx := withApplied(base, observed)

	result, err := p.templateEngine.Render(expr, ctx)
	if err != nil {
		return false, err
	}
	b, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("readyWhen must evaluate to bool, got %T", result)
	}
	return b, nil
}

// renderTemplate JSON-decodes a runtime.RawExtension template body into a
// map, evaluates CEL expressions in keys and values against ctx, and strips
// omit-sentinel keys.
func (p *Pipeline) renderTemplate(raw *runtime.RawExtension, ctx map[string]any) (map[string]any, error) {
	if raw == nil || len(raw.Raw) == 0 {
		return nil, fmt.Errorf("template is empty")
	}

	var data map[string]any
	if err := json.Unmarshal(raw.Raw, &data); err != nil {
		return nil, fmt.Errorf("unmarshal template: %w", err)
	}

	rendered, err := p.templateEngine.Render(data, ctx)
	if err != nil {
		return nil, err
	}

	cleaned := template.RemoveOmittedFields(rendered)

	out, ok := cleaned.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("rendered template is not a map (got %T)", cleaned)
	}

	return out, nil
}

// buildBaseContext returns the CEL context shared by manifest rendering and
// output resolution. ResolveOutputs and EvaluateReadyWhen call this and
// then layer applied.<id> on top via withApplied.
//
// Parameters are unmarshalled from Resource.Spec.Parameters.
// EnvironmentConfigs come from
// ResourceReleaseBinding.Spec.ResourceTypeEnvironmentConfigs when a binding
// is provided. Both are pruned to their respective OpenAPI v3 schemas with
// defaults applied before CEL evaluation (mirrors
// workflowpipeline.buildParameters at internal/pipeline/workflow/pipeline.go:251-279).
func buildBaseContext(input *RenderInput) (map[string]any, error) {
	spec := resourceTypeSpec(input)

	rawParams := input.Resource.Spec.Parameters
	rawEnvCfgs := bindingEnvironmentConfigs(input.ResourceReleaseBinding)

	parameters, err := extractAndDefault(rawParams, spec.Parameters)
	if err != nil {
		return nil, fmt.Errorf("resolve parameters: %w", err)
	}
	envConfigs, err := extractAndDefault(rawEnvCfgs, spec.EnvironmentConfigs)
	if err != nil {
		return nil, fmt.Errorf("resolve environmentConfigs: %w", err)
	}

	return map[string]any{
		"metadata":           metadataContextToMap(input.Metadata),
		"parameters":         parameters,
		"environmentConfigs": envConfigs,
		"dataplane":          dataPlaneContextToMap(input.DataPlane),
	}, nil
}

// bindingEnvironmentConfigs returns the raw environmentConfigs RawExtension
// from the binding, or nil when the binding (or the field) is unset.
// Webhook-style validation calls don't always have a binding; the binding
// controller always does.
func bindingEnvironmentConfigs(binding *v1alpha1.ResourceReleaseBinding) *runtime.RawExtension {
	if binding == nil {
		return nil
	}
	return binding.Spec.ResourceTypeEnvironmentConfigs
}

// extractAndDefault unmarshals raw into a map and overlays schema defaults.
// Both inputs are optional: nil raw yields an empty target before defaults;
// nil section yields the unmodified target. Mirrors
// workflowpipeline.extractParameters + buildParameters in spirit.
func extractAndDefault(raw *runtime.RawExtension, section *v1alpha1.SchemaSection) (map[string]any, error) {
	target, err := unmarshalRaw(raw)
	if err != nil {
		return nil, err
	}
	return applySchemaDefaults(target, section)
}

// unmarshalRaw decodes a RawExtension into a map. Empty input yields an
// empty map (absent values are valid; defaults are applied separately).
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

// withApplied returns a copy of base with applied.<id>.status populated from
// observed. The base map is not mutated. Output resolution and readyWhen
// evaluation share this layering: every entry in observed shows up under
// applied[id].status.* for CEL.
func withApplied(base map[string]any, observed map[string]map[string]any) map[string]any {
	ctx := make(map[string]any, len(base)+1)
	for k, v := range base {
		ctx[k] = v
	}
	applied := make(map[string]any, len(observed))
	for id, status := range observed {
		applied[id] = map[string]any{"status": status}
	}
	ctx["applied"] = applied
	return ctx
}

// applySchemaDefaults overlays schema defaults onto target. Returns target
// unchanged when the section is nil/empty (no schema to consult). Resolves
// the structural schema once per call; callers are expected to be in the
// hot path of a single render.
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

// dataPlaneContextToMap exposes DataPlaneContext fields under their CEL-facing
// keys. ObservabilityPlaneRef is exposed as an empty {kind, name} map when
// nil so PE templates referencing ${dataplane.observabilityPlaneRef.name}
// against a DataPlane without one get an empty string rather than a CEL
// evaluation error.
func dataPlaneContextToMap(d DataPlaneContext) map[string]any {
	obsRef := map[string]any{
		"kind": "",
		"name": "",
	}
	if d.ObservabilityPlaneRef != nil {
		obsRef["kind"] = d.ObservabilityPlaneRef.Kind
		obsRef["name"] = d.ObservabilityPlaneRef.Name
	}
	return map[string]any{
		"secretStore":           d.SecretStore,
		"observabilityPlaneRef": obsRef,
	}
}

// metadataContextToMap exposes MetadataContext fields under their CEL-facing
// keys. componentName / componentUID are deliberately absent (reserved for
// component-bound resources, not currently supported); a CEL reference to
// ${metadata.componentName} surfaces as an evaluation error.
func metadataContextToMap(m MetadataContext) map[string]any {
	labels := m.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	annotations := m.Annotations
	if annotations == nil {
		annotations = map[string]string{}
	}
	return map[string]any{
		"name":              m.Name,
		"namespace":         m.Namespace,
		"resourceNamespace": m.ResourceNamespace,
		"resourceName":      m.ResourceName,
		"resourceUID":       m.ResourceUID,
		"projectName":       m.ProjectName,
		"projectUID":        m.ProjectUID,
		"environmentName":   m.EnvironmentName,
		"environmentUID":    m.EnvironmentUID,
		"dataPlaneName":     m.DataPlaneName,
		"dataPlaneUID":      m.DataPlaneUID,
		"labels":            labels,
		"annotations":       annotations,
	}
}
