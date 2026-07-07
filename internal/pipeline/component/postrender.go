// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"
	"maps"
	"strings"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/renderer"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/trait"
	"github.com/openchoreo/openchoreo/internal/template"
)

// pendingPostRender carries a source's post-render validations together with the
// CEL context they evaluate against. The source is either a trait or the ComponentType
// itself. Collected during rendering and evaluated once, after every trait has been
// applied to the resource set.
type pendingPostRender struct {
	// label identifies the source for error messages, e.g. "Trait name/instanceName"
	// or "ComponentType name".
	label string
	// context is the source's CEL context map (parameters, environmentConfigs, etc.).
	context map[string]any
	// validations are the source's declared post-render validations.
	validations []v1alpha1.PostRenderValidation
}

// evaluatePostRenderValidations runs every pending source's post-render validations
// against the fully rendered resource set. All failures are collected (no
// short-circuit) and joined into a single error, matching EvaluateValidationRules.
func evaluatePostRenderValidations(
	engine *template.Engine,
	resources []renderer.RenderedResource,
	pending []pendingPostRender,
) error {
	var errs []string
	for _, p := range pending {
		for i := range p.validations {
			if err := evaluateOnePostRender(engine, resources, p, p.validations[i]); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// evaluateOnePostRender evaluates a single post-render validation. It evaluates the
// optional `when` guard once, then dispatches on `forEach`: without forEach it runs one
// selection against the trait context; with forEach it iterates the list, binding the
// loop variable into a cloned context per item and running one selection per iteration
// (mustMatch applies per item). All iteration failures are aggregated.
func evaluateOnePostRender(
	engine *template.Engine,
	resources []renderer.RenderedResource,
	p pendingPostRender,
	v v1alpha1.PostRenderValidation,
) error {
	if v.When != "" {
		include, err := renderer.ShouldInclude(engine, v.When, p.context)
		if err != nil {
			return fmt.Errorf("%q post-render validation: when evaluation error: %w", p.label, err)
		}
		if !include {
			return nil
		}
	}

	if v.ForEach == "" {
		return evaluatePostRenderSelection(engine, resources, p.label, "", p.context, v)
	}

	itemsRaw, err := engine.Render(v.ForEach, p.context)
	if err != nil {
		return fmt.Errorf("%q post-render validation: forEach evaluation error: %w", p.label, err)
	}
	items, err := renderer.ToIterableItems(itemsRaw)
	if err != nil {
		return fmt.Errorf("%q post-render validation: invalid forEach result: %w", p.label, err)
	}
	varName := v.Var
	if varName == "" {
		varName = "item"
	}
	var errs []string
	for _, item := range items {
		iterCtx := maps.Clone(p.context)
		iterCtx[varName] = item
		iterDesc := fmt.Sprintf("forEach %s=%v", varName, item)
		if err := evaluatePostRenderSelection(engine, resources, p.label, iterDesc, iterCtx, v); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// evaluatePostRenderSelection performs target selection (GVK + where), mustMatch, and
// rule evaluation for a single context (which may carry a forEach loop variable).
// iterDesc, when non-empty, describes the forEach iteration (e.g. "forEach route=...")
// and is appended to each error so callers can tell which loop item failed; it is empty
// for the no-forEach path, leaving those messages unchanged.
func evaluatePostRenderSelection(
	engine *template.Engine,
	resources []renderer.RenderedResource,
	label string,
	iterDesc string,
	ctx map[string]any,
	v v1alpha1.PostRenderValidation,
) error {
	// suffix annotates errors with the forEach iteration; empty for the no-forEach path.
	suffix := ""
	if iterDesc != "" {
		suffix = " (" + iterDesc + ")"
	}

	// FindTargetResources only matches on plane/GVK and ignores Where; the where
	// filter is applied separately below, so leave Where unset here to avoid implying
	// FindTargetResources honors it.
	target := trait.TargetSpec{
		Kind:        v.Target.Kind,
		Group:       v.Target.Group,
		Version:     v.Target.Version,
		TargetPlane: v.TargetPlaneOrDefault(),
	}
	matched := trait.FindTargetResources(resources, target)

	if v.Target.Where != "" {
		filtered, err := filterByWhere(engine, matched, v.Target.Where, ctx)
		if err != nil {
			return fmt.Errorf("%q post-render validation: %w", label, err)
		}
		matched = filtered
	}

	if len(matched) == 0 {
		if v.Target.MustMatchOrDefault() {
			return fmt.Errorf("%q post-render validation: no resource matched target %s/%s/%s%s",
				label, v.Target.Group, v.Target.Version, v.Target.Kind, suffix)
		}
		return nil
	}

	var errs []string
	for _, rr := range matched {
		rctx := maps.Clone(ctx)
		rctx["resource"] = rr.Resource
		result, err := engine.Render(v.Rule, rctx)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%q post-render rule evaluation error on %s: %v%s",
				label, resourceIdentity(rr), err, suffix))
			continue
		}
		boolResult, ok := result.(bool)
		if !ok {
			errs = append(errs, fmt.Sprintf("%q post-render rule on %s must evaluate to boolean, got %T%s",
				label, resourceIdentity(rr), result, suffix))
			continue
		}
		if !boolResult {
			errs = append(errs, fmt.Sprintf("%q post-render validation on %s failed: %s%s",
				label, resourceIdentity(rr), v.Message, suffix))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// filterByWhere returns the subset of resources for which the where CEL expression
// evaluates to true, with `resource` bound to each candidate. The expression must
// evaluate to a boolean.
func filterByWhere(
	engine *template.Engine,
	resources []renderer.RenderedResource,
	where string,
	baseContext map[string]any,
) ([]renderer.RenderedResource, error) {
	filtered := make([]renderer.RenderedResource, 0, len(resources))
	for _, rr := range resources {
		ctx := maps.Clone(baseContext)
		ctx["resource"] = rr.Resource
		result, err := engine.Render(where, ctx)
		if err != nil {
			return nil, fmt.Errorf("where clause %q evaluation error: %w", where, err)
		}
		boolResult, ok := result.(bool)
		if !ok {
			return nil, fmt.Errorf("where clause %q must evaluate to boolean, got %T", where, result)
		}
		if boolResult {
			filtered = append(filtered, rr)
		}
	}
	return filtered, nil
}

// resourceIdentity returns a "Kind/name" label for a rendered resource, for error messages.
func resourceIdentity(rr renderer.RenderedResource) string {
	kind, _ := rr.Resource["kind"].(string)
	name := ""
	if meta, ok := rr.Resource["metadata"].(map[string]any); ok {
		name, _ = meta["name"].(string)
	}
	if kind == "" && name == "" {
		return "unknown resource"
	}
	return fmt.Sprintf("%s/%s", kind, name)
}
