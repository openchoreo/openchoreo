// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package codeaction

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/schema"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/template"
	"github.com/openchoreo/openchoreo/tools/lsp/lsp-util"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// Handle computes the quick fixes for a code action request. serverDiags are
// the authoritative diagnostics recomputed by the server (Validate). resourceNames
// is the authoritative set of resource metadata.names; pool adds indexed file
// basenames, both used by the file-name reference fixes.
func Handle(uri, text string, clientDiags []protocol.Diagnostic, requestRange protocol.Range, serverDiags []protocol.Diagnostic, resourceNames, pool []string, scopes ...ReferenceScope) []protocol.CodeAction {
	// Use the client-reported diagnostics to pick which ranges the user is
	// looking at, then recompute the authoritative codes from the document.
	var actions []protocol.CodeAction
	for _, sd := range serverDiags {
		if !overlaps(sd.Range, requestRange) && !matchesAnyClient(sd.Range, clientDiags) {
			continue
		}
		switch diagCode(sd) {
		case "cel-unknown-name":
			if a := fixCELFunction(uri, text, sd); a != nil {
				actions = append(actions, *a)
			} else if a := fixCELRoot(uri, text, sd); a != nil {
				actions = append(actions, *a)
			}
		case "cel-type-error":
			if a := fixCELField(uri, text, sd); a != nil {
				actions = append(actions, *a)
			}
		case "missing-apiVersion":
			if a := fixMissingAPIVersion(uri, text, sd); a != nil {
				actions = append(actions, *a)
			}
		case "invalid-apiVersion":
			if a := fixInvalidAPIVersion(uri, text, sd); a != nil {
				actions = append(actions, *a)
			}
		case "missing-metadata-name":
			if a := fixMissingMetadataName(uri, text, sd); a != nil {
				actions = append(actions, *a)
			}
		case "missing-kind":
			if a := fixMissingKind(uri, text, sd); a != nil {
				actions = append(actions, *a)
			}
		}
	}

	// Schema-driven fixes for every field: missing required fields, invalid
	// enums, and misspelt field names at any nesting level.
	actions = append(actions, schemaFixes(uri, text, schemaForDocument(text), clientDiags, requestRange)...)

	// Resource file name fixes: correct a misspelt `name` value to the
	// most-matching resource name from the indexed list.
	actions = append(actions, fileNameRefFixes(uri, text, clientDiags, requestRange, resourceNames, pool, scopes...)...)

	if actions == nil {
		actions = []protocol.CodeAction{}
	}
	return actions
}

// schemaForDocument returns the schema for the kind declared in the text. If
// no kind is present or it is not recognized (e.g. misspelt), it returns a
// minimal "kind-resolver" schema so the misspelt kind value is still corrected
// through the same schema-driven enum path. The resolver preserves unknown
// fields, so it never offers bogus fixes for sibling keys.
func schemaForDocument(text string) *schema.FieldSchema {
	kind := lsputil.ExtractKindFromText(text)
	if kind != "" {
		if s := template.SchemaFor(kind); s != nil {
			return s
		}
	}
	return &schema.FieldSchema{
		Preserve: true,
		Properties: map[string]*schema.FieldSchema{
			"kind": {Type: "string", Enum: template.AllKinds()},
		},
	}
}

// overlaps reports whether two 0-indexed LSP ranges share at least one line.
func overlaps(a, b protocol.Range) bool {
	return a.Start.Line <= b.End.Line && b.Start.Line <= a.End.Line
}

// matchesAnyClient reports whether the server diagnostic range aligns with any
// of the client-reported diagnostic ranges, which the client sends near the
// code action request range.
func matchesAnyClient(r protocol.Range, client []protocol.Diagnostic) bool {
	for _, d := range client {
		if d.Range.Start.Line == r.Start.Line {
			return true
		}
	}
	return false
}

// diagCode extracts the string diagnostic code, handling both integer and
// string forms of the LSP diagnostic code field.
func diagCode(d protocol.Diagnostic) string {
	if d.Code == nil {
		return ""
	}
	if s, ok := d.Code.Value.(string); ok {
		return s
	}
	return ""
}

