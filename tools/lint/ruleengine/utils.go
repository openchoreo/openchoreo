// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package ruleengine

import (
	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

// PointRange returns a zero-length range at the given position.
func PointRange(pos parser.Position) parser.Range {
	return parser.Range{Start: pos, End: pos}
}

// MissingFieldRange returns a zero-length range anchored at a node's start,
// so missing-field diagnostics underline the location instead of spanning
// the whole mapping/document block.
func MissingFieldRange(node *yaml.Node) parser.Range {
	if node == nil {
		return parser.Range{}
	}
	return PointRange(parser.Position{Line: node.Line, Column: node.Column})
}

// nodeRange returns the parser.Range for a yaml.Node.
func nodeRange(node *yaml.Node) parser.Range {
	if node == nil {
		return parser.Range{}
	}
	return parser.Range{
		Start: parser.Position{Line: node.Line, Column: node.Column},
		End:   endPosition(node),
	}
}

// endPosition computes the end position of a yaml.Node.
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
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return parser.Position{Line: node.Line, Column: node.Column}
		}
		return endPosition(node.Content[0])
	case yaml.AliasNode:
		if node.Alias != nil {
			return endPosition(node.Alias)
		}
		return parser.Position{Line: node.Line, Column: node.Column}
	default:
		return parser.Position{Line: node.Line, Column: node.Column}
	}
}
