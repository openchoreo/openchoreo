// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// The cel-go release this branch pins (v0.22.1) ships no runtime cost trackers for the
// strings, lists and encoders extensions: ext.listsLib.ProgramOptions and its string and
// encoder equivalents all return an empty slice, and cel-go therefore charges every one of
// those overloads a flat unit no matter how much data it walks. Newer cel-go installs
// trackers for all of them, so on this branch customFunctionCostEstimator carries them
// instead.
//
// Each case below feeds the same expression a small and a large input. Without a tracker the
// two measurements are identical, which is what makes these tests fail if the estimator's
// cases are removed.
func TestExtOverloadCostGrowsWithInputSize(t *testing.T) {
	// String inputs are supplied as bound variables rather than built inside the expression.
	// A whole-collection identifier reference charges roughly constant, so the growth these
	// tests observe comes from the overload under test and not from the fixture.
	stringInput := func(size int) map[string]any {
		return map[string]any{"spec": map[string]any{}, "payload": strings.Repeat("a", size)}
	}
	base64Input := func(size int) map[string]any {
		return map[string]any{"spec": map[string]any{}, "payload": strings.Repeat("YWJj", size/4)}
	}
	bytesInput := func(size int) map[string]any {
		return map[string]any{"spec": map[string]any{}, "payload": []byte(strings.Repeat("a", size))}
	}
	stringListInput := func(size int) map[string]any {
		items := make([]any, size)
		for i := range items {
			items[i] = "x"
		}
		return map[string]any{"spec": map[string]any{}, "payload": items}
	}
	intListInput := func(size int) map[string]any {
		items := make([]any, size)
		for i := range items {
			items[i] = int64(i)
		}
		return map[string]any{"spec": map[string]any{}, "payload": items}
	}
	sized := func(format string) func(int) string {
		return func(n int) string { return fmt.Sprintf(format, n) }
	}
	fixed := func(expr string) func(int) string {
		return func(int) string { return expr }
	}

	tests := []struct {
		name   string
		expr   func(size int) string
		inputs func(size int) map[string]any
	}{
		{name: "lists_range", expr: sized("${size(lists.range(%d))}"), inputs: emptyInputsSized},
		{name: "list_reverse", expr: sized("${size(lists.range(%d).reverse())}"), inputs: emptyInputsSized},
		{name: "list_slice", expr: sized("${size(lists.range(%d).slice(0, %[1]d))}"), inputs: emptyInputsSized},
		{name: "list_flatten", expr: sized("${size([lists.range(%d)].flatten())}"), inputs: emptyInputsSized},
		{name: "list_flatten_int", expr: sized("${size([lists.range(%d)].flatten(1))}"), inputs: emptyInputsSized},
		{name: "list_distinct", expr: sized("${size(lists.range(%d).distinct())}"), inputs: emptyInputsSized},
		{name: "list_int_sort", expr: sized("${size(lists.range(%d).sort())}"), inputs: emptyInputsSized},
		{name: "list_int_sortByAssociatedKeys", expr: sized("${size(lists.range(%d).sortBy(e, e))}"), inputs: emptyInputsSized},

		// The dynamic forms are separate cases because cel-go hands the estimator an empty
		// overload ID when a DynType argument stops the planner resolving one, and every
		// template input is declared DynType.
		{name: "sort over a dyn list", expr: fixed("${size(payload.sort())}"), inputs: intListInput},
		{name: "sortBy over a dyn list", expr: fixed("${size(payload.sortBy(e, e))}"), inputs: intListInput},
		{name: "reverse over a dyn list", expr: fixed("${size(payload.reverse())}"), inputs: intListInput},
		{name: "reverse over a dyn string", expr: fixed("${size(payload.reverse())}"), inputs: stringInput},

		{name: "list_join", expr: fixed("${size(payload.join())}"), inputs: stringListInput},
		{name: "list_join_string", expr: fixed(`${size(payload.join(","))}`), inputs: stringListInput},

		{name: "string_split_string", expr: fixed(`${size(payload.split("a"))}`), inputs: stringInput},
		{name: "string_split_string_int", expr: fixed(`${size(payload.split("a", -1))}`), inputs: stringInput},
		{name: "string_replace_string_string", expr: fixed(`${size(payload.replace("a", "b"))}`), inputs: stringInput},
		{name: "string_replace_string_string_int", expr: fixed(`${size(payload.replace("a", "b", -1))}`), inputs: stringInput},
		{name: "string_substring_int", expr: fixed("${size(payload.substring(0))}"), inputs: stringInput},
		{name: "string_substring_int_int", expr: sized("${size(payload.substring(0, %d))}"), inputs: stringInput},
		{name: "string_trim", expr: fixed("${size(payload.trim())}"), inputs: stringInput},
		{name: "string_lower_ascii", expr: fixed("${size(payload.lowerAscii())}"), inputs: stringInput},
		{name: "string_upper_ascii", expr: fixed("${size(payload.upperAscii())}"), inputs: stringInput},
		{name: "string_char_at_int", expr: fixed("${payload.charAt(0)}"), inputs: stringInput},
		{name: "string_index_of_string", expr: fixed(`${payload.indexOf("zzz")}`), inputs: stringInput},
		{name: "string_index_of_string_int", expr: fixed(`${payload.indexOf("zzz", 0)}`), inputs: stringInput},
		{name: "string_last_index_of_string", expr: fixed(`${payload.lastIndexOf("zzz")}`), inputs: stringInput},
		{name: "string_last_index_of_string_int", expr: fixed(`${payload.lastIndexOf("zzz", 0)}`), inputs: stringInput},

		{name: "base64_encode_bytes", expr: fixed("${size(base64.encode(payload))}"), inputs: bytesInput},
		{name: "base64_decode_string", expr: fixed("${size(base64.decode(payload))}"), inputs: base64Input},
	}

	// The two inputs differ by 100x, so an overload still metered as O(1) reports the same
	// number twice and cannot pass.
	const (
		small = 100
		large = 10_000
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			smallCost := observedCost(t, tt.expr(small), tt.inputs(small))
			largeCost := observedCost(t, tt.expr(large), tt.inputs(large))
			t.Logf("%s: cost(%d)=%d cost(%d)=%d", tt.name, small, smallCost, large, largeCost)

			if largeCost <= smallCost {
				t.Fatalf("cost did not grow with input size: cost(%d)=%d, cost(%d)=%d — "+
					"the overload is still metered as O(1)", small, smallCost, large, largeCost)
			}
			if largeCost < smallCost*10 {
				t.Errorf("cost grew sublinearly for a 100x larger input: cost(%d)=%d, cost(%d)=%d",
					small, smallCost, large, largeCost)
			}
		})
	}
}

