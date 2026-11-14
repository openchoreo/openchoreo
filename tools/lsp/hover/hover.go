// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package hover

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	protocol "github.com/tliron/glsp/protocol_3_16"
	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/schema"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/template"
	lsputil "github.com/openchoreo/openchoreo/tools/lsp/lsp-util"
)

//go:embed hover_config.json
var hoverConfigData []byte

// KindInfo holds the description and API reference URL for resource kind.
type KindInfo struct {
	Description string `json:"description"`
	URL         string `json:"url"`
}

var fieldDescriptions map[string]string
var kindDescriptions map[string]KindInfo

var hoverOnce sync.Once
var hoverErr error

// load parses the embedded hover config. It runs lazily on first use and
// reports failures as errors instead of panicking, so a malformed config can
// never crash the server.
func load() error {
	hoverOnce.Do(func() {
		fieldDescriptions, kindDescriptions, hoverErr = parseHoverConfig(hoverConfigData)
	})
	return hoverErr
}

// LoadError returns the error that occurred while parsing the hover config,
// or nil when it parsed successfully (or has not been parsed yet).
func LoadError() error {
	return hoverErr
}

// parseHoverConfig decodes hover_config.json into the field and kind lookup
// tables. It is a pure function so it can be unit-tested without any global
// state.
func parseHoverConfig(data []byte) (map[string]string, map[string]KindInfo, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("failed to parse hover_config.json: %w", err)
	}

	fields := make(map[string]string)
	kinds := make(map[string]KindInfo)

	for key, val := range raw {
		if key == "goToDefinitions" {
			if err := json.Unmarshal(val, &kinds); err != nil {
				return nil, nil, fmt.Errorf("failed to parse goToDefinitions: %w", err)
			}
			continue
		}
		var s string
		if err := json.Unmarshal(val, &s); err == nil {
			fields[key] = s
		}
	}
	return fields, kinds, nil
}

// Handle computes the hover content for the given document text at the
// 1-indexed cursor position. Returns nil when there is nothing to show.
func Handle(docText string, cursorLine, cursorCol int, contexts ...context.Context) *protocol.Hover {
	if err := load(); err != nil {
		return nil
	}

	docs, err := parser.ParseYAML([]byte(docText))
	if err != nil || len(docs) == 0 {
		return nil
	}

	cursorCol = utf16ToRuneColumn(docText, cursorLine, cursorCol)
	document := parser.DocumentForPosition(docs, cursorLine, cursorCol)
	// Parser ranges end at the last scalar header, so a final block scalar
	// can extend beyond that range. Use document starts for those content lines.
	if document == nil && cursorLine > 0 && cursorLine <= len(strings.Split(docText, "\n")) {
		for _, candidate := range docs {
			if candidate.Range.Start.Line <= cursorLine {
				document = candidate
			} else {
				break
			}
		}
	}
	if document == nil {
		return nil
	}

	if len(contexts) > 0 {
		document.ValidationContext = contexts[0]
	}
	if result := expressionHover(document, cursorLine, cursorCol); result != nil {
		return result
	}
	path := lsputil.FindFieldPathAtPosition(document.Root, cursorLine, cursorCol)

	if kindVal, ok := findKindValueAtPosition(document.Root, path, cursorLine, cursorCol); ok {
		if info, exists := kindDescriptions[kindVal]; exists {
			content := buildKindHoverContent(kindVal, info)
			return &protocol.Hover{
				Contents: protocol.MarkupContent{
					Kind:  protocol.MarkupKindMarkdown,
					Value: content,
				},
			}
		}
	}

	schemaRoot := template.SchemaFor(document.Kind)
	if schemaRoot == nil {
		return nil
	}

	node := schema.FindSchema(schemaRoot, path)
	if node == nil {
		return nil
	}

	content := buildHoverContent(node, descriptionPath(schemaRoot, path), document.APIVersion, document.Kind)
	if content == "" {
		return nil
	}

	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.MarkupKindMarkdown,
			Value: content,
		},
	}
}

