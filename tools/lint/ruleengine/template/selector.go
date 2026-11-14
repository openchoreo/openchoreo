// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	celvalidator "github.com/openchoreo/openchoreo/tools/lint/cel-validator"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine"
)

// celKinds are the kinds whose templates carry ${...} CEL expressions that the
// schema-aware validator type-checks. Cluster variants inherit the same
// context surface as their namespaced counterparts.
var celKinds = map[string]bool{
	celvalidator.KindComponentType:        true,
	celvalidator.KindClusterComponentType: true,
	celvalidator.KindTrait:                true,
	celvalidator.KindClusterTrait:         true,
	celvalidator.KindResourceType:         true,
	celvalidator.KindClusterResourceType:  true,
	celvalidator.KindWorkflow:             true,
	celvalidator.KindClusterWorkflow:      true,
}

// RulesForKind returns the validation rules for a given Kubernetes kind name
// in the active template version. If the kind has no specific rules, it
// returns nil.
func RulesForKind(kind string) []ruleengine.Rule {
	_ = load()

	rules := make([]ruleengine.Rule, 0, 2)
	if ks, ok := kindIndex[kind]; ok {
		rules = append(rules, &SchemaRule{
			name:   ks.ruleName,
			prefix: ks.prefix,
			schema: schemasByVersion[currentVersion()][kind],
		})
	}
	if celKinds[kind] {
		rules = append(rules, &ruleengine.CELRules{})
	}
	if len(rules) == 0 {
		return nil
	}
	return rules
}

// AllKinds returns the names of every kind that has registered rules. It is
// used by tests and tooling to enumerate the full kind surface.
func AllKinds() []string {
	_ = load()
	out := make([]string, len(allKindNames))
	copy(out, allKindNames)
	return out
}
