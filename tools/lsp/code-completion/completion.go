// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package codecompletion

import (
	"sort"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/schema"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/template"
	lsputil "github.com/openchoreo/openchoreo/tools/lsp/lsp-util"
)

// ResourceIndexer is the subset of the workspace index that reference
// autocompletion needs: resource names grouped by kind. The lsp.Server's
// indexer satisfies this interface.
type ResourceIndexer interface {
	NamesByKind() map[string][]string
}

// contextMode describes what the user is about to complete at the cursor.
type contextMode int

const (
	// modeFields: the cursor is at a position where a new key (field) is
	// expected under a mapping or array item.
	modeFields contextMode = iota
	// modeValue: the cursor sits in the value slot of `key:` on the cursor
	// line, so value candidates (enum/default/reference names) belong here.
	modeValue
	// modeArrayItem: the cursor is on a `- ` item line, so the schema of the
	// sequence items is the completion target.
	modeArrayItem
)

// completionContext is the resolved schema location of the cursor. path is the
// dot-separated schema path; for modeValue it includes the field on the
// cursor line (e.g. "spec.effect"), for the other modes it is the container
// (e.g. "spec.roleMappings").
type completionContext struct {
	mode  contextMode
	path  string
	field string // field name on the cursor line in modeValue ("" otherwise)
	text  string // full document text (for value-region edits and kind completion)
	line  int    // 1-based cursor line
	col   int    // 1-based cursor column
}

// Handle generates autocompletion for the given (1-indexed) cursor position
// without a workspace index (no reference-name suggestions).
func Handle(text string, cursorLine, cursorCol int) (any, error) {
	return HandleWithIndex(text, cursorLine, cursorCol, nil)
}

// HandleWithIndex is Handle plus an optional workspace index used to suggest
// resource names for `kind`/`name` reference pairs.
func HandleWithIndex(text string, cursorLine, cursorCol int, idx ResourceIndexer, sourceURI ...string) (any, error) {
	uri := ""
	if len(sourceURI) > 0 {
		uri = sourceURI[0]
	}
	if items, ok := bindingReleaseItems(text, cursorLine, cursorCol, uri, idx); ok {
		return protocol.CompletionList{Items: items}, nil
	}
	if items, ok := workflowPolicyItems(text, cursorLine, cursorCol, uri, idx); ok {
		return protocol.CompletionList{Items: items}, nil
	}
	if items, ok := dynamicItems(text, cursorLine, cursorCol, idx, uri); ok {
		return protocol.CompletionList{Items: items}, nil
	}
	if items, handled := rootKindChoices(text, cursorLine, cursorCol); handled {
		return protocol.CompletionList{IsIncomplete: true, Items: items}, nil
	}
	docs, err := parser.ParseYAML([]byte(text))
	if err != nil || len(docs) == 0 {
		return completionFromText(text, cursorLine, cursorCol, idx)
	}
	return completionFromAST(docs[0], cursorLine, cursorCol, idx)
}

// completionFromAST generates completions from a successfully parsed document.
func completionFromAST(doc *parser.DocumentNode, cursorLine, cursorCol int, idx ResourceIndexer) (any, error) {
	schemaRoot := template.SchemaFor(doc.Kind)
	if schemaRoot == nil {
		// The document parsed but the resource kind isn't known yet (e.g. an
		// empty or missing `kind:`). Reconstruct the raw text and fall back to
		// line-based resolution.
		text := ""
		if doc.Source != nil {
			text = string(doc.Source)
		}
		return completionFromText(text, cursorLine, cursorCol, idx)
	}

	text := ""
	if doc.Source != nil {
		text = string(doc.Source)
	}
	ctx := resolveContext(text, cursorLine, cursorCol)
	items := suggest(ctx, schemaRoot, idx)

	if ctx.mode == modeFields || ctx.mode == modeArrayItem {
		items = filterExistingItems(items, existingKeysInBlock(text, cursorLine))
	}

	return protocol.CompletionList{IsIncomplete: false, Items: items}, nil
}

