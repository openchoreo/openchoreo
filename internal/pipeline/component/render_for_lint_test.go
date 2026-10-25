// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	// deploymentIdentity is the resourceIdentity form of the Deployment rendered by the
	// shared test fixtures in this file.
	deploymentIdentity = "Deployment/web"
	// kindHTTPRoute complements the existing kindDeployment/kindStatefulSet constants for
	// the HTTPRoute fixture in this file.
	kindHTTPRoute = "HTTPRoute"
)

// newRenderForLintInput parses the given YAML fragments into a RenderInput the same way
// renderPipelineYAML does, but returns the input so a test can tweak it (e.g.
// Metadata.Labels) before calling RenderForLint.
func newRenderForLintInput(t *testing.T, componentTypeYAML, componentYAML, traitsYAML string) *RenderInput {
	t.Helper()
	var componentType v1alpha1.ComponentType
	if err := yaml.Unmarshal([]byte(componentTypeYAML), &componentType); err != nil {
		t.Fatalf("Failed to parse componentType: %v", err)
	}
	var component v1alpha1.Component
	if err := yaml.Unmarshal([]byte(componentYAML), &component); err != nil {
		t.Fatalf("Failed to parse component: %v", err)
	}
	var traits []v1alpha1.Trait
	if traitsYAML != "" {
		if err := yaml.Unmarshal([]byte(traitsYAML), &traits); err != nil {
			t.Fatalf("Failed to parse traits: %v", err)
		}
	}
	return &RenderInput{
		ComponentType: &componentType,
		Component:     &component,
		Traits:        traits,
		Workload:      &v1alpha1.Workload{},
		Environment:   &v1alpha1.Environment{},
		DataPlane:     &v1alpha1.DataPlane{},
		Metadata:      postRenderTestMetadata(),
	}
}

// renderForLintPipelineYAML is the RenderForLint counterpart of renderPipelineYAML.
func renderForLintPipelineYAML(t *testing.T, componentTypeYAML, componentYAML, traitsYAML string) (*RenderOutput, []PostRenderFailure, error) {
	t.Helper()
	return NewPipeline().RenderForLint(t.Context(), newRenderForLintInput(t, componentTypeYAML, componentYAML, traitsYAML))
}

// failingTraitPostValidationYAMLs returns the ComponentType, Component, and Trait YAML for
// a render whose component-level trait "post/p1" declares a failing post-render validation
// against a Deployment with the given replicas.
func failingTraitPostValidationYAMLs(replicas int) (string, string, string) {
	ct := fmt.Sprintf(`
spec:
  resources:
    - id: deployment
      template: {apiVersion: apps/v1, kind: Deployment, metadata: {name: web}, spec: {replicas: %d}}
`, replicas)
	comp := `
spec:
  traits:
    - {name: post, instanceName: p1}
`
	traits := `
- metadata: {name: post}
  spec:
    postRenderValidations:
      - target:
          group: apps
          version: v1
          kind: Deployment
        rule: "${resource.spec.replicas == 1}"
        message: "must be single replica"
`
	return ct, comp, traits
}

func TestRenderForLint_PassReturnsResourcesWithoutFailures(t *testing.T) {
	ct, comp, traits := failingTraitPostValidationYAMLs(1)
	out, failures, err := renderForLintPipelineYAML(t, ct, comp, traits)
	if err != nil {
		t.Fatalf("expected render to pass, got %v", err)
	}
	if out == nil || len(out.Resources) == 0 {
		t.Fatalf("expected rendered resources")
	}
	if len(failures) != 0 {
		t.Fatalf("expected no post-render failures, got %d: %+v", len(failures), failures)
	}
}

