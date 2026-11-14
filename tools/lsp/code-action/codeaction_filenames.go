// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package codeaction

import (
	"fmt"
	"math"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/tools/lint/indexer"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	lsputil "github.com/openchoreo/openchoreo/tools/lsp/lsp-util"
)

// fileNameRefFixes walks the document for `name` values that reference another
// resource file in the workspace. When the typed value is a misspelling, it
// offers the most-matching resource name from the pool (resource names plus
// indexed file basenames) as a quick fix (e.g. `name: docker-gitops-releasd`
// when `docker-gitops-release` is a resource name -> `Change name to
// "docker-gitops-release"`). resourceNames is the authoritative set of
// `metadata.name` values that a reference can resolve to.
func fileNameRefFixes(uri string, text string, clientDiags []protocol.Diagnostic, resourceNames []string, pool []string, scopes ...ReferenceScope) []protocol.CodeAction {
	if len(pool) == 0 {
		return nil
	}
	docs, err := parser.ParseYAML([]byte(text))
	if err != nil {
		return nil
	}
	lines := splitLines(text)

	var actions []protocol.CodeAction
	for _, doc := range docs {
		if doc.Root == nil {
			continue
		}
		walkNameValues(doc.Root, func(valNode *yaml.Node) {
			names, candidates := referencePool(doc, valNode, resourceNames, pool, scopes)
			best, ok := mostMatchingFileName(valNode.Value, names, candidates)
			if !ok {
				return
			}
			// File-name fixes are always offered for the whole document,
			// regardless of the requested range, so the quick fix is visible
			// no matter which line the user is on.
			edit := replaceTokenAt(lines, valNode, best)
			if edit == nil {
				return
			}
			diag := fakeDiag(clientDiags, edit.Range, "unknown-resource-file", fmt.Sprintf("%q not found; did you mean %q?", valNode.Value, best))
			title := fmt.Sprintf("Change name to %q", best)
			actions = append(actions, *quickFix(title, uri, diag, []protocol.TextEdit{*edit}))
		})
	}
	return actions
}

// FileNameRefDiagnostics walks the document for `name` values that reference
// another resource in the workspace. Values that are not a current resource
// name but are a misspelling of one produce a diagnostic, so the client
// surfaces the lightbulb that exposes the corresponding quick fix.
func FileNameRefDiagnostics(text string, resourceNames, pool []string) []protocol.Diagnostic {
	if len(pool) == 0 {
		return nil
	}
	docs, err := parser.ParseYAML([]byte(text))
	if err != nil {
		return nil
	}

	return FileNameRefDiagnosticsDocs(docs, resourceNames, pool)
}

// FileNameRefDiagnosticsDocs is the shared core of FileNameRefDiagnostics for
// callers that have already parsed the document. It walks every `name` value
// that references another resource and flags values that are no longer a
// known resource name but remain a near-miss of an available one. resourceNames
// is the authoritative set of `metadata.name` values a reference can resolve
// to; pool adds file basenames so a resource whose file name differs from its
// metadata.name still receives a "did you mean" suggestion.
func FileNameRefDiagnosticsDocs(docs []*parser.DocumentNode, resourceNames, pool []string, scopes ...ReferenceScope) []protocol.Diagnostic {
	if len(pool) == 0 {
		return nil
	}

	var out []protocol.Diagnostic
	for _, doc := range docs {
		if doc == nil || doc.Root == nil {
			continue
		}
		var sourceLines []string
		if len(doc.Source) > 0 {
			sourceLines = splitLines(string(doc.Source))
		}
		walkNameValues(doc.Root, func(valNode *yaml.Node) {
			names, candidates := referencePool(doc, valNode, resourceNames, pool, scopes)
			if best, ok := mostMatchingFileName(valNode.Value, names, candidates); ok {
				severity := protocol.DiagnosticSeverity(2)
				out = append(out, protocol.Diagnostic{
					Range:    scalarRange(valNode, sourceLines),
					Severity: &severity,
					Code:     &protocol.IntegerOrString{Value: "unknown-resource-file"},
					Source:   lsputil.StringPtr("openchoreo"),
					Message:  fmt.Sprintf("%q not found; did you mean %q?", valNode.Value, best),
				})
			}
		})
	}
	return out
}

