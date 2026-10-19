// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package specdiff compares two API objects field by field and renders the
// result as a path-based list of added, removed and modified values.
package specdiff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

// ChangeType classifies a single difference.
type ChangeType string

const (
	Added    ChangeType = "added"
	Removed  ChangeType = "removed"
	Modified ChangeType = "modified"
)

// Change is one difference between two values, addressed by its field path.
// Old is unset for an addition and New is unset for a removal.
type Change struct {
	Path string
	Type ChangeType
	Old  any
	New  any
}

// Compare returns the differences between from and to, with every path
// starting at root (for example "spec"). Both values are first converted to
// their JSON form, so struct tags decide field names and omitted fields count
// as unset. A field that is missing on one side and null on the other is not
// a change. Lists are compared by index.
func Compare(root string, from, to any) ([]Change, error) {
	a, err := normalize(from)
	if err != nil {
		return nil, fmt.Errorf("failed to read the first value: %w", err)
	}
	b, err := normalize(to)
	if err != nil {
		return nil, fmt.Errorf("failed to read the second value: %w", err)
	}
	var changes []Change
	compare(root, a, b, &changes)
	return changes, nil
}

// normalize turns v into the generic map, slice and scalar form that JSON
// decodes to. Numbers stay json.Number so integers print without exponents.
func normalize(v any) (any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func compare(path string, a, b any, changes *[]Change) {
	switch {
	case a == nil && b == nil:
		return
	case a == nil:
		*changes = append(*changes, Change{Path: path, Type: Added, New: b})
		return
	case b == nil:
		*changes = append(*changes, Change{Path: path, Type: Removed, Old: a})
		return
	}

	switch av := a.(type) {
	case map[string]any:
		if bv, ok := b.(map[string]any); ok {
			compareMaps(path, av, bv, changes)
			return
		}
	case []any:
		if bv, ok := b.([]any); ok {
			compareLists(path, av, bv, changes)
			return
		}
	}
	if !reflect.DeepEqual(a, b) {
		*changes = append(*changes, Change{Path: path, Type: Modified, Old: a, New: b})
	}
}

func compareMaps(path string, a, b map[string]any, changes *[]Change) {
	// Size for a only, so the capacity never adds two input-sized lengths;
	// append grows it for keys found only in b.
	keys := make([]string, 0, len(a))
	for k := range a {
		keys = append(keys, k)
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		compare(joinKey(path, k), a[k], b[k], changes)
	}
}

func compareLists(path string, a, b []any, changes *[]Change) {
	for i := 0; i < max(len(a), len(b)); i++ {
		p := fmt.Sprintf("%s[%d]", path, i)
		switch {
		case i >= len(a):
			*changes = append(*changes, Change{Path: p, Type: Added, New: b[i]})
		case i >= len(b):
			*changes = append(*changes, Change{Path: p, Type: Removed, Old: a[i]})
		default:
			compare(p, a[i], b[i], changes)
		}
	}
}

var plainKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// joinKey appends a map key to a path, quoting keys such as label names that
// would otherwise read as several path segments.
func joinKey(path, key string) string {
	if plainKey.MatchString(key) {
		if path == "" {
			return key
		}
		return path + "." + key
	}
	return fmt.Sprintf("%s[%s]", path, formatValue(key))
}

// formatValue renders a value as compact JSON, leaving characters such as
// < and > unescaped so they read as written.
func formatValue(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Sprint(v)
	}
	return strings.TrimSuffix(buf.String(), "\n")
}
