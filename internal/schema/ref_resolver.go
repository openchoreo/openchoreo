// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"fmt"
	"slices"
	"strings"

	"github.com/openchoreo/openchoreo/internal/clone"
)

const maxRefDepth = 64

// ResolveRefs inlines all $ref references in a JSON Schema so that downstream code
// never sees $ref. Supports both $defs (JSON Schema 2020-12) and definitions (Draft 4/7).
// Circular references and remote refs are rejected with descriptive errors.
// The input is not mutated; a deep copy is returned.
// Only $defs (JSON Schema 2020-12) is supported; the older "definitions" keyword is not.
func ResolveRefs(schema map[string]any) (map[string]any, error) {
	if schema == nil {
		return nil, nil
	}

	result := clone.DeepCopyMap(schema)

	defs := extractDefs(result)
	if len(defs) == 0 {
		// No definitions — still walk tree to reject any $ref usage
		resolved, err := resolveNode(result, nil, nil, 0)
		if err != nil {
			return nil, err
		}
		return resolved.(map[string]any), nil
	}

	resolved, err := resolveNode(result, defs, nil, 0)
	if err != nil {
		return nil, err
	}

	out := resolved.(map[string]any)
	delete(out, "$defs")
	return out, nil
}

// extractDefs extracts the definitions map from $defs.
func extractDefs(schema map[string]any) map[string]any {
	if defs, ok := schema["$defs"].(map[string]any); ok {
		return defs
	}
	return nil
}

// resolveNode recursively walks a schema node, resolving any $ref encountered.
// visiting tracks the ref resolution stack to detect cycles.
func resolveNode(node any, defs map[string]any, visiting []string, depth int) (any, error) {
	if depth > maxRefDepth {
		return nil, fmt.Errorf("$ref resolution exceeded maximum depth of %d", maxRefDepth)
	}

	obj, ok := node.(map[string]any)
	if !ok {
		// Not an object — could be a slice (allOf, oneOf, etc.) or primitive
		if arr, ok := node.([]any); ok {
			resolved := make([]any, len(arr))
			for i, item := range arr {
				r, err := resolveNode(item, defs, visiting, depth+1)
				if err != nil {
					return nil, err
				}
				resolved[i] = r
			}
			return resolved, nil
		}
		return node, nil
	}

	// Check for $ref
	if ref, hasRef := obj["$ref"]; hasRef {
		refStr, ok := ref.(string)
		if !ok {
			return nil, fmt.Errorf("$ref must be a string, got %T", ref)
		}

		resolved, err := resolveRef(refStr, obj, defs, visiting, depth)
		if err != nil {
			return nil, err
		}
		return resolved, nil
	}

	// No $ref — walk into schema-bearing keywords
	for _, key := range schemaKeywords() {
		val, exists := obj[key]
		if !exists {
			continue
		}
		resolved, err := resolveNode(val, defs, visiting, depth+1)
		if err != nil {
			return nil, err
		}
		obj[key] = resolved
	}

	// Walk properties
	if props, ok := obj["properties"].(map[string]any); ok {
		for k, v := range props {
			resolved, err := resolveNode(v, defs, visiting, depth+1)
			if err != nil {
				return nil, err
			}
			props[k] = resolved
		}
	}

	// Walk patternProperties
	if pp, ok := obj["patternProperties"].(map[string]any); ok {
		for k, v := range pp {
			resolved, err := resolveNode(v, defs, visiting, depth+1)
			if err != nil {
				return nil, err
			}
			pp[k] = resolved
		}
	}

	return obj, nil
}

// schemaKeywords returns the keywords that contain schema objects or arrays of schemas.
func schemaKeywords() []string {
	return []string{
		"items", "additionalProperties", "not",
		"if", "then", "else",
		"allOf", "oneOf", "anyOf",
	}
}

// resolveRef resolves a single $ref, handling sibling keys and cycle detection.
func resolveRef(ref string, node map[string]any, defs map[string]any, visiting []string, depth int) (any, error) {
	// Reject remote/URL refs
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return nil, fmt.Errorf("only local $ref supported, got %q", ref)
	}

	// Parse local ref path
	defName, err := parseLocalRef(ref)
	if err != nil {
		return nil, err
	}

	// Cycle detection
	if slices.Contains(visiting, defName) {
		cycle := append(slices.Clone(visiting), defName)
		return nil, fmt.Errorf("circular $ref: %s", strings.Join(cycle, " → "))
	}

	if defs == nil {
		return nil, fmt.Errorf("$ref %q not found: no definitions available", ref)
	}

	defSchema, exists := defs[defName]
	if !exists {
		return nil, fmt.Errorf("$ref %q not found in definitions", ref)
	}

	defMap, ok := defSchema.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("$ref %q resolved to non-object type", ref)
	}

	// Deep copy the definition to avoid mutation
	resolved := clone.DeepCopyMap(defMap)

	// Recursively resolve refs within the definition
	newVisiting := make([]string, len(visiting)+1)
	copy(newVisiting, visiting)
	newVisiting[len(visiting)] = defName

	resolvedNode, err := resolveNode(resolved, defs, newVisiting, depth+1)
	if err != nil {
		return nil, err
	}
	resolved = resolvedNode.(map[string]any)

	// Handle sibling keys (JSON Schema 2020-12 allows $ref with siblings).
	// Merge siblings into the resolved definition, then re-resolve the merged
	// result so any $ref within sibling subtrees (e.g., properties, items) are
	// fully inlined.
	siblings := collectSiblings(node)
	if len(siblings) > 0 {
		for k, v := range siblings {
			resolved[k] = v
		}
		merged, err := resolveNode(resolved, defs, visiting, depth+1)
		if err != nil {
			return nil, err
		}
		return merged, nil
	}

	return resolved, nil
}

// collectSiblings returns all keys from a $ref node except $ref itself.
func collectSiblings(node map[string]any) map[string]any {
	if len(node) <= 1 {
		return nil
	}
	siblings := make(map[string]any, len(node)-1)
	for k, v := range node {
		if k == "$ref" {
			continue
		}
		siblings[k] = v
	}
	return siblings
}

// parseLocalRef extracts the definition name from a local $ref path.
// Supports #/$defs/Name and #/definitions/Name formats.
func parseLocalRef(ref string) (string, error) {
	if !strings.HasPrefix(ref, "#/") {
		return "", fmt.Errorf("only local $ref supported (must start with #/), got %q", ref)
	}

	parts := strings.Split(ref[2:], "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("unsupported $ref path %q: expected #/$defs/Name", ref)
	}

	prefix := parts[0]
	if prefix != "$defs" {
		return "", fmt.Errorf("unsupported $ref path %q: expected #/$defs/Name", ref)
	}

	name := parts[1]
	if name == "" {
		return "", fmt.Errorf("empty definition name in $ref %q", ref)
	}

	return name, nil
}