// quickFix builds a CodeAction of kind quickfix carrying the given edits.
func quickFix(title string, uri string, diag protocol.Diagnostic, edits []protocol.TextEdit) *protocol.CodeAction {
	if len(edits) == 0 {
		return nil
	}
	kind := protocol.CodeActionKindQuickFix
	preferred := true
	return &protocol.CodeAction{
		Title:       title,
		Kind:        &kind,
		Diagnostics: []protocol.Diagnostic{diag},
		IsPreferred: &preferred,
		Edit: &protocol.WorkspaceEdit{
			Changes: map[protocol.DocumentUri][]protocol.TextEdit{
				protocol.DocumentUri(uri): edits,
			},
		},
	}
}

// splitLines splits document text into lines, stripping any trailing carriage
// return so edits target clean LF content.
func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	// If the file ends with a newline, the final empty element is an artifact.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// fixMissingAPIVersion inserts the standard apiVersion line at the top of the
// document when it is absent.
func fixMissingAPIVersion(uri, text string, diag protocol.Diagnostic) *protocol.CodeAction {
	if strings.Contains(text, "apiVersion:") {
		return nil
	}
	insert := protocol.TextEdit{
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: 0, Character: 0},
		},
		NewText: "apiVersion: openchoreo.dev/v1alpha1\n",
	}
	return quickFix("Add apiVersion: openchoreo.dev/v1alpha1", uri, diag, []protocol.TextEdit{insert})
}

// fixInvalidAPIVersion replaces a malformed top-level apiVersion value with the
// canonical value accepted by OpenChoreo.
func fixInvalidAPIVersion(uri, text string, diag protocol.Diagnostic) *protocol.CodeAction {
	lines := splitLines(text)
	for i, ln := range lines {
		trimmed := strings.TrimPrefix(ln, " ")
		if strings.HasPrefix(trimmed, "apiVersion:") && !strings.HasPrefix(trimmed, " ") {
			edit := protocol.TextEdit{
				Range: protocol.Range{
					Start: protocol.Position{Line: protocol.UInteger(i), Character: 0},
					End:   protocol.Position{Line: protocol.UInteger(i), Character: protocol.UInteger(len(ln))},
				},
				NewText: "apiVersion: openchoreo.dev/v1alpha1",
			}
			return quickFix("Fix apiVersion to openchoreo.dev/v1alpha1", uri, diag, []protocol.TextEdit{edit})
		}
	}
	return nil
}

// fixMissingMetadataName inserts a `name` field as the first member of a
// block-style `metadata:` mapping when it is missing.
func fixMissingMetadataName(uri, text string, diag protocol.Diagnostic) *protocol.CodeAction {
	lines := splitLines(text)
	for i, ln := range lines {
		trimmed := strings.TrimPrefix(ln, " ")
		if strings.HasPrefix(trimmed, " ") {
			continue
		}
		if !strings.HasPrefix(trimmed, "metadata:") {
			continue
		}
		// Only handle block style with an empty value (e.g. `metadata:`).
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "metadata:"))
		if rest != "" {
			continue
		}
		edit := protocol.TextEdit{
			Range: protocol.Range{
				Start: protocol.Position{Line: protocol.UInteger(i), Character: protocol.UInteger(len(ln))},
				End:   protocol.Position{Line: protocol.UInteger(i), Character: protocol.UInteger(len(ln))},
			},
			NewText: "\n  name: default",
		}
		return quickFix("Add metadata.name", uri, diag, []protocol.TextEdit{edit})
	}
	return nil
}

// fixMissingKind inserts a `kind:` field when the document has none. Because
// the kind cannot be reliably inferred, a placeholder value is used so the
// user can complete it.
func fixMissingKind(uri, text string, diag protocol.Diagnostic) *protocol.CodeAction {
	if strings.Contains(text, "\nkind:") || strings.HasPrefix(text, "kind:") {
		return nil
	}
	lines := splitLines(text)
	// Insert after the apiVersion line if present, otherwise at the top.
	pos := protocol.Position{Line: 0, Character: 0}
	if lines[0] != "" {
		pos = protocol.Position{Line: 0, Character: protocol.UInteger(len(lines[0]))}
	}
	edit := protocol.TextEdit{
		Range: protocol.Range{Start: pos, End: pos},
		// Insert on the next line after apiVersion.
		NewText: "\nkind: <Kind>",
	}
	return quickFix("Add kind field", uri, diag, []protocol.TextEdit{edit})
}

// --- Spelling-mistake quick fixes ---

// editDistance returns the Levenshtein edit distance between two strings, a
// measure of how many single-character edits separate them. It is used to find
// the closest valid field/kind name to a misspelt token.
func editDistance(a, b string) int {
	if a == b {
		return 0
	}
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	cur := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if a < b {
		b = a
	}
	if c < b {
		b = c
	}
	return b
}

