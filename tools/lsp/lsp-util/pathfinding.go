// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsputil

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// FindPathAtPosition returns the dot-separated path of the mapping context
// at the given cursor position (e.g., "spec.owner" when inside owner's body).
func FindPathAtPosition(node *yaml.Node, line, col int) string {
	if node == nil {
		return ""
	}
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) > 0 {
			return FindPathAtPosition(node.Content[0], line, col)
		}
	case yaml.MappingNode:
		return FindPathInMapping(node, line, col, false)
	case yaml.SequenceNode:
		return FindPathInSequence(node, line, col, false)
	}
	return ""
}

// FindFieldPathAtPosition returns the full dot-separated path including the
// field name at the cursor position (e.g., "metadata.name").
func FindFieldPathAtPosition(node *yaml.Node, line, col int) string {
	if node == nil {
		return ""
	}
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) > 0 {
			return FindFieldPathAtPosition(node.Content[0], line, col)
		}
	case yaml.MappingNode:
		return FindPathInMapping(node, line, col, true)
	case yaml.SequenceNode:
		return FindPathInSequence(node, line, col, true)
	}
	return ""
}

// FindPathInMapping walks a YAML mapping node to determine which key's scope
// contains the cursor. When includeCurrentField is true, it returns the path
// including the field under the cursor; otherwise it returns the parent context.
func FindPathInMapping(node *yaml.Node, line, col int, includeCurrentField bool) string {
	if node == nil || node.Kind != yaml.MappingNode || len(node.Content) < 2 {
		return ""
	}

	startLine := node.Content[0].Line
	endLine := NodeEndLine(node)

	if line < startLine || line > endLine+1 {
		return ""
	}

	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]

		if includeCurrentField && line == keyNode.Line {
			return keyNode.Value
		}

		nextBoundary := endLine + 1
		if i+2 < len(node.Content) {
			nextBoundary = node.Content[i+2].Line
		}

		switch valNode.Kind {
		case yaml.MappingNode:
			if len(valNode.Content) >= 2 {
				nestedStart := valNode.Content[0].Line
				nestedEnd := NodeEndLine(valNode)

				// Cursor inside the nested mapping body.
				if line >= nestedStart && line <= nestedEnd {
					sub := FindPathInMapping(valNode, line, col, includeCurrentField)
					if sub != "" {
						return keyNode.Value + "." + sub
					}
					return keyNode.Value
				}
				// Cursor between key and nested start (blank lines).
				if line > keyNode.Line && line < nestedStart {
					return keyNode.Value
				}
				// Trailing blank line after nested mapping. For non-last keys the
				// boundary must be exclusive so the NEXT key's own line is not
				// attributed to this one; for the last key the fabricated
				// endLine+1 boundary stays inclusive so a trailing blank line
				// still belongs to it.
				if line > nestedEnd {
					if i+2 >= len(node.Content) {
						if line <= nextBoundary {
							return keyNode.Value
						}
					} else if line < nextBoundary {
						return keyNode.Value
					}
				}
			}

		case yaml.SequenceNode:
			if sub := mappingSequencePath(valNode, line, col, includeCurrentField); sub != "" {
				return keyNode.Value + "." + sub
			}

		default:
			// Scalar value — cursor is between this key and the next.
			if includeCurrentField {
				// FindFieldPathAtPosition mode: only match when value is on a different
				// line than the key, so cursor on the NEXT key's line isn't
				// falsely matched as this key's scope.
				if valNode.Line != keyNode.Line && withinScalarScope(line, keyNode.Line, nextBoundary) {
					return keyNode.Value
				}
			} else {
				// FindPathAtPosition mode: match any scalar where cursor is past the key.
				if withinScalarScope(line, keyNode.Line, nextBoundary) {
					isLast := i+2 >= len(node.Content)
					if isLast && line > endLine && valNode.Value != "" {
						continue
					}
					return keyNode.Value
				}
			}
		}
	}

	return ""
}

// FindPathInSequence delegates to FindPathInMapping for each mapping item in a sequence.
func FindPathInSequence(node *yaml.Node, line, col int, includeCurrentField bool) string {
	if node == nil || node.Kind != yaml.SequenceNode {
		return ""
	}
	for _, item := range node.Content {
		if item.Kind == yaml.MappingNode {
			if p := FindPathInMapping(item, line, col, includeCurrentField); p != "" {
				return p
			}
		}
	}
	return ""
}

// NodeEndLine returns the last line occupied by a YAML node.
func NodeEndLine(node *yaml.Node) int {
	if node == nil {
		return 0
	}
	switch node.Kind {
	case yaml.ScalarNode:
		return node.Line + strings.Count(node.Value, "\n")
	case yaml.MappingNode, yaml.SequenceNode:
		maxLine := node.Line
		for _, child := range node.Content {
			if end := NodeEndLine(child); end > maxLine {
				maxLine = end
			}
		}
		return maxLine
	default:
		return node.Line
	}
}

func mappingSequencePath(valNode *yaml.Node, line, col int, includeCurrentField bool) string {
	for _, item := range valNode.Content {
		if item.Kind == yaml.MappingNode && len(item.Content) >= 2 {
			itemStart := item.Content[0].Line
			itemEnd := NodeEndLine(item)
			if line >= itemStart && line <= itemEnd {
				sub := FindPathInMapping(item, line, col, includeCurrentField)
				if sub != "" {
					return sub
				}
			}
		}
	}
	return ""
}

func withinScalarScope(line, keyLine, nextBoundary int) bool {
	return line > keyLine && line <= nextBoundary
}
