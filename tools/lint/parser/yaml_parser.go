// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// missingDashHint rewrites a YAML parse error into a clearer message when the
// cause is a removed '-' (sequence indicator). This happens when a mapping key
// that was previously a sequence item loses its dash, leaving the following,
// deeper-indented keys dangling under an already-completed scalar value.
func missingDashHint(data []byte, err error) error {
	if !strings.Contains(err.Error(), "mapping values are not allowed in this context") {
		return err
	}

	lines := strings.Split(string(data), "\n")
	for i := 0; i+1 < len(lines); i++ {
		a := strings.TrimRight(lines[i], "\r")
		b := strings.TrimRight(lines[i+1], "\r")
		ta := strings.TrimLeft(a, " ")
		tb := strings.TrimLeft(b, " ")
		if ta == "" || tb == "" || strings.HasPrefix(ta, "#") || strings.HasPrefix(tb, "#") {
			continue
		}
		if strings.HasPrefix(ta, "- ") || strings.HasPrefix(ta, "|") || strings.HasPrefix(ta, ">") {
			continue
		}

		// Line a must be "key: value" with an inline (non-empty) value, so the
		// mapping entry is complete and deeper-indented keys cannot belong to it.
		colonA := strings.Index(ta, ":")
		if colonA <= 0 {
			continue
		}
		inline := strings.TrimSpace(ta[colonA+1:])
		if inline == "" || strings.HasPrefix(inline, "|") || strings.HasPrefix(inline, ">") {
			continue
		}

		// Line b must be a deeper-indented key line without a sequence dash.
		indentA := len(a) - len(ta)
		indentB := len(b) - len(tb)
		if indentB <= indentA {
			continue
		}
		colonB := strings.Index(tb, ":")
		if colonB <= 0 || strings.HasPrefix(tb, "- ") {
			continue
		}

		keyA := strings.TrimSpace(ta[:colonA])
		return fmt.Errorf("yaml: line %d: missing '-' sign (sequence indicator): expected %q to be a list item, e.g. \"- %s: ...\"", i+1, keyA, keyA)
	}
	return err
}

// ParseYAML decodes raw YAML bytes into a list of DocumentNode ASTs,
// preserving full positional information for every node.
func ParseYAML(data []byte) ([]*DocumentNode, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))

	var docs []*DocumentNode
	idx := 0
	for {
		var node yaml.Node
		err := decoder.Decode(&node)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if len(docs) == 0 {
				return nil, missingDashHint(data, err)
			}
			break
		}
		if node.Kind == 0 {
			break
		}

		doc := buildDocumentNode(&node, idx)
		if doc == nil {
			continue
		}

		doc.Source = data
		docs = append(docs, doc)
		idx++
	}

	return docs, nil
}

// buildDocumentNode converts a raw yaml.Node (expected to be a MappingNode)
// into a DocumentNode with extracted fields and positions.
func buildDocumentNode(node *yaml.Node, docIndex int) *DocumentNode {
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) == 0 {
			return nil
		}
		node = node.Content[0]
	}

	if node.Kind != yaml.MappingNode {
		return nil
	}

	doc := &DocumentNode{
		Root:          node,
		DocumentIndex: docIndex,
		Fields:        make(map[string]*Field),
		Range: Range{
			Start: nodePosition(node),
			End:   nodeEndPosition(node),
		},
	}

	fields := extractFields(node)
	for i := range fields {
		f := &fields[i]
		doc.Fields[f.Key] = f

		switch f.Key {
		case "apiVersion":
			doc.APIVersion = scalarValue(f.ValueNode)
		case "kind":
			doc.Kind = scalarValue(f.ValueNode)
		case "metadata":
			doc.Metadata = buildMetadataNode(f.ValueNode)
		case "spec":
			doc.Spec = f.ValueNode
		case "status":
			doc.Status = f.ValueNode
		}
	}

	return doc
}

// buildMetadataNode extracts name, namespace, labels, and annotations
// from a metadata mapping node.
func buildMetadataNode(node *yaml.Node) MetadataNode {
	m := MetadataNode{Node: node}
	if node == nil || node.Kind != yaml.MappingNode {
		return m
	}

	for _, f := range extractFields(node) {
		switch f.Key {
		case "name":
			m.Name = scalarValue(f.ValueNode)
		case "namespace":
			m.Namespace = scalarValue(f.ValueNode)
		case "labels":
			m.Labels = f.ValueNode
		case "annotations":
			m.Annotations = f.ValueNode
		}
	}

	return m
}

// extractFields returns all key-value pairs from a YAML mapping node.
func extractFields(node *yaml.Node) []Field {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}

	var fields []Field
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]

		fields = append(fields, Field{
			Key:       keyNode.Value,
			KeyNode:   keyNode,
			ValueNode: valNode,
			Range: Range{
				Start: nodePosition(keyNode),
				End:   nodeEndPosition(valNode),
			},
		})
	}

	return fields
}

// scalarValue extracts the string value from a scalar yaml.Node.
// Returns "" if the node is nil or not a scalar.
func scalarValue(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.ScalarNode {
		return ""
	}
	return node.Value
}

// nodePosition returns the Position of a yaml.Node.
func nodePosition(node *yaml.Node) Position {
	if node == nil {
		return Position{}
	}
	return Position{
		Line:   node.Line,
		Column: node.Column,
	}
}

// nodeEndPosition computes the end position of a node.
// For scalar nodes, this is the start position (single token).
// For mapping/sequence nodes, we look at the last child.
// For document nodes, we recurse into Content[0].
func nodeEndPosition(node *yaml.Node) Position {
	if node == nil {
		return Position{}
	}

	switch node.Kind {
	case yaml.ScalarNode:
		return nodePosition(node)
	case yaml.MappingNode, yaml.SequenceNode:
		if len(node.Content) == 0 {
			return nodePosition(node)
		}
		return nodeEndPosition(node.Content[len(node.Content)-1])
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return nodePosition(node)
		}
		return nodeEndPosition(node.Content[0])
	case yaml.AliasNode:
		if node.Alias != nil {
			return nodeEndPosition(node.Alias)
		}
		return nodePosition(node)
	default:
		return nodePosition(node)
	}
}
