// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsputil

import (
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/schema"
)

// FindKindByFieldName checks if any registered kind name is a substring of the field name.
// This is a standalone, future-proof solution that uses actual registered kinds
// instead of hardcoded prefix/suffix rules.
//
// Examples:
//
//	"allowedWorkflows" contains "Workflow" → "Workflow"
//	"deploymentPipelineRef" contains "DeploymentPipeline" → "DeploymentPipeline"
//	"targetEnvironmentRefs" contains "Environment" → "Environment"
//	"projectName" contains "Project" → "Project"
//	"component" contains "Component" → "Component"
func FindKindByFieldName(fieldName string, registeredKinds []string) string {
	if fieldName == "" || len(registeredKinds) == 0 {
		return ""
	}

	fieldLower := strings.ToLower(fieldName)
	for _, kind := range registeredKinds {
		if strings.Contains(fieldLower, strings.ToLower(kind)) {
			return kind
		}
	}
	return ""
}

// SplitPath splits a normalized YAML path into segments, filtering out empty strings.
func SplitPath(path string) []string {
	if path == "" {
		return nil
	}
	var segments []string
	for _, s := range strings.Split(path, ".") {
		if s != "" {
			segments = append(segments, s)
		}
	}
	return segments
}

// FindNodeAtPosition finds the scalar node at the given line and column.
func FindNodeAtPosition(node *yaml.Node, line, col int) *yaml.Node {
	if node == nil {
		return nil
	}

	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) > 0 {
			return FindNodeAtPosition(node.Content[0], line, col)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]

			if valNode.Kind == yaml.ScalarNode && valNode.Line == line {
				if keyNode.Line == line {
					if col >= keyNode.Column && col <= valNode.Column+len(valNode.Value) {
						return valNode
					}
				} else {
					if col >= valNode.Column && col <= valNode.Column+len(valNode.Value) {
						return valNode
					}
				}
			}

			if found := FindNodeAtPosition(valNode, line, col); found != nil {
				return found
			}
		}
	case yaml.SequenceNode:
		for _, item := range node.Content {
			if found := FindNodeAtPosition(item, line, col); found != nil {
				return found
			}
		}
	}

	return nil
}

// FindParentMappingAndKey finds the mapping node that directly contains the
// given scalar as a value, and returns the key under which it's stored.
// It properly recurses into all nested structures.
func FindParentMappingAndKey(root, target *yaml.Node) (*yaml.Node, string) {
	if root == nil || target == nil {
		return nil, ""
	}

	switch root.Kind {
	case yaml.DocumentNode:
		if len(root.Content) > 0 {
			return FindParentMappingAndKey(root.Content[0], target)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(root.Content); i += 2 {
			keyNode := root.Content[i]
			valNode := root.Content[i+1]

			if valNode == target {
				return root, keyNode.Value
			}

			if m, k := FindParentMappingAndKey(valNode, target); m != nil {
				return m, k
			}
		}
	case yaml.SequenceNode:
		for _, item := range root.Content {
			if m, k := FindParentMappingAndKey(item, target); m != nil {
				return m, k
			}
		}
	case yaml.ScalarNode:
		// Leaf node — target cannot be inside a scalar
	}
	return nil, ""
}

// FindSiblingKey finds a sibling key in the same mapping node.
func FindSiblingKey(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// BuildPathToNode builds the dot-separated path from root to the given node.
func BuildPathToNode(root, target *yaml.Node) string {
	if root == nil || target == nil {
		return ""
	}

	var path []string
	if findPathRecursive(root, target, &path) {
		return strings.Join(path, ".")
	}
	return ""
}

// findPathRecursive builds the path from root to target.
func findPathRecursive(node, target *yaml.Node, path *[]string) bool {
	if node == nil {
		return false
	}

	if node == target {
		return true
	}

	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) > 0 {
			return findPathRecursive(node.Content[0], target, path)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]

			if valNode == target {
				*path = append(*path, keyNode.Value)
				return true
			}

			if findPathRecursive(valNode, target, path) {
				*path = append([]string{keyNode.Value}, *path...)
				return true
			}
		}
	case yaml.SequenceNode:
		for _, item := range node.Content {
			if findPathRecursive(item, target, path) {
				// Append [] to the last path segment (the key before this sequence)
				if len(*path) > 0 {
					(*path)[len(*path)-1] += "[]"
				}
				return true
			}
		}
	case yaml.ScalarNode:
		// Leaf node — target cannot be inside a scalar
	}

	return false
}

// NormalizePath normalizes a YAML path for matching.
// It removes array index markers ([]) and ensures clean dot-separated segments.
func NormalizePath(path string) string {
	path = strings.ReplaceAll(path, "[]", ".")
	// Collapse any double dots created by replacement
	for strings.Contains(path, "..") {
		path = strings.ReplaceAll(path, "..", ".")
	}
	path = strings.Trim(path, ".")
	return path
}

