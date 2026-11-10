// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

type plaintextCredential struct {
	path   string
	redact func()
}

var sensitiveName = regexp.MustCompile(
	`(?i)(^|[-_.])(password|passwd|pass|pwd|passphrase|secret|token|api[-_]?key|access[-_]?key|private[-_]?key|signing[-_]?key|client[-_]?secret|credentials?|auth[-_]?config)([-_.]|$)|(password|passwd|passphrase|secret|token|api[-_]?key|access[-_]?key|private[-_]?key|signing[-_]?key|client[-_]?secret|credentials?)$`)

// credentialReference matches direct template references without literal defaults or surrounding text.
var credentialReference = regexp.MustCompile(
	`^(\{\{\s*\.[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*\s*\}\}|` +
		`\$\{\s*[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*|\[('[^'\r\n]+'|"[^"\r\n]+"|[0-9]+)\])*\s*\})$`)

var resourceBindingPath = regexp.MustCompile(`^\.dependencies\.resources\[[0-9]+\]\.(envBindings|fileBindings)$`)

var connectionAttributeName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9 _.-]*$`)

var keywordDSNName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func isPlaintextCredential(name string, value any) bool {
	text, ok := value.(string)
	if !ok || text == "" || credentialReference.MatchString(strings.TrimSpace(text)) {
		return false
	}
	_, hasEmbeddedCredentials := redactEmbeddedCredentials(text)
	return sensitiveName.MatchString(name) || hasEmbeddedCredentials
}

func redactEmbeddedCredentials(value any) (string, bool) {
	if redacted, ok := redactURLUserinfo(value); ok {
		return redacted, true
	}
	return redactConnectionString(value)
}

// redactConnectionString clears literal credential attributes while retaining
// the other attributes and delimiters in semicolon and keyword/value formats.
func redactConnectionString(value any) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	if redacted, found, valid := redactKeywordDSN(text); valid {
		return redacted, found
	}
	var redacted strings.Builder
	last := 0
	changed := false
	for start := 0; start < len(text); {
		remaining := text[start:]
		if strings.TrimSpace(remaining) == "" {
			break
		}
		separator := strings.IndexByte(remaining, ';')
		if separator >= 0 && strings.TrimSpace(remaining[:separator]) == "" {
			start += separator + 1
			continue
		}
		equals := strings.IndexByte(remaining, '=')
		if equals < 0 || (separator >= 0 && separator < equals) {
			return "", false
		}
		key := strings.TrimSpace(remaining[:equals])
		if !connectionAttributeName.MatchString(key) {
			return "", false
		}
		valueStart := start + equals + 1
		end := connectionValueEnd(text, valueStart)
		if (sensitiveName.MatchString(key) || strings.EqualFold(key, "pwd")) && literalConnectionValue(text[valueStart:end]) {
			redacted.WriteString(text[last:valueStart])
			last = end
			changed = true
		}
		start = end + 1
	}
	if !changed {
		return "", false
	}
	redacted.WriteString(text[last:])
	return redacted.String(), true
}

// The third result distinguishes a recognized DSN without literal credentials
// from other formats, so reference-only DSNs do not fall back to semicolon parsing.
func redactKeywordDSN(text string) (string, bool, bool) {
	var redacted strings.Builder
	last := 0
	found := false
	attributes := 0
	for start := 0; start < len(text); {
		start = skipDSNSpace(text, start)
		if start == len(text) {
			break
		}
		equals := strings.IndexByte(text[start:], '=')
		if equals < 0 {
			return "", false, false
		}
		key := strings.TrimSpace(text[start : start+equals])
		if !keywordDSNName.MatchString(key) {
			return "", false, false
		}
		valueStart := skipDSNSpace(text, start+equals+1)
		end, valid := keywordDSNValueEnd(text, valueStart)
		if !valid {
			return "", false, false
		}
		attributes++
		if (sensitiveName.MatchString(key) || strings.EqualFold(key, "pwd")) && literalConnectionValue(text[valueStart:end]) {
			redacted.WriteString(text[last:valueStart])
			redacted.WriteString("''")
			last = end
			found = true
		}
		start = end
	}
	if !found {
		// A single non-sensitive value can also be an entire semicolon string
		// whose separators were consumed as libpq backslash escapes.
		return "", false, attributes > 1
	}
	redacted.WriteString(text[last:])
	return redacted.String(), true, true
}

func skipDSNSpace(text string, position int) int {
	for position < len(text) && strings.ContainsRune(" \t\r\n\v\f", rune(text[position])) {
		position++
	}
	return position
}

func keywordDSNValueEnd(text string, start int) (int, bool) {
	position := start
	quoted := position < len(text) && text[position] == '\''
	if quoted {
		position++
	}
	for position < len(text) {
		switch {
		case text[position] == '\\':
			if position+1 == len(text) {
				return 0, false
			}
			position += 2
		case quoted && text[position] == '\'':
			position++
			return position, position == len(text) || skipDSNSpace(text, position) > position
		case !quoted && skipDSNSpace(text, position) > position:
			return position, true
		case !quoted && text[position] == ';':
			// Leave unquoted semicolon separators to the existing parser.
			return 0, false
		default:
			position++
		}
	}
	return position, !quoted
}

func connectionValueEnd(text string, start int) int {
	value := strings.TrimLeft(text[start:], " \t\r\n")
	position := len(text) - len(value)
	if position == len(text) {
		return position
	}
	var closing byte
	switch value[0] {
	case '\'', '"':
		closing = value[0]
	case '{':
		if !strings.HasPrefix(value, "{{") {
			closing = '}'
		}
	}
	if closing != 0 {
		position++
		for position < len(text) {
			if text[position] != closing {
				position++
				continue
			}
			position++
			if position < len(text) && text[position] == closing {
				position++
				continue
			}
			break
		}
	}
	if end := strings.IndexByte(text[position:], ';'); end >= 0 {
		return position + end
	}
	return len(text)
}

func literalConnectionValue(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == last && (first == '\'' || first == '"')) ||
			(first == '{' && last == '}' && !strings.HasPrefix(value, "{{")) {
			value = strings.TrimSpace(value[1 : len(value)-1])
		}
	}
	return value != "" && !credentialReference.MatchString(value)
}

// redactURLUserinfo removes credentials without rewriting the rest of the URL.
func redactURLUserinfo(value any) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	trimmed := strings.TrimSpace(text)
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" || parsed.User == nil || parsed.User.String() == "" {
		return "", false
	}
	start := strings.Index(trimmed, "//") + 2
	end := len(trimmed)
	if i := strings.IndexAny(trimmed[start:], "/?#"); i >= 0 {
		end = start + i
	}
	lastAt := strings.LastIndex(trimmed[start:end], "@")
	if lastAt < 0 {
		return "", false
	}
	start += strings.Index(text, trimmed)
	return text[:start] + text[start+lastAt+1:], true
}

func mapCredential(fieldPath string, parent map[string]any, key string) plaintextCredential {
	redacted, hasEmbeddedCredentials := redactEmbeddedCredentials(parent[key])
	return plaintextCredential{path: fieldPath, redact: func() {
		if hasEmbeddedCredentials {
			parent[key] = redacted
		} else {
			delete(parent, key)
		}
	}}
}

// findPlaintextSecrets returns sorted locations of possible plaintext credentials.
func findPlaintextSecrets(object map[string]any) []string {
	credentials := plaintextCredentials(object)
	findings := make([]string, 0, len(credentials))
	for _, credential := range credentials {
		findings = append(findings, credential.path)
	}
	return findings
}

// stripPlaintextSecrets redacts detected credentials and returns the count.
func stripPlaintextSecrets(object map[string]any) int {
	credentials := plaintextCredentials(object)
	for _, credential := range credentials {
		credential.redact()
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
		paired := false
		if key, ok := value["key"].(string); ok && isPlaintextCredential(key, value["value"]) {
			*findings = append(*findings, mapCredential(fmt.Sprintf("%s.value (key %q)", currentPath, key), value, "value"))
			paired = true
		}
		for key, child := range value {
			if paired && key == "value" {
				continue
			}
			childPath := currentPath + "." + key
			if isPlaintextCredential(key, child) {
				*findings = append(*findings, mapCredential(childPath, value, key))
				continue
			}
			walk(child, kind, childPath, findings)
		}
	case []any:
		for i, child := range value {
			childPath := fmt.Sprintf("%s[%d]", currentPath, i)
			if redacted, ok := redactEmbeddedCredentials(child); ok {
				*findings = append(*findings, plaintextCredential{path: childPath, redact: func() { value[i] = redacted }})
				continue
			}
			walk(child, kind, childPath, findings)
		}
	}
}