func TestRenderForLint_CapturesFailureAndStillReturnsResources(t *testing.T) {
	ct, comp, traits := failingTraitPostValidationYAMLs(3)
	out, failures, err := renderForLintPipelineYAML(t, ct, comp, traits)
	if err != nil {
		t.Fatalf("expected RenderForLint to report no error for a declared failure, got %v", err)
	}
	if out == nil || len(out.Resources) != 1 {
		t.Fatalf("expected the rendered resources alongside the failure, got %+v", out)
	}
	if len(failures) != 1 {
		t.Fatalf("expected exactly one failure, got %d: %+v", len(failures), failures)
	}
	f := failures[0]
	if f.Label != "Trait post/p1" {
		t.Errorf("Label = %q, want %q", f.Label, "Trait post/p1")
	}
	if f.Kind != kindDeployment {
		t.Errorf("Kind = %q, want %q", f.Kind, kindDeployment)
	}
	if f.Resource != deploymentIdentity {
		t.Errorf("Resource = %q, want %q", f.Resource, deploymentIdentity)
	}
	if f.Message != "must be single replica" {
		t.Errorf("Message = %q, want %q", f.Message, "must be single replica")
	}
	if f.Iteration != "" {
		t.Errorf("Iteration = %q, want empty for a non-forEach failure", f.Iteration)
	}
}

// TestRenderForLint_RenderAbortBehaviorUnchanged pins the Step 0 contract from both
// sides: Render must still abort with nil resources and the joined error text, while
// RenderForLint on an identical input continues, and the structured failure's Message is
// exactly the fragment Render's error carries.
func TestRenderForLint_RenderAbortBehaviorUnchanged(t *testing.T) {
	ct, comp, traits := failingTraitPostValidationYAMLs(3)

	renderOut, renderErr := NewPipeline().Render(t.Context(), newRenderForLintInput(t, ct, comp, traits))
	if renderErr == nil {
		t.Fatalf("expected Render to fail on the declared post-render failure")
	}
	if renderOut != nil {
		t.Fatalf("expected Render to return nil resources when it aborts, got %+v", renderOut)
	}
	if !strings.Contains(renderErr.Error(), "post-render validation") {
		t.Fatalf("expected Render error wrapped as post-render validation, got %v", renderErr)
	}

	lintOut, failures, lintErr := NewPipeline().RenderForLint(t.Context(), newRenderForLintInput(t, ct, comp, traits))
	if lintErr != nil {
		t.Fatalf("expected RenderForLint to continue past the declared failure, got %v", lintErr)
	}
	if lintOut == nil || len(lintOut.Resources) == 0 {
		t.Fatalf("expected RenderForLint to return resources")
	}
	if len(failures) != 1 {
		t.Fatalf("expected one failure, got %d", len(failures))
	}
	if !strings.Contains(renderErr.Error(), failures[0].Message) {
		t.Fatalf("expected Render's error text to contain the structured message %q, got %v",
			failures[0].Message, renderErr)
	}
	if !strings.Contains(renderErr.Error(), fmt.Sprintf("post-render validation on %s failed", deploymentIdentity)) {
		t.Fatalf("expected Render error to identify the failing resource, got %v", renderErr)
	}
}

// TestRenderForLint_FailureResourcesArePostProcessed verifies RenderForLint runs
// postProcessResources even when a post-render failure was captured: the returned
// resources carry the labels applied in post-processing, matching production fidelity
// (locked decision 5 in the implementation plan).
func TestRenderForLint_FailureResourcesArePostProcessed(t *testing.T) {
	ct, comp, traits := failingTraitPostValidationYAMLs(3)
	input := newRenderForLintInput(t, ct, comp, traits)
	input.Metadata.Labels = map[string]string{"app.kubernetes.io/managed-by": "render-for-lint"}

	out, failures, err := NewPipeline().RenderForLint(t.Context(), input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("expected one failure, got %d", len(failures))
	}
	if out == nil || len(out.Resources) != 1 {
		t.Fatalf("expected one rendered resource, got %+v", out)
	}
	meta, ok := out.Resources[0].Resource["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("rendered resource missing metadata")
	}
	resourceLabels, _ := meta["labels"].(map[string]any)
	if resourceLabels["app.kubernetes.io/managed-by"] != "render-for-lint" {
		t.Fatalf("expected post-processed label on the returned resource, got %v", resourceLabels)
	}
}

