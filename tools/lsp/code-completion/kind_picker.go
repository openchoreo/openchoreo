// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0
package codecompletion

import (
	"regexp"
	"strings"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/template"
	protocol "github.com/tliron/glsp/protocol_3_16"
	"gopkg.in/yaml.v3"
)

// rootKindChoices handles the current document's root kind independently of
// parsing a partially typed/unsupported kind. Nested reference kinds continue
// through normal schema enum completion.
func rootKindChoices(text string, line, column int) ([]protocol.CompletionItem, bool) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if line < 1 || line > len(lines) {
		return nil, false
	}
	current := lines[line-1]
	match := regexp.MustCompile(`^kind[ \t]*:`).FindStringIndex(current)
	if match == nil {
		return nil, false
	}
	cursor := column - 1
	if cursor < match[1] || cursor > len(current) {
		return nil, false
	}
	separator := regexp.MustCompile(`^(---|\.\.\.)([ \t]*(#.*)?)?$`)
	start, end := 0, len(lines)
	for i, l := range lines {
		if separator.MatchString(l) {
			if i < line-1 {
				start = i + 1
			} else if i > line-1 {
				end = i
				break
			}
		}
	}
	openChoreo := false
	for _, l := range lines[start:end] {
		if !regexp.MustCompile(`^apiVersion[ \t]*:`).MatchString(l) {
			continue
		}
		var header map[string]string
		if yaml.Unmarshal([]byte(l), &header) == nil && parser.IsOpenChoreoAPIVersion(header["apiVersion"]) {
			openChoreo = true
		}
	}
	if !openChoreo {
		return nil, true
	}
	endValue := len(current)
	if hash := strings.Index(current[match[1]:], "#"); hash >= 0 {
		endValue = match[1] + hash
		if cursor > endValue {
			return nil, true
		}
	}
	startValue := match[1]
	for startValue < endValue && (current[startValue] == ' ' || current[startValue] == '\t') {
		startValue++
	}
	if startValue > cursor {
		startValue = cursor
	}
	var items []protocol.CompletionItem
	for _, name := range template.AllKinds() {
		if template.SchemaFor(name) == nil {
			continue
		}
		replacement := name
		if startValue == match[1] {
			replacement = " " + replacement
		}
		if endValue < len(current) {
			replacement += " "
		}
		completionKind := protocol.CompletionItemKindEnum
		detail := "OpenChoreo resource kind"
		insert, filter, sortKey := replacement, replacement, name
		edit := protocol.TextEdit{Range: protocol.Range{Start: protocol.Position{Line: uint32(line - 1), Character: uint32(startValue)}, End: protocol.Position{Line: uint32(line - 1), Character: uint32(endValue)}}, NewText: replacement}
		items = append(items, protocol.CompletionItem{Label: name, Kind: &completionKind, Detail: &detail, InsertText: &insert, FilterText: &filter, SortText: &sortKey, TextEdit: &edit})
	}
	return items, true
}