func buildHoverContent(node *schema.FieldSchema, path string, resource ...string) string {
	if node == nil {
		return ""
	}

	lines := make([]string, 0, 5)

	fieldName := path
	if idx := strings.LastIndex(path, "."); idx >= 0 {
		fieldName = path[idx+1:]
	}

	lines = append(lines, fmt.Sprintf("### `%s`", fieldName))

	if desc := contextDescription(fieldName, path, resource...); desc != "" {
		lines = append(lines, desc)
	}

	lines = append(lines, "")

	if node.Type != "" {
		typeStr := node.Type
		if node.Type == valueObject && node.Properties != nil {
			typeStr = valueObject
		}
		if node.Type == valueArray && node.Items != nil {
			typeStr = valueArray
		}
		lines = append(lines, fmt.Sprintf("**Type:** `%s`", typeStr))
	}

	if node.Required {
		lines = append(lines, "**Status:** Required")
	} else {
		lines = append(lines, "**Status:** Optional")
	}

	if len(node.Enum) > 0 {
		lines = append(lines, "")
		lines = append(lines, "**Allowed values:**")
		for _, v := range node.Enum {
			lines = append(lines, fmt.Sprintf("- `%s`", v))
		}
	}

	if node.Pattern != "" {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("**Pattern:** `%s`", node.Pattern))
	}

	if node.MinLength > 0 || node.MaxLength > 0 {
		lengthStr := ""
		if node.MinLength > 0 && node.MaxLength > 0 {
			lengthStr = fmt.Sprintf("%d-%d characters", node.MinLength, node.MaxLength)
		} else if node.MinLength > 0 {
			lengthStr = fmt.Sprintf("minimum %d characters", node.MinLength)
		} else {
			lengthStr = fmt.Sprintf("maximum %d characters", node.MaxLength)
		}
		lines = append(lines, fmt.Sprintf("**Length:** %s", lengthStr))
	}

	if node.Preserve {
		lines = append(lines, "")
		lines = append(lines, "**preserveUnknownFields:** `true`")
	}

	if node.Type == valueArray && node.Items != nil && node.Items.Properties != nil {
		lines = append(lines, "")
		lines = append(lines, "**Array item fields:**")
		lines = append(lines, "")
		lines = append(lines, "| Field | Type | Required |")
		lines = append(lines, "|-------|------|----------|")
		for childName, child := range node.Items.Properties {
			req := "no"
			if child.Required {
				req = "yes"
			}
			lines = append(lines, fmt.Sprintf("| `%s` | `%s` | %s |", childName, child.Type, req))
		}
	}

	if node.Type == valueObject && node.Properties != nil {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("**Fields (%d):**", len(node.Properties)))
		lines = append(lines, "")
		lines = append(lines, "| Field | Type | Required | Description |")
		lines = append(lines, "|-------|------|----------|-------------|")
		for childName, child := range node.Properties {
			req := "no"
			if child.Required {
				req = "yes"
			}
			childPath := path + "." + childName
			desc := contextDescription(childName, childPath, resource...)
			if desc == "" {
				desc = "-"
			}
			lines = append(lines, fmt.Sprintf("| `%s` | `%s` | %s | %s |", childName, child.Type, req, desc))
		}
	}

	return strings.Join(lines, "\n")
}

func fieldDescription(name, path string) string {
	if desc, ok := fieldDescriptions[name]; ok {
		return desc
	}

	if strings.HasSuffix(path, ".spec") {
		return "Resource-specific configuration."
	}

	return ""
}

// findKindValueAtPosition checks if the cursor is on a kind field's VALUE (not the key).
// Returns the kind value and true if the cursor is on the value side, false otherwise.
func findKindValueAtPosition(node *yaml.Node, path string, cursorLine, cursorCol int) (string, bool) {
	if !strings.HasSuffix(path, "kind") || node == nil {
		return "", false
	}

	valNode, keyEndCol := findKindValuePairAtLine(node, cursorLine)
	if valNode == nil || valNode.Kind != yaml.ScalarNode {
		return "", false
	}

	if cursorCol < keyEndCol {
		return "", false
	}

	return valNode.Value, true
}

