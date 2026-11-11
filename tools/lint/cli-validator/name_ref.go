// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clivalidator

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine"
	"github.com/openchoreo/openchoreo/tools/lint/targetschema"
	"gopkg.in/yaml.v3"
)

// referenceFields are the spec fields whose mapping (or sequence of mappings)
// carries a *reference* `name` pointing at another OpenChoreo resource. Only
// `name` values nested under these fields are checked for existence, so that
// non-reference `name` keys (container names, ports, workflow parameters, etc.)
// are not falsely flagged.
var referenceFields = map[string]bool{
	"allowedWorkflows":      true,
	"allowedTraits":         true,
	"deploymentPipelineRef": true,
	"componentType":         true,
	"workflow":              true,
	"type":                  true,
}

// ValidateYAMLWithNames is like ValidateYAML but additionally emits
// "location-not-found" warnings for name references that do not exist in the
// given set of known resource names.
func ValidateYAMLWithNames(data []byte, names map[string]bool, resolvers ...targetschema.Resolver) ([]DocumentResult, error) {
	results, err := ValidateYAML(data, resolvers...)
	if err != nil {
		return nil, err
	}

	docs, perr := parser.ParseYAML(data)
	if perr != nil {
		return results, nil
	}

	for i := range results {
		if i < len(docs) && docs[i] != nil {
			results[i].Diagnostics = append(results[i].Diagnostics, CheckNameReferences(docs[i], names)...)
		}
	}

	return results, nil
}

// ResourceNames returns the set of all resource names defined across the YAML
// files under the given folder. When recursive is true, subfolders are scanned.
func ResourceNames(folder string, recursive bool) (map[string]bool, error) {
	names := map[string]bool{}

	files, err := collectFiles(folder, recursive)
	if err != nil {
		return nil, err
	}

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if !parser.HasOpenChoreoAPIVersion(data) {
			continue
		}
		docs, err := parser.ParseYAML(data)
		if err != nil {
			continue
		}
		for _, doc := range docs {
			if doc.Metadata.Name != "" {
				names[doc.Metadata.Name] = true
				// Component types are referenced by their composite
				// "{workloadType}/{name}" form, so index that too.
				if doc.Kind == "ComponentType" || doc.Kind == "ClusterComponentType" {
					if wt := workloadTypeOf(doc); wt != "" {
						names[wt+"/"+doc.Metadata.Name] = true
					}
				}
			}
		}
	}

	return names, nil
}

// workloadTypeOf reads the spec.workloadType scalar of a ComponentType resource.
func workloadTypeOf(doc *parser.DocumentNode) string {
	if doc == nil || doc.Spec == nil || doc.Spec.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(doc.Spec.Content); i += 2 {
		if doc.Spec.Content[i].Value == "workloadType" &&
			doc.Spec.Content[i+1].Kind == yaml.ScalarNode {
			return doc.Spec.Content[i+1].Value
		}
	}
	return ""
}

// collectFiles returns the .yaml/.yml files under the given folder (optionally
// recursive into subfolders).
func collectFiles(folder string, recursive bool) ([]string, error) {
	if recursive {
		var files []string
		err := filepath.WalkDir(folder, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".yaml" || ext == ".yml" {
				files = append(files, path)
			}
			return nil
		})
		return files, err
	}

	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, d := range entries {
		if d.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext == ".yaml" || ext == ".yml" {
			files = append(files, filepath.Join(folder, d.Name()))
		}
	}
	return files, nil
}

// CheckNameReferences walks a document's YAML tree and emits a "location-not-found"
// warning for every reference `name` value (nested under a known reference field)
// that does not exist in the given set of known resource names. The resource's
// own top-level metadata.name is skipped.
func CheckNameReferences(doc *parser.DocumentNode, names map[string]bool) ruleengine.Diagnostics {
	if doc == nil || doc.Root == nil {
		return nil
	}

	var diags ruleengine.Diagnostics
	walkForNameRefs(doc.Root, doc, "", names, &diags)
	return diags
}

// walkForNameRefs recursively walks the YAML tree. `selfKey` is the mapping key
// (or sequence element key) that produced the current node; a `name` value is a
// reference when its enclosing mapping's selfKey is a known reference field.
func walkForNameRefs(
	node *yaml.Node,
	doc *parser.DocumentNode,
	selfKey string,
	names map[string]bool,
	diags *ruleengine.Diagnostics,
) {
	if node == nil {
		return
	}

	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) > 0 {
			walkForNameRefs(node.Content[0], doc, selfKey, names, diags)
		}
	case yaml.MappingNode:
		// If this mapping is itself a reference object (its key is a known ref
		// field), its direct "name" child is a resource reference.
		if referenceFields[selfKey] {
			for i := 0; i+1 < len(node.Content); i += 2 {
				keyNode := node.Content[i]
				valNode := node.Content[i+1]
				if keyNode.Value == "name" && valNode.Kind == yaml.ScalarNode {
					value := valNode.Value
					if value != "" &&
						!strings.HasPrefix(value, "$") &&
						!isOwnMetadataName(doc, keyNode, valNode) &&
						!names[value] {
						*diags = append(*diags, ruleengine.Diagnostic{
							Range: ruleengine.PointRange(parser.Position{
								Line:   valNode.Line,
								Column: valNode.Column,
							}),
							Severity: ruleengine.SeverityWarning,
							Code:     "location-not-found",
							Message:  "no resource with name \"" + value + "\" was found in the workspace",
						})
					}
				}
			}
		}

		// Recurse into children, carrying each child's key as its selfKey.
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			walkForNameRefs(valNode, doc, keyNode.Value, names, diags)
		}
	case yaml.SequenceNode:
		// Sequence items are mappings that inherit the sequence's key as selfKey
		// (e.g. allowedWorkflows[].name).
		for _, item := range node.Content {
			walkForNameRefs(item, doc, selfKey, names, diags)
		}
	}
}

// isOwnMetadataName reports whether the given name key/value is the resource's
// own top-level metadata.name (the definition), as opposed to a reference.
func isOwnMetadataName(doc *parser.DocumentNode, keyNode, valNode *yaml.Node) bool {
	if doc.Metadata.Node == nil || doc.Metadata.Name == "" {
		return false
	}
	if valNode.Value != doc.Metadata.Name {
		return false
	}
	return parentMappingOf(doc.Root, keyNode) == doc.Metadata.Node
}

// parentMappingOf returns the mapping node that directly contains keyNode.
func parentMappingOf(root, keyNode *yaml.Node) *yaml.Node {
	if root == nil || keyNode == nil {
		return nil
	}

	switch root.Kind {
	case yaml.DocumentNode:
		if len(root.Content) > 0 {
			return parentMappingOf(root.Content[0], keyNode)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(root.Content); i += 2 {
			if root.Content[i] == keyNode {
				return root
			}
			if m := parentMappingOf(root.Content[i+1], keyNode); m != nil {
				return m
			}
		}
	case yaml.SequenceNode:
		for _, item := range root.Content {
			if m := parentMappingOf(item, keyNode); m != nil {
				return m
			}
		}
	}
	return nil
}
