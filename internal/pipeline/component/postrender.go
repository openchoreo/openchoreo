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
		varName = valueItem
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

// PostRenderFailure is the structured form of one declared post-render validation
// failure. RenderForLint captures these instead of discarding them the way Render's
// joined error does, so a caller such as the linter can turn each failure into a
// diagnostic without parsing error text. The fields mirror the parts of the joined
// error message that identify where the failure came from and what it said.
type PostRenderFailure struct {
	// Label identifies the declaring source, e.g. "Trait post/p1" or
	// "ComponentType web". It matches the label embedded in the joined error.
	Label string

	// Kind is the target kind the validation selected, e.g. "Deployment".
	Kind string

	// Resource names the matched resource the failure is about, as "Kind/name".
	// It is empty when the failure is not tied to a specific resource: a mustMatch
	// failure that matched nothing, or a when/forEach/where evaluation error.
	Resource string

	// Message is the failure message: the declared message of a rule failure, or the
	// error description of an evaluation failure. Declared messages are truncated the
	// same way the joined error truncates them.
	Message string

	// Iteration describes the failing forEach iteration, e.g. "forEach route=...".
	// It is empty when the validation has no forEach or the failure is not per-item.
	Iteration string
}

// lintPostRenderCollector collects structured post-render failures and their joined
// errors for the lint path. It mirrors the bounded-collection behavior of
// postRenderErrors but additionally records each failure's structured form, so a
// linter can report individual diagnostics without parsing error text.
type lintPostRenderCollector struct {
	errs     []error
	failures []PostRenderFailure
	capped   bool
}

// add records a failure - as joined-error text and as a structured PostRenderFailure -
// and reports whether the sweep should stop: either the cap is now full, or the render
// was aborted and every remaining validation would fail the same way. A nil error
// records nothing and never stops the sweep.
func (c *lintPostRenderCollector) add(err error, failure PostRenderFailure) (stop bool) {
	if err == nil {
		return false
	}
	c.errs = append(c.errs, err)
	c.failures = append(c.failures, failure)
	if len(c.errs) >= maxPostRenderErrors {
		c.capped = true
	}
	return c.capped || template.IsRenderAborted(err)
}

func (c *lintPostRenderCollector) err() error {
	errs := c.errs
	if c.capped {
		errs = append(append([]error(nil), errs...),
			fmt.Errorf("validation stopped after %d failures", maxPostRenderErrors))
	}
	return errors.Join(errs...)
}

// evaluatePostRenderValidationsForLint runs pending validations against the fully
// rendered resource set and returns each failure's structured PostRenderFailure form
// alongside the same joined error that evaluatePostRenderValidations would produce.
// It calls the same existing helpers (filterByWhere, resourceIdentity, iterationDescriber,
// trait.FindTargetResources, renderer.ShouldInclude, engine.Render, renderer.ToIterableItems)
// that the production evaluation uses, so the selection and rule-evaluation semantics are
// identical; the only difference is that structured failures are captured for the caller.
func evaluatePostRenderValidationsForLint(
	ctx context.Context,
	engine *template.Engine,
	resources []renderer.RenderedResource,
	pending []pendingPostRender,
) ([]PostRenderFailure, error) {
	collected := &lintPostRenderCollector{}
	for _, p := range pending {
		for i := range p.validations {
			if stop := evaluateOnePostRenderForLint(ctx, engine, resources, p, p.validations[i], collected); stop {
				return collected.failures, collected.err()
			}
		}
	}
	return collected.failures, collected.err()
}