// completionFromText generates completions by analyzing raw text when YAML
// parsing fails or the resource kind is unresolved.
func completionFromText(text string, cursorLine, cursorCol int, idx ResourceIndexer) (any, error) {
	kind := extractKindFromText(text)
	if kind != "" {
		schemaRoot := template.SchemaFor(kind)
		if schemaRoot == nil {
			return protocol.CompletionList{IsIncomplete: false, Items: nil}, nil
		}
		ctx := resolveContext(text, cursorLine, cursorCol)
		items := suggest(ctx, schemaRoot, idx)
		if ctx.mode == modeFields || ctx.mode == modeArrayItem {
			items = filterExistingItems(items, existingKeysInBlock(text, cursorLine))
		}
		return protocol.CompletionList{IsIncomplete: false, Items: items}, nil
	}

	// No kind found yet — only suggest the universal root-level fields that
	// are common to every OpenChoreo resource kind. We must NOT guess a schema
	// for nested fields because different kinds have different spec/status.

	// If the cursor is on a bare "kind:" line whose value is still empty,
	// suggest the valid resource kind names so the user can pick one.
	if isKindValueLine(text, cursorLine) {
		return protocol.CompletionList{
			IsIncomplete: false,
			Items:        kindNameCompletions(text, cursorLine, cursorCol),
		}, nil
	}

	// Root-level fields (apiVersion/kind/metadata/spec/status) must only be
	// suggested at the top level (no indentation), never nested inside a block.
	if cursorLineIndent(text, cursorLine) > 0 {
		return protocol.CompletionList{IsIncomplete: false, Items: nil}, nil
	}

	var rootSchema *schema.FieldSchema
	for _, k := range template.AllKinds() {
		if s := template.SchemaFor(k); s != nil {
			rootSchema = s
			break
		}
	}
	if rootSchema == nil {
		return protocol.CompletionList{IsIncomplete: false, Items: nil}, nil
	}

	// Re-resolve the full context: a blank root line may itself be the value
	// slot of a partially typed root key.
	ctx := resolveContext(text, cursorLine, cursorCol)
	if ctx.mode == modeValue && ctx.field != "" {
		items := suggest(ctx, rootSchema, idx)
		return protocol.CompletionList{IsIncomplete: false, Items: items}, nil
	}

	existingKeys := extractExistingKeys(text)
	rootFields := []string{"apiVersion", valueKind, "metadata", "spec", "status"}
	items := make([]protocol.CompletionItem, 0, len(rootFields))
	for _, name := range rootFields {
		if existingKeys[name] {
			continue
		}
		if fieldSchema := schema.FindSchema(rootSchema, name); fieldSchema != nil {
			items = append(items, buildFieldItem(name, fieldSchema, completionContext{mode: modeFields}))
		}
	}
	return protocol.CompletionList{IsIncomplete: false, Items: items}, nil
}

// resolveContext determines the schema location and completion mode at the
// cursor by analyzing the raw text lines (same signal a partially-typed or
// broken document provides, mirroring how the reference yaml-language-server
// normalizes the cursor before matching a schema).
func resolveContext(text string, cursorLine, cursorCol int) completionContext {
	if cursorLine < 1 {
		return completionContext{mode: modeFields, text: text, line: cursorLine, col: cursorCol}
	}
	lines := strings.Split(text, "\n")
	if cursorLine > len(lines) {
		return completionContext{mode: modeFields, text: text, line: cursorLine, col: cursorCol}
	}
	line := lines[cursorLine-1]
	trimmed := strings.TrimSpace(line)

	// Extract the key of the cursor line (if any) and the 1-based column of
	// its colon. Dash-prefixed lines are array items `- <key>: <value>`.
	key := ""
	keyCol := -1
	if strings.HasPrefix(trimmed, "-") {
		rest := strings.TrimSpace(trimmed[1:])
		if c := strings.Index(rest, ":"); c > 0 {
			key = strings.TrimSpace(rest[:c])
			keyCol = len(line) - len(rest) + c + 1
		}
	} else if c := strings.Index(trimmed, ":"); c > 0 {
		key = strings.TrimSpace(trimmed[:c])
		keyCol = len(line) - len(trimmed) + c + 1
	}

	base := pathBeforeCursor(text, cursorLine)

	// A cursor at or after the colon is editing the value slot of `key`.
	if key != "" && keyCol >= 0 && cursorCol >= keyCol {
		return completionContext{
			mode:  modeValue,
			path:  joinSchemaPath(base, key),
			field: key,
			text:  text,
			line:  cursorLine,
			col:   cursorCol,
		}
	}

	// A `- ` item line without its own key is a new array element.
	if strings.HasPrefix(trimmed, "-") {
		return completionContext{mode: modeArrayItem, path: base, text: text, line: cursorLine, col: cursorCol}
	}

	return completionContext{mode: modeFields, path: base, text: text, line: cursorLine, col: cursorCol}
}

