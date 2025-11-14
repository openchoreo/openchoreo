// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package patch

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// filterExpr recognizes the `[?(@.field=='value')]` selectors used in array filter expressions.
// The pattern captures the field path (group 1) and the expected value (group 2).
// Example: `[?(@.name=='app')]` matches items where the 'name' field equals 'app'.
var filterExpr = regexp.MustCompile(`^@\.([A-Za-z0-9_.-]+)\s*==\s*['"](.*)['"]$`)

// pathState represents a single location within the document tree during path expansion.
// As we traverse the path, we maintain both the JSON Pointer segments and the actual
// value at that location, allowing us to evaluate filters and determine valid next steps.
type pathState struct {
	pointer []string // JSON Pointer segments (without leading "/" or escaping applied)
	value   any      // The value at this location in the document
}

// expandPaths converts a path expression into one or more JSON Pointers.
//
// Path expressions extend standard JSON Pointer with:
//   - Array filters: /containers[?(@.name=='app')]/env
//   - Array indices: /containers/0/env
//   - Append marker: /env/-
//
// A single path can expand to multiple JSON Pointers when filters match multiple elements.
// For example, /containers[?(@.role=='worker')]/image might expand to:
//   - /containers/0/image
//   - /containers/2/image
//   - /containers/5/image
//
// The algorithm maintains a set of possible states as it processes each segment,
// allowing filters to fan out into multiple parallel paths.
func expandPaths(root map[string]any, rawPath string) ([]string, error) {
	if rawPath == "" {
		return []string{""}, nil
	}

	segments := splitRawPath(rawPath)
	// Start with a single state representing the root
	states := []pathState{{pointer: []string{}, value: root}}

	// Process each segment, potentially expanding to multiple states
	for _, segment := range segments {
		// Handle the append marker specially (doesn't need the current value)
		if segment == "-" {
			states = applyDash(states)
			continue
		}

		// Expand each current state by applying this segment
		nextStates := make([]pathState, 0, len(states))
		for _, st := range states {
			expanded, err := applySegment(st, segment)
			if err != nil {
				return nil, err
			}
			nextStates = append(nextStates, expanded...)
		}
		states = nextStates

		// If we have no states, a filter matched nothing or a path was invalid
		if len(states) == 0 {
			break
		}
	}

	// Convert final states to JSON Pointers
	pointers := make([]string, 0, len(states))
	for _, st := range states {
		pointers = append(pointers, buildJSONPointer(st.pointer))
	}
	return pointers, nil
}

