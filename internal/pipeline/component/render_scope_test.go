// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"errors"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	pipelinecontext "github.com/openchoreo/openchoreo/internal/pipeline/component/context"
	"github.com/openchoreo/openchoreo/internal/template"
)

// breachingRule measures 150,009 units on this branch, so a pipeline configured with
// aggregatorCostLimit trips the per-expression cost limit on it while every other
// expression in these fixtures stays free (the templates are static).
//
// It folds over the range rather than sizing it because cel-go v0.22.1, which this branch
// pins, ships no runtime tracker for lists.range; the estimator in internal/template now
// supplies one, but the comprehension is what this fixture is really about.
const (
	breachingRule       = "${size(lists.range(9999).map(i, i + 1)) > 0}"
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

func renderScopeTestMetadata() pipelinecontext.MetadataContext {
	return pipelinecontext.MetadataContext{
		Name:               "test",
		Namespace:          "ns",
		ComponentName:      "app",
		ComponentUID:       "uid1",
		ComponentNamespace: "ns",
		ProjectName:        "proj",
		ProjectUID:         "uid2",
		DataPlaneName:      "dp",
		DataPlaneUID:       "uid3",
		EnvironmentName:    "dev",
		EnvironmentUID:     "uid4",
		Labels:             map[string]string{},
		Annotations:        map[string]string{},
		PodSelectors:       map[string]string{"k": "v"},
	}
}

func renderScopeInput(t *testing.T, componentTypeYAML, traitsYAML string) *RenderInput {
	t.Helper()
	var componentType v1alpha1.ComponentType
	if err := yaml.Unmarshal([]byte(componentTypeYAML), &componentType); err != nil {
		t.Fatalf("failed to parse componentType: %v", err)
	}
	var traits []v1alpha1.Trait
	if traitsYAML != "" {
		if err := yaml.Unmarshal([]byte(traitsYAML), &traits); err != nil {
			t.Fatalf("failed to parse traits: %v", err)
		}
	}
	return &RenderInput{
		ComponentType: &componentType,
		Component:     &v1alpha1.Component{},
		Traits:        traits,
		Workload:      &v1alpha1.Workload{},
		Environment:   &v1alpha1.Environment{},
		DataPlane:     &v1alpha1.DataPlane{},
		Metadata:      renderScopeTestMetadata(),
	}
}

// TestValidationAggregatorPreservesErrorIdentity covers the place the pipeline aggregates
// rule failures into one error. ComponentType and trait validations both funnel through
// renderer.EvaluateValidationRules, so one case covers the aggregator. The aggregator used to collect strings, which erased the
// sentinel - a reconciler could then not tell a cost breach (terminal, retrying is
// pointless) from an ordinary rule failure. The errors.Is chain must survive aggregation.
func TestValidationAggregatorPreservesErrorIdentity(t *testing.T) {
	cases := []struct {
		name              string
		componentTypeYAML string
		traitsYAML        string
	}{
		{
			name: "componentType validations",
			componentTypeYAML: staticComponentType + `
  validations:
    - rule: "` + breachingRule + `"
      message: "unreachable"
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := renderScopeInput(t, tc.componentTypeYAML, tc.traitsYAML)
			_, err := NewPipeline(WithCostLimit(aggregatorCostLimit)).Render(t.Context(), input)
			if err == nil {
				t.Fatal("expected the breaching rule to fail, got nil")
			}
			if !errors.Is(err, template.ErrCostLimitExceeded) {
				t.Fatalf("aggregation erased the sentinel: %v", err)
			}
			if !template.IsTerminalRenderError(err) {
				t.Errorf("a cost breach must classify as a terminal render error: %v", err)
			}
		})
	}
}

// TestRenderStopsOnCancelledContext confirms the pipeline threads the caller's context
// all the way to the engine rather than substituting a background one.
func TestRenderStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	input := renderScopeInput(t, staticComponentType+`
  validations:
    - rule: "${size(lists.range(5000).map(i, i + 1)) > 0}"
      message: "unreachable"
`, "")

	if _, err := NewPipeline().Render(ctx, input); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the cancelled context to reach the engine, got %v", err)
	}
}
