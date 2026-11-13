package codecompletion

import (
	"fmt"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/schema"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/template"
	"gopkg.in/yaml.v3"
	"regexp"
	"sort"
	"strings"
)

// RequiredPreview uses the existing completion context resolver, but generates
// only a single safe insertion of missing authoring fields.
func RequiredPreview(text string, line, column int, indent string) string {
	return requiredPreviewWithSchema(text, line, column, indent, template.SchemaFor)
}

func requiredPreviewWithSchema(text string, line, column int, indent string, lookup func(string) *schema.FieldSchema) string {
	allLines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if line < 0 || line >= len(allLines) || column != len(allLines[line]) {
		return ""
	}
	// Resolve only the current YAML document; a neighbouring kind is unrelated.
	start, end := 0, len(allLines)
	separator := regexp.MustCompile(`^(---|\.\.\.)(\s+#.*)?\s*$`)
	for i, l := range allLines {
		if separator.MatchString(l) {
			if i == line {
				return ""
			}
			if i < line {
				start = i + 1
			} else {
				end = i
				break
			}
		}
	}
	lines := allLines[start:end]
	localLine := line - start
	originalLines := append([]string{}, lines...)
	originalLine := lines[localLine]
	typedKey := ""
	if regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`).MatchString(strings.TrimSpace(originalLine)) {
		typedKey = strings.TrimSpace(originalLine)
		lines = append([]string{}, lines...)
		lines[localLine] = strings.TrimRight(originalLine, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-")
	}
	source := strings.Join(lines, "\n")
	docs, err := parser.ParseYAML([]byte(source))
	if err != nil || len(docs) != 1 {
		return ""
	}
	d := docs[0]
	if !parser.IsOpenChoreoAPIVersion(d.APIVersion) {
		return ""
	}
	root := lookup(d.Kind)
	if root == nil {
		return ""
	}
	current := strings.TrimSpace(lines[localLine])
	header := strings.HasPrefix(lines[localLine], "kind:") && strings.TrimSpace(strings.TrimPrefix(lines[localLine], "kind:")) == d.Kind
	if current != "" && !header {
		return ""
	}
	path := ""
	if !header {
		ctx := resolveContext(source, localLine+1, len(lines[localLine])+1)
		if ctx.mode != modeFields {
			return ""
		}
		path = ctx.path
	}
	existing, path, ok := previewMappingContext(d.Root, path, lines, localLine)
	if !ok {
		return ""
	}

	if existing.Kind != yaml.MappingNode && !(existing.Kind == yaml.ScalarNode && existing.Tag == "!!null") {
		return ""
	}
	prefix := lines[localLine]
	if header {
		prefix = ""
	}
	// YAML indentation must contain spaces, even if the editor uses tabs elsewhere.
	if strings.Contains(prefix, "\t") {
		return ""
	}
	if indent == "" || strings.Contains(indent, "\t") {
		indent = "  "
	}
	count := 0
	var expand func(*schema.FieldSchema, string, string) string
	expand = func(n *schema.FieldSchema, prefix string, objectPath string) string {
		if n == nil {
			return ""
		}
		out := ""
		for _, name := range previewFieldNames(n) {
			child := n.Properties[name]
			childPath := joinSchemaPath(objectPath, name)
			body := ""
			if child.Type == "object" {
				body = expand(child, prefix+indent, childPath)
			}
			if !child.Required && body == "" && !referenceKindHint(n, name, child) {
				continue
			}
			if child.Type == "object" {
				if body == "" {
					out += prefix + name + ": {}\n"
				} else {
					out += prefix + name + ":\n" + body
				}
				continue
			}
			if child.Type == "array" {
				listPrefix := prefix + indent
				if child.Items == nil {
					out += prefix + name + ":\n" + listPrefix + "- {}\n"
					continue
				}
				item := *child.Items
				item.Required = true
				wrapper := &schema.FieldSchema{Type: "object", Properties: map[string]*schema.FieldSchema{"item": &item}}
				itemBody := expand(wrapper, listPrefix, childPath+"[]")
				rendered := strings.TrimPrefix(itemBody, listPrefix+"item:")
				if item.Type == "object" && strings.HasPrefix(rendered, "\n"+listPrefix+indent) {
					lines := strings.Split(strings.TrimPrefix(rendered, "\n"), "\n")
					// A compact sequence item's keys align two columns after
					// the dash, independently of the editor's indent width.
					for i, line := range lines {
						if strings.HasPrefix(line, listPrefix+indent) {
							lines[i] = listPrefix + "  " + strings.TrimPrefix(line, listPrefix+indent)
						}
					}
					lines[0] = strings.TrimPrefix(lines[0], listPrefix+"  ")
					out += prefix + name + ":\n" + listPrefix + "- " + strings.Join(lines, "\n")
				} else {
					out += prefix + name + ":\n" + listPrefix + "-" + rendered
				}
				continue
			}
			count++
			placeholder := "<" + placeholderLabel(name) + ">"
			if childPath == "metadata.name" {
				placeholder = placeholderLabel(d.Kind) + "-name"
			}
			value := fmt.Sprintf("${%d:%s}", count, placeholder)
			switch child.Type {
			case "integer", "number":
				value = fmt.Sprintf("${%d:0}", count)
			case "boolean":
				value = fmt.Sprintf("${%d:false}", count)
			case "array":
				value = "[]"
			}
			if child.Type == "string" && child.Default == "" && len(child.Enum) == 0 && child.Pattern != "" {
				if example, ok := patternExample(child.Pattern); ok {
					encoded, err := yaml.Marshal(example)
					if err != nil {
						return ""
					}
					escaped := strings.NewReplacer("\\", "\\\\", "$", "\\$", "}", "\\}").Replace(strings.TrimSuffix(string(encoded), "\n"))
					value = fmt.Sprintf("${%d:%s}", count, escaped)
				}
			}
			literal := child.Default
			editableEnum := literal == "" && len(child.Enum) > 1
			if literal == "" && len(child.Enum) > 0 {
				literal = child.Enum[0]
			}
			if literal != "" || len(child.Enum) > 0 {
				if child.Type == "string" {
					encoded, err := yaml.Marshal(literal)
					if err != nil {
						return ""
					}
					literal = strings.TrimSuffix(string(encoded), "\n")
				}
				value = strings.NewReplacer("\\", "\\\\", "$", "\\$", "}", "\\}").Replace(literal)
				if editableEnum {
					value = fmt.Sprintf("${%d:%s}", count, value)
				}
			}

			out += prefix + name + ": " + value + "\n"
		}
		return out
	}
	// Existing headers must never be generated again.
	copyRoot := *root
	copyRoot.Properties = map[string]*schema.FieldSchema{}
	for k, v := range root.Properties {
		if k != "apiVersion" && k != "kind" && k != "status" {
			copyRoot.Properties[k] = v
		}
	}
	// Metadata identity is required by the shared RuleEngine, even where the
	// versioned structural template marks it optional. Copy authoring hints;
	// never change the cached template or its validation semantics.
	if original := root.Properties["metadata"]; original != nil {
		metadata := *original
		metadata.Properties = map[string]*schema.FieldSchema{}
		for name, field := range original.Properties {
			metadata.Properties[name] = field
		}
		for _, name := range []string{"name", "namespace"} {
			if name == "namespace" && strings.HasPrefix(d.Kind, "Cluster") {
				continue
			}
			if field := original.Properties[name]; field != nil {
				copyField := *field
				copyField.Required = true
				if name == "namespace" && copyField.Default == "" {
					copyField.Default = "default"
				}
				metadata.Properties[name] = &copyField
			}
		}
		copyRoot.Properties["metadata"] = &metadata
	}
	target := &copyRoot
	if path != "" {
		target = schema.FindSchema(&copyRoot, path)
	}
	if target == nil || target.Type != "object" || target.Preserve {
		return ""
	}
	missing := *target
	missing.Properties = map[string]*schema.FieldSchema{}
	for name, field := range target.Properties {
		if mappingValue(existing, name) == nil && (typedKey == "" || strings.HasPrefix(name, typedKey)) {
			if referenceKindHint(target, name, field) {
				copyField := *field
				copyField.Required = true
				missing.Properties[name] = &copyField
			} else {
				missing.Properties[name] = field
			}
		}
	}
	result := strings.TrimSuffix(expand(&missing, prefix, path), "\n")
	if result == "" {
		return ""
	}
	if header {
		result = "\n" + result
	} else {
		result = strings.TrimPrefix(result, prefix)
		if typedKey != "" {
			if !strings.HasPrefix(result, typedKey) {
				return ""
			}
			result = strings.TrimPrefix(result, typedKey)
		}
	}
	// Parse the accepted placeholder defaults, not snippet syntax. This catches
	// unsafe indentation, duplicate mappings and joins with following YAML.
	concrete := regexp.MustCompile(`\$\{[0-9]+:([^}]*)\}`).ReplaceAllString(result, "$1")
	concrete = strings.NewReplacer(`\\`, `\`, `\$`, `$`, `\}`, `}`).Replace(concrete)
	candidate := append([]string{}, originalLines...)
	candidate[localLine] += concrete
	var decoded any
	if err := yaml.Unmarshal([]byte(strings.Join(candidate, "\n")), &decoded); err != nil {
		return ""
	}
	if strings.Contains(text, "\r\n") {
		result = strings.ReplaceAll(result, "\n", "\r\n")
	}
	return result
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// Reference templates may mark kind optional because they declare a default,
// while shared reference validation requires it explicitly. Recognize the
// schema shape instead of keeping a resource-specific reference-field list.
func referenceKindHint(parent *schema.FieldSchema, name string, child *schema.FieldSchema) bool {
	if name != "kind" || child == nil || child.Type != "string" || parent.Properties["name"] == nil {
		return false
	}
	if parser.KnownKinds[child.Default] {
		return true
	}
	for _, kind := range child.Enum {
		if parser.KnownKinds[kind] {
			return true
		}
	}
	return false
}

// Convert schema field identifiers into readable, resource-independent labels.
func placeholderLabel(field string) string {
	field = regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`).ReplaceAllString(field, "${1}-${2}")
	field = regexp.MustCompile(`([a-z0-9])([A-Z])`).ReplaceAllString(field, "${1}-${2}")
	return strings.ToLower(strings.ReplaceAll(field, "_", "-"))
}