func emptyInputsSized(int) map[string]any {
	return emptyInputs()
}

// distinct() compares every element against every other, so cel-go's newer releases charge it
// as O(n^2). Unmetered on this branch it costs three units and runs for ~160ms, which is what
// let the shape below sit inside the default limit and be repeated at will.
func TestDistinctOverALargeRangeBreachesTheDefaultLimit(t *testing.T) {
	e := NewEngine()

	var err error
	var elapsed time.Duration
	withWatchdog(t, func() {
		start := time.Now()
		_, err = e.Render(t.Context(), "${size(lists.range(10000).distinct())}", emptyInputs())
		elapsed = time.Since(start)
	})
	t.Logf("elapsed=%s err=%v", elapsed, err)

	if !errors.Is(err, ErrCostLimitExceeded) {
		t.Fatalf("expected an O(n^2) distinct over 10,000 elements to breach the per-expression limit, got: %v", err)
	}
}

// A list literal is charged once for its construction, so repeating a cheap-to-meter call
// inside one literal multiplied the allocation without moving the meter: 4,500 ranges of
// 10,000 ints measured 4,511 units and allocated on the order of a gigabyte.
func TestLargeListLiteralOfRangesBreachesTheDefaultLimit(t *testing.T) {
	const copies = 4500
	parts := make([]string, copies)
	for i := range parts {
		parts[i] = "lists.range(10000)"
	}
	expr := "${size([" + strings.Join(parts, ", ") + "])}"

	e := NewEngine()
	var err error
	withWatchdog(t, func() {
		_, err = e.Render(t.Context(), expr, emptyInputs())
	})
	if !errors.Is(err, ErrCostLimitExceeded) {
		t.Fatalf("expected %d materialized ranges to breach the per-expression limit, got: %v", copies, err)
	}
}

// refusalBound is what "quickly" means for a guard that charges after the fact: cel-go
// meters distinct() only once it returns, so one quadratic pass over 10,000 elements always
// completes before the breach can fire. That pass is ~150ms here and ~1.7s under -race, so
// the bound follows watchdogTimeout's convention and budgets for the ~10x slowdown. It is
// still two orders of magnitude below the ~30s an unmetered render of this template took.
const refusalBound = 5 * time.Second

