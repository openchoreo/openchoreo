// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package ruleengine

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

// ReferenceRules validates cross-references between resources.
// It checks kind+name reference pairs like:
//   - spec.componentType.kind + spec.componentType.name
//   - spec.workflow.kind + spec.workflow.name
//   - spec.deploymentPipelineRef.name
//   - spec.type.kind + spec.type.name
//
// Cross-document resolution is handled by the indexer; this rule
// validates structural correctness of the references.
type ReferenceRules struct{}

func (r *ReferenceRules) Name() string { return "references" }

func (r *ReferenceRules) Evaluate(doc *parser.DocumentNode) Diagnostics {
	var diags Diagnostics

	diags = append(diags, r.validateKindNameRef(doc, "componentType")...)
	diags = append(diags, r.validateKindNameRef(doc, "workflow")...)
	diags = append(diags, r.validateKindNameRef(doc, "type")...)
	diags = append(diags, r.validateNameRef(doc, "deploymentPipelineRef")...)

	return diags
}

// validateKindNameRef checks that a nested kind+name reference has both
// fields present and that the kind is a recognized resource kind.
func (r *ReferenceRules) validateKindNameRef(doc *parser.DocumentNode, field string) Diagnostics {
	if doc.Spec == nil || doc.Spec.Kind != yaml.MappingNode {
		return nil
	}

	refNode := findInMapping(doc.Spec, field)
	if refNode == nil || refNode.Kind != yaml.MappingNode {
		return nil
	}

	var diags Diagnostics

	kindNode := findInMapping(refNode, "kind")
	nameNode := findInMapping(refNode, "name")

	if kindNode == nil || kindNode.Kind != yaml.ScalarNode {
		diags = append(diags, Diagnostic{
			Range:    nodeRange(refNode),
			Severity: SeverityError,
			Code:     "missing-ref-kind",
			Message:  fmt.Sprintf("spec.%s.kind is required", field),
		})
	} else if !parser.KnownKinds[kindNode.Value] {
		diags = append(diags, Diagnostic{
			Range:    nodeRange(kindNode),
			Severity: SeverityWarning,
			Code:     "unknown-ref-kind",
			Message:  fmt.Sprintf("spec.%s.kind %q is not a recognized OpenChoreo resource kind", field, kindNode.Value),
		})
	}

	if nameNode == nil || nameNode.Kind != yaml.ScalarNode || nameNode.Value == "" {
		diags = append(diags, Diagnostic{
			Range:    nodeRange(refNode),
			Severity: SeverityError,
			Code:     "missing-ref-name",
			Message:  fmt.Sprintf("spec.%s.name is required", field),
		})
	}

	return diags
}

// validateNameRef checks that a name-only reference has a non-empty value.
func (r *ReferenceRules) validateNameRef(doc *parser.DocumentNode, field string) Diagnostics {
	if doc.Spec == nil || doc.Spec.Kind != yaml.MappingNode {
		return nil
	}

	refNode := findInMapping(doc.Spec, field)
	if refNode == nil || refNode.Kind != yaml.MappingNode {
		return nil
	}

	nameNode := findInMapping(refNode, "name")

	if nameNode == nil || nameNode.Kind != yaml.ScalarNode || nameNode.Value == "" {
		return Diagnostics{{
			Range:    nodeRange(refNode),
			Severity: SeverityError,
			Code:     "missing-ref-name",
			Message:  fmt.Sprintf("spec.%s.name is required", field),
		}}
	}

	return nil
}

// findInMapping searches a yaml.MappingNode for a nested key.
// If parent is empty, it searches the node directly.
func findInMapping(node *yaml.Node, key string) *yaml.Node {
	parent := ""

	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}

	target := node
	if parent != "" {
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == parent {
				target = node.Content[i+1]
				break
			}
		}
		if target == node {
			return nil
		}
	}

	for i := 0; i+1 < len(target.Content); i += 2 {
		if target.Content[i].Value == key {
			return target.Content[i+1]
		}
	}
	return nil
}
