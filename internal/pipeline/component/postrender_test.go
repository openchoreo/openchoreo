// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/renderer"
	"github.com/openchoreo/openchoreo/internal/template"
)

func boolPtr(b bool) *bool { return &b }

func deployment(name string, replicas int) renderer.RenderedResource {
	return renderer.RenderedResource{
		TargetPlane: v1alpha1.TargetPlaneDataPlane,
		Resource: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]any{"name": name},
			"spec":       map[string]any{"replicas": replicas},
		},
	}
}

func replicaValidation(rule, msg string, mustMatch *bool, when string) v1alpha1.PostRenderValidation {
	return v1alpha1.PostRenderValidation{
		When: when,
		Target: v1alpha1.PostRenderTarget{
			PatchTarget: v1alpha1.PatchTarget{Group: "apps", Version: "v1", Kind: "Deployment"},
			MustMatch:   mustMatch,
		},
		Rule:    rule,
		Message: msg,
	}
}

func runPostRender(t *testing.T, resources []renderer.RenderedResource, v v1alpha1.PostRenderValidation, ctx map[string]any) error {
	t.Helper()
	engine := template.NewEngine()
	if ctx == nil {
		ctx = map[string]any{}
	}
	pending := []pendingPostRender{{
		label:       "acme/inst",
		context:     ctx,
		validations: []v1alpha1.PostRenderValidation{v},
	}}
	return evaluatePostRenderValidations(engine, resources, pending)
}

func TestPostRender_RulePasses(t *testing.T) {
	err := runPostRender(t, []renderer.RenderedResource{deployment("web", 1)},
		replicaValidation("${resource.spec.replicas == 1}", "must be 1", nil, ""), nil)
	if err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
}

func TestPostRender_RuleFails(t *testing.T) {
	err := runPostRender(t, []renderer.RenderedResource{deployment("web", 3)},
		replicaValidation("${resource.spec.replicas == 1}", "must be single replica", nil, ""), nil)
	if err == nil || !strings.Contains(err.Error(), "must be single replica") {
		t.Fatalf("expected failure mentioning message, got %v", err)
	}
}

func TestPostRender_MustMatchZeroFails(t *testing.T) {
	// No Deployment in the resource set; default mustMatch=true must fail.
	svc := renderer.RenderedResource{Resource: map[string]any{
		"apiVersion": "v1", "kind": "Service", "metadata": map[string]any{"name": "svc"},
	}}
	err := runPostRender(t, []renderer.RenderedResource{svc},
		replicaValidation("${resource.spec.replicas == 1}", "must be 1", nil, ""), nil)
	if err == nil || !strings.Contains(err.Error(), "no resource matched target") {
		t.Fatalf("expected mustMatch failure, got %v", err)
	}
}

func TestPostRender_MustMatchFalseZeroPasses(t *testing.T) {
	svc := renderer.RenderedResource{Resource: map[string]any{
		"apiVersion": "v1", "kind": "Service", "metadata": map[string]any{"name": "svc"},
	}}
	err := runPostRender(t, []renderer.RenderedResource{svc},
		replicaValidation("${resource.spec.replicas == 1}", "must be 1", boolPtr(false), ""), nil)
	if err != nil {
		t.Fatalf("expected pass when mustMatch=false and no match, got %v", err)
	}
}

func TestPostRender_WhenGatesOut(t *testing.T) {
	ctx := map[string]any{"parameters": map[string]any{"mode": "read"}}
	err := runPostRender(t, []renderer.RenderedResource{deployment("web", 3)},
		replicaValidation("${resource.spec.replicas == 1}", "must be 1", nil, "${parameters.mode == 'write'}"), ctx)
	if err != nil {
		t.Fatalf("expected skip when when=false, got %v", err)
	}
}

func TestPostRender_WhenGatesIn(t *testing.T) {
	ctx := map[string]any{"parameters": map[string]any{"mode": "write"}}
	err := runPostRender(t, []renderer.RenderedResource{deployment("web", 3)},
		replicaValidation("${resource.spec.replicas == 1}", "write mode needs one replica", nil, "${parameters.mode == 'write'}"), ctx)
	if err == nil || !strings.Contains(err.Error(), "write mode needs one replica") {
		t.Fatalf("expected failure when when=true, got %v", err)
	}
}

func TestPostRender_WhereFiltersSelection(t *testing.T) {
	// Two deployments; where selects only "primary", which has replicas=3 → fail.
	resources := []renderer.RenderedResource{deployment("primary", 3), deployment("sidecar", 1)}
	v := replicaValidation("${resource.spec.replicas == 1}", "primary must be single", nil, "")
	v.Target.Where = "${resource.metadata.name == 'primary'}"
	err := runPostRender(t, resources, v, nil)
	if err == nil || !strings.Contains(err.Error(), "primary must be single") {
		t.Fatalf("expected failure on primary, got %v", err)
	}
}

