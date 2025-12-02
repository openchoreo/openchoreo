// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// YAMLBuilder constructs yaml.Node trees with comment support.
// Commented fields are tracked explicitly via the commentedNodes map and
// rendered with "# " prefix during encoding.
type YAMLBuilder struct {
	root           *yaml.Node
	current        *yaml.Node          // Current mapping node (for nested navigation)
	stack          []*yaml.Node        // Stack for nested navigation
	commentedNodes map[*yaml.Node]bool // Tracks which key nodes should be commented
}

// NewYAMLBuilder creates a new YAMLBuilder instance.
func NewYAMLBuilder() *YAMLBuilder {
	rootMapping := &yaml.Node{Kind: yaml.MappingNode}
	return &YAMLBuilder{
		root: &yaml.Node{
			Kind:    yaml.DocumentNode,
			Content: []*yaml.Node{rootMapping},
		},
		current:        rootMapping,
		stack:          []*yaml.Node{rootMapping},
		commentedNodes: make(map[*yaml.Node]bool),
	}
}

// AddField adds a key-value pair to the current mapping.
func (b *YAMLBuilder) AddField(key, value string, opts ...FieldOption) *YAMLBuilder {
	keyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: key,
	}

	valueNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: value,
	}

	for _, opt := range opts {
		opt(keyNode, valueNode)
	}

	b.current.Content = append(b.current.Content, keyNode, valueNode)
	return b
}

// AddCommentedField adds a field that will be rendered as a comment (e.g., "# key: value").
func (b *YAMLBuilder) AddCommentedField(key, value string, opts ...FieldOption) *YAMLBuilder {
	keyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: key,
	}

	valueNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: value,
	}

	for _, opt := range opts {
		opt(keyNode, valueNode)
	}

	// Mark this key node as commented
	b.commentedNodes[keyNode] = true

	b.current.Content = append(b.current.Content, keyNode, valueNode)
	return b
}

// AddMapping adds a nested mapping node and returns the builder for chaining.
func (b *YAMLBuilder) AddMapping(key string, opts ...FieldOption) *YAMLBuilder {
	keyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: key,
	}

	valueNode := &yaml.Node{
		Kind: yaml.MappingNode,
	}

	for _, opt := range opts {
		opt(keyNode, valueNode)
	}

	b.current.Content = append(b.current.Content, keyNode, valueNode)
	return b
}

// AddInlineArray adds an array field with inline flow style (e.g., [a, b, c]).
func (b *YAMLBuilder) AddInlineArray(key string, items []any, opts ...FieldOption) *YAMLBuilder {
	keyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: key,
	}

	sequenceNode := &yaml.Node{
		Kind:  yaml.SequenceNode,
		Style: yaml.FlowStyle,
	}

	for _, item := range items {
		sequenceNode.Content = append(sequenceNode.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: formatScalarValue(item),
		})
	}

	for _, opt := range opts {
		opt(keyNode, sequenceNode)
	}

	b.current.Content = append(b.current.Content, keyNode, sequenceNode)
	return b
}

// AddCommentedArray adds a commented array field with block style.
// Each array item is rendered on its own line with "# - " prefix for better readability.
func (b *YAMLBuilder) AddCommentedArray(key string, items []any, opts ...FieldOption) *YAMLBuilder {
	keyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: key,
	}

	sequenceNode := &yaml.Node{
		Kind: yaml.SequenceNode,
		// No Style set - defaults to block style
	}

	for _, item := range items {
		sequenceNode.Content = append(sequenceNode.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: formatScalarValue(item),
		})
	}

	for _, opt := range opts {
		opt(keyNode, sequenceNode)
	}

	// Mark this key node as commented
	b.commentedNodes[keyNode] = true

	b.current.Content = append(b.current.Content, keyNode, sequenceNode)
	return b
}

// AddCommentedInlineArray adds a commented array field with inline flow style (e.g., # key: [a, b]).
// Use this for empty arrays or when inline format is preferred.
func (b *YAMLBuilder) AddCommentedInlineArray(key string, items []any, opts ...FieldOption) *YAMLBuilder {
	keyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: key,
	}

	sequenceNode := &yaml.Node{
		Kind:  yaml.SequenceNode,
		Style: yaml.FlowStyle,
	}

	for _, item := range items {
		sequenceNode.Content = append(sequenceNode.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: formatScalarValue(item),
		})
	}

	for _, opt := range opts {
		opt(keyNode, sequenceNode)
	}

	// Mark this key node as commented
	b.commentedNodes[keyNode] = true

	b.current.Content = append(b.current.Content, keyNode, sequenceNode)
	return b
}

