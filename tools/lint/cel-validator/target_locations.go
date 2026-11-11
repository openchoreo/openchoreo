// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0
package celvalidator

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"gopkg.in/yaml.v3"
)

var undeclaredToken = regexp.MustCompile(`undeclared reference to '([^']+)'`)

// Recover a unique erroneous identifier directly from its scalar's source.
// This preserves coordinates across YAML quoting, escaped quotes and folded
// lines instead of assuming decoded scalar offsets equal source offsets.
func (c *checker) exactScalarToken(node *yaml.Node, token string) (parser.Range, bool) {
	if node.Line < 1 || node.Line > len(c.lines) {
		return parser.Range{}, false
	}
	startLine, startColumn := node.Line, node.Column
	var raw string
	if isBlockScalar(node) {
		startLine = node.Line + 1
		startColumn = 1
		end := node.Line
		for end < len(c.lines) {
			line := c.lines[end]
			if strings.TrimSpace(line) != "" && c.lineIndent(end+1) <= c.lineIndent(node.Line) {
				break
			}
			end++
		}
		raw = strings.Join(c.lines[node.Line:end], "\n")
	} else {
		line := c.lines[node.Line-1]
		byteColumn := 0
		for i := 0; i < node.Column-1 && byteColumn < len(line); i++ {
			_, size := utf8.DecodeRuneInString(line[byteColumn:])
			byteColumn += size
		}
		raw = line[byteColumn:]
		if node.Style == yaml.SingleQuotedStyle || node.Style == yaml.DoubleQuotedStyle {
			raw = strings.Join(append([]string{raw}, c.lines[node.Line:]...), "\n")
			quote := byte('\'')
			if node.Style == yaml.DoubleQuotedStyle {
				quote = '"'
			}
			if len(raw) == 0 || raw[0] != quote {
				return parser.Range{}, false
			}
			end := 1
			for end < len(raw) {
				if quote == '"' && raw[end] == '\\' {
					end += 2
					continue
				}
				if raw[end] == quote {
					if quote == '\'' && end+1 < len(raw) && raw[end+1] == quote {
						end += 2
						continue
					}
					break
				}
				end++
			}
			if end >= len(raw) {
				return parser.Range{}, false
			}
			raw = raw[1:end]
			startColumn++
		}
	}
	matches := regexp.MustCompile(`\b`+regexp.QuoteMeta(token)+`\b`).FindAllStringIndex(raw, -1)
	if len(matches) != 1 {
		return parser.Range{}, false
	}
	before := raw[:matches[0][0]]
	line := startLine + strings.Count(before, "\n")
	column := startColumn + utf8.RuneCountInString(before)
	if newline := strings.LastIndex(before, "\n"); newline >= 0 {
		column = 1 + utf8.RuneCountInString(before[newline+1:])
	}
	start := parser.Position{Line: line, Column: column}
	end := start
	end.Column += utf8.RuneCountInString(token)
	return parser.Range{Start: start, End: end}, true
}