// StringPtr returns a pointer to the given string.
func StringPtr(s string) *string {
	return &s
}

// ExtractKindFromText pulls the value of `kind:` from raw YAML text.
func ExtractKindFromText(text string) string {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "kind:") {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "kind:"))
			if val != "" {
				return val
			}
		}
	}
	return ""
}

// SortedFieldNames returns the object's property names, required fields first
// and then alphabetically, so expansions are deterministic and schema-accurate.
func SortedFieldNames(node *schema.FieldSchema) []string {
	if node == nil {
		return nil
	}
	names := make([]string, 0, len(node.Properties))
	for name := range node.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	sort.SliceStable(names, func(i, j int) bool {
		ri := node.Properties[names[i]].Required
		rj := node.Properties[names[j]].Required
		if ri != rj {
			return ri
		}
		return false
	})
	return names
}

// DummyValue returns a schema-aware placeholder value for a leaf field.
func DummyValue(name string, node *schema.FieldSchema) string {
	if node == nil {
		return ""
	}
	switch node.Type {
	case "string":
		var val string
		if node.Default != "" {
			val = node.Default
		} else if len(node.Enum) > 0 {
			val = node.Enum[0]
		} else {
			val = dummyString(name, node)
		}
		return YAMLSafeString(val)
	case "integer", "number":
		if node.Default != "" {
			return node.Default
		}
		return "1"
	case "boolean":
		if node.Default != "" {
			return node.Default
		}
		return "true"
	default:
		return ""
	}
}

// YAMLSafeString quotes a produced string value when YAML would otherwise
// reinterpret it as a boolean or null scalar (e.g. True, yes, null).
func YAMLSafeString(s string) string {
	lower := strings.ToLower(s)
	switch lower {
	case "true", "false", "yes", "no", "on", "off", "null", "~", "":
		return `"` + s + `"`
	}
	return s
}

// dummyString picks a plausible placeholder for a string field.
func dummyString(name string, node *schema.FieldSchema) string {
	switch name {
	case "apiVersion":
		return "openchoreo.dev/v1alpha1"
	case "namespace":
		return "default"
	case "name":
		return "default"
	case "kind":
		return "Resource"
	case "hash":
		return "a1b2c3d4e5f60718"
	case "uid":
		return "12345678-1234-1234-1234-123456789abc"
	case "lastTransitionTime", "creationTimestamp", "time":
		return "2026-08-01T00:00:00Z"
	case "message":
		return "Resource is reconciled"
	case "reason":
		return "Reconciled"
	case "type":
		return "Ready"
	case "version", "revision":
		return "v1"
	case "email":
		return "user@example.com"
	case "url", "uri", "endpoint":
		return "https://example.com"
	case "status":
		if len(node.Enum) > 0 {
			return node.Enum[0]
		}
		return "Ready"
	default:
		return "example"
	}
}

// ExpandObjectBody expands an object's children, each indented under the key.
func ExpandObjectBody(name string, node *schema.FieldSchema, indent string) string {
	if node == nil {
		return "\n" + indent
	}
	if node.Preserve {
		return preserveBody(name, indent)
	}
	names := SortedFieldNames(node)
	if len(names) == 0 {
		return "\n" + indent
	}
	var b strings.Builder
	for _, n := range names {
		child := node.Properties[n]
		b.WriteString("\n" + indent)
		switch child.Type {
		case "object":
			b.WriteString(n + ":" + ExpandObjectBody(n, child, indent+"  "))
		case "array":
			b.WriteString(n + ":" + ExpandArrayBody(child, indent+"  ", 1))
		default:
			b.WriteString(n + ": " + DummyValue(n, child))
		}
	}
	return b.String()
}

// preserveBody returns a representative example entry for a free-form map.
func preserveBody(name string, indent string) string {
	var b strings.Builder
	entryIndent := indent
	switch name {
	case "annotations":
		b.WriteString("\n" + entryIndent + "openchoreo.dev/description: Your project description\n")
		b.WriteString(entryIndent + "openchoreo.dev/display-name: My Project")
	case "labels":
		b.WriteString("\n" + entryIndent + "openchoreo.dev/name: default")
	default:
		b.WriteString("\n" + entryIndent + "key: value")
	}
	return b.String()
}

// ExpandArrayBody expands an array with a single sample item.
func ExpandArrayBody(node *schema.FieldSchema, indent string, depth int) string {
	if node == nil || depth > 3 {
		return "\n" + indent + "- "
	}
	item := node.Items
	if item == nil {
		return "\n" + indent + "- "
	}
	childIndent := indent + "  "
	switch item.Type {
	case "object":
		return "\n" + indent + "-" + ExpandObjectBody("item", item, childIndent)
	case "array":
		return "\n" + indent + "-" + ExpandArrayBody(item, childIndent, depth+1)
	default:
		return "\n" + indent + "- " + DummyValue("item", item)
	}
}