// formatScalarValue formats a scalar value for YAML node.
func formatScalarValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case int, int64:
		return fmt.Sprintf("%d", val)
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%v", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// InMapping navigates into a mapping to add nested fields, then returns to parent.
func (b *YAMLBuilder) InMapping(key string, fn func(*YAMLBuilder), opts ...FieldOption) *YAMLBuilder {
	keyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: key,
	}

	valueNode := &yaml.Node{
		Kind: yaml.MappingNode,
	}

	for _, opt := range opts {
		opt(keyNode, valueNode)
	}

	b.current.Content = append(b.current.Content, keyNode, valueNode)

	// Push current and navigate into new mapping
	b.stack = append(b.stack, b.current)
	b.current = valueNode

	// Execute the nested building function
	fn(b)

	// Pop back to parent
	b.current = b.stack[len(b.stack)-1]
	b.stack = b.stack[:len(b.stack)-1]

	return b
}

// InCommentedMapping creates a commented nested mapping structure.
// The key and all nested content will be rendered as comments.
func (b *YAMLBuilder) InCommentedMapping(key string, fn func(*YAMLBuilder), opts ...FieldOption) *YAMLBuilder {
	keyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: key,
	}

	valueNode := &yaml.Node{
		Kind: yaml.MappingNode,
	}

	for _, opt := range opts {
		opt(keyNode, valueNode)
	}

	// Mark this key node as commented
	b.commentedNodes[keyNode] = true

	b.current.Content = append(b.current.Content, keyNode, valueNode)

	// Push current and navigate into new mapping
	b.stack = append(b.stack, b.current)
	b.current = valueNode

	// Execute the nested building function
	fn(b)

	// Pop back to parent
	b.current = b.stack[len(b.stack)-1]
	b.stack = b.stack[:len(b.stack)-1]

	return b
}

// InCommentedMappingWithFunc creates a commented nested mapping structure.
// This is an alias for InCommentedMapping for API consistency with nested type renderer.
func (b *YAMLBuilder) InCommentedMappingWithFunc(key string, fn func(*YAMLBuilder), opts ...FieldOption) *YAMLBuilder {
	return b.InCommentedMapping(key, fn, opts...)
}

// AddSequence adds a sequence (array) node and returns the sequence node for adding items.
func (b *YAMLBuilder) AddSequence(key string, opts ...FieldOption) *yaml.Node {
	keyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: key,
	}

	valueNode := &yaml.Node{
		Kind: yaml.SequenceNode,
	}

	for _, opt := range opts {
		opt(keyNode, valueNode)
	}

	b.current.Content = append(b.current.Content, keyNode, valueNode)
	return valueNode
}

// AddSequenceMapping adds a mapping item to a sequence node.
func (b *YAMLBuilder) AddSequenceMapping(seq *yaml.Node, fn func(*YAMLBuilder)) *YAMLBuilder {
	itemNode := &yaml.Node{
		Kind: yaml.MappingNode,
	}
	seq.Content = append(seq.Content, itemNode)

	// Push current and navigate into sequence item mapping
	b.stack = append(b.stack, b.current)
	b.current = itemNode

	fn(b)

	// Pop back to parent
	b.current = b.stack[len(b.stack)-1]
	b.stack = b.stack[:len(b.stack)-1]

	return b
}

// AddSequenceScalar adds a scalar item to a sequence node.
func (b *YAMLBuilder) AddSequenceScalar(seq *yaml.Node, value string) *YAMLBuilder {
	itemNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: value,
	}
	seq.Content = append(seq.Content, itemNode)
	return b
}

// AddInlineSequence adds a sequence with flow style (inline: [a, b, c]).
func (b *YAMLBuilder) AddInlineSequence(key string, values []string, opts ...FieldOption) *YAMLBuilder {
	keyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: key,
	}

	seqNode := &yaml.Node{
		Kind:  yaml.SequenceNode,
		Style: yaml.FlowStyle, // Renders as [a, b, c] instead of block style
	}

	for _, v := range values {
		seqNode.Content = append(seqNode.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: v,
		})
	}

	for _, opt := range opts {
		opt(keyNode, seqNode)
	}

	b.current.Content = append(b.current.Content, keyNode, seqNode)
	return b
}

// AddCommentedInlineSequence adds a commented-out inline sequence.
func (b *YAMLBuilder) AddCommentedInlineSequence(key string, values []string, opts ...FieldOption) *YAMLBuilder {
	keyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: key,
	}

	seqNode := &yaml.Node{
		Kind:  yaml.SequenceNode,
		Style: yaml.FlowStyle,
	}

	for _, v := range values {
		seqNode.Content = append(seqNode.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: v,
		})
	}

	for _, opt := range opts {
		opt(keyNode, seqNode)
	}

	// Mark this key node as commented
	b.commentedNodes[keyNode] = true

	b.current.Content = append(b.current.Content, keyNode, seqNode)
	return b
}

