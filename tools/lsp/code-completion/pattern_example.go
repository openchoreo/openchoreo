// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0
package codecompletion

import (
	"regexp"
	"regexp/syntax"
	"strings"
)

// patternExample constructs a bounded example from a template's regex. It is
// an authoring hint, not resource-name resolution or a second validator.
func patternExample(pattern string) (string, bool) {
	tree, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return "", false
	}
	var build func(*syntax.Regexp, int) (string, bool)
	build = func(node *syntax.Regexp, depth int) (string, bool) {
		if depth > 32 {
			return "", false
		}
		switch node.Op {
		case syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText, syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary:
			return "", true
		case syntax.OpLiteral:
			return string(node.Rune), true
		case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
			return "a", true
		case syntax.OpCharClass:
			for _, candidate := range []rune{'a', '0', 'A', '-', '_'} {
				for i := 0; i+1 < len(node.Rune); i += 2 {
					if candidate >= node.Rune[i] && candidate <= node.Rune[i+1] {
						return string(candidate), true
					}
				}
			}
			for i := 0; i+1 < len(node.Rune); i += 2 {
				if node.Rune[i] >= 32 && node.Rune[i] <= 126 {
					return string(node.Rune[i]), true
				}
			}
			return "", false
		case syntax.OpCapture:
			return build(node.Sub[0], depth+1)
		case syntax.OpAlternate:
			for _, sub := range node.Sub {
				if value, ok := build(sub, depth+1); ok {
					return value, true
				}
			}
			return "", false
		case syntax.OpConcat:
			var result strings.Builder
			for _, sub := range node.Sub {
				value, ok := build(sub, depth+1)
				if !ok || result.Len()+len(value) > 512 {
					return "", false
				}
				result.WriteString(value)
			}
			return result.String(), true
		case syntax.OpStar, syntax.OpQuest:
			return "", true
		case syntax.OpPlus, syntax.OpRepeat:
			n := 1
			if node.Op == syntax.OpRepeat {
				n = node.Min
			}
			if n > 64 {
				return "", false
			}
			value, ok := build(node.Sub[0], depth+1)
			if !ok || len(value)*n > 512 {
				return "", false
			}
			return strings.Repeat(value, n), true
		}
		return "", false
	}
	value, ok := build(tree, 0)
	if !ok || value == "" {
		return "", false
	}
	compiled, err := regexp.Compile(pattern)
	return value, err == nil && compiled.MatchString(value)
}