// Preview ordering groups leaf values before nested structures, without
// changing dropdown completion ordering or the user's existing mappings.
func previewFieldNames(node *schema.FieldSchema) []string {
	names := sortedFieldNames(node)
	nested := func(name string) bool { typ := node.Properties[name].Type; return typ == "object" || typ == "array" }
	sort.Slice(names, func(i, j int) bool {
		a, b := nested(names[i]), nested(names[j])
		if a != b {
			return !a
		}
		return names[i] < names[j]
	})
	return names
}

// RequiredPreviewWithIndex overlays only resolved dynamic parameter schemas on
// a private copy of the template, retaining the existing insertion safeguards.
func RequiredPreviewWithIndex(text string, line, column int, indent, uri string, idx ResourceIndexer) string {
	var dynamic *schema.FieldSchema
	var path string
	dynamicItems(text, line+1, column+1, idx, uri, func(p string, s *schema.FieldSchema) { path, dynamic = p, s })
	choices, policyKnown, doc := allowedWorkflowChoices(text, line+1, uri, idx)
	bindingFields := bindingPreviewFields(text, line+1, uri, idx)
	if dynamic == nil && !policyKnown && len(bindingFields) == 0 {
		return RequiredPreview(text, line, column, indent)
	}
	// Required is used only as an authoring inclusion flag on this private
	// resolved schema copy. It does not change the validation schema.
	var mark func(*schema.FieldSchema)
	mark = func(n *schema.FieldSchema) {
		if n == nil {
			return
		}
		n.Required = true
		for _, child := range n.Properties {
			mark(child)
		}
		mark(n.Items)
	}
	mark(dynamic)
	for _, field := range bindingFields {
		mark(field)
	}
	return requiredPreviewWithSchema(text, line, column, indent, func(kind string) *schema.FieldSchema {
		base := template.SchemaFor(kind)
		if base == nil {
			return nil
		}
		copied := cloneCompletionSchema(base)
		if copied == nil {
			return nil
		}
		root := *copied
		if spec := schema.FindSchema(&root, "spec"); spec != nil {
			for name, field := range bindingFields {
				spec.Properties[name] = field
			}
		}
		if policyKnown {
			applyWorkflowPolicy(&root, choices, doc)
		}
		if dynamic == nil {
			return &root
		}
		parts := strings.Split(path, ".")
		parent := schema.FindSchema(&root, strings.Join(parts[:len(parts)-1], "."))
		if parent != nil {
			if parent.Type == "array" {
				parent = parent.Items
			}
			if parent != nil {
				if parent.Properties == nil {
					parent.Properties = map[string]*schema.FieldSchema{}
				}
				parent.Properties[parts[len(parts)-1]] = dynamic
			}
		}
		return &root
	})
}