// The volume form of the same attack. Each entry is one expression, so the per-expression
// limit is what has to stop the first one; before the overloads were metered the whole render
// completed successfully after ~30 seconds of CPU.
func TestVolumeOfDistinctRangesIsRefusedQuickly(t *testing.T) {
	const entries = 200
	data := make(map[string]any, entries)
	for i := range entries {
		data[fmt.Sprintf("f%d", i)] = "${size(lists.range(10000).distinct())}"
	}

	e := NewEngine()
	var err error
	var elapsed time.Duration
	withWatchdog(t, func() {
		start := time.Now()
		_, err = e.Render(t.Context(), data, emptyInputs())
		elapsed = time.Since(start)
	})
	t.Logf("elapsed=%s err=%v", elapsed, err)

	if !errors.Is(err, ErrCostLimitExceeded) && !errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("expected %d distinct-over-10,000 entries to breach a cost guard, got: %v", entries, err)
	}
	if elapsed > refusalBound {
		t.Errorf("expected the refusal within %s, took %s: only the first entry should evaluate",
			refusalBound, elapsed)
	}
}

// The trackers must not price ordinary template work out of existence: everything below is
// the size a real template uses, and has to keep evaluating for a handful of units.
func TestOrdinaryListAndStringOperationsStayCheap(t *testing.T) {
	inputs := map[string]any{
		"spec":    map[string]any{},
		"payload": "a,b,c",
	}
	tests := []struct {
		name string
		expr string
		want any
	}{
		{name: "lists.range", expr: "${size(lists.range(5))}", want: int64(5)},
		{name: "sort", expr: "${[3,1,2].sort()}", want: []any{int64(1), int64(2), int64(3)}},
		{name: "split", expr: `${"a,b".split(",")}`, want: []any{"a", "b"}},
		{name: "split a bound string", expr: `${payload.split(",")}`, want: []any{"a", "b", "c"}},
		{name: "join", expr: `${["a","b"].join("-")}`, want: "a-b"},
		{name: "distinct", expr: "${[1,1,2].distinct()}", want: []any{int64(1), int64(2)}},
		{name: "lowerAscii", expr: `${"AB".lowerAscii()}`, want: "ab"},
	}

	// Every fixture here is smaller than a hundred elements or bytes, so anything past a few
	// hundred units would mean a formula that scales on something other than the input.
	const ceiling uint64 = 200

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewEngine().Render(t.Context(), tt.expr, inputs)
			if err != nil {
				t.Fatalf("%s must still evaluate: %v", tt.expr, err)
			}
			if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", tt.want) {
				t.Fatalf("%s = %v, want %v", tt.expr, got, tt.want)
			}
			if cost := observedCost(t, tt.expr, inputs); cost > ceiling {
				t.Fatalf("%s cost %d units, above the %d ceiling for template-sized input", tt.expr, cost, ceiling)
			}
		})
	}
}

