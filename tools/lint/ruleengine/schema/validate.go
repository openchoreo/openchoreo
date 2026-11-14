// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine"
)

// Validate checks a YAML document root against a schema and returns
// diagnostics. prefix is used as the root of the diagnostic path (e.g.
// "component" produces codes like "missing-component.spec").
func Validate(root *yaml.Node, s *FieldSchema, prefix string) ruleengine.Diagnostics {
	if s == nil {
		return nil
	}
	if root == nil {
		if s.Required {
			return ruleengine.Diagnostics{{
				Range:    parser.Range{},
				Severity: ruleengine.SeverityError,
				Code:     "missing-" + prefix,
				Message:  fmt.Sprintf("%s is required", prefix),
			}}
		}
		return nil
	}
	return validateNode(root, s, prefix)
}

// validateNode checks a yaml.Node against a FieldSchema and returns
// diagnostics. path is the dot-separated field path for error messages.
func validateNode(node *yaml.Node, schema *FieldSchema, path string) ruleengine.Diagnostics {
	if schema == nil {
		return nil
	}
	if node == nil {
		if schema.Required {
			return ruleengine.Diagnostics{{
				Range:    parser.Range{},
				Severity: ruleengine.SeverityError,
				Code:     "missing-" + path,
				Message:  fmt.Sprintf("%s is required", path),
			}}
		}
		return nil
	}

	var diags ruleengine.Diagnostics

	switch node.Kind {
	case yaml.MappingNode:
		diags = append(diags, validateMapping(node, schema, path)...)
		if schema.Type == valueString || schema.Type == valueInteger || schema.Type == valueBoolean {
			diags = append(diags, typeMismatch(node, schema.Type, "a mapping", path))
		}
	case yaml.ScalarNode:
		diags = append(diags, validateScalar(node, schema, path)...)
		if schema.Type == valueObject || schema.Type == valueArray {
			diags = append(diags, typeMismatch(node, schema.Type, "a scalar", path))
		}
	case yaml.SequenceNode:
		diags = append(diags, validateSequence(node, schema, path)...)
		if schema.Type == valueString || schema.Type == valueInteger || schema.Type == valueBoolean || schema.Type == valueObject {
			diags = append(diags, typeMismatch(node, schema.Type, "a sequence", path))
		}
	}

	return diags
}

// validateMapping checks all keys in a mapping against the schema properties.
func validateMapping(node *yaml.Node, schema *FieldSchema, path string) ruleengine.Diagnostics {
	var diags ruleengine.Diagnostics

	type entry struct {
		key *yaml.Node
		val *yaml.Node
	}
	present := make(map[string]entry)
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		if k.Kind != yaml.ScalarNode {
			continue
		}
		if _, exists := present[k.Value]; exists {
			diags = append(diags, ruleengine.Diagnostic{
				Range:    nodeRange(k),
				Severity: ruleengine.SeverityError,
				Code:     "duplicate-key",
				Message:  fmt.Sprintf("duplicate key: %s", k.Value),
			})
			continue
		}
		present[k.Value] = entry{key: k, val: v}
	}

	// Check each schema field.
	for fieldName, fieldSchema := range schema.Properties {
		fieldPath := path
		if fieldPath != "" {
			fieldPath += "."
		}
		fieldPath += fieldName

		e, exists := present[fieldName]
		if !exists {
			if fieldSchema.Required {
				diags = append(diags, ruleengine.Diagnostic{
					Range:    ruleengine.MissingFieldRange(node),
					Severity: ruleengine.SeverityError,
					Code:     "missing-" + fieldPath,
					Message:  fmt.Sprintf("%s is required", fieldPath),
				})
			}
			continue
		}

		// Preserve-unknown-fields: skip children validation.
		if fieldSchema.Preserve {
			continue
		}

		diags = append(diags, validateNode(e.val, fieldSchema, fieldPath)...)
	}

	// Check for unknown fields (only when schema defines properties).
	if len(schema.Properties) > 0 {
		for key, e := range present {
			if _, ok := schema.Properties[key]; !ok {
				alreadyReported := false
				for _, d := range diags {
					if d.Code == "unknown-field" && d.Message == "unknown field: "+key {
						alreadyReported = true
						break
					}
				}
				if !alreadyReported {
					diags = append(diags, ruleengine.Diagnostic{
						Range:    nodeRange(e.key),
						Severity: ruleengine.SeverityError,
						Code:     "unknown-field",
						Message:  "unknown field: " + key,
					})
				}
			}
		}
	}

	return diags
}