// findKindValuePairAtLine finds the kind key node on the given line and returns
// its value node and the column just past the key name (so we can distinguish
// key vs value cursor position).
func findKindValuePairAtLine(node *yaml.Node, line int) (*yaml.Node, int) {
	if node == nil {
		return nil, 0
	}
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) > 0 {
			return findKindValuePairAtLine(node.Content[0], line)
		}
	case yaml.MappingNode:
		return findKindValuePairInMapping(node, line)
	case yaml.SequenceNode:
		for _, item := range node.Content {
			if val, col := findKindValuePairAtLine(item, line); val != nil {
				return val, col
			}
		}
	}
	return nil, 0
}

// findKindValuePairInMapping finds a key named "kind" on the given line and
// returns (valueNode, keyEndCol) where keyEndCol is the column just past the key.
func findKindValuePairInMapping(node *yaml.Node, line int) (*yaml.Node, int) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, 0
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]

		if keyNode.Line == line && keyNode.Kind == yaml.ScalarNode && keyNode.Value == "kind" {
			keyEndCol := keyNode.Column + len(keyNode.Value)
			return valNode, keyEndCol
		}

		if valNode.Kind == yaml.MappingNode || valNode.Kind == yaml.SequenceNode {
			if val, col := findKindValuePairAtLine(valNode, line); val != nil {
				return val, col
			}
		}
	}
	return nil, 0
}

// buildKindHoverContent builds the hover markdown for a resource kind value.
func buildKindHoverContent(kind string, info KindInfo) string {
	lines := make([]string, 0, 5)
	lines = append(lines, fmt.Sprintf("### `%s`", kind))
	lines = append(lines, "")
	lines = append(lines, info.Description)
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("[API Reference](%s)", info.URL))
	return strings.Join(lines, "\n")
}

//go:embed field_descriptions.json
var descriptionData []byte
var scopedDescriptions = func() map[string]map[string]string {
	var out map[string]map[string]string
	_ = json.Unmarshal(descriptionData, &out)
	return out
}()

func contextDescription(name, path string, resource ...string) string {
	if len(resource) < 2 {
		return fieldDescription(name, path)
	}
	fields := scopedDescriptions[resource[0]+"/"+resource[1]]
	if text := fields[path]; text != "" {
		return text
	}
	// Match map-value schema paths without borrowing another field's prose.
	parts := strings.Split(path, ".")
	best, bestSpecific := "", -1
	patterns := make([]string, 0, len(fields))
	for key := range fields {
		patterns = append(patterns, key)
	}
	sort.Strings(patterns)
	for _, pattern := range patterns {
		text := fields[pattern]
		segments := strings.Split(pattern, ".")
		if len(segments) != len(parts) {
			continue
		}
		match, specific := true, 0
		for i, segment := range segments {
			if segment != "*" && segment != parts[i] {
				match = false
				break
			}
			if segment != "*" {
				specific++
			}
		}
		if match && specific > bestSpecific {
			best, bestSpecific = text, specific
		}
	}
	if best != "" {
		return best
	}
	safe := map[string]string{"apiVersion": "API version of this resource.", "kind": "Resource kind.", "metadata": "Resource metadata.", "metadata.name": "Name of this resource.", "metadata.namespace": "Namespace of this resource.", "metadata.labels": "Labels attached to this resource.", "metadata.annotations": "Annotations attached to this resource."}
	return safe[path]
}

func descriptionPath(root *schema.FieldSchema, path string) string {
	parts := strings.Split(path, ".")
	node := root
	for i, part := range parts {
		if node == nil {
			break
		}
		if node.Type == valueArray {
			node = node.Items
		}
		if node == nil {
			break
		}
		node = node.Properties[strings.TrimSuffix(part, "[]")]
		if node != nil && node.Type == valueArray && i < len(parts)-1 {
			parts[i] = strings.TrimSuffix(part, "[]") + "[]"
		}
	}
	return strings.Join(parts, ".")
}