func TestPostRender_TargetPlaneScopesSelection(t *testing.T) {
	// A dataplane Deployment (replicas=1, passes) alongside an observability-plane
	// Deployment of the same GVK (replicas=3, would fail). targetPlane=dataplane must
	// scope the rule to the dataplane resource only.
	obs := renderer.RenderedResource{
		TargetPlane: v1alpha1.TargetPlaneObservabilityPlane,
		Resource: map[string]any{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]any{"name": "obs"},
			"spec":     map[string]any{"replicas": 3},
		},
	}
	resources := []renderer.RenderedResource{deployment("web", 1), obs}

	// Empty targetPlane resolves to dataplane (TargetPlaneOrDefault), so only the dataplane
	// Deployment (replicas=1) is selected → passes; the obs Deployment is out of scope.
	def := replicaValidation("${resource.spec.replicas == 1}", "must be single replica", nil, "")
	if err := runPostRender(t, resources, def, nil); err != nil {
		t.Fatalf("expected default targetPlane (dataplane) to scope out the obs Deployment and pass, got %v", err)
	}

	// Explicit observabilityplane scopes to the obs Deployment (replicas=3) → fails the rule.
	obsScoped := replicaValidation("${resource.spec.replicas == 1}", "must be single replica", nil, "")
	obsScoped.TargetPlane = v1alpha1.TargetPlaneObservabilityPlane
	if err := runPostRender(t, resources, obsScoped, nil); err == nil {
		t.Fatalf("expected observabilityplane-scoped rule to match the obs Deployment (replicas=3) and fail")
	}
}

func TestPostRender_NonBoolRuleErrors(t *testing.T) {
	err := runPostRender(t, []renderer.RenderedResource{deployment("web", 1)},
		replicaValidation("${resource.spec.replicas}", "not a bool", nil, ""), nil)
	if err == nil || !strings.Contains(err.Error(), "boolean") {
		t.Fatalf("expected non-bool error, got %v", err)
	}
}

func TestPostRender_AggregatesAcrossTraits(t *testing.T) {
	engine := template.NewEngine()
	resources := []renderer.RenderedResource{deployment("web", 3)}
	pending := []pendingPostRender{
		{label: "Trait a/a", context: map[string]any{}, validations: []v1alpha1.PostRenderValidation{
			replicaValidation("${resource.spec.replicas == 1}", "A failed", nil, "")}},
		{label: "Trait b/b", context: map[string]any{}, validations: []v1alpha1.PostRenderValidation{
			replicaValidation("${resource.spec.replicas < 2}", "B failed", nil, "")}},
	}
	err := evaluatePostRenderValidations(engine, resources, pending)
	if err == nil || !strings.Contains(err.Error(), "A failed") || !strings.Contains(err.Error(), "B failed") {
		t.Fatalf("expected both failures aggregated, got %v", err)
	}
}

func TestPostRender_AggregatesAcrossMatchedResources(t *testing.T) {
	// One validation, two matching Deployments both violating → both reported (no short-circuit).
	resources := []renderer.RenderedResource{deployment("web-a", 3), deployment("web-b", 5)}
	err := runPostRender(t, resources,
		replicaValidation("${resource.spec.replicas == 1}", "needs one replica", nil, ""), nil)
	if err == nil {
		t.Fatalf("expected failure, got nil")
	}
	if !strings.Contains(err.Error(), "web-a") || !strings.Contains(err.Error(), "web-b") {
		t.Fatalf("expected both resources named in aggregated error, got %v", err)
	}
}

func TestPostRender_NoResourceBindingLeak(t *testing.T) {
	// The caller's context must not retain a `resource` binding after evaluation,
	// so a subsequent validation (or trait) never sees a leaked resource.
	ctx := map[string]any{"parameters": map[string]any{"mode": "read"}}
	_ = runPostRender(t, []renderer.RenderedResource{deployment("web", 1)},
		replicaValidation("${resource.spec.replicas == 1}", "ok", nil, ""), ctx)
	if _, leaked := ctx["resource"]; leaked {
		t.Fatalf("expected no `resource` key left in caller context, but it leaked")
	}
}