// applySegment processes a single path segment, which may contain multiple sub-parts.
//
// Segments can be complex expressions like:
//   - "containers" (simple key)
//   - "0" (numeric index)
//   - "[0]" (bracketed index)
//   - "[?(@.name=='app')]" (filter)
//   - "containers[0]" (key followed by index)
//   - "[?(@.role=='worker')][0]" (filter followed by index)
//
// The function iteratively parses these sub-parts rather than using simple splitting,
// because brackets may be nested or combined in complex ways.
//
// Returns a slice of states representing all possible locations after traversing this segment.
func applySegment(state pathState, segment string) ([]pathState, error) {
	current := []pathState{state}
	remaining := segment

	// Parse the segment character by character, handling brackets specially
	for len(remaining) > 0 {
		if strings.HasPrefix(remaining, "[") {
			// Extract bracket content: [...]
			closeIdx := strings.Index(remaining, "]")
			if closeIdx == -1 {
				return nil, fmt.Errorf("unclosed bracket segment in %q", segment)
			}
			content := remaining[1:closeIdx]
			remaining = remaining[closeIdx+1:]

			// Determine bracket type and apply appropriate operation
			var err error
			switch {
			case strings.HasPrefix(content, "?(") && strings.HasSuffix(content, ")"):
				// Array filter: [?(@.field=='value')]
				expr := content[2 : len(content)-1]
				current, err = applyFilter(current, expr)
			case content == "-":
				// Append marker: [-]
				current = applyDash(current)
			default:
				// Numeric index: [0], [1], etc.
				index, parseErr := strconv.Atoi(content)
				if parseErr != nil {
					return nil, fmt.Errorf("unsupported array index %q", content)
				}
				current, err = applyIndex(current, index)
			}
			if err != nil {
				return nil, err
			}
		} else {
			// Non-bracket content: parse until the next bracket or end
			nextBracket := strings.Index(remaining, "[")
			var token string
			if nextBracket == -1 {
				token = remaining
				remaining = ""
			} else {
				token = remaining[:nextBracket]
				remaining = remaining[nextBracket:]
			}
			if token == "" {
				continue
			}

			// Token could be a bare number (array index) or a key
			if idx, err := strconv.Atoi(token); err == nil {
				current, err = applyIndex(current, idx)
				if err != nil {
					return nil, err
				}
			} else {
				var err error
				current, err = applyKey(current, token)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	return current, nil
}

// applyKey traverses an object key for all current states.
// Each state should have an object value; we extract the specified key and create new states.
func applyKey(states []pathState, key string) ([]pathState, error) {
	if key == "" {
		return states, nil
	}

	next := make([]pathState, 0, len(states))
	for _, st := range states {
		var child any
		switch current := st.value.(type) {
		case map[string]any:
			child = current[key]
		case nil:
			// Traversing through nil is allowed; the child will also be nil
			child = nil
		default:
			return nil, fmt.Errorf("path segment %q expects an object, got %T", key, st.value)
		}
		next = append(next, pathState{
			pointer: appendPointer(st.pointer, key),
			value:   child,
		})
	}
	return next, nil
}

// applyIndex traverses an array index for all current states.
// Each state should have an array value; we extract the element at the specified index.
func applyIndex(states []pathState, index int) ([]pathState, error) {
	next := make([]pathState, 0, len(states))
	for _, st := range states {
		arr, ok := st.value.([]any)
		if !ok {
			return nil, fmt.Errorf("path segment expects an array, got %T", st.value)
		}
		if index < 0 || index >= len(arr) {
			return nil, fmt.Errorf("array index %d out of bounds", index)
		}
		next = append(next, pathState{
			pointer: appendPointer(st.pointer, strconv.Itoa(index)),
			value:   arr[index],
		})
	}
	return next, nil
}

// applyDash adds the array append marker "-" to all current states.
// The value is set to nil since "-" doesn't point to an existing element.
func applyDash(states []pathState) []pathState {
	next := make([]pathState, len(states))
	for i, st := range states {
		next[i] = pathState{
			pointer: appendPointer(st.pointer, "-"),
			value:   nil,
		}
	}
	return next
}

// applyFilter evaluates a filter expression against array elements.
//
// For each state that contains an array, we iterate through its elements
// and test each one against the filter. Elements that match become new states.
//
// This allows a single filter to fan out into multiple paths. For example,
// if containers = [{name: "app"}, {name: "sidecar"}, {name: "app"}],
// then [?(@.name=='app')] produces two states: [0] and [2].
//
// Note: Filters are evaluated using simple field lookups, not CEL, for simplicity.
func applyFilter(states []pathState, expr string) ([]pathState, error) {
	next := []pathState{}
	for _, st := range states {
		arr, ok := st.value.([]any)
		if !ok || len(arr) == 0 {
			// Not an array or empty array; skip this state
			continue
		}
		for idx, item := range arr {
			match, err := matchesFilter(item, expr)
			if err != nil {
				return nil, err
			}
			if match {
				next = append(next, pathState{
					pointer: appendPointer(st.pointer, strconv.Itoa(idx)),
					value:   item,
				})
			}
		}
	}
	return next, nil
}

// matchesFilter tests if an item matches a filter expression.
//
// Currently supports only equality filters of the form: @.field.path=='value'
// The field path can contain dots for nested fields: @.metadata.labels.app=='web'
//
// Returns false (without error) if the field path doesn't exist or types don't match.
func matchesFilter(item any, expr string) (bool, error) {
	matches := filterExpr.FindStringSubmatch(strings.TrimSpace(expr))
	if len(matches) != 3 {
		return false, fmt.Errorf("unsupported filter expression: %s", expr)
	}

	fieldPath := strings.Split(matches[1], ".")
	expected := matches[2]

	// Navigate through nested fields
	current := item
	for _, segment := range fieldPath {
		m, ok := current.(map[string]any)
		if !ok {
			// Field path expects an object but got something else
			return false, nil
		}
		current, ok = m[segment]
		if !ok {
			// Field doesn't exist
			return false, nil
		}
	}

	// Compare the final value
	if current == nil {
		return expected == "", nil
	}
	return fmt.Sprintf("%v", current) == expected, nil
}

// splitRawPath splits a path expression into segments and unescapes RFC 6901 sequences.
// This is used during path expansion to parse user input paths with advanced features
// like array filters and special syntax.
//
// RFC 6901 escape sequences (~0 for ~, ~1 for /) are decoded so that segments
// can be used directly as map keys. For example:
//
//	"/metadata/annotations/app.kubernetes.io~1name" becomes ["metadata", "annotations", "app.kubernetes.io/name"]
func splitRawPath(path string) []string {
	return splitAndUnescapePath(path)
}

// appendPointer creates a new pointer slice with an additional segment.
// This preserves immutability of the original pointer.
func appendPointer(base []string, segment string) []string {
	next := make([]string, len(base)+1)
	copy(next, base)
	next[len(base)] = segment
	return next
}

// buildJSONPointer converts pointer segments into a proper RFC 6901 JSON Pointer string.
//
// Each segment is prefixed with "/" and escaped according to RFC 6901:
//   - "~" becomes "~0"
//   - "/" becomes "~1"
//
// The append marker "-" is not escaped since it has special meaning in JSON Pointer.
func buildJSONPointer(segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	var b strings.Builder
	for _, seg := range segments {
		b.WriteByte('/')
		if seg == "-" {
			// Don't escape the append marker
			b.WriteString(seg)
		} else {
			b.WriteString(escapePointerSegment(seg))
		}
	}
	return b.String()
}