// walkNameValues invokes fn for every `name` scalar that references another
// resource file, recursing through nested mappings and sequence items (e.g. the
// items of `allowedWorkflows: - kind: ... name: ...`). The document's own
// top-level `metadata.name` is the resource's own name, not a reference to
// another file, so it is skipped.
func walkNameValues(root *yaml.Node, fn func(n *yaml.Node)) {
	if root == nil {
		return
	}
	switch root.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(root.Content); i += 2 {
			keyNode := root.Content[i]
			valNode := root.Content[i+1]
			// Own resource name (top-level metadata.name): not a reference.
			if keyNode.Value == "metadata" && valNode.Kind == yaml.MappingNode {
				continue
			}
			if keyNode.Value == "name" && valNode.Kind == yaml.ScalarNode {
				fn(valNode)
			}
			walkNameValues(valNode, fn)
		}
	case yaml.SequenceNode:
		for _, item := range root.Content {
			walkNameValues(item, fn)
		}
	}
}

// mostMatchingFileName returns the closest available name to the given value
// when it is a genuine near-miss (case-insensitive edit distance <= 2) of a
// resource name or file basename, and the value is not itself a current
// resource name. A `name:` reference resolves only against resource
// metadata.names; a value that merely coincides with a file basename (which
// may outlive the resource, e.g. after a metadata.name edit) is still an
// unresolved reference and is flagged.
func mostMatchingFileName(current string, resourceNames, pool []string) (string, bool) {
	for _, prefix := range []string{"deployment/", "statefulset/", "cronjob/", "job/", "proxy/"} {
		if strings.HasPrefix(current, prefix) {
			best, ok := mostMatchingFileName(strings.TrimPrefix(current, prefix), resourceNames, pool)
			if ok {
				return prefix + best, true
			}
			return "", false
		}
	}
	if current == "" {
		return "", false
	}
	for _, n := range resourceNames {
		if strings.EqualFold(n, current) {
			return "", false
		}
	}
	matches := closestMatches(current, pool, 1)
	if len(matches) == 0 || matches[0] == current {
		return "", false
	}
	if editDistance(strings.ToLower(current), strings.ToLower(matches[0])) > 2 {
		return "", false
	}
	return matches[0], true
}

// scalarRange returns the 0-indexed LSP range covering a yaml scalar's value.
func scalarRange(n *yaml.Node, source ...[]string) protocol.Range {
	li := lsputil.PositionIndex(max(n.Line, 1) - 1)
	start := lsputil.PositionIndex(max(n.Column, 1) - 1)
	if len(source) > 0 && len(source[0]) > 0 {
		lines := source[0]
		if n.Line > 0 && n.Line <= len(lines) {
			start = lsputil.PositionIndex(utf16Column(lines[n.Line-1], n.Column))
		}
	}
	return protocol.Range{
		Start: protocol.Position{Line: li, Character: start},
		End:   protocol.Position{Line: li, Character: min(start+lsputil.PositionIndex(utf16Len(n.Value)), math.MaxInt32)},
	}
}

type ReferenceScope struct {
	Index *indexer.Indexer
	URI   string
}

func referencePool(doc *parser.DocumentNode, node *yaml.Node, names, pool []string, scopes []ReferenceScope) ([]string, []string) {
	if len(scopes) == 0 || scopes[0].Index == nil {
		return names, pool
	}
	parent, _ := lsputil.FindParentMappingAndKey(doc.Root, node)
	kindNode := lsputil.FindSiblingKey(parent, "kind")
	kind := ""
	if kindNode != nil {
		kind = kindNode.Value
	}
	if kind == "" {
		_, field := lsputil.FindParentMappingAndKey(doc.Root, parent)
		if field == "componentType" {
			kind = "ComponentType"
		}
	}
	if kind == "" {
		return names, pool
	}
	var candidates []string
	for _, entry := range scopes[0].Index.Snapshot(scopes[0].URI) {
		d := entry.Document
		if d.Kind != kind {
			continue
		}
		if !strings.HasPrefix(kind, "Cluster") && indexer.NamespaceOrDefault(d.Metadata.Namespace) != indexer.NamespaceOrDefault(doc.Metadata.Namespace) {
			continue
		}
		candidates = append(candidates, d.Metadata.Name)
	}
	return candidates, candidates
}
