// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type plaintextCredential struct {
	path   string
	parent map[string]any
	key    string
}

var sensitiveName = regexp.MustCompile(
	`(?i)(^|[-_.])(password|passwd|secret|token|api[-_]?key|private[-_]?key|client[-_]?secret|credentials?)([-_.]|$)|(password|passwd|secret|token|api[-_]?key|private[-_]?key|client[-_]?secret|credentials?)$`)

// credentialReference matches direct template references without literal defaults or surrounding text.
var credentialReference = regexp.MustCompile(
	`^(\{\{\s*\.[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*\s*\}\}|` +
		`\$\{\s*[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*|\[('[^'\r\n]+'|"[^"\r\n]+"|[0-9]+)\])*\s*\})$`)

var resourceBindingPath = regexp.MustCompile(`^\.dependencies\.resources\[[0-9]+\]\.(envBindings|fileBindings)$`)

func isPlaintextCredential(name string, value any) bool {
	text, ok := value.(string)
	return ok && text != "" && sensitiveName.MatchString(name) &&
		!credentialReference.MatchString(strings.TrimSpace(text))
}

// findPlaintextSecrets returns sorted locations of possible plaintext credentials.
func findPlaintextSecrets(object map[string]any) []string {
	credentials := plaintextCredentials(object)
	var findings []string
	for _, credential := range credentials {
		findings = append(findings, credential.path)
	}
	return findings
}

// stripPlaintextSecrets removes detected credential values and returns the count.
func stripPlaintextSecrets(object map[string]any) int {
	credentials := plaintextCredentials(object)
	for _, credential := range credentials {
		delete(credential.parent, credential.key)
	}
	return len(credentials)
}

func plaintextCredentials(object map[string]any) []plaintextCredential {
	var findings []plaintextCredential
	kind, _ := object["kind"].(string)
	walk(object, kind, "", &findings)
	sort.Slice(findings, func(i, j int) bool { return findings[i].path < findings[j].path })
	return findings
}

// isWorkloadResourceBinding identifies fields containing environment variable names
// or mount paths. Other resource kinds and arbitrary parameters must still be inspected.
func isWorkloadResourceBinding(kind, currentPath string) bool {
	var prefix string
	switch kind {
	case "Workload":
		prefix = ".spec"
	case KindComponentRelease:
		prefix = ".spec.workload"
	default:
		return false
	}
	relativePath, ok := strings.CutPrefix(currentPath, prefix)
	return ok && resourceBindingPath.MatchString(relativePath)
}

func walk(node any, kind, currentPath string, findings *[]plaintextCredential) {
	if isWorkloadResourceBinding(kind, currentPath) {
		return
	}
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
			walk(child, kind, childPath, findings)
		}
	case []any:
		for i, child := range value {
			walk(child, kind, fmt.Sprintf("%s[%d]", currentPath, i), findings)
		}
	}
}
