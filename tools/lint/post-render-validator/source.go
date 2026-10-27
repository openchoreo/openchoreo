// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package postrendervalidator

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine"
)

// postRenderSource pairs a parsed document with the file it came from, so a
// post-render finding can be reported against the document that declares it.
type postRenderSource struct {
	doc  *parser.DocumentNode
	file string
}

// resolvePostRenderDiagnostic finds the source file and position a post-render
// diagnostic should point at. Rendered resources carry no source positions, so
// the location is recovered from the parsed documents: a declared failure
// points at its declaring document, and a policy finding points at the field
// that caused it (for example the privileged security context in a
// ComponentType). When nothing better is found, the fallback document's start
// is used.
func resolvePostRenderDiagnostic(
	d ruleengine.Diagnostic,
	sources []postRenderSource,
	fallback string,
) (string, parser.Position) {
	fbFile, fbPos := fallbackLocation(sources, fallback)

	switch d.Code {
	case "post-render-validation-failed":
		if kind, name, ok := declaringSource(d.Message); ok {
			if s := findSource(sources, func(s postRenderSource) bool {
				return s.doc.Kind == kind && s.doc.Metadata.Name == name
			}); s != nil {
				if n := findKeyNode(s.doc.Root, "postRenderValidations"); n != nil {
					return s.file, position(n)
				}
				return s.file, docPosition(s.doc)
			}
		}
	case "image-latest":
		if file, pos, ok := searchKey(sources, []string{"Workload"}, "image"); ok {
			return file, pos
		}
	case "limits-required":
		if file, pos, ok := searchKey(sources, []string{"ComponentType", "ClusterComponentType"}, "limits"); ok {
			return file, pos
		}
	case "requests-required":
		if file, pos, ok := searchKey(sources, []string{"ComponentType", "ClusterComponentType"}, "requests"); ok {
			return file, pos
		}
	case "replicas-positive":
		if file, pos, ok := searchKey(sources, []string{"Component"}, "replicas"); ok {
			return file, pos
		}
	case "no-privileged":
		if file, pos, ok := searchKey(sources, []string{"ComponentType", "ClusterComponentType"}, "privileged"); ok {
			return file, pos
		}
	case "no-host-namespaces":
		for _, key := range []string{"hostNetwork", "hostPID", "hostIPC"} {
			if file, pos, ok := searchKey(sources, []string{"ComponentType", "ClusterComponentType"}, key); ok {
				return file, pos
			}
		}
	case "post-render-unresolved-expression":
		if expr := quotedAfter(d.Message, "unresolved expression "); expr != "" {
			if file, pos, ok := searchScalar(sources, expr); ok {
				return file, pos
			}
		}
		if file, pos, ok := searchScalar(sources, "${"); ok {
			return file, pos
		}
	}
	return fbFile, fbPos
}

// fallbackLocation returns the file and start position used when a diagnostic
// cannot be matched to a more specific field. Prefer the ComponentType or
// Workload that actually shapes the rendered resources over the Component
// object itself.
func fallbackLocation(sources []postRenderSource, fallback string) (string, parser.Position) {
	for _, kind := range []string{"ComponentType", "ClusterComponentType"} {
		if s := findSource(sources, func(s postRenderSource) bool { return s.doc.Kind == kind }); s != nil {
			return s.file, docPosition(s.doc)
		}
	}
	if s := findSource(sources, func(s postRenderSource) bool { return s.doc.Kind == "Workload" }); s != nil {
		return s.file, docPosition(s.doc)
	}
	if s := findSource(sources, func(s postRenderSource) bool { return s.file == fallback }); s != nil {
		return s.file, docPosition(s.doc)
	}
	if len(sources) > 0 {
		return sources[0].file, docPosition(sources[0].doc)
	}
	return fallback, parser.Position{Line: 1, Column: 1}
}

