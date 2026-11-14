// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package fixer

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/schema"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/template"
	"github.com/openchoreo/openchoreo/tools/lint/strdist"
)

// Diagnostic codes reported for the repairs this package makes. They match the
// codes the linter already reports for the same problems, so a fix can be
// suppressed with the existing # occ:ignore comments.
const (
	codeUnknownField = "unknown-field"
	codeUnknownKind  = "unknown-kind"
	codeFormat       = "format"
)

// imagePullPolicies are the accepted imagePullPolicy values. The generated
// minimal schemas leave the field out entirely, so it is the one field the
// fixer repairs from its own list rather than from a schema enum - the same
// values enum_rules.go validates.
var imagePullPolicies = []string{"Always", "IfNotPresent", "Never"}

// valueOnlyFields are fields the generated schemas omit but the linter still
// validates, mapped to the values they accept.
var valueOnlyFields = map[string][]string{
	"imagePullPolicy": imagePullPolicies,
}

// scanner walks parsed documents against their schema and collects the
// typo-class repairs to apply, plus a reason for every candidate it left alone.
type scanner struct {
	opts Options
	add  func(fix)
	skip func(string)
}

// schemaFor returns the schema a document is validated against: the schema of
// its declared kind, or - when the kind is not recognized - the schema of the
// kind inferred from its spec fields, so a misspelled kind still gets its field
// names checked.
func schemaFor(doc *parser.DocumentNode) *schema.FieldSchema {
	if s := template.SchemaFor(doc.Kind); s != nil {
		return s
	}
	if inferred := template.InferKind(doc); inferred != "" {
		return template.SchemaFor(inferred)
	}
	return nil
}

// collectEdits returns the repairs for every document in the source, and why
// each rejected candidate was left alone. Two fixes landing on the same token
// keep the first one found.
func collectEdits(docs []*parser.DocumentNode, opts Options) ([]fix, []string) {
	var out []fix
	var reasons []string
	seen := make(map[[2]int]bool)
	s := &scanner{opts: opts, add: func(f fix) {
		key := [2]int{f.edit.Line, f.edit.Col}
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, f)
	}, skip: func(reason string) {
		reasons = append(reasons, reason)
	}}
	for _, doc := range docs {
		if doc == nil || doc.Root == nil {
			continue
		}
		if sc := schemaFor(doc); sc != nil {
			s.walk(doc.Root, sc, "", true)
			continue
		}
		// Without a schema there are no field names to check, but the kind is
		// still a closed set of registered names, so a misspelled kind can be
		// repaired on its own.
		s.fixRootKind(kindNode(doc.Root))
	}
	return out, reasons
}

// walk checks one mapping node against an object schema, recursing into nested
// mappings and sequences.
func (s *scanner) walk(m *yaml.Node, sc *schema.FieldSchema, path string, isRoot bool) {
	if m == nil || m.Kind != yaml.MappingNode || sc == nil {
		return
	}

	// Sibling keys, so a rename never introduces a duplicate key.
	present := make(map[string]bool, len(m.Content)/2)
	for i := 0; i+1 < len(m.Content); i += 2 {
		present[m.Content[i].Value] = true
	}

	names := sortedProperties(sc)

	for i := 0; i+1 < len(m.Content); i += 2 {
		keyNode := m.Content[i]
		valNode := m.Content[i+1]
		if keyNode.Kind != yaml.ScalarNode || keyNode.Value == "" {
			continue
		}
		key := keyNode.Value
		child, known := sc.Properties[key]

		if !known {
			s.fixUnknownKey(keyNode, valNode, sc, names, present, path)
			continue
		}
		if child.Preserve {
			continue
		}

		fieldPath := key
		if path != "" {
			fieldPath = path + "." + key
		}

		if isRoot && key == "kind" {
			s.fixRootKind(valNode)
			continue
		}

		if valNode.Kind == yaml.ScalarNode && valNode.Value != "" {
			if values, code, label := allowedValues(key, child, fieldPath); len(values) > 0 && !contains(values, valNode.Value) {
				// Same code the validator reports, so an existing
				// # occ:ignore for it also suppresses the fix.
				s.fixValue(valNode, values, code, label)
			}
		}

		switch valNode.Kind {
		case yaml.MappingNode:
			s.walk(valNode, child, fieldPath, false)
		case yaml.SequenceNode:
			if child.Items == nil {
				continue
			}
			for idx, item := range valNode.Content {
				if item.Kind == yaml.MappingNode {
					s.walk(item, child.Items, fmt.Sprintf("%s[%d]", fieldPath, idx), false)
				}
			}
		}
	}
}