// evaluateOnePostRenderForLint mirrors evaluateOnePostRender but feeds the
// lintPostRenderCollector so each failure is also recorded as a structured
// PostRenderFailure. It calls the same existing helpers for when-guard evaluation,
// forEach iteration, and selection.
func evaluateOnePostRenderForLint(
	ctx context.Context,
	engine *template.Engine,
	resources []renderer.RenderedResource,
	p pendingPostRender,
	v v1alpha1.PostRenderValidation,
	collected *lintPostRenderCollector,
) (stop bool) {
	if v.When != "" {
		include, err := renderer.ShouldInclude(ctx, engine, v.When, p.context)
		if err != nil {
			return collected.add(
				fmt.Errorf("%q post-render validation: when evaluation error: %w", p.label, err),
				PostRenderFailure{
					Label:   p.label,
					Kind:    v.Target.Kind,
					Message: fmt.Sprintf("when evaluation error: %v", err),
				})
		}
		if !include {
			return false
		}
	}

	if v.ForEach == "" {
		return evaluatePostRenderSelectionForLint(ctx, engine, resources, p.label, nil, p.context, v, collected)
	}

	itemsRaw, err := engine.Render(ctx, v.ForEach, p.context)
	if err != nil {
		return collected.add(
			fmt.Errorf("%q post-render validation: forEach evaluation error: %w", p.label, err),
			PostRenderFailure{
				Label:   p.label,
				Kind:    v.Target.Kind,
				Message: fmt.Sprintf("forEach evaluation error: %v", err),
			})
	}
	items, err := renderer.ToIterableItems(ctx, itemsRaw)
	if err != nil {
		return collected.add(
			fmt.Errorf("%q post-render validation: invalid forEach result: %w", p.label, err),
			PostRenderFailure{
				Label:   p.label,
				Kind:    v.Target.Kind,
				Message: fmt.Sprintf("invalid forEach result: %v", err),
			})
	}
	varName := v.Var
	if varName == "" {
		varName = valueItem
	}
	for _, item := range items {
		iterCtx := maps.Clone(p.context)
		iterCtx[varName] = item
		if stop := evaluatePostRenderSelectionForLint(ctx, engine, resources, p.label,
			iterationDescriber(varName, item), iterCtx, v, collected); stop {
			return true
		}
	}
	return false
}

// evaluatePostRenderSelectionForLint mirrors evaluatePostRenderSelection but records
// each failure as a structured PostRenderFailure in the lint collector. It calls the
// same existing helpers for target selection (trait.FindTargetResources), where
// filtering (filterByWhere), and resource identification (resourceIdentity).
func evaluatePostRenderSelectionForLint(
	ctx context.Context,
	engine *template.Engine,
	resources []renderer.RenderedResource,
	label string,
	iterDesc func() string,
	celContext map[string]any,
	v v1alpha1.PostRenderValidation,
	collected *lintPostRenderCollector,
) (stop bool) {
	suffix := func() string {
		if iterDesc == nil {
			return ""
		}
		return " (" + iterDesc() + ")"
	}

	iteration := ""
	if iterDesc != nil {
		iteration = iterDesc()
	}

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
			return collected.add(
				fmt.Errorf("%q post-render validation: %w", label, err),
				PostRenderFailure{
					Label:   label,
					Kind:    v.Target.Kind,
					Message: err.Error(),
				})
		}
		matched = filtered
	}

	if len(matched) == 0 {
		if v.Target.MustMatchOrDefault() {
			return collected.add(
				fmt.Errorf("%q post-render validation: no resource matched target %s/%s/%s%s",
					label, v.Target.Group, v.Target.Version, v.Target.Kind, suffix()),
				PostRenderFailure{
					Label:     label,
					Kind:      v.Target.Kind,
					Message:   fmt.Sprintf("no resource matched target %s/%s/%s", v.Target.Group, v.Target.Version, v.Target.Kind),
					Iteration: iteration,
				})
		}
		return false
	}

	for _, rr := range matched {
		rctx := maps.Clone(celContext)
		rctx["resource"] = rr.Resource
		result, err := engine.Render(ctx, v.Rule, rctx)
		if err != nil {
			if collected.add(
				fmt.Errorf("%q post-render rule evaluation error on %s: %w%s",
					label, resourceIdentity(rr), err, suffix()),
				PostRenderFailure{
					Label:     label,
					Kind:      v.Target.Kind,
					Resource:  resourceIdentity(rr),
					Message:   fmt.Sprintf("rule evaluation error: %v", err),
					Iteration: iteration,
				}) {
				return true
			}
			continue
		}
		boolResult, ok := result.(bool)
		if !ok {
			if collected.add(
				fmt.Errorf("%q post-render rule on %s must evaluate to boolean, got %T%s",
					label, resourceIdentity(rr), result, suffix()),
				PostRenderFailure{
					Label:     label,
					Kind:      v.Target.Kind,
					Resource:  resourceIdentity(rr),
					Message:   fmt.Sprintf("rule must evaluate to boolean, got %T", result),
					Iteration: iteration,
				}) {
				return true
			}
			continue
		}
		if !boolResult {
			if collected.add(
				fmt.Errorf("%q post-render validation on %s failed: %s%s",
					label, resourceIdentity(rr), template.TruncateFragment(v.Message), suffix()),
				PostRenderFailure{
					Label:     label,
					Kind:      v.Target.Kind,
					Resource:  resourceIdentity(rr),
					Message:   template.TruncateFragment(v.Message),
					Iteration: iteration,
				}) {
				return true
			}
		}
	}
	return false
}
