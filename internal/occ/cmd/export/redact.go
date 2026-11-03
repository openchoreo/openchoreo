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
	credentials := plaintextCredentials(object)
	var findings []string
	for _, credential := range credentials {
		findings = append(findings, credential.path)
	}
	return findings
}

func stripPlaintextSecrets(object map[string]any) int {
	credentials := plaintextCredentials(object)
	for _, credential := range credentials {
		delete(credential.parent, credential.key)
	}
	return len(credentials)
}

type plaintextCredential struct {
	path   string
	parent map[string]any
	key    string
}

func plaintextCredentials(object map[string]any) []plaintextCredential {
	var findings []plaintextCredential
	walk(object, "", &findings)
	sort.Slice(findings, func(i, j int) bool { return findings[i].path < findings[j].path })
	return findings
}

func walk(node any, currentPath string, findings *[]plaintextCredential) {
	switch value := node.(type) {
	case map[string]any:
		if key, ok := value["key"].(string); ok && isPlaintextCredential(key, value["value"]) {
			*findings = append(*findings, plaintextCredential{
				path: fmt.Sprintf("%s.value (key %q)", currentPath, key), parent: value, key: "value",
			})
		}
		for key, child := range value {
			childPath := currentPath + "." + key
			if key != "key" && isPlaintextCredential(key, child) {
				*findings = append(*findings, plaintextCredential{path: childPath, parent: value, key: key})
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
