// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

// breachingRule costs about ten thousand units, so a pipeline configured with
// aggregatorCostLimit trips the per-expression cost limit on it while every other
// expression in these fixtures stays free (the templates are static).
const (
	breachingRule       = "${size(lists.range(9999)) > 0}"
	aggregatorCostLimit = 1_000
)

// staticComponentType emits one Deployment with no CEL in it, so the only cost these
// tests incur is the rule under test.
const staticComponentType = `
spec:
  resources:
    - id: deployment
      template: {apiVersion: apps/v1, kind: Deployment, metadata: {name: web}, spec: {replicas: 1}}
`

// renderWithCostLimit runs the pipeline over the given YAML fragments with a tight
// per-expression cost limit. It mirrors renderPipelineYAML but lets the test choose
// the limit.
func renderWithCostLimit(t *testing.T, limit uint64, componentTypeYAML, componentYAML, traitsYAML string) error {
	t.Helper()
	var componentType v1alpha1.ComponentType
	if err := yaml.Unmarshal([]byte(componentTypeYAML), &componentType); err != nil {
		t.Fatalf("failed to parse componentType: %v", err)
	}
	var component v1alpha1.Component
	if err := yaml.Unmarshal([]byte(componentYAML), &component); err != nil {
		t.Fatalf("failed to parse component: %v", err)
	}
	var traits []v1alpha1.Trait
	if traitsYAML != "" {
		if err := yaml.Unmarshal([]byte(traitsYAML), &traits); err != nil {
			t.Fatalf("failed to parse traits: %v", err)
		}
	}
	input := &RenderInput{
		ComponentType: &componentType,
		Component:     &component,
		Traits:        traits,
		Workload:      &v1alpha1.Workload{},
		Environment:   &v1alpha1.Environment{},
		DataPlane:     &v1alpha1.DataPlane{},
		Metadata:      postRenderTestMetadata(),
	}
	_, err := NewPipeline(WithCostLimit(limit)).Render(t.Context(), input)
	return err
}

// TestValidationAggregatorsPreserveErrorIdentity covers every place the pipeline
// aggregates rule failures into one error. Each aggregator used to collect strings, which
// erased the sentinel — a reconciler could then not tell a cost breach (terminal, retrying
// is pointless) from an ordinary rule failure. The errors.Is chain must survive the
// aggregation.
func TestValidationAggregatorsPreserveErrorIdentity(t *testing.T) {
	// A trait carrying one post-render validation, parameterised by which field holds
	// the breaching expression.
	traitWithPostRender := func(when, rule string) string {
		body := "" +
			"- metadata: {name: post}\n" +
			"  spec:\n" +
			"    postRenderValidations:\n" +
			"      - target: {group: apps, version: v1, kind: Deployment}\n"
		if when != "" {
			body += fmt.Sprintf("        when: %q\n", when)
		}
		body += fmt.Sprintf("        rule: %q\n", rule)
		body += "        message: \"unreachable\"\n"
		return body
	}
	componentWithTrait := `
spec:
  traits:
    - name: post
      instanceName: p1
`

	tests := []struct {
		name              string
		componentTypeYAML string
		componentYAML     string
		traitsYAML        string
	}{
		{
			name: "pre-render validation rule",
			componentTypeYAML: staticComponentType + fmt.Sprintf(`
  preRenderValidations:
    - rule: %q
      message: "unreachable"
`, breachingRule),
			componentYAML: `spec: {}`,
		},
		{
			name:              "post-render when guard",
			componentTypeYAML: staticComponentType,
			componentYAML:     componentWithTrait,
			traitsYAML:        traitWithPostRender(breachingRule, "${resource.spec.replicas == 1}"),
		},
		{
			name:              "post-render validation rule",
			componentTypeYAML: staticComponentType,
			componentYAML:     componentWithTrait,
			traitsYAML:        traitWithPostRender("", breachingRule),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := renderWithCostLimit(t, aggregatorCostLimit, tt.componentTypeYAML, tt.componentYAML, tt.traitsYAML)
			if err == nil {
				t.Fatal("expected the breaching rule to fail the render, got nil")
			}
			if !errors.Is(err, template.ErrCostLimitExceeded) {
				t.Errorf("cost-limit breach must survive the aggregator, got %v", err)
			}
			if !template.IsTerminalRenderError(err) {
				t.Errorf("a cost-limit breach must classify as terminal, got %v", err)
			}
		})
	}
}

// TestAggregatorsStillReportOrdinaryFailures guards the short-circuit added alongside the
// identity fix: a plain rule failure is not a render abort, so every rule must still be
// evaluated and every failure reported.
func TestAggregatorsStillReportOrdinaryFailures(t *testing.T) {
	componentTypeYAML := staticComponentType + `
  preRenderValidations:
    - rule: "${1 == 2}"
      message: "first rule failed"
    - rule: "${2 == 3}"
      message: "second rule failed"
`
	err := renderWithCostLimit(t, aggregatorCostLimit, componentTypeYAML, `spec: {}`, "")
	if err == nil {
		t.Fatal("expected validation failure, got nil")
	}
	for _, want := range []string{"first rule failed", "second rule failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected error %q to mention %q", err.Error(), want)
		}
	}
	if template.IsTerminalRenderError(err) {
		t.Error("an ordinary rule failure must not classify as a terminal render error")
	}
}

// TestRenderStopsOnCancelledContext confirms the pipeline threads the caller's context
// all the way to the engine rather than substituting a background one.
func TestRenderStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var componentType v1alpha1.ComponentType
	if err := yaml.Unmarshal([]byte(staticComponentType+`
  preRenderValidations:
    - rule: "${size(lists.range(5000).map(i, i + 1)) > 0}"
      message: "unreachable"
`), &componentType); err != nil {
		t.Fatalf("failed to parse componentType: %v", err)
	}
	input := &RenderInput{
		ComponentType: &componentType,
		Component:     &v1alpha1.Component{},
		Workload:      &v1alpha1.Workload{},
		Environment:   &v1alpha1.Environment{},
		DataPlane:     &v1alpha1.DataPlane{},
		Metadata:      postRenderTestMetadata(),
	}

	if _, err := NewPipeline().Render(ctx, input); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the cancelled context to reach the engine, got %v", err)
	}
}