// pathBeforeCursor computes the schema path of the mapping context containing
// the cursor by walking the lines above it. Container keys (empty values) are
// pushed onto a stack; `- <key>:` item keys are pushed one level below their
// dash line, matching the schema path that treats sequence items without an
// index (e.g. "spec.roleMappings").
func pathBeforeCursor(text string, cursorLine int) string {
	lines := strings.Split(text, "\n")
	type stackEntry struct {
		key    string
		indent int
	}
	var stack []stackEntry

	for i := 0; i < cursorLine-1 && i < len(lines); i++ {
		ln := lines[i]
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(ln) - len(strings.TrimLeft(ln, " "))

		key := ""
		keyIndent := -1
		if strings.HasPrefix(trimmed, "-") {
			rest := strings.TrimSpace(trimmed[1:])
			if c := strings.Index(rest, ":"); c > 0 {
				k := strings.TrimSpace(rest[:c])
				if v := strings.TrimSpace(rest[c+1:]); v == "" {
					key, keyIndent = k, indent+1
				}
			}
		} else if c := strings.Index(trimmed, ":"); c > 0 {
			k := strings.TrimSpace(trimmed[:c])
			if v := strings.TrimSpace(trimmed[c+1:]); v == "" {
				key, keyIndent = k, indent
			}
		}

		if keyIndent >= 0 {
			for len(stack) > 0 && stack[len(stack)-1].indent >= keyIndent {
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, stackEntry{key: key, indent: keyIndent})
		}
	}

	parts := make([]string, len(stack))
	for i, e := range stack {
		parts[i] = e.key
	}
	return strings.Join(parts, ".")
}

func joinSchemaPath(base, key string) string {
	if base == "" {
		return key
	}
	return base + "." + key
}

// existingKeysInBlock returns the keys already present in the mapping block
// that contains the cursor (same indentation, between the previous shallower
// line and the cursor). Used to avoid re-suggesting fields already written.
func existingKeysInBlock(text string, cursorLine int) map[string]bool {
	lines := strings.Split(text, "\n")
	if cursorLine < 1 || cursorLine > len(lines) {
		return nil
	}

	curIndent := len(lines[cursorLine-1]) - len(strings.TrimLeft(lines[cursorLine-1], " "))
	// A blank cursor line takes the indentation of the nearest preceding
	// non-blank line so nested blocks are resolved against their keys.
	if strings.TrimSpace(lines[cursorLine-1]) == "" {
		for k := cursorLine - 2; k >= 0; k-- {
			t := strings.TrimSpace(lines[k])
			if t != "" && !strings.HasPrefix(t, "#") {
				curIndent = len(lines[k]) - len(strings.TrimLeft(lines[k], " "))
				break
			}
		}
	}

	blockStart := 0
	for i := cursorLine - 2; i >= 0; i-- {
		if ind := len(lines[i]) - len(strings.TrimLeft(lines[i], " ")); ind < curIndent {
			blockStart = i + 1
			break
		}
	}

	keys := make(map[string]bool)
	for i := blockStart; i < cursorLine-1 && i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "-") {
			continue
		}
		if c := strings.Index(trimmed, ":"); c > 0 {
			keys[strings.TrimSpace(trimmed[:c])] = true
		}
	}
	return keys
}