// fixUnknownKey repairs a key the schema does not define. A key the linter
// still validates through its own value list (imagePullPolicy) has its value
// repaired instead; anything else is offered the schema property nearest to
// it. Subtrees the schema preserves are left untouched, matching the validator.
func (s *scanner) fixUnknownKey(keyNode, valNode *yaml.Node, sc *schema.FieldSchema, names []string, present map[string]bool, path string) {
	key := keyNode.Value
	if values, ok := valueOnlyFields[key]; ok && valNode.Kind == yaml.ScalarNode && valNode.Value != "" {
		if !contains(values, valNode.Value) {
			s.fixValue(valNode, values, "invalid-"+key, "fixed "+key)
		}
		return
	}
	if sc.Preserve {
		return
	}
	fieldPath := key
	if path != "" {
		fieldPath = path + "." + key
	}
	if s.opts.skip(keyNode.Line, codeUnknownField) {
		return
	}
	candidate, _, ok := strdist.Closest(key, names, s.opts.MaxDistance)
	switch {
	case !ok:
		s.skip(fmt.Sprintf("%d:%d %s %q not fixed: no schema field within %d edits",
			keyNode.Line, keyNode.Column, codeUnknownField, key, s.opts.MaxDistance))
		return
	case present[candidate]:
		// Renaming here would add a second key with the same name.
		s.skip(fmt.Sprintf("%d:%d %s %q not fixed: %q is already defined here",
			keyNode.Line, keyNode.Column, codeUnknownField, key, candidate))
		return
	}
	s.add(fix{
		edit:    Edit{Line: keyNode.Line, Col: keyNode.Column, Token: key, NewText: candidate},
		code:    codeUnknownField,
		message: fixMessage(fmt.Sprintf("fixed field %s", fieldPath), key, candidate),
		from:    key,
		to:      candidate,
	})
}

// fixRootKind repairs the document kind when it is not a recognized kind. The
// candidates are every registered kind, because the root schema only knows the
// kind it was generated for.
func (s *scanner) fixRootKind(valNode *yaml.Node) {
	if valNode == nil || valNode.Kind != yaml.ScalarNode || valNode.Value == "" {
		return
	}
	if parser.KnownKinds[valNode.Value] {
		return
	}
	if s.opts.skip(valNode.Line, codeUnknownKind) {
		return
	}
	candidate, _, ok := strdist.Closest(valNode.Value, template.AllKinds(), s.opts.MaxDistance)
	if !ok {
		s.skip(fmt.Sprintf("%d:%d %s %q not fixed: no known kind within %d edits",
			valNode.Line, valNode.Column, codeUnknownKind, valNode.Value, s.opts.MaxDistance))
		return
	}
	s.add(fix{
		edit:    Edit{Line: valNode.Line, Col: valNode.Column, Token: valNode.Value, NewText: candidate},
		code:    codeUnknownKind,
		message: fixMessage("fixed kind", valNode.Value, candidate),
		from:    valNode.Value,
		to:      candidate,
	})
}

// kindNode returns the value node of the root "kind" key, or nil when the
// document has no plain "kind" scalar.
func kindNode(root *yaml.Node) *yaml.Node {
	if root == nil || root.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Kind == yaml.ScalarNode && root.Content[i].Value == "kind" {
			return root.Content[i+1]
		}
	}
	return nil
}

// fixValue repairs a scalar value against a fixed set of allowed values.
func (s *scanner) fixValue(valNode *yaml.Node, candidates []string, code, label string) {
	if s.opts.skip(valNode.Line, code) {
		return
	}
	candidate, _, ok := strdist.Closest(valNode.Value, candidates, s.opts.MaxDistance)
	if !ok {
		s.skip(fmt.Sprintf("%d:%d %s %q not fixed: no allowed value within %d edits",
			valNode.Line, valNode.Column, code, valNode.Value, s.opts.MaxDistance))
		return
	}
	s.add(fix{
		edit:    Edit{Line: valNode.Line, Col: valNode.Column, Token: valNode.Value, NewText: candidate},
		code:    code,
		message: fixMessage(label, valNode.Value, candidate),
		from:    valNode.Value,
		to:      candidate,
	})
}

// allowedValues returns the values a field may hold - the enum from the
// schema, or the list of a field the schemas omit - together with the
// diagnostic code the validator reports for a bad value and a label for the
// repair message.
func allowedValues(key string, child *schema.FieldSchema, fieldPath string) (values []string, code, label string) {
	if len(child.Enum) > 0 {
		return child.Enum, "invalid-" + fieldPath, "fixed value for " + fieldPath
	}
	if values, ok := valueOnlyFields[key]; ok {
		return values, "invalid-" + key, "fixed " + key
	}
	return nil, "", ""
}

// sortedProperties returns the schema property names in a stable order, so the
// same document always produces the same fixes.
func sortedProperties(sc *schema.FieldSchema) []string {
	names := make([]string, 0, len(sc.Properties))
	for name := range sc.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}
