// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/internal/pipeline/component/renderer"
	"github.com/openchoreo/openchoreo/internal/template"
)

// Post-render errors reach a status condition verbatim, so nothing they quote may be
// unbounded: not the where clause a tenant authored, and not the forEach item value,
// which is rendered data and can be a whole resource. An oversized condition is rejected
// by the API server, and a failed status write replaces the paced requeue with a hot
// backoff - the failure the cost guards exist to prevent.
const maxPostRenderErrorLen = 4096

// hugePayload is large enough that any raw quote of it is unmistakable in the message.
var hugePayload = strings.Repeat("b", 50_000)

// A where clause that fails at evaluation time (index out of range) while carrying a
// padded literal: a wrap that quotes it raw shows up immediately in the message length.
func hugeWhereExpr() string {
	return `${["` + hugePayload + `"][7]}`
}

func TestPostRenderWhereErrorStaysBounded(t *testing.T) {
	v := replicaValidation("${resource.spec.replicas == 1}", "must be 1", nil, "")
	v.Target.Where = hugeWhereExpr()

	err := runPostRender(t, []renderer.RenderedResource{deployment("web", 1)}, v, nil)
	if err == nil {
		t.Fatal("expected the where clause to fail, got nil")
	}
	if got := len(err.Error()); got > maxPostRenderErrorLen {
		t.Fatalf("error message is %d bytes, want at most %d - the raw where clause leaked in: %.300s",
			got, maxPostRenderErrorLen, err.Error())
	}
}

// The forEach iteration description names the item so an operator can tell which loop
// pass failed. The item is rendered data of arbitrary size, so the description has to be
// bounded like every other fragment.
func TestPostRenderForEachItemDescriptionStaysBounded(t *testing.T) {
	v := replicaValidation("${resource.spec.replicas == 999}", "must be 999", nil, "")
	v.ForEach = `${["` + hugePayload + `"]}`
	v.Var = "item"

	err := runPostRender(t, []renderer.RenderedResource{deployment("web", 1)}, v, nil)
	if err == nil {
		t.Fatal("expected the rule to fail, got nil")
	}
	if got := len(err.Error()); got > maxPostRenderErrorLen {
		t.Fatalf("error message is %d bytes, want at most %d - the raw forEach item leaked in: %.300s",
			got, maxPostRenderErrorLen, err.Error())
	}
}

// A short item must still be named in full: bounding the description must not cost an
// operator the detail that makes it useful.
func TestPostRenderForEachItemDescriptionKeepsShortItems(t *testing.T) {
	v := replicaValidation("${resource.spec.replicas == 999}", "must be 999", nil, "")
	v.ForEach = `${["alpha"]}`
	v.Var = "route"

	err := runPostRender(t, []renderer.RenderedResource{deployment("web", 1)}, v, nil)
	if err == nil {
		t.Fatal("expected the rule to fail, got nil")
	}
	if !strings.Contains(err.Error(), "forEach route=alpha") {
		t.Fatalf("expected the message to name the failing iteration, got: %v", err)
	}
}

func TestPostRenderFailureAggregationIsBounded(t *testing.T) {
	resources := make([]renderer.RenderedResource, 100)
	for i := range resources {
		resources[i] = deployment(fmt.Sprintf("web-%d", i), 1)
	}
	v := replicaValidation("${false}", hugePayload, nil, "")

	err := runPostRender(t, resources, v, nil)
	if err == nil {
		t.Fatal("expected validation failures, got nil")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("validation stopped after %d failures", maxPostRenderErrors)) {
		t.Fatalf("expected a bounded-aggregation summary, got: %v", err)
	}
	if got := len(err.Error()); got > 16_000 {
		t.Fatalf("aggregated validation error is still too large: %d bytes", got)
	}
}

// resourceIdentity names the rendered resource a rule failed on. Both halves are
// tenant-authored and post-render validation runs before validateResources, so nothing has
// yet rejected an oversized metadata.name - and the collector may join many of these into
// one error.
func TestPostRenderResourceIdentityStaysBounded(t *testing.T) {
	huge := deployment(hugePayload, 1)
	v := replicaValidation("${resource.spec.replicas == 999}", "must be 999", nil, "")

	err := runPostRender(t, []renderer.RenderedResource{huge}, v, nil)
	if err == nil {
		t.Fatal("expected the rule to fail, got nil")
	}
	if got := len(err.Error()); got > maxPostRenderErrorLen {
		t.Fatalf("error message is %d bytes, want at most %d - the raw resource name leaked in: %.300s",
			got, maxPostRenderErrorLen, err.Error())
	}
}

// The description of a forEach iteration is only ever read out of an error message, but the
// loop that builds it runs for every item on every render. Formatting is therefore deferred
// until an error actually needs it; these two check both halves of that contract directly,
// since the cost of the eager version is invisible in the output it produces.
func TestPostRenderIterDescIsNotBuiltWhenTheRulePasses(t *testing.T) {
	calls, collected := countIterDescCalls(t, "${resource.spec.replicas == 1}")
	if collected.err() != nil {
		t.Fatalf("expected the rule to pass, got: %v", collected.err())
	}
	if calls != 0 {
		t.Fatalf("built the iteration description %d times on a passing render, want 0", calls)
	}
}

func TestPostRenderIterDescIsBuiltWhenTheRuleFails(t *testing.T) {
	calls, collected := countIterDescCalls(t, "${resource.spec.replicas == 999}")
	if collected.err() == nil {
		t.Fatal("expected the rule to fail, got nil")
	}
	if calls == 0 {
		t.Fatal("failing render did not build the iteration description, so the error cannot name the item")
	}
	if !strings.Contains(collected.err().Error(), "forEach item=counted") {
		t.Fatalf("expected the message to carry the description, got: %v", collected.err())
	}
}

func countIterDescCalls(t *testing.T, rule string) (int, *postRenderErrors) {
	t.Helper()
	calls := 0
	iterDesc := func() string {
		calls++
		return "forEach item=counted"
	}
	collected := &postRenderErrors{}
	evaluatePostRenderSelection(t.Context(), template.NewEngine(),
		[]renderer.RenderedResource{deployment("web", 1)},
		"acme/inst", iterDesc, map[string]any{},
		replicaValidation(rule, "must match", nil, ""), collected)
	return calls, collected
}

// countingItem reports how many times it has been formatted, which is what makes the
// deferral observable: a rendered forEach item cannot carry a counter through CEL.
type countingItem struct{ formatted *int }

func (c countingItem) String() string {
	*c.formatted++
	return "counted-item"
}

// The loop that describes forEach iterations runs for every item on every render, while the
// description itself is read only out of an error. Formatting is therefore deferred to the
// point the description is asked for - this checks that the deferral holds, and that asking
// still produces the description.
func TestIterationDescriberDefersFormattingUntilAsked(t *testing.T) {
	formatted := 0
	desc := iterationDescriber("route", countingItem{&formatted})
	if formatted != 0 {
		t.Fatalf("formatted the item %d times before the description was asked for, want 0", formatted)
	}

	got := desc()
	if formatted != 1 {
		t.Fatalf("asking for the description formatted the item %d times, want 1", formatted)
	}
	if got != "forEach route=counted-item" {
		t.Fatalf("description = %q, want %q", got, "forEach route=counted-item")
	}
}