// suggest creates the LSP completion items for the resolved context against
// the kind's schema. It only surfaces what the schema declares (fields from
// properties, values from enum/default, reference names from the index) —
// nothing is invented.
func suggest(ctx completionContext, root *schema.FieldSchema, idx ResourceIndexer) []protocol.CompletionItem {
	switch ctx.mode {
	case modeValue:
		node := schema.FindSchema(root, ctx.path)
		if node == nil {
			return nil
		}
		// The cursor rests on an object/array key's value slot (`metadata:`):
		// the user wants that block's fields.
		if node.Type == valueObject || len(node.Properties) > 0 {
			return buildFieldItems(node, ctx)
		}
		if node.Type == valueArray {
			if item := node.Items; item != nil && len(item.Properties) > 0 {
				return buildFieldItems(item, ctx)
			}
			return nil
		}
		// `kind:` values are the valid resource kind names.
		if ctx.field == valueKind && ctx.path == valueKind {
			return kindNameCompletions(ctx.text, ctx.line, ctx.col)
		}
		// A `name:` value under an object with a sibling `kind:` is a resource
		// reference → suggest resource names from the workspace index.
		if names, ok := referenceNames(ctx, idx); ok {
			return buildNameItems(names, ctx)
		}
		if len(node.Enum) > 0 || node.Default != "" {
			return buildValueItems(node, ctx)
		}
		return nil

	case modeArrayItem:
		node := schema.FindSchema(root, ctx.path)
		if node == nil {
			return nil
		}
		item := node
		if node.Type == valueArray {
			item = node.Items
		}
		if item == nil {
			return nil
		}
		if len(item.Properties) > 0 {
			return buildFieldItems(item, ctx)
		}
		return nil

	default: // modeFields
		node := schema.FindSchema(root, ctx.path)
		if node == nil {
			return nil
		}
		if node.Type == valueArray {
			if item := node.Items; item != nil && len(item.Properties) > 0 {
				return buildFieldItems(item, ctx)
			}
			return nil
		}
		if node.Type == valueObject || len(node.Properties) > 0 {
			return buildFieldItems(node, ctx)
		}
		return nil
	}
}

// ---------------------------------------------------------------------------
// Value candidates (enum / default / reference names)
// ---------------------------------------------------------------------------

// buildValueItems renders the schema-declared value candidates (enum and a
// real default) for a `key:` value slot.
func buildValueItems(node *schema.FieldSchema, ctx completionContext) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	seen := make(map[string]bool)

	if len(node.Enum) > 0 {
		for _, e := range node.Enum {
			if seen[e] || e == "" {
				continue
			}
			seen[e] = true
			items = append(items, valueItem(e, "enum value", protocol.CompletionItemKindValue, e, ctx))
		}
	}
	if node.Default != "" && !seen[node.Default] {
		items = append(items, valueItem(node.Default, "default value", protocol.CompletionItemKindValue, node.Default, ctx))
	}
	return items
}

// buildNameItems renders workspace resource names for a `name:` reference.
func buildNameItems(names []string, ctx completionContext) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	seen := make(map[string]bool)
	for _, n := range names {
		if seen[n] || n == "" {
			continue
		}
		seen[n] = true
		items = append(items, valueItem(n, "resource name", protocol.CompletionItemKindReference, n, ctx))
	}
	return items
}

// valueItem builds a completion item for a scalar value, replacing the value
// slot of the cursor line so accepting yields `key: <value>` instead of
// appending to whatever partial text is already there.
func valueItem(label, detail string, kind protocol.CompletionItemKind, value string, ctx completionContext) protocol.CompletionItem {
	item := protocol.CompletionItem{
		Label:      label,
		Kind:       &kind,
		Detail:     &detail,
		FilterText: &label,
		SortText:   &label,
		InsertText: &value,
	}
	if r, ok := valueEditRange(ctx); ok {
		edit := protocol.TextEdit{Range: r, NewText: value}
		item.TextEdit = &edit
	}
	return item
}

