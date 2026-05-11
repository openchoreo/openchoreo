// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"

	"github.com/google/cel-go/cel"
)

// AttributeSpec describes one ABAC attribute that can be referenced in CEL condition expressions.
type AttributeSpec struct {
	// Key is the full dotted path (e.g. "resource.environment").
	Key string
	// Description is a short human-readable description used in error messages.
	Description string
	// CELType is the expected CEL type for the attribute at the leaf.
	CELType *cel.Type
}

// Root returns the CEL root variable name (portion before the first '.').
func (s AttributeSpec) Root() string {
	root, _, _ := strings.Cut(s.Key, ".")
	return root
}

// Leaf returns the leaf field name (portion after the first '.').
func (s AttributeSpec) Leaf() string {
	_, leaf, _ := strings.Cut(s.Key, ".")
	return leaf
}

// AttrResourceEnvironment declares resource.environment — the environment
// associated with the resource being acted upon (e.g. "dev", "staging", "prod").
var AttrResourceEnvironment = AttributeSpec{
	Key:         "resource.environment",
	Description: "Environment associated with the resource",
	CELType:     cel.StringType,
}

// conditionRegistry maps concrete action names to the attributes available to CEL
// expressions scoped to that action. Treat as immutable after init.
var conditionRegistry = map[string][]AttributeSpec{
	ActionCreateReleaseBinding: {AttrResourceEnvironment},
	ActionViewReleaseBinding:   {AttrResourceEnvironment},
	ActionUpdateReleaseBinding: {AttrResourceEnvironment},
	ActionDeleteReleaseBinding: {AttrResourceEnvironment},
	ActionViewLogs:             {AttrResourceEnvironment},
	ActionViewMetrics:          {AttrResourceEnvironment},
	ActionViewTraces:           {AttrResourceEnvironment},
}

// LookupConditions returns the attribute specs available for a given concrete action.
// Returns nil if the action has no registered attributes.
func LookupConditions(action string) []AttributeSpec {
	return conditionRegistry[action]
}

// IntersectConditionsForActions returns the attribute specs allowed for every action
// in the input list. Wildcard patterns ("*", "<resource>:*") are expanded to their
// concrete actions before intersection, so a condition targeting "releasebinding:*"
// is treated as targeting every concrete releasebinding action.
//
// Returns an empty (non-nil) slice if any pattern matches no known public action,
// or if the expanded action set has no common attributes.
func IntersectConditionsForActions(actions []string) []AttributeSpec {
	if len(actions) == 0 {
		return nil
	}

	counts := make(map[string]int)
	specs := make(map[string]AttributeSpec)

	for _, pattern := range actions {
		expanded := ExpandActionPattern(pattern)
		if len(expanded) == 0 {
			return []AttributeSpec{}
		}

		// Per-pattern intersection: an attribute is supported by this pattern
		// only if every concrete action it expands to registers that attribute.
		perPatternCount := make(map[string]int)
		perPatternSpec := make(map[string]AttributeSpec)
		for _, a := range expanded {
			for _, s := range conditionRegistry[a] {
				perPatternCount[s.Key]++
				perPatternSpec[s.Key] = s
			}
		}
		for key, c := range perPatternCount {
			if c == len(expanded) {
				counts[key]++
				specs[key] = perPatternSpec[key]
			}
		}
	}

	result := make([]AttributeSpec, 0, len(specs))
	for key, count := range counts {
		if count == len(actions) {
			result = append(result, specs[key])
		}
	}
	return result
}