// validateScalar checks a scalar value against schema constraints.
func validateScalar(node *yaml.Node, schema *FieldSchema, path string) ruleengine.Diagnostics {
	if node.Kind != yaml.ScalarNode {
		return nil
	}

	var diags ruleengine.Diagnostics
	val := node.Value

	// Required field with an empty or null value is effectively missing.
	if schema.Required && (node.Tag == "!!null" || strings.TrimSpace(val) == "") {
		return append(diags, ruleengine.Diagnostic{
			Range:    nodeRange(node),
			Severity: ruleengine.SeverityError,
			Code:     "empty-" + path,
			Message:  fmt.Sprintf("%s must not be empty", path),
		})
	}

	// String type check: YAML resolves unquoted scalars like numbers or
	// booleans to non-string tags; timestamps are still valid strings.
	if schema.Type == valueString && node.Tag != "" && node.Tag != "!!str" && node.Tag != "!!timestamp" {
		diags = append(diags, ruleengine.Diagnostic{
			Range:    nodeRange(node),
			Severity: ruleengine.SeverityError,
			Code:     "invalid-" + path,
			Message:  fmt.Sprintf("%s must be a string, got %s", path, strings.TrimPrefix(node.Tag, "!!")),
		})
	}

	// Boolean type check: only YAML-native booleans are accepted.
	if schema.Type == valueBoolean && node.Tag != "!!bool" {
		diags = append(diags, ruleengine.Diagnostic{
			Range:    nodeRange(node),
			Severity: ruleengine.SeverityError,
			Code:     "invalid-" + path,
			Message:  fmt.Sprintf("%s must be a boolean, got %s", path, strings.TrimPrefix(node.Tag, "!!")),
		})
	}

	// Integer type check: only YAML-native integers are accepted.
	if schema.Type == valueInteger {
		if _, err := strconv.Atoi(val); err != nil {
			diags = append(diags, ruleengine.Diagnostic{
				Range:    nodeRange(node),
				Severity: ruleengine.SeverityError,
				Code:     "invalid-" + path,
				Message:  fmt.Sprintf("%s must be an integer", path),
			})
		}
	}

	// Enum check.
	if len(schema.Enum) > 0 {
		valid := false
		for _, e := range schema.Enum {
			if val == e {
				valid = true
				break
			}
		}
		if !valid {
			diags = append(diags, ruleengine.Diagnostic{
				Range:    nodeRange(node),
				Severity: ruleengine.SeverityError,
				Code:     "invalid-" + path,
				Message:  fmt.Sprintf("%s must be one of %v, got %q", path, schema.Enum, val),
			})
		}
	}

	// Pattern check.
	if schema.Pattern != "" {
		re, err := regexp.Compile(schema.Pattern)
		if err == nil && !re.MatchString(val) {
			diags = append(diags, ruleengine.Diagnostic{
				Range:    nodeRange(node),
				Severity: ruleengine.SeverityError,
				Code:     "invalid-" + path,
				Message:  fmt.Sprintf("%s must match pattern %q, got %q", path, schema.Pattern, val),
			})
		}
	}

	// MinLength check.
	if schema.MinLength > 0 && len(val) < schema.MinLength {
		diags = append(diags, ruleengine.Diagnostic{
			Range:    nodeRange(node),
			Severity: ruleengine.SeverityError,
			Code:     "invalid-" + path,
			Message:  fmt.Sprintf("%s must be at least %d characters, got %d", path, schema.MinLength, len(val)),
		})
	}

	// MaxLength check.
	if schema.MaxLength > 0 && len(val) > schema.MaxLength {
		diags = append(diags, ruleengine.Diagnostic{
			Range:    nodeRange(node),
			Severity: ruleengine.SeverityError,
			Code:     "invalid-" + path,
			Message:  fmt.Sprintf("%s must be at most %d characters, got %d", path, schema.MaxLength, len(val)),
		})
	}

	return diags
}

// validateSequence checks array elements against the schema.Items schema.
func validateSequence(node *yaml.Node, schema *FieldSchema, path string) ruleengine.Diagnostics {
	var diags ruleengine.Diagnostics

	// A required array with no elements is effectively missing.
	if schema.Required && len(node.Content) == 0 {
		diags = append(diags, ruleengine.Diagnostic{
			Range:    nodeRange(node),
			Severity: ruleengine.SeverityError,
			Code:     "empty-" + path,
			Message:  fmt.Sprintf("%s must not be empty", path),
		})
	}

	if schema.Items == nil {
		return diags
	}

	for i, item := range node.Content {
		elemPath := fmt.Sprintf("%s[%d]", path, i)
		diags = append(diags, validateNode(item, schema.Items, elemPath)...)
	}
	return diags
}

func nodeRange(node *yaml.Node) parser.Range {
	if node == nil {
		return parser.Range{}
	}
	return parser.Range{
		Start: parser.Position{Line: node.Line, Column: node.Column},
		End:   endPosition(node),
	}
}

// typeMismatch reports that a node's YAML kind does not match the expected schema type.
func typeMismatch(node *yaml.Node, schemaType, got string, path string) ruleengine.Diagnostic {
	expected := "a scalar"
	switch schemaType {
	case valueObject:
		expected = "a mapping"
	case valueArray:
		expected = "a sequence"
	}
	return ruleengine.Diagnostic{
		Range:    nodeRange(node),
		Severity: ruleengine.SeverityError,
		Code:     "invalid-" + path,
		Message:  fmt.Sprintf("%s must be %s, got %s", path, expected, got),
	}
}

func endPosition(node *yaml.Node) parser.Position {
	if node == nil {
		return parser.Position{}
	}
	switch node.Kind {
	case yaml.ScalarNode:
		return parser.Position{Line: node.Line, Column: node.Column}
	case yaml.MappingNode, yaml.SequenceNode:
		if len(node.Content) == 0 {
			return parser.Position{Line: node.Line, Column: node.Column}
		}
		return endPosition(node.Content[len(node.Content)-1])
	default:
		return parser.Position{Line: node.Line, Column: node.Column}
	}
}