// valueEditRange returns the range of the value slot on the cursor line
// (from just past the `key:` spacing to end of line).
func valueEditRange(ctx completionContext) (protocol.Range, bool) {
	if ctx.text == "" || ctx.line < 1 {
		return protocol.Range{}, false
	}
	lines := strings.Split(ctx.text, "\n")
	if ctx.line-1 >= len(lines) {
		return protocol.Range{}, false
	}
	line := lines[ctx.line-1]

	colonAbs := -1
	if c := strings.Index(line, ":"); c >= 0 {
		colonAbs = c
	}
	if colonAbs < 0 {
		return protocol.Range{}, false
	}
	start := colonAbs + 1
	for start < len(line) && line[start] == ' ' {
		start++
	}
	if start > len(line) {
		start = len(line)
	}
	return protocol.Range{
		Start: protocol.Position{Line: lsputil.PositionIndex(ctx.line - 1), Character: lsputil.PositionIndex(lsputil.UTF16Length(line[:start]))},
		End:   protocol.Position{Line: lsputil.PositionIndex(ctx.line - 1), Character: lsputil.PositionIndex(lsputil.UTF16Length(line))},
	}, true
}

// referenceNames resolves the indexer names for the `name:` reference on the
// cursor line. A reference is recognized structurally: the enclosing mapping
// has a `kind` sibling (the kind+name pattern shared by every OpenChoreo
// reference object). Suggestions are filtered to the sibling kind's resource
// names, falling back to all kinds when no/unknown kind is present.
func referenceNames(ctx completionContext, idx ResourceIndexer) ([]string, bool) {
	if idx == nil || ctx.field != "name" {
		return nil, false
	}
	kindRef, ok := siblingKindValue(ctx)
	if !ok {
		return nil, false
	}

	byKind := idx.NamesByKind()
	if kindRef != "" {
		if names := byKind[kindRef]; len(names) > 0 {
			return names, true
		}
	}
	var all []string
	seen := make(map[string]bool)
	for _, ns := range byKind {
		for _, n := range ns {
			if !seen[n] {
				seen[n] = true
				all = append(all, n)
			}
		}
	}
	return all, len(all) > 0
}

// siblingKindValue scans upward from the cursor line for a `kind:` key at the
// same indentation (a sibling inside the enclosing mapping), returning its
// trimmed value.
func siblingKindValue(ctx completionContext) (string, bool) {
	if ctx.text == "" || ctx.line < 1 {
		return "", false
	}
	lines := strings.Split(ctx.text, "\n")
	if ctx.line-1 >= len(lines) {
		return "", false
	}
	curIndent := len(lines[ctx.line-1]) - len(strings.TrimLeft(lines[ctx.line-1], " "))

	for i := ctx.line - 2; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		ind := len(lines[i]) - len(strings.TrimLeft(lines[i], " "))
		if ind < curIndent {
			break // left the enclosing mapping
		}
		if strings.HasPrefix(trimmed, "kind:") && ind == curIndent {
			v := strings.TrimSpace(strings.TrimPrefix(trimmed, "kind:"))
			return strings.Trim(v, `"'`), true
		}
	}
	return "", false
}

// ---------------------------------------------------------------------------
// Field completion items
// ---------------------------------------------------------------------------

// buildFieldItems creates one completion item per property of an object
// schema node, required fields first.
func buildFieldItems(node *schema.FieldSchema, ctx completionContext) []protocol.CompletionItem {
	names := sortedFieldNames(node)
	items := make([]protocol.CompletionItem, 0, len(names))
	for _, n := range names {
		items = append(items, buildFieldItem(n, node.Properties[n], ctx))
	}
	return items
}

