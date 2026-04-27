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

// IntersectConditionsForActions returns the attribute specs allowed for every action in the input list.
// i.e. the set usable by a condition entry targeting all of those actions at once.
func IntersectConditionsForActions(actions []string) []AttributeSpec {
	if len(actions) == 0 {
		return nil
	}

	counts := make(map[string]int)
	specs := make(map[string]AttributeSpec)
	for _, action := range actions {
		for _, s := range conditionRegistry[action] {
			counts[s.Key]++
			specs[s.Key] = s
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