// previewMappingContext trims lexical container paths to the cursor's actual
// indentation. Both schema lookup and rendering must use this same context.
func previewMappingContext(root *yaml.Node, path string, lines []string, localLine int) (*yaml.Node, string, bool) {
	existing := root
	if existing.Kind == yaml.DocumentNode {
		existing = existing.Content[0]
	}
	// The ordinary resolver retains open containers across blank lines. Trim
	// its path to the actual cursor indentation so Shift+Tab returns to the
	// parent mapping rather than suggesting nested keys at the root.
	var resolved []string
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			continue
		}
		if existing.Kind == yaml.SequenceNode {
			var selected *yaml.Node
			for _, item := range existing.Content {
				if item.Line <= localLine+1 {
					selected = item
				}
			}
			if selected == nil {
				return nil, "", false
			}
			existing = selected
		}
		var key, value *yaml.Node
		if existing.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(existing.Content); i += 2 {
				if existing.Content[i].Value == part {
					key = existing.Content[i]
					value = existing.Content[i+1]
					break
				}
			}
		}
		if key == nil {
			return nil, "", false
		}
		if key.Column-1 >= len(lines[localLine]) {
			break
		}
		resolved = append(resolved, part)
		existing = value
	}
	path = strings.Join(resolved, ".")

	return existing, path, true
}
