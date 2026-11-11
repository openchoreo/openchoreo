// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package postrendervalidator

import (
	"testing"

	"github.com/openchoreo/openchoreo/internal/pipeline/component/renderer"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine"
)

func rendered(resource map[string]any) renderer.RenderedResource {
	return renderer.RenderedResource{Resource: resource}
}

func deploymentWith(container map[string]any, podExtras map[string]any) renderer.RenderedResource {
	podSpec := map[string]any{
		"containers": []any{container},
	}
	for k, v := range podExtras {
		podSpec[k] = v
	}
	return rendered(map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": "web"},
		"spec": map[string]any{
			"template": map[string]any{
				"spec": podSpec,
			},
		},
	})
}

// codes returns the diagnostic codes in a result, for compact assertions.
func codes(diags ruleengine.Diagnostics) []string {
	out := make([]string, 0, len(diags))
	for _, d := range diags {
		out = append(out, d.Code)
	}
	return out
}

func hasCode(diags ruleengine.Diagnostics, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

func TestImageTagRule(t *testing.T) {
	cases := []struct {
		name      string
		image     string
		wantAlert bool
	}{
		{"pinned", "nginx:1.25", false},
		{"digest", "nginx@sha256:abc", false},
		{"latest", "nginx:latest", false},
		{"empty-tag", "nginx:", true},
		{"registry-latest", "registry:5000/nginx:latest", false},
		{"untagged", "nginx", true},
		{"port-not-a-tag", "registry:5000/nginx", true},
		{"port-and-tag", "registry:5000/nginx:1.25", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := deploymentWith(map[string]any{"name": "app", "image": tc.image}, nil)
			diags := imageTagRule{}.Evaluate([]renderer.RenderedResource{res})
			if got := hasCode(diags, "image-latest"); got != tc.wantAlert {
				t.Fatalf("image %q: alert = %v (%v), want %v", tc.image, got, codes(diags), tc.wantAlert)
			}
		})
	}
}

func TestResourceQuantitiesRules(t *testing.T) {
	withResources := func(res map[string]any) renderer.RenderedResource {
		return deploymentWith(map[string]any{"name": "app", "image": "nginx:1.25", "resources": res}, nil)
	}

	t.Run("complete", func(t *testing.T) {
		res := withResources(map[string]any{
			"limits":   map[string]any{"cpu": "500m", "memory": "256Mi"},
			"requests": map[string]any{"cpu": "100m", "memory": "128Mi"},
		})
		if diags := (resourceLimitsRule{}).Evaluate([]renderer.RenderedResource{res}); len(diags) != 0 {
			t.Fatalf("limits: unexpected %v", codes(diags))
		}
		if diags := (resourceRequestsRule{}).Evaluate([]renderer.RenderedResource{res}); len(diags) != 0 {
			t.Fatalf("requests: unexpected %v", codes(diags))
		}
	})

	t.Run("missing-limits", func(t *testing.T) {
		res := withResources(map[string]any{
			"requests": map[string]any{"cpu": "100m", "memory": "128Mi"},
		})
		if diags := (resourceLimitsRule{}).Evaluate([]renderer.RenderedResource{res}); !hasCode(diags, "limits-required") {
			t.Fatalf("expected limits-required, got %v", codes(diags))
		}
	})

	t.Run("missing-memory-request", func(t *testing.T) {
		res := withResources(map[string]any{
			"requests": map[string]any{"cpu": "100m"},
		})
		diags := resourceRequestsRule{}.Evaluate([]renderer.RenderedResource{res})
		if len(diags) != 1 || diags[0].Code != "requests-required" {
			t.Fatalf("expected one requests-required, got %v", codes(diags))
		}
	})
}

func TestReplicasRule(t *testing.T) {
	deployment := func(replicas any) renderer.RenderedResource {
		spec := map[string]any{}
		if replicas != nil {
			spec["replicas"] = replicas
		}
		return rendered(map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]any{"name": "web"},
			"spec":       spec,
		})
	}
	cases := []struct {
		name      string
		replicas  any
		wantAlert bool
	}{
		{"zero", 0, true},
		{"zero-float", float64(0), true},
		{"one", 1, false},
		{"three", 3, false},
		{"absent", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := replicasRule{}.Evaluate([]renderer.RenderedResource{deployment(tc.replicas)})
			if got := hasCode(diags, "replicas-positive"); got != tc.wantAlert {
				t.Fatalf("replicas %v: alert = %v (%v), want %v", tc.replicas, got, codes(diags), tc.wantAlert)
			}
		})
	}

	t.Run("non-replicated-kind", func(t *testing.T) {
		job := rendered(map[string]any{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata":   map[string]any{"name": "task"},
			"spec":       map[string]any{"replicas": 0},
		})
		if diags := (replicasRule{}).Evaluate([]renderer.RenderedResource{job}); len(diags) != 0 {
			t.Fatalf("Job should not be checked, got %v", codes(diags))
		}
	})
}

func TestHostNamespaceRule(t *testing.T) {
	res := deploymentWith(
		map[string]any{"name": "app", "image": "nginx:1.25"},
		map[string]any{"hostNetwork": true},
	)
	diags := hostNamespaceRule{}.Evaluate([]renderer.RenderedResource{res})
	if !hasCode(diags, "no-host-namespaces") {
		t.Fatalf("expected no-host-namespaces, got %v", codes(diags))
	}
}

func TestPrivilegedRule(t *testing.T) {
	res := deploymentWith(map[string]any{
		"name":            "app",
		"image":           "nginx:1.25",
		"securityContext": map[string]any{"privileged": true},
	}, nil)
	diags := privilegedRule{}.Evaluate([]renderer.RenderedResource{res})
	if !hasCode(diags, "no-privileged") {
		t.Fatalf("expected no-privileged, got %v", codes(diags))
	}
}

func TestUnresolvedExprRule(t *testing.T) {
	res := deploymentWith(map[string]any{"name": "app", "image": "${workload.container.image}"}, nil)
	diags := unresolvedExprRule{}.Evaluate([]renderer.RenderedResource{res})
	if !hasCode(diags, "post-render-unresolved-expression") {
		t.Fatalf("expected unresolved-expression, got %v", codes(diags))
	}
	if len(diags) != 1 {
		t.Fatalf("expected one diagnostic, got %v", codes(diags))
	}
}

func TestMissingFieldsRule(t *testing.T) {
	res := rendered(map[string]any{"kind": "Service", "metadata": map[string]any{"name": "svc"}})
	diags := missingFieldsRule{}.Evaluate([]renderer.RenderedResource{res})
	if !hasCode(diags, "post-render-missing-field") {
		t.Fatalf("expected missing-field for absent apiVersion, got %v", codes(diags))
	}
}

// TestPolicyRulesIgnoreNonWorkloads pins that the container rules do not fire
// for resources that carry no pod spec, such as a Service or ConfigMap.
func TestPolicyRulesIgnoreNonWorkloads(t *testing.T) {
	svc := rendered(map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": "svc"},
		"spec":       map[string]any{"ports": []any{}},
	})
	for _, r := range defaultRules() {
		if diags := r.Evaluate([]renderer.RenderedResource{svc}); len(diags) != 0 {
			t.Errorf("rule %s fired on a Service: %v", r.Name(), codes(diags))
		}
	}
}