// docPosition returns the start of a document, guarding against a zero range.
func docPosition(doc *parser.DocumentNode) parser.Position {
	if doc != nil && doc.Range.Start.Line > 0 {
		return doc.Range.Start
	}
	return parser.Position{Line: 1, Column: 1}
}

// position converts a YAML node's location into a parser.Position.
func position(n *yaml.Node) parser.Position {
	if n == nil {
		return parser.Position{Line: 1, Column: 1}
	}
	return parser.Position{Line: n.Line, Column: n.Column}
}

// findSource returns the first source matching pred, or nil.
func findSource(sources []postRenderSource, pred func(postRenderSource) bool) *postRenderSource {
	for i := range sources {
		if source := sources[i]; pred(source) {
			return &sources[i]
		}
	}
	return nil
}

// declaringSource parses a "<kind> <name>[/<instance>]" prefix from a declared
// failure message, returning the declaring kind and name.
func declaringSource(msg string) (kind, name string, ok bool) {
	label := msg
	if i := strings.Index(label, ":"); i >= 0 {
		label = label[:i]
	}
	for _, k := range []string{"ComponentType", "ClusterComponentType", "Trait", "ClusterTrait"} {
		rest, found := strings.CutPrefix(label, k+" ")
		if !found {
			continue
		}
		if slash := strings.Index(rest, "/"); slash >= 0 {
			rest = rest[:slash]
		}
		return k, rest, true
	}
	return "", "", false
}

// searchKey finds the first mapping key named key in the given sources. Sources
// whose kind is in preferredKinds are searched first; the rest follow.
func searchKey(sources []postRenderSource, preferredKinds []string, key string) (string, parser.Position, bool) {
	if len(preferredKinds) > 0 {
		for _, s := range sources {
			for _, kind := range preferredKinds {
				if s.doc.Kind != kind {
					continue
				}
				if n := findKeyNode(s.doc.Root, key); n != nil {
					return s.file, position(n), true
				}
			}
		}
	}
	for _, s := range sources {
		if n := findKeyNode(s.doc.Root, key); n != nil {
			return s.file, position(n), true
		}
	}
	return "", parser.Position{}, false
}

// searchScalar finds the first scalar value containing substr in the sources.
func searchScalar(sources []postRenderSource, substr string) (string, parser.Position, bool) {
	if substr == "" {
		return "", parser.Position{}, false
	}
	for _, s := range sources {
		if n := findScalarNode(s.doc.Root, substr); n != nil {
			return s.file, position(n), true
		}
	}
	return "", parser.Position{}, false
}

// findKeyNode walks a YAML node depth-first and returns the key node with the
// given name.
func findKeyNode(root *yaml.Node, key string) *yaml.Node {
	if root == nil {
		return nil
	}
	if root.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(root.Content); i += 2 {
			k := root.Content[i]
			if k.Value == key {
				return k
			}
			if n := findKeyNode(root.Content[i+1], key); n != nil {
				return n
			}
		}
		return nil
	}
	for _, child := range root.Content {
		if n := findKeyNode(child, key); n != nil {
			return n
		}
	}
	return nil
}

// findScalarNode walks a YAML node depth-first and returns the first scalar
// whose value contains substr.
func findScalarNode(root *yaml.Node, substr string) *yaml.Node {
	if root == nil {
		return nil
	}
	if root.Kind == yaml.ScalarNode && strings.Contains(root.Value, substr) {
		return root
	}
	for _, child := range root.Content {
		if n := findScalarNode(child, substr); n != nil {
			return n
		}
	}
	return nil
}

// quotedAfter extracts the quoted text that follows prefix in msg, e.g.
// `unresolved expression "${foo}"` -> `${foo}`.
func quotedAfter(msg, prefix string) string {
	i := strings.Index(msg, prefix)
	if i < 0 {
		return ""
	}
	rest := msg[i+len(prefix):]
	if !strings.HasPrefix(rest, `"`) {
		return ""
	}
	rest = rest[1:]
	if j := strings.Index(rest, `"`); j >= 0 {
		return rest[:j]
	}
	return ""
}
