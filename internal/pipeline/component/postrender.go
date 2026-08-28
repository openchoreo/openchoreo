// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"errors"
	"fmt"
	"maps"

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

const maxPostRenderErrors = 20

// postRenderErrors collects validation failures for one render, bounded at
// maxPostRenderErrors. The count is tenant-driven - validations x forEach items x matching
// resources - so an unbounded collection would build an arbitrarily large error long before
// the condition writer's own truncation could see it.
type postRenderErrors struct {
	errs   []error
	capped bool
}

// add records a failure and reports whether the sweep should stop: either the cap is now
// full, or the render was aborted and every remaining validation would fail the same way.
// A nil error records nothing and never stops the sweep.
func (c *postRenderErrors) add(err error) (stop bool) {
	if err == nil {
		return false
	}
	c.errs = append(c.errs, err)
	if len(c.errs) >= maxPostRenderErrors {
		c.capped = true
	}
	return c.capped || template.IsRenderAborted(err)
}

func (c *postRenderErrors) err() error {
	errs := c.errs
	if c.capped {
		errs = append(append([]error(nil), errs...),
			fmt.Errorf("validation stopped after %d failures", maxPostRenderErrors))
	}
	return errors.Join(errs...)
}

// evaluatePostRenderValidations runs pending validations against the fully rendered resource
// set. It returns the first maxPostRenderErrors failures as one joined error; collecting
// beyond that would let a forEach/resource cross product allocate an enormous error string.
// An aborted render also stops immediately. Evaluation failures stay wrapped so callers can
// classify them with errors.Is.
func evaluatePostRenderValidations(
	ctx context.Context,
	engine *template.Engine,
	resources []renderer.RenderedResource,
	pending []pendingPostRender,
) error {
	collected := &postRenderErrors{}
	for _, p := range pending {
		for i := range p.validations {
			if stop := evaluateOnePostRender(ctx, engine, resources, p, p.validations[i], collected); stop {
				return collected.err()
			}
		}
	}
	return collected.err()
}

// evaluateOnePostRender evaluates a single post-render validation. It evaluates the
// optional `when` guard once, then dispatches on `forEach`: without forEach it runs one
// selection against the trait context; with forEach it iterates the list, binding the
// loop variable into a cloned context per item and running one selection per iteration
// (mustMatch applies per item). Failures feed the render-wide bounded collector, and the
// returned bool reports whether that collector wants the whole sweep stopped.
func evaluateOnePostRender(
	ctx context.Context,
	engine *template.Engine,
	resources []renderer.RenderedResource,
	p pendingPostRender,
	v v1alpha1.PostRenderValidation,
	collected *postRenderErrors,
) (stop bool) {
	if v.When != "" {
		include, err := renderer.ShouldInclude(ctx, engine, v.When, p.context)
		if err != nil {
			return collected.add(fmt.Errorf("%q post-render validation: when evaluation error: %w", p.label, err))
		}
		if !include {
			return false
		}
	}

	if v.ForEach == "" {
		return evaluatePostRenderSelection(ctx, engine, resources, p.label, nil, p.context, v, collected)
	}

	itemsRaw, err := engine.Render(ctx, v.ForEach, p.context)
	if err != nil {
		return collected.add(fmt.Errorf("%q post-render validation: forEach evaluation error: %w", p.label, err))
	}
	items, err := renderer.ToIterableItems(ctx, itemsRaw)
	if err != nil {
		return collected.add(fmt.Errorf("%q post-render validation: invalid forEach result: %w", p.label, err))
	}
	varName := v.Var
	if varName == "" {
		varName = "item"
	}
	for _, item := range items {
		iterCtx := maps.Clone(p.context)
		iterCtx[varName] = item
		if stop := evaluatePostRenderSelection(ctx, engine, resources, p.label,
			iterationDescriber(varName, item), iterCtx, v, collected); stop {
			return true
		}
	}
	return false
}

// iterationDescriber returns the description of one forEach iteration, deferred. The loop
// that calls it runs for every item on every render, but only a failing item is ever
// described, so the item is formatted when the description is asked for rather than when it
// is created. FormatFragment then bounds the item as it walks it, so even the failing case
// never serializes a whole rendered value to show the 256 bytes of it that survive.
func iterationDescriber(varName string, item any) func() string {
	return func() string {
		return fmt.Sprintf("forEach %s=%s", varName, template.FormatFragment(item))
	}
}