// buildFieldItem creates a single CompletionItem for a field.
func buildFieldItem(name string, node *schema.FieldSchema, ctx completionContext) protocol.CompletionItem {
	kind := completionItemKind(node)
	detail := schemaDetail(node)

	// Sort required fields first ("0" prefix) then optional ("1" prefix).
	sortPrefix := "1"
	if node != nil && node.Required {
		sortPrefix = "0"
	}
	sortText := sortPrefix + name

	item := protocol.CompletionItem{
		Label:      name,
		Kind:       &kind,
		Detail:     &detail,
		FilterText: &name,
		SortText:   &sortText,
	}

	if node != nil && node.Required {
		preselect := true
		item.Preselect = &preselect
	}

	// Array items (`- ` lines) get shallow `key:` inserts; the editor already
	// prefixes the dash, so deep object expansion would misalign indentation.
	if ctx.mode == modeArrayItem {
		insertText := name + shallowLeafSuffix(node)
		item.InsertText = &insertText
		return item
	}

	insertText := buildInsertText(name, node)
	item.InsertText = &insertText
	if strings.Contains(insertText, "\n") {
		format := protocol.InsertTextFormatSnippet
		item.InsertTextFormat = &format
	}
	return item
}

// shallowLeafSuffix is the `: value`/`:` suffix used when completing a key on
// a `- ` array-item line (no deep expansion).
func shallowLeafSuffix(node *schema.FieldSchema) string {
	if node == nil {
		return ": ${1:value}"
	}
	switch node.Type {
	case valueObject, valueArray:
		return ":"
	default:
		return ": " + leafValuePlaceholder(node)
	}
}

// buildInsertText produces the snippet text for a field completion.
// Object/array fields are expanded from the schema so accepting a field
// yields a complete, valid structure instead of a bare `key:`.
func buildInsertText(name string, node *schema.FieldSchema) string {
	if node == nil {
		return name + ": "
	}

	switch node.Type {
	case valueObject:
		return name + ":" + expandObjectBody(name, node, "  ")
	case valueArray:
		return name + ":" + expandArrayBody(node, "  ", 1)
	default:
		return name + ": " + dummyValue(node)
	}
}

// sortedFieldNames returns the object's property names, required fields first
// and then alphabetically, so expansions are deterministic and schema-accurate.
func sortedFieldNames(node *schema.FieldSchema) []string {
	if node == nil {
		return nil
	}
	names := make([]string, 0, len(node.Properties))
	for name := range node.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	sort.SliceStable(names, func(i, j int) bool {
		ri := node.Properties[names[i]].Required
		rj := node.Properties[names[j]].Required
		if ri != rj {
			return ri
		}
		return false
	})
	return names
}

// dummyValue returns the value for a leaf field inside a snippet expansion.
// Resolution order: a real schema "default" wins, then a single enum option
// (options are never invented — a multi-option enum becomes a tab-stop
// choice). Without either, a typed tab-stop placeholder is emitted.
func dummyValue(node *schema.FieldSchema) string {
	return leafValuePlaceholder(node)
}

// leafValuePlaceholder is the schema-only placeholder for a leaf value.
func leafValuePlaceholder(node *schema.FieldSchema) string {
	if node == nil {
		return "${1:value}"
	}
	switch node.Type {
	case valueString:
		if node.Default != "" {
			return yamlSafeString(node.Default)
		}
		if len(node.Enum) == 1 {
			return yamlSafeString(node.Enum[0])
		}
		if len(node.Enum) > 1 {
			return "${1|" + strings.Join(node.Enum, ",") + "|}"
		}
		return "${1:value}"
	case valueInteger, valueNumber:
		if node.Default != "" {
			return node.Default
		}
		return "${1:0}"
	case valueBoolean:
		if node.Default != "" {
			return node.Default
		}
		return "${1:true}"
	default:
		return ""
	}
}

// yamlSafeString quotes a produced string value when YAML would otherwise
// reinterpret it as a boolean or null scalar (e.g. True, yes, null). Plain
// strings like names and timestamps are left unquoted.
func yamlSafeString(s string) string {
	lower := strings.ToLower(s)
	switch lower {
	case "true", "false", "yes", "no", "on", "off", "null", "~", "":
		return `"` + s + `"`
	}
	return s
}