func TestRenderForLint_MultipleFailuresCaptured(t *testing.T) {
	ct := `
metadata: {name: web-ct}
spec:
  postRenderValidations:
    - target: {group: apps, version: v1, kind: Deployment}
      rule: "${resource.spec.replicas == 1}"
      message: "must be single replica"
    - target: {group: apps, version: v1, kind: Deployment}
      rule: "${resource.metadata.name == 'other'}"
      message: "name must be other"
  resources:
    - id: deployment
      template: {apiVersion: apps/v1, kind: Deployment, metadata: {name: web}, spec: {replicas: 3}}
`
	out, failures, err := renderForLintPipelineYAML(t, ct, "spec: {}", "")
	if err != nil {
		t.Fatalf("expected no error for declared failures, got %v", err)
	}
	if out == nil || len(out.Resources) != 1 {
		t.Fatalf("expected resources alongside the failures, got %+v", out)
	}
	if len(failures) != 2 {
		t.Fatalf("expected two failures, got %d: %+v", len(failures), failures)
	}
	if failures[0].Message != "must be single replica" {
		t.Errorf("failures[0].Message = %q, want %q", failures[0].Message, "must be single replica")
	}
	if failures[1].Message != "name must be other" {
		t.Errorf("failures[1].Message = %q, want %q", failures[1].Message, "name must be other")
	}
	for i, f := range failures {
		if f.Label != "ComponentType web-ct" {
			t.Errorf("failures[%d].Label = %q, want %q", i, f.Label, "ComponentType web-ct")
		}
		if f.Resource != deploymentIdentity {
			t.Errorf("failures[%d].Resource = %q, want %q", i, f.Resource, deploymentIdentity)
		}
	}
}

// TestRenderForLint_ForEachIterationCaptured feeds the per-item mustMatch scenario
// (an HTTPRoute the forEach list references but no resource matches) through
// RenderForLint and pins that the failing iteration is carried structurally: Iteration
// describes the loop item and Resource stays empty, since a mustMatch failure matched
// nothing.
func TestRenderForLint_ForEachIterationCaptured(t *testing.T) {
	ct := `
spec:
  resources:
    - id: route-a
      template: {apiVersion: gateway.networking.k8s.io/v1, kind: HTTPRoute, metadata: {name: a}, spec: {rules: [{}]}}
    - id: route-b
      template: {apiVersion: gateway.networking.k8s.io/v1, kind: HTTPRoute, metadata: {name: b}, spec: {rules: [{}]}}
`
	comp := `
spec:
  traits:
    - name: routecheck
      instanceName: rc
      parameters:
        routes:
          - name: a
          - name: b
          - name: gone
`
	traits := `
- metadata: {name: routecheck}
  spec:
    parameters:
      openAPIV3Schema:
        type: object
        properties:
          routes:
            type: array
            items:
              type: object
              properties:
                name: {type: string}
    postRenderValidations:
      - forEach: "${parameters.routes}"
        var: route
        target:
          group: gateway.networking.k8s.io
          version: v1
          kind: HTTPRoute
          where: "${resource.metadata.name == route.name}"
        rule: "${resource.spec.rules.size() > 0}"
        message: "route ${route.name} lost its rules"
`
	out, failures, err := renderForLintPipelineYAML(t, ct, comp, traits)
	if err != nil {
		t.Fatalf("expected no error for the declared per-item failure, got %v", err)
	}
	if out == nil || len(out.Resources) != 2 {
		t.Fatalf("expected the two rendered HTTPRoutes alongside the failure, got %+v", out)
	}
	if len(failures) != 1 {
		t.Fatalf("expected exactly one failure (the missing route), got %d: %+v", len(failures), failures)
	}
	f := failures[0]
	if f.Label != "Trait routecheck/rc" {
		t.Errorf("Label = %q, want %q", f.Label, "Trait routecheck/rc")
	}
	if f.Kind != kindHTTPRoute {
		t.Errorf("Kind = %q, want %q", f.Kind, kindHTTPRoute)
	}
	if !strings.Contains(f.Message, "no resource matched target") {
		t.Errorf("Message = %q, want it to describe the unmatched target", f.Message)
	}
	if !strings.HasPrefix(f.Iteration, "forEach route=") {
		t.Errorf("Iteration = %q, want prefix %q", f.Iteration, "forEach route=")
	}
	if f.Resource != "" {
		t.Errorf("Resource = %q, want empty for a mustMatch failure that matched nothing", f.Resource)
	}
}