// evaluatePostRenderSelection performs target selection (GVK + where), mustMatch, and
// rule evaluation for a single context (which may carry a forEach loop variable).
// iterDesc, when non-nil, describes the forEach iteration (e.g. "forEach route=...") and is
// appended to each error so callers can tell which loop item failed; it is nil for the
// no-forEach path, leaving those messages unchanged. It is a function because describing an
// iteration means formatting a rendered item, and only a failing iteration is ever described.
func evaluatePostRenderSelection(
	ctx context.Context,
	engine *template.Engine,
	resources []renderer.RenderedResource,
	label string,
	iterDesc func() string,
	celContext map[string]any,
	v v1alpha1.PostRenderValidation,
	collected *postRenderErrors,
) (stop bool) {
	// suffix annotates errors with the forEach iteration; empty for the no-forEach path.
	suffix := func() string {
		if iterDesc == nil {
			return ""
		}
		return " (" + iterDesc() + ")"
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
		filtered, err := filterByWhere(ctx, engine, matched, v.Target.Where, celContext)
		if err != nil {
			return collected.add(fmt.Errorf("%q post-render validation: %w", label, err))
		}
		matched = filtered
	}

	if len(matched) == 0 {
		if v.Target.MustMatchOrDefault() {
			return collected.add(fmt.Errorf("%q post-render validation: no resource matched target %s/%s/%s%s",
				label, v.Target.Group, v.Target.Version, v.Target.Kind, suffix()))
		}
		return false
	}

	for _, rr := range matched {
		rctx := maps.Clone(celContext)
		rctx["resource"] = rr.Resource
		result, err := engine.Render(ctx, v.Rule, rctx)
		if err != nil {
			if collected.add(fmt.Errorf("%q post-render rule evaluation error on %s: %w%s",
				label, resourceIdentity(rr), err, suffix())) {
				return true
			}
			continue
		}
		boolResult, ok := result.(bool)
		if !ok {
			if collected.add(fmt.Errorf("%q post-render rule on %s must evaluate to boolean, got %T%s",
				label, resourceIdentity(rr), result, suffix())) {
				return true
			}
			continue
		}
		if !boolResult {
			if collected.add(fmt.Errorf("%q post-render validation on %s failed: %s%s",
				label, resourceIdentity(rr), template.TruncateFragment(v.Message), suffix())) {
				return true
			}
		}
	}
	return false
}

// filterByWhere returns the subset of resources for which the where CEL expression
// evaluates to true, with `resource` bound to each candidate. The expression must
// evaluate to a boolean.
func filterByWhere(
	ctx context.Context,
	engine *template.Engine,
	resources []renderer.RenderedResource,
	where string,
	baseContext map[string]any,
) ([]renderer.RenderedResource, error) {
	filtered := make([]renderer.RenderedResource, 0, len(resources))
	for _, rr := range resources {
		celContext := maps.Clone(baseContext)
		celContext["resource"] = rr.Resource
		result, err := engine.Render(ctx, where, celContext)
		if err != nil {
			return nil, fmt.Errorf("where clause %q evaluation error: %w", template.TruncateFragment(where), err)
		}
		boolResult, ok := result.(bool)
		if !ok {
			return nil, fmt.Errorf("where clause %q must evaluate to boolean, got %T", template.TruncateFragment(where), result)
		}
		if boolResult {
			filtered = append(filtered, rr)
		}
	}
	return filtered, nil
}

// resourceIdentity returns a "Kind/name" label for a rendered resource, for error messages.
//
// Both halves are tenant-authored and neither has been validated yet - post-render
// validation runs before validateResources, so a template is free to render a metadata.name
// of any size - and the collector may join up to maxPostRenderErrors of these into one
// error. They are bounded here, at the point they enter the message, rather than left for
// the condition backstop to cut afterwards.
func resourceIdentity(rr renderer.RenderedResource) string {
	kind, _ := rr.Resource["kind"].(string)
	name := ""
	if meta, ok := rr.Resource["metadata"].(map[string]any); ok {
		name, _ = meta["name"].(string)
	}
	if kind == "" && name == "" {
		return "unknown resource"
	}
	return fmt.Sprintf("%s/%s", template.TruncateFragment(kind), template.TruncateFragment(name))
}
