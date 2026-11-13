// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0
package hover

import (
	"fmt"
	"strings"
	"unicode/utf16"

	celvalidator "github.com/openchoreo/openchoreo/tools/lint/cel-validator"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func expressionHover(doc *parser.DocumentNode, line, column int) *protocol.Hover {
	schemas, err := celvalidator.ExtractSchemas(doc.Spec)
	if err != nil {
		return nil
	}
	info, err := celvalidator.InspectExpression(doc, schemas, line, column)
	if err != nil || info == nil {
		return nil
	}
	text := fmt.Sprintf("### `%s`\n\n**Type:** `%s`", info.Name, info.Type)
	if info.Description != "" {
		text += "\n\n" + info.Description
	}
	if info.Suggestion != "" {
		text += fmt.Sprintf("\n\nUnknown identifier. Did you mean `%s`?", info.Suggestion)
	}
	if info.OpenKeys {
		text += "\n\nMap keys are dynamic; there is no fixed list of valid keys."
	}
	if len(info.Children) > 0 {
		text += fmt.Sprintf("\n\n**Children of `%s` (%d):**\n\n| Field | Type |\n|---|---|", info.ChildrenParent, len(info.Children))
		for _, child := range info.Children {
			text += fmt.Sprintf("\n| `%s` | `%s` |", child.Name, child.Type)
		}
	}
	if len(info.Functions) > 0 {
		text += "\n\n**Available functions:**\n\n| Function | Return type |\n|---|---|"
		for _, fn := range info.Functions {
			text += fmt.Sprintf("\n| `%s` | `%s` |", fn.Name, fn.Type)
		}
	}
	r := protocol.Range{Start: protocol.Position{Line: protocol.UInteger(info.Range.Start.Line - 1), Character: protocol.UInteger(runeToUTF16Column(string(doc.Source), info.Range.Start.Line, info.Range.Start.Column) - 1)}, End: protocol.Position{Line: protocol.UInteger(info.Range.End.Line - 1), Character: protocol.UInteger(runeToUTF16Column(string(doc.Source), info.Range.End.Line, info.Range.End.Column) - 1)}}
	return &protocol.Hover{Contents: protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: text}, Range: &r}
}

func utf16ToRuneColumn(source string, line, column int) int {
	lines := strings.Split(source, "\n")
	if line < 1 || line > len(lines) {
		return column
	}
	units, count := 0, 0
	for _, r := range lines[line-1] {
		if units >= column-1 {
			break
		}
		units += utf16.RuneLen(r)
		count++
	}
	return count + 1
}
func runeToUTF16Column(source string, line, column int) int {
	lines := strings.Split(source, "\n")
	if line < 1 || line > len(lines) {
		return column
	}
	units, count := 0, 0
	for _, r := range lines[line-1] {
		if count >= column-1 {
			break
		}
		units += utf16.RuneLen(r)
		count++
	}
	return units + 1
}
