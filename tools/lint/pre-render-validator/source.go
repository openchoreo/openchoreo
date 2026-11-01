// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schemavalidator

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

// position returns the line and column of a YAML node.
func position(n *yaml.Node) (int, int) {
	if n == nil {
		return 1, 1
	}
	return n.Line, n.Column
}

// findKeyNode walks a YAML mapping node depth-first and returns the key node
// with the given name.
func findKeyNode(root *yaml.Node, key string) *yaml.Node {
	if root == nil {
		return nil
	}
	if root.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(root.Content); i += 2 {
			k := root.Content[i]
			if k.Value == key {
				return k
			}
			if n := findKeyNode(root.Content[i+1], key); n != nil {
				return n
			}
		}
		return nil
	}
	for _, child := range root.Content {
		if n := findKeyNode(child, key); n != nil {
			return n
		}
	}
	return nil
}

// findTraitParametersKey returns the key node for parameters at a specific
// trait index under spec.traits.
func findTraitParametersKey(doc *parser.DocumentNode, traitIndex int) *yaml.Node {
	if doc == nil || doc.Root == nil {
		return nil
	}
	spec := findMappingValue(doc.Root, "spec")
	if spec == nil {
		return nil
	}
	traits := findMappingValue(spec, "traits")
	if traits == nil || traits.Kind != yaml.SequenceNode {
		return nil
	}
	if traitIndex < 0 || traitIndex >= len(traits.Content) {
		return nil
	}
	trait := traits.Content[traitIndex]
	if trait == nil || trait.Kind != yaml.MappingNode {
		return nil
	}
	return findKeyInMapping(trait, "parameters")
}

func findMappingValue(root *yaml.Node, key string) *yaml.Node {
	if root == nil {
		return nil
	}
	if root.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(root.Content); i += 2 {
			if root.Content[i].Value == key {
				return root.Content[i+1]
			}
		}
		return nil
	}
	for _, child := range root.Content {
		if n := findMappingValue(child, key); n != nil {
			return n
		}
	}
	return nil
}

func findKeyInMapping(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i]
		}
	}
	return nil
}

// fallbackLineColumn returns 1,1 as a safe fallback.
func fallbackLineColumn() (int, int) {
	return 1, 1
}

// componentNameFromDoc returns the component name for error messages.
func componentNameFromDoc(doc *parser.DocumentNode) string {
	if doc == nil {
		return ""
	}
	if doc.Metadata.Name != "" {
		return doc.Metadata.Name
	}
	return ""
}

func stringsContains(s, substr string) bool {
	return strings.Contains(s, substr)
}