// expandObjectBody expands an object's children, each indented under the key.
func expandObjectBody(name string, node *schema.FieldSchema, indent string) string {
	if node == nil {
		return "\n" + indent
	}
	if node.Preserve {
		// Free-form map whose keys are not defined in the schema: emit a
		// representative OpenChoreo-style example entry so the field is not
		// left empty.
		return preserveBody(name, indent)
	}
	names := sortedFieldNames(node)
	if len(names) == 0 {
		return "\n" + indent
	}
	var b strings.Builder
	for _, n := range names {
		child := node.Properties[n]
		b.WriteString("\n" + indent)
		switch child.Type {
		case valueObject:
			b.WriteString(n + ":" + expandObjectBody(n, child, indent+"  "))
		case valueArray:
			b.WriteString(n + ":" + expandArrayBody(child, indent+"  ", 1))
		default:
			b.WriteString(n + ": " + dummyValue(child))
		}
	}
	return b.String()
}

// preserveBody returns a representative example entry for a free-form map
// whose keys are not defined in the schema. Well-known OpenChoreo/Kubernetes
// metadata maps get their conventional keys; anything else gets a generic
// `key: value` placeholder. The body is returned as newline + indented entries
// to be appended directly after the owning `key:`.
func preserveBody(name string, indent string) string {
	var b strings.Builder
	entryIndent := indent
	switch name {
	case "annotations":
		b.WriteString("\n" + entryIndent + "openchoreo.dev/description: ${1:Your project description}\n")
		b.WriteString(entryIndent + "openchoreo.dev/display-name: ${1:My Project}")
	case "labels":
		b.WriteString("\n" + entryIndent + "openchoreo.dev/name: ${1:default}")
	default:
		b.WriteString("\n" + entryIndent + "key: ${1:value}")
	}
	return b.String()
}

// expandArrayBody expands an array with a single sample item (for arrays of
// objects/arrays, that item's children are expanded underneath it).
func expandArrayBody(node *schema.FieldSchema, indent string, depth int) string {
	if node == nil || depth > 3 {
		return "\n" + indent + "- "
	}
	item := node.Items
	if item == nil {
		return "\n" + indent + "- "
	}
	childIndent := indent + "  "
	switch item.Type {
	case valueObject:
		return "\n" + indent + "-" + expandObjectBody("item", item, childIndent)
	case valueArray:
		return "\n" + indent + "-" + expandArrayBody(item, childIndent, depth+1)
	default:
		return "\n" + indent + "- " + dummyValue(item)
	}
}

// completionItemKind maps a schema node type to an LSP CompletionItemKind.
func completionItemKind(node *schema.FieldSchema) protocol.CompletionItemKind {
	if node == nil {
		return protocol.CompletionItemKindField
	}
	switch node.Type {
	case valueObject:
		if len(node.Properties) > 0 {
			return protocol.CompletionItemKindModule
		}
		return protocol.CompletionItemKindField
	case valueArray:
		return protocol.CompletionItemKindEnum
	case valueString:
		if len(node.Enum) > 0 {
			return protocol.CompletionItemKindEnum
		}
		return protocol.CompletionItemKindValue
	case valueInteger, valueNumber, valueBoolean:
		return protocol.CompletionItemKindValue
	default:
		return protocol.CompletionItemKindField
	}
}

// schemaDetail builds the detail string shown alongside a completion item.
func schemaDetail(node *schema.FieldSchema) string {
	if node == nil {
		return ""
	}
	var parts []string
	if node.Type != "" {
		parts = append(parts, node.Type)
	}
	if node.Required {
		parts = append(parts, "required")
	} else {
		parts = append(parts, "optional")
	}
	if len(node.Enum) > 0 {
		parts = append(parts, "enum: "+strings.Join(node.Enum, ", "))
	}
	if node.Pattern != "" {
		parts = append(parts, "pattern: "+node.Pattern)
	}
	return strings.Join(parts, " | ")
}