// Encode returns the YAML string with commented fields rendered as comments.
func (b *YAMLBuilder) Encode() (string, error) {
	var buf bytes.Buffer

	// Write document head comment (the "# Generated by..." header)
	if b.root.Content[0].HeadComment != "" {
		for _, line := range strings.Split(b.root.Content[0].HeadComment, "\n") {
			buf.WriteString(line)
			buf.WriteString("\n")
		}
	}

	if err := b.encodeNode(&buf, b.root.Content[0], 0); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// encodeNode recursively renders a YAML node tree with comment support.
func (b *YAMLBuilder) encodeNode(buf *bytes.Buffer, node *yaml.Node, indent int) error {
	switch node.Kind {
	case yaml.MappingNode:
		return b.encodeMappingWithContext(buf, node, indent, false)
	case yaml.SequenceNode:
		if node.Style == yaml.FlowStyle {
			return b.encodeFlowSequence(buf, node)
		}
		return b.encodeBlockSequenceWithContext(buf, node, indent, false)
	case yaml.ScalarNode:
		buf.WriteString(node.Value)
		return nil
	default:
		return fmt.Errorf("unsupported node kind: %v", node.Kind)
	}
}

// encodeMappingWithContext renders a mapping node, propagating comment state to children.
func (b *YAMLBuilder) encodeMappingWithContext(buf *bytes.Buffer, node *yaml.Node, indent int, parentCommented bool) error {
	indentStr := strings.Repeat("  ", indent)

	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		// Determine if this key is commented (either directly or inherited from parent)
		isCommented := parentCommented || b.commentedNodes[keyNode]

		// Head comment (appears above the field)
		if keyNode.HeadComment != "" {
			for _, line := range strings.Split(keyNode.HeadComment, "\n") {
				if line == "" {
					// Empty line - just add blank line (no comment prefix)
					buf.WriteString("\n")
				} else {
					buf.WriteString(indentStr)
					buf.WriteString("# ")
					buf.WriteString(line)
					buf.WriteString("\n")
				}
			}
		}

		// Key
		buf.WriteString(indentStr)
		if isCommented {
			buf.WriteString("# ")
		}
		buf.WriteString(keyNode.Value)
		buf.WriteString(":")

		// Value
		if valueNode.Kind == yaml.MappingNode && len(valueNode.Content) > 0 {
			// Line comment for nested mappings appears on the key line
			if valueNode.LineComment != "" {
				buf.WriteString(" # ")
				buf.WriteString(valueNode.LineComment)
			}
			buf.WriteString("\n")
			if err := b.encodeMappingWithContext(buf, valueNode, indent+1, isCommented); err != nil {
				return err
			}
		} else if valueNode.Kind == yaml.SequenceNode {
			if valueNode.Style == yaml.FlowStyle {
				// Inline array: key: [a, b, c]
				buf.WriteString(" ")
				if err := b.encodeFlowSequence(buf, valueNode); err != nil {
					return err
				}
				// Line comment for inline arrays
				if valueNode.LineComment != "" {
					buf.WriteString(" # ")
					buf.WriteString(valueNode.LineComment)
				}
				buf.WriteString("\n")
			} else {
				// Block array: key:\n  - item1\n  - item2
				// Line comment appears on the key line
				if valueNode.LineComment != "" {
					buf.WriteString(" # ")
					buf.WriteString(valueNode.LineComment)
				}
				buf.WriteString("\n")
				if err := b.encodeBlockSequenceWithContext(buf, valueNode, indent+1, isCommented); err != nil {
					return err
				}
			}
		} else if valueNode.Kind == yaml.ScalarNode {
			// Only add space and value if value is non-empty
			if valueNode.Value != "" {
				buf.WriteString(" ")
				buf.WriteString(quoteIfNeeded(valueNode.Value))
			}
			// Line comment for scalars appears on the same line
			if valueNode.LineComment != "" {
				buf.WriteString(" # ")
				buf.WriteString(valueNode.LineComment)
			}
			buf.WriteString("\n")
		} else if valueNode.Kind == yaml.MappingNode && len(valueNode.Content) == 0 {
			// Empty mapping: key: {}
			buf.WriteString(" {}")
			// Line comment for empty mappings
			if valueNode.LineComment != "" {
				buf.WriteString(" # ")
				buf.WriteString(valueNode.LineComment)
			}
			buf.WriteString("\n")
		}
	}

	return nil
}

