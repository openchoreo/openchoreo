// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderer

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/internal/template"
)

func TestToIterableItems(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    []any
		wantErr bool
	}{
		{
			name: "array of any",
			input: []any{
				"item1",
				"item2",
				"item3",
			},
			want: []any{
				"item1",
				"item2",
				"item3",
			},
			wantErr: false,
		},
		{
			name: "array of maps",
			input: []map[string]any{
				{"name": "config1", "value": "val1"},
				{"name": "config2", "value": "val2"},
			},
			want: []any{
				map[string]any{"name": "config1", "value": "val1"},
				map[string]any{"name": "config2", "value": "val2"},
			},
			wantErr: false,
		},
		{
			name:    "empty array",
			input:   []any{},
			want:    []any{},
			wantErr: false,
		},
		{
			name: "map with string values",
			input: map[string]any{
				"database": "postgres://localhost:5432",
				"cache":    "redis://localhost:6379",
			},
			want: []any{
				map[string]any{"key": "cache", "value": "redis://localhost:6379"},
				map[string]any{"key": "database", "value": "postgres://localhost:5432"},
			},
			wantErr: false,
		},
		{
			name: "map with complex values",
			input: map[string]any{
				"db": map[string]any{
					"host": "localhost",
					"port": 5432,
				},
				"api": map[string]any{
					"host": "api.example.com",
					"port": 443,
				},
			},
			want: []any{
				map[string]any{
					"key": "api",
					"value": map[string]any{
						"host": "api.example.com",
						"port": 443,
					},
				},
				map[string]any{
					"key": "db",
					"value": map[string]any{
						"host": "localhost",
						"port": 5432,
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "empty map",
			input:   map[string]any{},
			want:    []any{},
			wantErr: false,
		},
		{
			name: "map keys are sorted alphabetically",
			input: map[string]any{
				"zebra": "z-value",
				"alpha": "a-value",
				"beta":  "b-value",
			},
			want: []any{
				map[string]any{"key": "alpha", "value": "a-value"},
				map[string]any{"key": "beta", "value": "b-value"},
				map[string]any{"key": "zebra", "value": "z-value"},
			},
			wantErr: false,
		},
		{
			name:    "invalid type - string",
			input:   "not-an-array-or-map",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid type - number",
			input:   42,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid type - boolean",
			input:   true,
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToIterableItems(t.Context(), tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToIterableItems() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToIterableItems() got = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestToIterableItemsRejectsOversizedForEach verifies the fan-out cap: a collection at
// exactly maxForEachItems is accepted, one item more is rejected. Every supported input
// type is checked because each branch enforces the cap on its own.
func TestToIterableItemsRejectsOversizedForEach(t *testing.T) {
	anyList := func(n int) any {
		items := make([]any, n)
		for i := range items {
			items[i] = i
		}
		return items
	}
	mapList := func(n int) any {
		items := make([]map[string]any, n)
		for i := range items {
			items[i] = map[string]any{"index": i}
		}
		return items
	}
	keyedMap := func(n int) any {
		items := make(map[string]any, n)
		for i := 0; i < n; i++ {
			items[fmt.Sprintf("key%d", i)] = i
		}
		return items
	}

	builders := map[string]func(int) any{
		"[]any":            anyList,
		"[]map[string]any": mapList,
		"map[string]any":   keyedMap,
	}

	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			got, err := ToIterableItems(t.Context(), build(maxForEachItems))
			if err != nil {
				t.Fatalf("ToIterableItems() with %d items: unexpected error = %v", maxForEachItems, err)
			}
			if len(got) != maxForEachItems {
				t.Fatalf("ToIterableItems() returned %d items, want %d", len(got), maxForEachItems)
			}

			over := maxForEachItems + 1
			got, err = ToIterableItems(t.Context(), build(over))
			if err == nil {
				t.Fatalf("ToIterableItems() with %d items: expected error, got %d items", over, len(got))
			}
			want := fmt.Sprintf("forEach expanded to %d items, exceeding the limit of %d", over, maxForEachItems)
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("ToIterableItems() error = %v, want it to contain %q", err, want)
			}
			// The cap depends only on the item count, so it trips identically on every
			// reconcile; reconcilers must be able to recognize that and pace the retry.
			if !errors.Is(err, template.ErrExpansionLimitExceeded) {
				t.Errorf("ToIterableItems() error must wrap template.ErrExpansionLimitExceeded, got %v", err)
			}
			if !template.IsTerminalRenderError(err) {
				t.Errorf("ToIterableItems() cap breach must classify as terminal, got %v", err)
			}
			if got != nil {
				t.Errorf("ToIterableItems() returned %d items alongside the error, want nil", len(got))
			}
		})
	}
}

func TestToIterableItemsRejectsCumulativeExpansion(t *testing.T) {
	ctx := WithForEachBudget(t.Context())
	items := make([]any, maxForEachItems)

	for i := 0; i < maxForEachItemsPerRender/maxForEachItems; i++ {
		if _, err := ToIterableItems(ctx, items); err != nil {
			t.Fatalf("expansion %d unexpectedly exceeded the cumulative limit: %v", i, err)
		}
	}

	_, err := ToIterableItems(ctx, []any{"one more"})
	if !errors.Is(err, template.ErrExpansionLimitExceeded) {
		t.Fatalf("expected cumulative expansions to breach the limit, got: %v", err)
	}
	if !strings.Contains(err.Error(), "cumulative limit") {
		t.Fatalf("expected the error to identify the cumulative cap, got: %v", err)
	}
}

// TestToIterableItemsDeterminism verifies that map iteration is deterministic
func TestToIterableItemsDeterminism(t *testing.T) {
	input := map[string]any{
		"key5": "value5",
		"key1": "value1",
		"key3": "value3",
		"key2": "value2",
		"key4": "value4",
	}

	// Run multiple times to ensure consistent ordering
	var previous []any
	for i := 0; i < 10; i++ {
		result, err := ToIterableItems(t.Context(), input)
		if err != nil {
			t.Fatalf("ToIterableItems() error = %v", err)
		}

		if i > 0 && !reflect.DeepEqual(result, previous) {
			t.Errorf("ToIterableItems() iteration %d differs from previous: got %v, previous %v", i, result, previous)
		}
		previous = result
	}

	// Verify the order is alphabetically sorted
	expected := []any{
		map[string]any{"key": "key1", "value": "value1"},
		map[string]any{"key": "key2", "value": "value2"},
		map[string]any{"key": "key3", "value": "value3"},
		map[string]any{"key": "key4", "value": "value4"},
		map[string]any{"key": "key5", "value": "value5"},
	}

	if !reflect.DeepEqual(previous, expected) {
		t.Errorf("ToIterableItems() keys not sorted alphabetically: got %v, want %v", previous, expected)
	}
}
