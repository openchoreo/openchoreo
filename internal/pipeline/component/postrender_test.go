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
