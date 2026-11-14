// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package ruleengine

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

const (
	// CodeNegativeValue reports a numeric field that must not be negative
	// carrying a negative value (for example a replica count or port number).
	CodeNegativeValue = "negative-value"
	// CodeInvalidInteger reports a field that must be an integer holding a
	// value of some other type (float, string, boolean, list, mapping, ...).
	CodeInvalidInteger = "invalid-integer"
)

// integerFields lists parameter keys whose values must be non-negative
// integers. These appear in literal parameter blocks such as a Component's
// spec.parameters:
//
//	parameters:
//	  replicas: 1
//	  port: 80
//
// Adding a field here extends the rule without further changes.
var integerFields = map[string]bool{
	"replicas": true,
	"port":     true,
}

// RuntimeRules performs logical value checks on runtime parameter fields:
// known fields such as replica counts and port numbers must be plain,
// non-negative integers. The check only inspects literal parameters blocks
// (the concrete values a user writes), never schema definitions or template
// expressions, so it does not fire on a ComponentType's openAPIV3Schema or on
// ${...} template references.
type RuntimeRules struct{}

func (r *RuntimeRules) Name() string { return "runtime-rules" }

// Evaluate inspects the document's spec for invalid values on known
// non-negative integer fields.
func (r *RuntimeRules) Evaluate(doc *parser.DocumentNode) Diagnostics {
	if doc.Spec == nil {
		return nil
	}

	var diags Diagnostics
	r.walk(doc.Spec, false, &diags)
	return diags
}

// walk recurses through a yaml.Node tree. inParameters reports whether node
// sits inside a literal parameters block, which is where concrete,
// user-supplied values live as opposed to schema definitions or templates.
func (r *RuntimeRules) walk(node *yaml.Node, inParameters bool, diags *Diagnostics) {
	if node == nil {
		return
	}

	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			val := node.Content[i+1]

			if inParameters && integerFields[key.Value] {
				checkIntegerField(key.Value, val, diags)
			}

			childInParameters := inParameters
			if key.Value == "parameters" && isLiteralParameters(val) {
				childInParameters = true
			}
			r.walk(val, childInParameters, diags)
		}
	case yaml.SequenceNode, yaml.DocumentNode:
		for _, child := range node.Content {
			r.walk(child, inParameters, diags)
		}
	}
}

// isLiteralParameters reports whether node is a parameters mapping holding
// concrete values rather than an OpenAPI schema definition (which carries an
// openAPIV3Schema child).
func isLiteralParameters(node *yaml.Node) bool {
	if node == nil || node.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == "openAPIV3Schema" {
			return false
		}
	}
	return true
}

// checkIntegerField validates that a non-negative integer field holds a plain
// integer and is not negative.
func checkIntegerField(field string, node *yaml.Node, diags *Diagnostics) {
	if node.Kind != yaml.ScalarNode {
		*diags = append(*diags, Diagnostic{
			Range:    nodeRange(node),
			Severity: SeverityError,
			Code:     CodeInvalidInteger,
			Message:  fmt.Sprintf("%s must be an integer, got %s", field, describeKind(node)),
		})
		return
	}

	// Allow templated expressions; their value is resolved at render time.
	if strings.Contains(node.Value, "${") {
		return
	}

	if node.Tag != "!!int" {
		*diags = append(*diags, Diagnostic{
			Range:    nodeRange(node),
			Severity: SeverityError,
			Code:     CodeInvalidInteger,
			Message:  fmt.Sprintf("%s must be an integer, got %s", field, describeScalar(node)),
		})
		return
	}

	n, err := strconv.ParseInt(node.Value, 0, 64)
	if err != nil {
		n, err = strconv.ParseInt(node.Value, 10, 64)
	}
	if err != nil {
		*diags = append(*diags, Diagnostic{
			Range:    nodeRange(node),
			Severity: SeverityError,
			Code:     CodeInvalidInteger,
			Message:  fmt.Sprintf("%s must be an integer, got %q", field, node.Value),
		})
		return
	}

	if n < 0 {
		*diags = append(*diags, Diagnostic{
			Range:    nodeRange(node),
			Severity: SeverityError,
			Code:     CodeNegativeValue,
			Message:  fmt.Sprintf("%s cannot be a negative value, got %d", field, n),
		})
	}
}

// describeScalar renders a scalar node's value for a diagnostic message.
func describeScalar(node *yaml.Node) string {
	switch node.Tag {
	case "!!null":
		return "null"
	case "!!str":
		return strconv.Quote(node.Value)
	case "!!bool", "!!float":
		return node.Value
	default:
		return strconv.Quote(node.Value)
	}
}

// describeKind renders a non-scalar node's kind for a diagnostic message.
func describeKind(node *yaml.Node) string {
	switch node.Kind {
	case yaml.MappingNode:
		return "a mapping"
	case yaml.SequenceNode:
		return "a list"
	default:
		return "a non-integer value"
	}
}