// TestRenderForLint_NonPostRenderErrorMatchesRender pins that a non-post-render error
// behaves exactly as in Render: nil resources, no failures, and identical error text.
func TestRenderForLint_NonPostRenderErrorMatchesRender(t *testing.T) {
	comp := `
spec:
  traits:
    - name: missing
      instanceName: m1
`
	renderOut, renderErr := NewPipeline().Render(t.Context(), newRenderForLintInput(t, staticComponentType, comp, ""))
	if renderErr == nil {
		t.Fatalf("expected Render to fail on the missing trait")
	}
	if renderOut != nil {
		t.Fatalf("expected Render to return nil resources, got %+v", renderOut)
	}

	lintOut, failures, lintErr := NewPipeline().RenderForLint(t.Context(), newRenderForLintInput(t, staticComponentType, comp, ""))
	if lintErr == nil {
		t.Fatalf("expected RenderForLint to fail on the missing trait")
	}
	if lintOut != nil {
		t.Fatalf("expected RenderForLint to return nil resources, got %+v", lintOut)
	}
	if len(failures) != 0 {
		t.Fatalf("expected no post-render failures for a pre-post-render error, got %d", len(failures))
	}
	if lintErr.Error() != renderErr.Error() {
		t.Fatalf("error text diverged:\n Render: %v\nRenderForLint: %v", renderErr, lintErr)
	}
}

// TestRenderForLint_PostRenderFailuresReturnedWithLaterError covers the RenderForLint
// doc comment: when a captured post-render failure is followed by a post-processing
// error, the failures are returned alongside that error even though Resources is nil.
// The post-processing error is produced by a Deployment without spec: with
// workloadType=deployment and a second dataplane resource present, postProcessResources
// computes the dp-resource-hash annotation and fails on the missing spec.
func TestRenderForLint_PostRenderFailuresReturnedWithLaterError(t *testing.T) {
	ct := `
spec:
  workloadType: deployment
  postRenderValidations:
    - target: {group: apps, version: v1, kind: Deployment}
      rule: "${resource.metadata.name == 'other'}"
      message: "deployment must be named other"
  resources:
    - id: deployment
      template: {apiVersion: apps/v1, kind: Deployment, metadata: {name: web}}
    - id: config
      template: {apiVersion: v1, kind: ConfigMap, metadata: {name: cm}, data: {k: v}}
`
	out, failures, err := renderForLintPipelineYAML(t, ct, "spec: {}", "")
	if err == nil {
		t.Fatalf("expected the post-processing error for the Deployment missing spec")
	}
	if !strings.Contains(err.Error(), "post-process") {
		t.Fatalf("expected a post-process error, got %v", err)
	}
	if out != nil {
		t.Fatalf("expected nil resources when post-processing fails, got %+v", out)
	}
	if len(failures) != 1 {
		t.Fatalf("expected the captured post-render failure alongside the error, got %d: %+v", len(failures), failures)
	}
	if failures[0].Message != "deployment must be named other" {
		t.Errorf("failure message = %q, want %q", failures[0].Message, "deployment must be named other")
	}

	// Render aborts at the post-render failure before post-processing ever runs, so its
	// error names the validation rather than the post-process step.
	renderOut, renderErr := NewPipeline().Render(t.Context(), newRenderForLintInput(t, ct, "spec: {}", ""))
	if renderErr == nil || !strings.Contains(renderErr.Error(), "post-render validation failed") {
		t.Fatalf("expected Render to abort at the post-render failure, got %v", renderErr)
	}
	if renderOut != nil {
		t.Fatalf("expected Render to return nil resources, got %+v", renderOut)
	}
}