// closestMatches returns candidate strings nearest to the typo, sorted by edit
// distance ascending. Ties are broken alphabetically for determinism.
func closestMatches(typo string, candidates []string, max int) []string {
	type match struct {
		s    string
		dist int
	}
	var ms []match
	for _, c := range candidates {
		if strings.EqualFold(c, typo) {
			continue
		}
		ms = append(ms, match{s: c, dist: editDistance(strings.ToLower(typo), strings.ToLower(c))})
	}
	sort.SliceStable(ms, func(i, j int) bool {
		if ms[i].dist != ms[j].dist {
			return ms[i].dist < ms[j].dist
		}
		return ms[i].s < ms[j].s
	})
	out := make([]string, 0, max)
	for i := 0; i < len(ms) && i < max; i++ {
		out = append(out, ms[i].s)
	}
	return out
}

var celFunctionSuggestion = regexp.MustCompile(`undeclared reference to '([A-Za-z_][A-Za-z0-9_]*)'.*Did you mean "([A-Za-z_][A-Za-z0-9_]*)"\?`)

func fixCELFunction(uri, text string, diag protocol.Diagnostic) *protocol.CodeAction {
	match := celFunctionSuggestion.FindStringSubmatch(diag.Message)
	if len(match) != 3 {
		return nil
	}
	lines := strings.Split(text, "\n")
	line := int(diag.Range.Start.Line)
	if line >= len(lines) {
		return nil
	}
	pattern := regexp.MustCompile(`\.` + regexp.QuoteMeta(match[1]) + `\s*\(`)
	spans := pattern.FindAllStringIndex(lines[line], -1)
	if len(spans) != 1 {
		return nil
	}
	start := spans[0][0] + 1
	column := len(utf16.Encode([]rune(lines[line][:start])))
	r := protocol.Range{Start: protocol.Position{Line: diag.Range.Start.Line, Character: protocol.UInteger(column)}, End: protocol.Position{Line: diag.Range.Start.Line, Character: protocol.UInteger(column + len(match[1]))}}
	return quickFix("Replace with "+match[2], uri, diag, []protocol.TextEdit{{Range: r, NewText: match[2]}})
}

// fixCELRoot uses the authoritative token range, never a text-wide replacement.
func fixCELRoot(uri, text string, diag protocol.Diagnostic) *protocol.CodeAction {
	match := celFunctionSuggestion.FindStringSubmatch(diag.Message)
	if len(match) != 3 || diag.Range.Start.Line != diag.Range.End.Line {
		return nil
	}
	lines := strings.Split(text, "\n")
	line := int(diag.Range.Start.Line)
	if line >= len(lines) {
		return nil
	}
	units := utf16.Encode([]rune(lines[line]))
	start, end := int(diag.Range.Start.Character), int(diag.Range.End.Character)
	if start < 0 || end < start || end > len(units) {
		return nil
	}
	if string(utf16.Decode(units[start:end])) != match[1] {
		return nil
	}
	// Member names have their own fix; this fallback is for root identifiers.
	if start > 0 && units[start-1] == '.' {
		return nil
	}
	return quickFix("Replace with "+match[2], uri, diag, []protocol.TextEdit{{Range: diag.Range, NewText: match[2]}})
}

var celFieldSuggestion = regexp.MustCompile(`undefined field '([A-Za-z_][A-Za-z0-9_]*)'.*Did you mean "([A-Za-z_][A-Za-z0-9_]*)"\?`)

// fixCELField edits only the checker-selected identifier, preserving patch syntax.
func fixCELField(uri, text string, diag protocol.Diagnostic) *protocol.CodeAction {
	match := celFieldSuggestion.FindStringSubmatch(diag.Message)
	if len(match) != 3 || diag.Range.Start.Line != diag.Range.End.Line {
		return nil
	}
	lines := strings.Split(text, "\n")
	line := int(diag.Range.Start.Line)
	if line >= len(lines) {
		return nil
	}
	units := utf16.Encode([]rune(lines[line]))
	start, end := int(diag.Range.Start.Character), int(diag.Range.End.Character)
	if start < 0 || end < start || end > len(units) || string(utf16.Decode(units[start:end])) != match[1] {
		return nil
	}
	return quickFix("Replace with "+match[2], uri, diag, []protocol.TextEdit{{Range: diag.Range, NewText: match[2]}})
}
