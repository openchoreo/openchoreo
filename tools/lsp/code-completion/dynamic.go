// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package codecompletion

import (
	"encoding/json"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
	"gopkg.in/yaml.v3"

	celvalidator "github.com/openchoreo/openchoreo/tools/lint/cel-validator"
	"github.com/openchoreo/openchoreo/tools/lint/indexer"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/schema"
)

func dynamicItems(text string, line, col int, idx ResourceIndexer, uri string, resolved ...func(string, *schema.FieldSchema)) ([]protocol.CompletionItem, bool) {
	workspace, ok := idx.(interface {
		Snapshot(string) []indexer.SnapshotDocument
	})
	if !ok {
		return nil, false
	}
	originalText, originalLine := text, line
	doc, text, line, ok := dynamicDocument(text, line)
	if !ok {
		return nil, false
	}
	ctx := resolveContext(text, line, col)
	if ctx.mode == modeFields && strings.TrimSpace(strings.Split(text, "\n")[line-1]) == "" {
		_, path, ok := previewMappingContext(doc.Root, ctx.path, strings.Split(text, "\n"), line-1)
		if !ok {
			return nil, false
		}
		ctx.path = path
	}

	rootPath, definition, environment := dynamicRelationship(doc, ctx.path, line, workspace.Snapshot(uri))
	if definition == nil || rootPath == "" {
		return nil, false
	}
	parent := rootPath[:strings.LastIndex(rootPath, ".")]
	if ctx.path != rootPath && !strings.HasPrefix(ctx.path, rootPath+".") && !(len(resolved) > 0 && ctx.path == parent) {
		return nil, false
	}
	pair, err := celvalidator.ExtractSchemas(definition)
	if err != nil {
		return nil, false
	}
	structural := pair.Parameters
	if environment {
		structural = pair.EnvironmentConfigs
	}
	if structural == nil {
		return nil, false
	}
	data, err := json.Marshal(structural.ToKubeOpenAPI())
	if err != nil {
		return nil, false
	}
	var raw map[string]any
	if json.Unmarshal(data, &raw) != nil {
		return nil, false
	}
	adaptDynamicSchema(raw)
	data, _ = json.Marshal(raw)
	fields, err := schema.Parse(data)
	if err != nil {
		return nil, false
	}
	if len(resolved) > 0 {
		resolved[0](rootPath, fields)
		return nil, true
	}
	absolutePath := ctx.path
	ctx.path = strings.TrimPrefix(strings.TrimPrefix(ctx.path, rootPath), ".")
	schemaPath := ctx.path
	ctx.text, ctx.line = originalText, originalLine
	items := suggest(ctx, fields, nil)
	for i := range items {
		path := schemaPath
		if ctx.mode != modeValue || schema.FindSchema(fields, path) == nil || schema.FindSchema(fields, path).Type == valueObject {
			path = joinSchemaPath(path, items[i].Label)
		}
		if n := dynamicRawSchema(raw, path); n != nil {
			if desc, ok := n["description"].(string); ok {
				items[i].Documentation = desc
			}
		}
	}
	if ctx.mode == modeFields || ctx.mode == modeArrayItem {
		existing := dynamicAtCursor(doc.Spec, strings.TrimPrefix(absolutePath, "spec."), line)
		keys := map[string]bool{}
		if existing != nil && existing.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(existing.Content); i += 2 {
				keys[existing.Content[i].Value] = true
			}
		}
		items = filterExistingItems(items, keys)
	}
	return items, true
}
func dynamicNode(n *yaml.Node, key string) *yaml.Node {
	if n == nil {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

// Adapt JSON Schema required arrays to completion's per-property flags.
func adaptDynamicSchema(n map[string]any) {
	props, _ := n["properties"].(map[string]any)
	required, _ := n["required"].([]any)
	delete(n, "required")
	for _, name := range required {
		if child, ok := props[name.(string)].(map[string]any); ok {
			child["required"] = true
		}
	}
	for _, v := range props {
		if child, ok := v.(map[string]any); ok {
			flag := child["required"]
			adaptDynamicSchema(child)
			if flag == true {
				child["required"] = true
			}
		}
	}
	if item, ok := n["items"].(map[string]any); ok {
		adaptDynamicSchema(item)
	}
}

func dynamicRawSchema(n map[string]any, path string) map[string]any {
	if path == "" {
		return n
	}
	for _, part := range strings.Split(path, ".") {
		if item, ok := n["items"].(map[string]any); ok {
			n = item
		}
		props, _ := n["properties"].(map[string]any)
		n, _ = props[part].(map[string]any)
		if n == nil {
			return nil
		}
	}
	return n
}

func dynamicAtCursor(n *yaml.Node, path string, line int) *yaml.Node {
	for _, part := range strings.Split(path, ".") {
		if n != nil && n.Kind == yaml.SequenceNode {
			n = selectedInstance(n, line)
		}
		n = dynamicNode(n, part)
	}
	if n != nil && n.Kind == yaml.SequenceNode {
		n = selectedInstance(n, line)
	}
	return n
}

func dynamicDocument(text string, line int) (*parser.DocumentNode, string, int, bool) {
	// Limit both schema context and parsing to the cursor's YAML document.
	lines := strings.Split(text, "\n")
	if line < 1 || line > len(lines) {
		return nil, "", 0, false
	}
	start, end := 0, len(lines)
	for i := 0; i < line-1; i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			start = i + 1
		}
	}
	for i := line; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	line -= start
	text = strings.Join(lines[start:end], "\n")
	docs, err := parser.ParseYAML([]byte(text))
	if err != nil || len(docs) == 0 {
		// A partially typed key is safe to omit for reference extraction.
		local := strings.Split(text, "\n")
		local[line-1] = strings.Repeat(" ", len(local[line-1])-len(strings.TrimLeft(local[line-1], " ")))
		docs, err = parser.ParseYAML([]byte(strings.Join(local, "\n")))
	}
	if err != nil || len(docs) != 1 {
		return nil, "", 0, false
	}
	doc := docs[0]
	if !parser.IsOpenChoreoAPIVersion(doc.APIVersion) {
		return nil, "", 0, false
	}

	return doc, text, line, true
}
