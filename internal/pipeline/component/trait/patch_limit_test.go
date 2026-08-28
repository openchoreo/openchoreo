// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package trait

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/renderer"
	"github.com/openchoreo/openchoreo/internal/template"
)

// buildTargets returns n distinct ConfigMaps for a patch to match.
func buildTargets(n int) []renderer.RenderedResource {
	resources := make([]renderer.RenderedResource, n)
	for i := range resources {
		resources[i] = renderer.RenderedResource{
			Resource: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]any{"name": fmt.Sprintf("cm-%d", i)},
				"data":       map[string]any{"seed": "v"},
			},
		}
	}
	return resources
}

// forEachPatchTrait returns a trait whose single patch iterates itemCount times and targets
// every ConfigMap, so it applies itemCount x targets times.
func forEachPatchTrait(t *testing.T, itemCount int) *v1alpha1.Trait {
	t.Helper()
	items := make([]string, itemCount)
	for i := range items {
		items[i] = fmt.Sprintf("%q", fmt.Sprintf("k%d", i))
	}
	traitYAML := fmt.Sprintf(`
apiVersion: openchoreo.dev/v1alpha1
kind: Trait
metadata:
  name: fanout
spec:
  patches:
    - target:
        kind: ConfigMap
      forEach: '${[%s]}'
      var: item
      operations:
        - op: add
          path: /data/patched
          value: "${item}"
`, strings.Join(items, ", "))

	var trait v1alpha1.Trait
	if err := yaml.Unmarshal([]byte(traitYAML), &trait); err != nil {
		t.Fatalf("failed to parse trait YAML: %v", err)
	}
	return &trait
}

// forEach multiplies on both sides of a trait patch: items on one side, matched targets on
// the other. The product is JSON-Patch work rather than CEL evaluation, so neither the
// per-expression cost limit nor the reconcile cost budget can see it. maxPatchOperations
// is the only thing standing between a component and a million applications.
func TestTraitPatchesRejectOversizedCrossProduct(t *testing.T) {
	// 200 targets x 100 items = 20,000 applications, twice the cap.
	const (
		targets = 200
		items   = 100
	)

	processor := NewProcessor(template.NewEngine())
	err := processor.ApplyTraitPatches(t.Context(), buildTargets(targets), forEachPatchTrait(t, items), map[string]any{})
	if err == nil {
		t.Fatalf("expected %d x %d applications to be refused, got nil", items, targets)
	}
	if !errors.Is(err, template.ErrExpansionLimitExceeded) {
		t.Fatalf("expected the error to wrap template.ErrExpansionLimitExceeded, got: %v", err)
	}
	if !template.IsTerminalRenderError(err) {
		t.Fatalf("a patch cross-product breach must classify as terminal, got: %v", err)
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("limit of %d", maxPatchOperations)) {
		t.Fatalf("expected the error to name the cap, got: %v", err)
	}
	// The running total tells an operator how far over the cap the component is, and it is
	// deterministic for fixed inputs: 200 targets divide 10,000 exactly, so the 50th item
	// charges the counter to the cap and the 51st batch of 200 is the one refused.
	breachTotal := maxPatchOperations + targets
	if !strings.Contains(err.Error(), fmt.Sprintf("apply %d operations", breachTotal)) {
		t.Fatalf("expected the error to name the running total %d, got: %v", breachTotal, err)
	}
}

func TestTraitPatchLimitCountsEveryOperation(t *testing.T) {
	const (
		targets    = 100
		items      = 100
		operations = 2
	)

	trait := forEachPatchTrait(t, items)
	op := trait.Spec.Patches[0].Operations[0]
	trait.Spec.Patches[0].Operations = make([]v1alpha1.JSONPatchOperation, operations)
	for i := range trait.Spec.Patches[0].Operations {
		trait.Spec.Patches[0].Operations[i] = op
		trait.Spec.Patches[0].Operations[i].Path = fmt.Sprintf("/data/patched%d", i)
	}

	processor := NewProcessor(template.NewEngine())
	err := processor.ApplyTraitPatches(t.Context(), buildTargets(targets), trait, map[string]any{})
	if !errors.Is(err, template.ErrExpansionLimitExceeded) {
		t.Fatalf("expected %d targets x %d items x %d operations to breach the limit, got: %v",
			targets, items, operations, err)
	}
}

// A cross product comfortably inside the cap must still apply, and apply everywhere.
func TestTraitPatchesWithinCapStillApply(t *testing.T) {
	const (
		targets = 10
		items   = 5
	)

	resources := buildTargets(targets)
	processor := NewProcessor(template.NewEngine())
	if err := processor.ApplyTraitPatches(t.Context(), resources, forEachPatchTrait(t, items), map[string]any{}); err != nil {
		t.Fatalf("a %d x %d cross product is well inside the cap and must apply: %v", items, targets, err)
	}
	if processor.patchOperations != targets*items {
		t.Fatalf("counted %d operations, want %d", processor.patchOperations, targets*items)
	}
	for i := range resources {
		data, _ := resources[i].Resource["data"].(map[string]any)
		if data["patched"] != fmt.Sprintf("k%d", items-1) {
			t.Fatalf("resource %d was not patched by the last iteration: %v", i, data)
		}
	}
}

// The cap is per render, not per trait: the pipeline builds one Processor per
// RenderComponent call, so a component that spreads the same fan-out across several traits
// must not slip past it.
func TestPatchOperationsAccumulateAcrossTraits(t *testing.T) {
	// Each pass is 100 x 60 = 6,000 applications: under the cap alone, over it together.
	const (
		targets = 100
		items   = 60
	)

	processor := NewProcessor(template.NewEngine())
	resources := buildTargets(targets)

	if err := processor.ApplyTraitPatches(t.Context(), resources, forEachPatchTrait(t, items), map[string]any{}); err != nil {
		t.Fatalf("the first trait is under the cap and must apply: %v", err)
	}
	err := processor.ApplyTraitPatches(t.Context(), resources, forEachPatchTrait(t, items), map[string]any{})
	if err == nil {
		t.Fatal("expected the second trait to push the render over the cap, got nil")
	}
	if !errors.Is(err, template.ErrExpansionLimitExceeded) {
		t.Fatalf("expected the error to wrap template.ErrExpansionLimitExceeded, got: %v", err)
	}
}