func TestRenderForLint_TraitConflict_AutoscalingSetsThree_StorageRequiresAtMostOne(t *testing.T) {
	ct := `
metadata: {name: web-ct}
spec:
  resources:
    - id: deployment
      template: {apiVersion: apps/v1, kind: Deployment, metadata: {name: web}, spec: {replicas: 1}}
`
	comp := `
spec:
  traits:
    - name: autoscaling
      instanceName: auto
    - name: storage
      instanceName: stor
`
	traits := `
- metadata: {name: autoscaling}
  spec:
    creates: []
    patches:
      - target:
          group: apps
          version: v1
          kind: Deployment
        operations:
          - {op: replace, path: /spec/replicas, value: 3}
- metadata: {name: storage}
  spec:
    creates: []
    postRenderValidations:
      - target: {group: apps, version: v1, kind: Deployment}
        rule: "${resource.spec.replicas <= 1}"
        message: "storage requires at most 1 replica"
`
	out, failures, err := renderForLintPipelineYAML(t, ct, comp, traits)
	if err != nil {
		t.Fatalf("expected no error for declared post-render failures, got %v", err)
	}
	if out == nil || len(out.Resources) == 0 {
		t.Fatalf("expected resources alongside failures")
	}
	if len(failures) == 0 {
		t.Fatalf("expected PostRenderFailure for conflicting replicas")
	}
	found := false
	for _, f := range failures {
		if strings.Contains(f.Message, "at most 1") || strings.Contains(strings.ToLower(f.Label), "storage") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected storage validation failure, got %+v", failures)
	}
}

func TestLintPostRenderCollector_AddRecordsErrorAndStructuredFailure(t *testing.T) {
	c := &lintPostRenderCollector{}
	f := PostRenderFailure{Label: "Trait post/p1", Kind: kindDeployment, Resource: deploymentIdentity, Message: "boom"}

	if stop := c.add(errors.New("boom"), f); stop {
		t.Fatalf("expected no stop on the first failure")
	}
	if stop := c.add(nil, f); stop {
		t.Fatalf("expected nil error never to stop the sweep")
	}

	if len(c.errs) != 1 {
		t.Fatalf("errs = %d, want 1 (nil error records nothing)", len(c.errs))
	}
	if len(c.failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(c.failures))
	}
	if c.failures[0] != f {
		t.Errorf("failure = %+v, want %+v", c.failures[0], f)
	}
}

func TestLintPostRenderCollector_FailuresCappedWithoutSentinel(t *testing.T) {
	c := &lintPostRenderCollector{}
	for i := range maxPostRenderErrors {
		stop := c.add(
			fmt.Errorf("err %d", i),
			PostRenderFailure{Message: fmt.Sprintf("msg %d", i)},
		)
		wantStop := i == maxPostRenderErrors-1
		if stop != wantStop {
			t.Fatalf("add %d: stop = %t, want %t", i, stop, wantStop)
		}
	}

	if len(c.failures) != maxPostRenderErrors {
		t.Fatalf("failures = %d, want %d", len(c.failures), maxPostRenderErrors)
	}
	// failures holds exactly the real failures: no "stopped after N" sentinel leaks in.
	for i, f := range c.failures {
		if want := fmt.Sprintf("msg %d", i); f.Message != want {
			t.Errorf("failures[%d].Message = %q, want %q", i, f.Message, want)
		}
	}
	if len(c.failures) != len(c.errs) {
		t.Fatalf("failures (%d) must mirror errs (%d) exactly", len(c.failures), len(c.errs))
	}
	if !strings.Contains(c.err().Error(), fmt.Sprintf("validation stopped after %d failures", maxPostRenderErrors)) {
		t.Errorf("expected the joined error to carry the cap sentinel, got %v", c.err())
	}
}
