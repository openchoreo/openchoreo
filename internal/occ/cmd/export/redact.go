// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var sensitiveName = regexp.MustCompile(
	`(?i)(^|[-_.])(password|passwd|secret|token|api[-_]?key|private[-_]?key|client[-_]?secret|credentials?)([-_.]|$)|(password|passwd|secret|token|api[-_]?key|private[-_]?key|client[-_]?secret|credentials?)$`)

// Only direct references are exempt. Expressions containing literal defaults,
// function calls, or surrounding text may still contain plaintext credentials.
var credentialReference = regexp.MustCompile(
	`^(\{\{\s*\.[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*\s*\}\}|` +
		`\$\{\s*[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*|\[('[^'\r\n]+'|"[^"\r\n]+"|[0-9]+)\])*\s*\})$`)

func isPlaintextCredential(name string, value any) bool {
	text, ok := value.(string)
	return ok && text != "" && sensitiveName.MatchString(name) &&
		!credentialReference.MatchString(strings.TrimSpace(text))
}

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
		if key, ok := value["key"].(string); ok && isPlaintextCredential(key, value["value"]) {
			delete(value, "value")
			stripped++
		}
		for key, child := range value {
			if key != "key" && isPlaintextCredential(key, child) {
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
		if key, ok := value["key"].(string); ok && isPlaintextCredential(key, value["value"]) {
			*findings = append(*findings, fmt.Sprintf("%s.value (key %q)", currentPath, key))
		}
		for key, child := range value {
			childPath := currentPath + "." + key
			if key != "key" && isPlaintextCredential(key, child) {
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
