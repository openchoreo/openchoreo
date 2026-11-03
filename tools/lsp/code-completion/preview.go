package codecompletion

import (
	"fmt"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/schema"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/template"
	"gopkg.in/yaml.v3"
	"regexp"
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
	existing := d.Root
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
			return ""
		}
		if key.Column-1 >= len(lines[localLine]) {
			break
		}
		resolved = append(resolved, part)
		existing = value
	}
	path = strings.Join(resolved, ".")

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
	var expand func(*schema.FieldSchema, string) string
	expand = func(n *schema.FieldSchema, prefix string) string {
		if n == nil {
			return ""
		}
		out := ""
		for _, name := range sortedFieldNames(n) {
			child := n.Properties[name]
			body := ""
			if child.Type == "object" {
				body = expand(child, prefix+indent)
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
				itemBody := expand(wrapper, listPrefix)
				out += prefix + name + ":\n" + listPrefix + "-" + strings.TrimPrefix(itemBody, listPrefix+"item:")
				continue
			}
			count++
			value := fmt.Sprintf("${%d:value}", count)
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
	result := strings.TrimSuffix(expand(&missing, prefix), "\n")
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