// encodeBlockSequenceWithContext renders a block-style sequence, propagating comment state.
func (b *YAMLBuilder) encodeBlockSequenceWithContext(buf *bytes.Buffer, node *yaml.Node, indent int, parentCommented bool) error {
	indentStr := strings.Repeat("  ", indent)

	for _, item := range node.Content {
		buf.WriteString(indentStr)
		if parentCommented {
			buf.WriteString("# - ")
		} else {
			buf.WriteString("- ")
		}

		if item.Kind == yaml.MappingNode {
			if len(item.Content) > 0 {
				// First field on same line as dash
				firstKey := item.Content[0]
				firstValue := item.Content[1]

				buf.WriteString(firstKey.Value)
				buf.WriteString(":")

				// Handle first value based on its type
				if firstValue.Kind == yaml.MappingNode && len(firstValue.Content) > 0 {
					// Nested mapping - render on new lines
					if firstValue.LineComment != "" {
						buf.WriteString(" # ")
						buf.WriteString(firstValue.LineComment)
					}
					buf.WriteString("\n")
					if err := b.encodeMappingWithContext(buf, firstValue, indent+2, parentCommented); err != nil {
						return err
					}
				} else if firstValue.Kind == yaml.MappingNode && len(firstValue.Content) == 0 {
					// Empty mapping
					buf.WriteString(" {}")
					if firstValue.LineComment != "" {
						buf.WriteString(" # ")
						buf.WriteString(firstValue.LineComment)
					}
					buf.WriteString("\n")
				} else if firstValue.Kind == yaml.SequenceNode {
					// Array value
					if firstValue.Style == yaml.FlowStyle {
						buf.WriteString(" ")
						if err := b.encodeFlowSequence(buf, firstValue); err != nil {
							return err
						}
						if firstValue.LineComment != "" {
							buf.WriteString(" # ")
							buf.WriteString(firstValue.LineComment)
						}
						buf.WriteString("\n")
					} else {
						if firstValue.LineComment != "" {
							buf.WriteString(" # ")
							buf.WriteString(firstValue.LineComment)
						}
						buf.WriteString("\n")
						if err := b.encodeBlockSequenceWithContext(buf, firstValue, indent+2, parentCommented); err != nil {
							return err
						}
					}
				} else {
					// Scalar value
					buf.WriteString(" ")
					buf.WriteString(quoteIfNeeded(firstValue.Value))
					if firstValue.LineComment != "" {
						buf.WriteString(" # ")
						buf.WriteString(firstValue.LineComment)
					}
					buf.WriteString("\n")
				}

				// Remaining fields indented
				if len(item.Content) > 2 {
					tempMapping := &yaml.Node{
						Kind:    yaml.MappingNode,
						Content: item.Content[2:],
					}
					if err := b.encodeMappingWithContext(buf, tempMapping, indent+1, parentCommented); err != nil {
						return err
					}
				}
			} else {
				buf.WriteString("{}\n")
			}
		} else if item.Kind == yaml.ScalarNode {
			buf.WriteString(quoteIfNeeded(item.Value))
			buf.WriteString("\n")
		}
	}

	return nil
}

// encodeFlowSequence renders an inline flow-style sequence.
func (b *YAMLBuilder) encodeFlowSequence(buf *bytes.Buffer, node *yaml.Node) error {
	buf.WriteString("[")
	for i, item := range node.Content {
		if i > 0 {
			buf.WriteString(", ")
		}
		if item.Kind == yaml.ScalarNode {
			buf.WriteString(item.Value)
		}
	}
	buf.WriteString("]")
	return nil
}

// FieldOption configures comment and style for a field.
type FieldOption func(key, value *yaml.Node)

// WithHeadComment adds a comment above the field.
func WithHeadComment(comment string) FieldOption {
	return func(key, value *yaml.Node) {
		if comment != "" {
			key.HeadComment = comment
		}
	}
}

// WithLineComment adds an inline comment after the value.
func WithLineComment(comment string) FieldOption {
	return func(key, value *yaml.Node) {
		if comment != "" {
			value.LineComment = comment
		}
	}
}

// WithFootComment adds a comment below the field.
func WithFootComment(comment string) FieldOption {
	return func(key, value *yaml.Node) {
		if comment != "" {
			value.FootComment = comment
		}
	}
}

// quoteIfNeeded adds quotes to a string if it needs them for YAML.
// Note: This does NOT quote boolean-like values (true, false, yes, no, etc.) because
// in the scaffolding context, values come from typed schema defaults. If the schema
// has a boolean default, it should remain as a boolean in the output YAML.
func quoteIfNeeded(s string) string {
	if s == "" {
		return s
	}

	// Strings that start with special chars need quoting
	firstChar := s[0]
	if firstChar == '[' || firstChar == '{' || firstChar == '&' || firstChar == '*' ||
		firstChar == '!' || firstChar == '|' || firstChar == '>' || firstChar == '\'' ||
		firstChar == '"' || firstChar == '%' || firstChar == '@' || firstChar == '`' {
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}

	return s
}