func TestPostRender_ForEach_PerItemDistinctResource(t *testing.T) {
	// Two HTTPRoutes; parameters.routes declares three. The third ("gone") has no
	// resource → its iteration must fail mustMatch, naming the missing selection.
	httproute := func(name string) renderer.RenderedResource {
		return renderer.RenderedResource{Resource: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1", "kind": "HTTPRoute",
			"metadata": map[string]any{"name": name},
			"spec":     map[string]any{"rules": []any{map[string]any{}}},
		}}
	}
	resources := []renderer.RenderedResource{httproute("a"), httproute("b")}
	ctx := map[string]any{"parameters": map[string]any{
		"routes": []any{
			map[string]any{"name": "a"}, map[string]any{"name": "b"}, map[string]any{"name": "gone"},
		},
	}}
	v := v1alpha1.PostRenderValidation{
		ForEach: "${parameters.routes}",
		Var:     "route",
		Target: v1alpha1.PostRenderTarget{
			PatchTarget: v1alpha1.PatchTarget{
				Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRoute",
				Where: "${resource.metadata.name == route.name}",
			},
		},
		Rule:    "${resource.spec.rules.size() > 0}",
		Message: "route ${route.name} lost its rules",
	}
	engine := template.NewEngine()
	err := evaluatePostRenderValidations(engine, resources,
		[]pendingPostRender{{label: "Trait r/r", context: ctx, validations: []v1alpha1.PostRenderValidation{v}}})
	if err == nil || !strings.Contains(err.Error(), "no resource matched target") {
		t.Fatalf("expected per-item mustMatch failure for the missing route, got %v", err)
	}
	// The error must identify WHICH forEach item's resource is missing.
	if !strings.Contains(err.Error(), "forEach route=") || !strings.Contains(err.Error(), "gone") {
		t.Fatalf("expected mustMatch error to name the missing forEach item (route=gone), got %v", err)
	}
}

func TestPostRender_WhenEvaluationErrorFails(t *testing.T) {
	// A `when` guard that evaluates to a non-boolean must surface as a hard error, not be
	// silently coerced. Exercises the runtime guard around ShouldInclude (when is evaluated
	// against the trait context, before target selection).
	ctx := map[string]any{"parameters": map[string]any{"count": 5}}
	v := replicaValidation("${resource.spec.replicas == 1}", "unused", nil, "${parameters.count}")
	err := runPostRender(t, []renderer.RenderedResource{deployment("web", 1)}, v, ctx)
	if err == nil || !strings.Contains(err.Error(), "when evaluation error") {
		t.Fatalf("expected when evaluation error, got %v", err)
	}
}

func TestPostRender_WhereNonBoolErrors(t *testing.T) {
	// A `where` filter that returns a non-boolean must error rather than silently include or
	// exclude. GVK selection happens first (the helper Deployment matches), then the where filter.
	v := replicaValidation("${resource.spec.replicas == 1}", "unused", nil, "")
	v.Target.Where = "${resource.spec.replicas}" // int, not bool
	err := runPostRender(t, []renderer.RenderedResource{deployment("web", 1)}, v, nil)
	if err == nil || !strings.Contains(err.Error(), "where clause") || !strings.Contains(err.Error(), "must evaluate to boolean") {
		t.Fatalf("expected non-bool where error, got %v", err)
	}
}

func TestPostRender_ForEachNonIterableErrors(t *testing.T) {
	// A `forEach` that evaluates to a scalar (not a list/map) must error via ToIterableItems.
	ctx := map[string]any{"parameters": map[string]any{"scalar": "notalist"}}
	v := v1alpha1.PostRenderValidation{
		ForEach: "${parameters.scalar}",
		Var:     "x",
		Target: v1alpha1.PostRenderTarget{
			PatchTarget: v1alpha1.PatchTarget{Group: "apps", Version: "v1", Kind: "Deployment"},
		},
		Rule:    "${resource.spec.replicas == 1}",
		Message: "unused",
	}
	err := runPostRender(t, []renderer.RenderedResource{deployment("web", 1)}, v, ctx)
	if err == nil || !strings.Contains(err.Error(), "invalid forEach result") {
		t.Fatalf("expected non-iterable forEach error, got %v", err)
	}
}

func TestPostRender_ForEachDefaultsVarToItem(t *testing.T) {
	// When forEach is set but var is empty, the loop variable defaults to "item" at runtime
	// (a safety net; the CRD requires var). A rule referencing `item` must resolve — if the
	// default were removed, `item` would be unbound and the rule would error instead of passing.
	ctx := map[string]any{"parameters": map[string]any{"nums": []any{1}}}
	v := v1alpha1.PostRenderValidation{
		ForEach: "${parameters.nums}",
		Var:     "", // empty → defaults to "item"
		Target: v1alpha1.PostRenderTarget{
			PatchTarget: v1alpha1.PatchTarget{Group: "apps", Version: "v1", Kind: "Deployment"},
		},
		Rule:    "${resource.spec.replicas == item}",
		Message: "replicas must equal item",
	}
	if err := runPostRender(t, []renderer.RenderedResource{deployment("web", 1)}, v, ctx); err != nil {
		t.Fatalf("expected empty-var forEach to bind `item` and pass, got %v", err)
	}
}