// The tests above drive the estimator through real expressions, which is what proves the
// trackers are wired to the same meter the limit bounds. These call it directly, to pin the
// two things an expression cannot isolate: the formula for a quadratic overload, and the
// dispatch that has to work when cel-go resolves no overload ID at all.
func TestExtOverloadCostDispatch(t *testing.T) {
	ints := func(n int) ref.Val {
		items := make([]ref.Val, n)
		for i := range items {
			items[i] = types.Int(i)
		}
		return types.NewRefValList(types.DefaultTypeAdapter, items)
	}
	strs := func(n int) ref.Val {
		items := make([]ref.Val, n)
		for i := range items {
			items[i] = types.String("x")
		}
		return types.NewRefValList(types.DefaultTypeAdapter, items)
	}

	// An n-element self-compare is charged n^2 at cel-go's factor of two per comparison, plus
	// one for the dispatch and ten for allocating the result.
	const selfCompare100 uint64 = 100*100*2 + 1 + 10

	tests := []struct {
		name       string
		function   string
		overloadID string
		args       []ref.Val
		result     ref.Val
		want       uint64
	}{
		{
			name:     "distinct is quadratic in its input",
			function: "distinct", overloadID: "list_distinct",
			args: []ref.Val{ints(100)}, result: ints(100),
			want: selfCompare100,
		},
		{
			name:     "sort resolved to a typed overload",
			function: "sort", overloadID: "list_int_sort",
			args: []ref.Val{ints(100)}, result: ints(100),
			want: selfCompare100,
		},
		{
			name:     "sort left unresolved by a dyn argument",
			function: "sort", overloadID: "",
			args: []ref.Val{ints(100)}, result: ints(100),
			want: selfCompare100,
		},
		{
			name:     "comparing strings costs their contents too",
			function: "sort", overloadID: "",
			args: []ref.Val{strs(100)}, result: strs(100),
			// 2.0 + StringTraversalCostFactor per comparison.
			want: 100*100*21/10 + 1 + 10,
		},
		{
			name:     "sortBy is charged over the key list, not the sorted list",
			function: "@sortByAssociatedKeys", overloadID: "",
			args: []ref.Val{ints(1), ints(100)}, result: ints(1),
			want: selfCompare100,
		},
		{
			name:     "reverse over an unresolved list uses the list formula",
			function: "reverse", overloadID: "",
			args: []ref.Val{ints(100)}, result: ints(100),
			want: 100 + 1 + 10,
		},
		{
			name:     "reverse over an unresolved string uses the string formula",
			function: "reverse", overloadID: "",
			args: []ref.Val{types.String(strings.Repeat("a", 100))}, result: types.String(strings.Repeat("a", 100)),
			// ceil(100 x 0.1) read, plus dispatch, plus the 100 bytes written.
			want: 10 + 1 + 100,
		},
		{
			name:     "an empty list still costs the dispatch and the allocation",
			function: "distinct", overloadID: "list_distinct",
			args: []ref.Val{ints(0)}, result: ints(0),
			want: 0 + 1 + 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extOverloadCost(tt.function, tt.overloadID, tt.args, tt.result)
			if got == nil {
				t.Fatalf("%s/%q was left unmetered", tt.function, tt.overloadID)
			}
			if *got != tt.want {
				t.Fatalf("%s/%q cost = %d, want %d", tt.function, tt.overloadID, *got, tt.want)
			}
		})
	}
}

// Anything the trackers do not recognize has to fall through to cel-go's own accounting.
// Returning a cost instead would replace the built-in formula for that overload with this
// one, and returning zero would silently unmeter it.
func TestExtOverloadCostLeavesUnknownCallsToCELGo(t *testing.T) {
	for _, tt := range []struct{ function, overloadID string }{
		{"startsWith", "starts_with_string"},
		{"_+_", "add_string"},
		{"format", "string_format"},
		{"sets.contains", "list_sets_contains_list"},
		{"math.abs", "math_abs_int"},
		{"oc_omit", "oc_omit"},
	} {
		if got := extOverloadCost(tt.function, tt.overloadID, []ref.Val{types.String("a")}, types.String("a")); got != nil {
			t.Errorf("%s/%s must be left to cel-go, got a cost of %d", tt.function, tt.overloadID, *got)
		}
	}
}

// A quadratic term over a list that reports an implausible size must saturate rather than
// wrap. A wrapped product reads as a tiny cost and would fund the very evaluation the guard
// exists to refuse.
func TestExtOverloadCostSaturates(t *testing.T) {
	if got := saturatingMul(math.MaxUint64/2, 4); got != math.MaxUint64 {
		t.Fatalf("saturatingMul wrapped: got %d", got)
	}
	if got := scaledCost(math.MaxUint64, 1000); got != math.MaxUint64 {
		t.Fatalf("scaledCost wrapped: got %d", got)
	}
	if got := allocatingListCost(2, math.MaxUint64); got != math.MaxUint64 {
		t.Fatalf("allocatingListCost wrapped: got %d", got)
	}
}

// The trackers run inside cel-go's cost observer, where a panic takes down the reconcile
// rather than failing the render. Arity is cel-go's to choose, so nothing here may index an
// argument it was not handed.
func TestExtOverloadCostToleratesUnexpectedArity(t *testing.T) {
	for _, tt := range []struct {
		function, overloadID string
		args                 []ref.Val
	}{
		{"distinct", "list_distinct", nil},
		{"sort", "", nil},
		{"@sortByAssociatedKeys", "", []ref.Val{types.NewRefValList(types.DefaultTypeAdapter, nil)}},
		{"reverse", "", nil},
		{"replace", "string_replace_string_string", []ref.Val{types.String("a")}},
		{"indexOf", "string_index_of_string", nil},
		{"flatten", "list_flatten_int", nil},
		{"base64.encode", "base64_encode_bytes", nil},
	} {
		if got := extOverloadCost(tt.function, tt.overloadID, tt.args, types.String("")); got == nil {
			t.Errorf("%s/%s returned no cost", tt.function, tt.overloadID)
		}
	}
}
