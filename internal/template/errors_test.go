// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestIsTerminalRenderError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"cost limit", ErrCostLimitExceeded, true},
		{"cost budget", ErrCostBudgetExceeded, true},
		{
			name: "wrapped cost limit",
			err:  fmt.Errorf("render resource %q: %w", "deployment", ErrCostLimitExceeded),
			want: true,
		},
		{
			name: "doubly wrapped cost budget",
			err:  fmt.Errorf("render: %w", fmt.Errorf("rule 3: %w", ErrCostBudgetExceeded)),
			want: true,
		},
		{
			name: "joined with a plain error",
			err:  errors.Join(errors.New("output foo failed"), ErrCostLimitExceeded),
			want: true,
		},
		{
			name: "deadline exceeded is transient, not terminal",
			err:  context.DeadlineExceeded,
			want: false,
		},
		{
			name: "wrapped deadline exceeded is transient, not terminal",
			err:  fmt.Errorf("render aborted: %w", context.DeadlineExceeded),
			want: false,
		},
		{"cancelled is transient, not terminal", context.Canceled, false},
		{"plain error", errors.New("template parse failed"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTerminalRenderError(tt.err); got != tt.want {
				t.Fatalf("IsTerminalRenderError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// Render errors are copied into status conditions, and a tenant may author an expression far
// larger than an object can hold. The engine bounds how much of it any error quotes.
func TestErrorMessagesBoundExpressionText(t *testing.T) {
	e := NewEngine()
	// A long but syntactically broken expression: the compile error names it.
	expr := "${" + strings.Repeat("averyverylongidentifier.", 5_000) + "&&&}"

	_, err := e.Render(t.Context(), expr, emptyInputs())
	if err == nil {
		t.Fatal("expected a compilation error, got nil")
	}
	if len(err.Error()) > 8_000 {
		t.Fatalf("error message quoted an unbounded expression: %d bytes", len(err.Error()))
	}
	if !strings.Contains(err.Error(), TruncationMarker) {
		t.Fatalf("expected the quoted expression to be marked truncated, got: %v", err)
	}
}

// Truncation must only apply beyond the bound, so error text naming an ordinary expression
// reads exactly as it always has.
func TestShortExpressionsAreNotTruncated(t *testing.T) {
	e := NewEngine()
	_, err := e.Render(t.Context(), "${spec.missing.field}", map[string]any{"spec": map[string]any{}})
	if err == nil {
		t.Fatal("expected an evaluation error, got nil")
	}
	if !strings.Contains(err.Error(), "spec.missing.field") {
		t.Fatalf("expected the error to name the short expression in full, got: %v", err)
	}
	if strings.Contains(err.Error(), TruncationMarker) {
		t.Fatalf("a short expression must not be truncated, got: %v", err)
	}
}

// A cut landing mid-rune produces invalid UTF-8, which the API server rejects in a status
// condition — recreating the failure the bound exists to prevent.
func TestTruncateExprCutsOnRuneBoundaries(t *testing.T) {
	// Three-byte runes do not divide evenly into the bound, so at least one offset lands
	// mid-rune; sweep the surrounding lengths to catch every alignment.
	for length := maxErrorExprLen - 4; length <= maxErrorExprLen+4; length++ {
		runes := strings.Repeat("あ", length)
		got := TruncateFragment(runes)
		body := strings.TrimSuffix(got, TruncationMarker)
		if !utf8.ValidString(body) {
			t.Fatalf("truncation produced invalid UTF-8 for a %d-rune input", length)
		}
	}
}

func TestTruncateToNonPositiveBoundReturnsEmpty(t *testing.T) {
	for _, maxBytes := range []int{0, -1} {
		if got := TruncateTo("tenant-authored text", maxBytes); got != "" {
			t.Fatalf("TruncateTo(..., %d) = %q, want empty", maxBytes, got)
		}
	}
}

// FormatFragment exists so that bounding a value's preview does not first cost the whole
// value. Its output has to satisfy the same contract as TruncateFragment's: within the
// bound, valid UTF-8, and unchanged when the value is small enough to show in full.
func TestFormatFragmentBoundsLargeValues(t *testing.T) {
	huge := strings.Repeat("z", 100_000)
	cases := map[string]any{
		"bare string":  huge,
		"list":         []any{huge, huge},
		"map":          map[string]any{"a": huge, "b": huge},
		"nested":       []any{map[string]any{"k": []any{huge}}},
		"huge map key": map[string]any{huge: 1},
	}
	for name, val := range cases {
		t.Run(name, func(t *testing.T) {
			got := FormatFragment(val)
			if len(got) > maxErrorExprLen {
				t.Fatalf("preview is %d bytes, want at most %d", len(got), maxErrorExprLen)
			}
			if !strings.HasSuffix(got, TruncationMarker) {
				t.Fatalf("a cut preview must say so, got: %q", got)
			}
		})
	}
}

// Small values must read exactly as fmt's %v did before the bound was introduced: the
// preview is what tells an operator which item failed, and most items are small.
func TestFormatFragmentLeavesSmallValuesAsFmtWouldRenderThem(t *testing.T) {
	for _, val := range []any{
		"alpha",
		42,
		3.5,
		true,
		nil,
		[]any{"a", "b"},
		map[string]any{"b": 2, "a": 1},
		[]any{map[string]any{"name": "web", "port": 8080}},
	} {
		want := fmt.Sprintf("%v", val)
		if got := FormatFragment(val); got != want {
			t.Errorf("FormatFragment(%#v) = %q, want %q", val, got, want)
		}
	}
}

// A preview is assembled from pieces, so a cut can land inside any of them, not just at the
// end. The API server rejects invalid UTF-8 in a status condition, which is the failure the
// whole bound exists to prevent.
func TestFormatFragmentCutsOnRuneBoundaries(t *testing.T) {
	multibyte := strings.Repeat("世", 10_000)
	for _, val := range []any{
		multibyte,
		[]any{multibyte},
		map[string]any{multibyte: multibyte},
	} {
		got := FormatFragment(val)
		if !utf8.ValidString(got) {
			t.Fatalf("preview is not valid UTF-8: %q", got)
		}
		if len(got) > maxErrorExprLen {
			t.Fatalf("preview is %d bytes, want at most %d", len(got), maxErrorExprLen)
		}
	}
}

// countingValue reports how many times it has been formatted. FormatFragment's whole reason
// to exist is the work it does *not* do, which the output alone cannot show.
type countingValue struct {
	formatted *int
	text      string
}

func (c countingValue) String() string {
	*c.formatted++
	return c.text
}

// The point of walking the value instead of handing it to fmt is that a large container is
// abandoned as soon as there is enough to show. fmt.Sprintf("%v", …) formats every element
// first and truncates afterwards, so it would pay for all 1,000 of these to display three.
func TestFormatFragmentStopsWalkingOnceBounded(t *testing.T) {
	formatted := 0
	items := make([]any, 1000)
	for i := range items {
		items[i] = countingValue{&formatted, strings.Repeat("x", 100)}
	}

	got := FormatFragment(items)
	if len(got) > maxErrorExprLen {
		t.Fatalf("preview is %d bytes, want at most %d", len(got), maxErrorExprLen)
	}

	// 256 bytes of preview at ~100 bytes an element: a handful, nowhere near all of them.
	if want := 10; formatted > want {
		t.Fatalf("formatted %d of %d elements to fill a %d-byte preview, want at most %d",
			formatted, len(items), maxErrorExprLen, want)
	}
}
