// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package ruleengine

import (
	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

// RequiredFieldsRules validates that mandatory top-level fields are present.
type RequiredFieldsRules struct{}

func (r *RequiredFieldsRules) Name() string { return "required-fields" }

func (r *RequiredFieldsRules) Evaluate(doc *parser.DocumentNode) Diagnostics {
	var diags Diagnostics

	if doc.APIVersion == "" {
		diags = append(diags, Diagnostic{
			Range:    PointRange(doc.Range.Start),
			Severity: SeverityError,
			Code:     "missing-apiVersion",
			Message:  "apiVersion is required",
		})
	}

	if doc.Kind == "" {
		diags = append(diags, Diagnostic{
			Range:    PointRange(doc.Range.Start),
			Severity: SeverityError,
			Code:     "missing-kind",
			Message:  "kind is required",
		})
	}

	if doc.Metadata.Name == "" {
		r := PointRange(doc.Range.Start)
		if metaField := doc.GetField("metadata"); metaField != nil {
			if nameKey := findKeyNode(metaField.ValueNode, "name"); nameKey != nil {
				r = nodeRange(nameKey)
			} else {
				r = MissingFieldRange(metaField.ValueNode)
			}
		}
		diags = append(diags, Diagnostic{
			Range:    r,
			Severity: SeverityError,
			Code:     "missing-metadata-name",
			Message:  "metadata.name is required",
		})
	}

	return diags
}

// findKeyNode searches a mapping node for the given key and returns its key node.
func findKeyNode(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i]
		if k.Kind == yaml.ScalarNode && k.Value == key {
			return k
		}
	}
	return nil
}
