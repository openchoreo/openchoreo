// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clivalidator

import (
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/template"
	"github.com/openchoreo/openchoreo/tools/lint/targetschema"
)

// DocumentResult holds the validation outcome for a single YAML document.
type DocumentResult struct {
	Kind        string
	Diagnostics ruleengine.Diagnostics
}

// ValidateYAML parses the given YAML text and runs the common rules plus the
// kind-specific rules for each document's kind. Text that does not declare an
// OpenChoreo apiVersion is ignored entirely (no results, no documents).
func ValidateYAML(data []byte, resolvers ...targetschema.Resolver) ([]DocumentResult, error) {
	if !parser.HasOpenChoreoAPIVersion(data) {
		return nil, nil
	}

	docs, err := parser.ParseYAML(data)
	if err != nil {
		return nil, err
	}

	if len(resolvers) > 0 && resolvers[0] != nil {
		for _, doc := range docs {
			doc.ValidationContext = targetschema.WithResolver(doc.ValidationContext, resolvers[0])
		}
	}
	return ValidateDocuments(docs), nil
}

// ValidateDocuments preserves RuleEngine behavior while passing immutable
// request-scoped dependencies through the existing parsed documents.
func ValidateDocuments(docs []*parser.DocumentNode) []DocumentResult {
	results := make([]DocumentResult, 0, len(docs))
	for _, doc := range docs {
		e := ruleengine.NewEngine()
		if kindRules := template.RulesForKind(doc.Kind); len(kindRules) > 0 {
			e.AddKindRules(doc.Kind, kindRules)
		} else if inferred := template.InferKind(doc); inferred != "" {
			// Unrecognized kind: still validate deeply against the closest
			// matching schema so all issues surface, not just unknown-kind.
			for _, r := range template.RulesForKind(inferred) {
				e.AddRule(r)
			}
		}

		diagnostics := e.Evaluate(doc)
		for _, warning := range targetschema.Warnings(targetschema.FromContext(doc.ValidationContext)) {
			diagnostics = append(diagnostics, ruleengine.Diagnostic{Range: ruleengine.PointRange(doc.Range.Start), Severity: ruleengine.SeverityWarning, Code: "cel-schema-catalog-error", Message: warning.Path + ": " + warning.Message})
		}
		results = append(results, DocumentResult{
			Kind:        doc.Kind,
			Diagnostics: diagnostics,
		})
	}

	return results
}
