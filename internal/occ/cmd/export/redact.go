// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"fmt"
	"regexp"
	"sort"
)

var sensitiveName = regexp.MustCompile(
	`(?i)(^|[-_.])(password|passwd|secret|token|api[-_]?key|private[-_]?key|client[-_]?secret|credentials?)([-_.]|$)|(password|passwd|secret|token|api[-_]?key|private[-_]?key|client[-_]?secret|credentials?)$`)

func findPlaintextSecrets(object map[string]any) []string {
	var findings []string
	walk(object, "", &findings)
	sort.Strings(findings)
	return findings
}

func stripPlaintextSecrets(object map[string]any) int {
	return stripValues(object)
}

func stripValues(node any) int {
	stripped := 0
	switch value := node.(type) {
	case map[string]any:
		if key, ok := value["key"].(string); ok && sensitiveName.MatchString(key) {
			if secretValue, ok := value["value"].(string); ok && secretValue != "" {
				delete(value, "value")
				stripped++
			}
		}
		for key, child := range value {
			if secretValue, ok := child.(string); ok && secretValue != "" && key != "key" && sensitiveName.MatchString(key) {
				delete(value, key)
				stripped++
				continue
			}
			stripped += stripValues(child)
		}
	case []any:
		for _, child := range value {
			stripped += stripValues(child)
		}
	}
	return stripped
}

func walk(node any, currentPath string, findings *[]string) {
	switch value := node.(type) {
	case map[string]any:
		if key, ok := value["key"].(string); ok && sensitiveName.MatchString(key) {
			if secretValue, ok := value["value"].(string); ok && secretValue != "" {
				*findings = append(*findings, fmt.Sprintf("%s.value (key %q)", currentPath, key))
			}
		}
		for key, child := range value {
			childPath := currentPath + "." + key
			if text, ok := child.(string); ok && text != "" && key != "key" && sensitiveName.MatchString(key) {
				*findings = append(*findings, childPath)
				continue
			}
			walk(child, childPath, findings)
		}
	case []any:
		for i, child := range value {
			walk(child, fmt.Sprintf("%s[%d]", currentPath, i), findings)
		}
	}
}