// filterExistingItems removes completion items whose label matches an
// existing key in the current mapping.
func filterExistingItems(items []protocol.CompletionItem, existing map[string]bool) []protocol.CompletionItem {
	if len(existing) == 0 {
		return items
	}
	filtered := items[:0]
	for _, item := range items {
		if existing[item.Label] {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

// ---------------------------------------------------------------------------
// Raw text analysis (fallback for broken YAML)
// ---------------------------------------------------------------------------

// cursorLineIndent returns the number of leading spaces on the cursor line
// (1-based cursorLine), or -1 when the line is out of range. A zero-indent
// line means the cursor is at the root level, about to start a top-level key.
func cursorLineIndent(text string, cursorLine int) int {
	lines := strings.Split(text, "\n")
	if cursorLine < 1 || cursorLine > len(lines) {
		return -1
	}
	line := lines[cursorLine-1]
	return len(line) - len(strings.TrimLeft(line, " "))
}

// extractKindFromText pulls the value of `kind:` from raw YAML text.
func extractKindFromText(text string) string {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "kind:") {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "kind:"))
			if val != "" {
				return val
			}
		}
	}
	return ""
}

// extractExistingKeys returns a set of top-level keys found in raw text.
func extractExistingKeys(text string) map[string]bool {
	keys := make(map[string]bool)
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		colonIdx := strings.Index(trimmed, ":")
		if colonIdx > 0 {
			keys[strings.TrimSpace(trimmed[:colonIdx])] = true
		}
	}
	return keys
}

// isKindValueLine reports whether the cursor is on a bare "kind:" line whose
// value is still empty, i.e. exactly where the user would type a resource kind.
func isKindValueLine(text string, cursorLine int) bool {
	lines := strings.Split(text, "\n")
	if cursorLine < 1 || cursorLine > len(lines) {
		return false
	}
	line := strings.TrimSpace(lines[cursorLine-1])
	if !strings.HasPrefix(line, "kind:") {
		return false
	}
	val := strings.TrimSpace(strings.TrimPrefix(line, "kind:"))
	return val == ""
}

// kindNameCompletions returns the valid OpenChoreo resource kind names as
// completion items, for autocompleting the `kind` field value. The returned
// items replace the existing `kind:` prefix (whether it already has a trailing
// space or not) with `kind: <name>` so the result is always valid YAML.
func kindNameCompletions(text string, cursorLine, cursorCol int) []protocol.CompletionItem {
	line := ""
	if lines := strings.Split(text, "\n"); cursorLine >= 1 && cursorLine <= len(lines) {
		line = lines[cursorLine-1]
	}
	if cursorCol > lsputil.UTF16Length(line) {
		cursorCol = lsputil.UTF16Length(line)
	}
	// Locate the start of the valueKind prefix on this line (its leading
	// indentation), then replace everything from there up to the cursor.
	kindPos := strings.Index(line, "kind:")
	if kindPos < 0 {
		kindPos = 0
	}

	kinds := template.AllKinds()
	items := make([]protocol.CompletionItem, 0, len(kinds))
	for _, k := range kinds {
		name := k
		kind := protocol.CompletionItemKindEnum
		detail := "resource kind"
		replacement := line[:kindPos] + "kind: " + name + "\n"
		newText := name + "\n"
		edit := protocol.TextEdit{
			Range: protocol.Range{
				Start: protocol.Position{Line: lsputil.PositionIndex(max(cursorLine, 1) - 1), Character: lsputil.PositionIndex(lsputil.UTF16Length(line[:kindPos]))},
				End:   protocol.Position{Line: lsputil.PositionIndex(max(cursorLine, 1) - 1), Character: lsputil.PositionIndex(cursorCol)},
			},
			NewText: replacement,
		}
		format := protocol.InsertTextFormatSnippet
		sortText := "0" + name
		item := protocol.CompletionItem{
			Label:            name,
			Kind:             &kind,
			Detail:           &detail,
			TextEdit:         &edit,
			InsertText:       &newText,
			InsertTextFormat: &format,
			FilterText:       &name,
			SortText:         &sortText,
		}
		items = append(items, item)
	}
	return items
}
