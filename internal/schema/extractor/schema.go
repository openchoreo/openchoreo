// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package extractor

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

const (
	typeString  = "string"
	typeInteger = "integer"
	typeNumber  = "number"
	typeBoolean = "boolean"
	typeObject  = "object"
	typeArray   = "array"
)

// Options configures schema extraction behavior.
type Options struct {
	// RequiredByDefault determines whether fields without explicit 'required' or 'default'
	// markers are treated as required. Default: true.
	RequiredByDefault bool

	// ErrorOnUnknownMarkers causes parsing to fail when encountering unknown constraint markers.
	// Default: false (unknown markers are silently ignored).
	ErrorOnUnknownMarkers bool
}

// DefaultOptions returns the default options for schema extraction.
func DefaultOptions() Options {
	return Options{
		RequiredByDefault:     true,
		ErrorOnUnknownMarkers: false,
	}
}

// ExtractSchema converts a field map using shorthand schema syntax into OpenAPI v3 JSON Schema.
//
// This is the primary API for converting ComponentTypeDefinition/Addon schemas from the
// compact shorthand format into full JSON Schema that Kubernetes can validate against.
//
// The shorthand syntax allows concise schema definitions:
//   - Basic types: replicas: "integer"
//   - With constraints: port: "integer | minimum=1 | maximum=65535"
//   - With defaults: environment: "string | default=dev"
//   - Arrays: tags: "[]string"
//   - Maps: labels: "map<string>"
//   - Custom types: database: "DatabaseConfig" (references types parameter)
//
// The types parameter provides custom type definitions that can be referenced in field schemas.
// Fields are required by default unless they have a default value or explicit required=false marker.
//
// Uses default options (required by default, unknown markers ignored).
func ExtractSchema(fields map[string]any, types map[string]any) (*extv1.JSONSchemaProps, error) {
	return ExtractSchemaWithOptions(fields, types, DefaultOptions())
}

// ExtractSchemaWithOptions converts a field map with custom extraction options.
func ExtractSchemaWithOptions(fields map[string]any, types map[string]any, opts Options) (*extv1.JSONSchemaProps, error) {
	if len(fields) == 0 {
		return &extv1.JSONSchemaProps{
			Type:       typeObject,
			Properties: map[string]extv1.JSONSchemaProps{},
		}, nil
	}

	c := &converter{
		types:                 types,
		typeCache:             map[string]*extv1.JSONSchemaProps{},
		typeStack:             map[string]bool{},
		requiredByDefault:     opts.RequiredByDefault,
		errorOnUnknownMarkers: opts.ErrorOnUnknownMarkers,
	}

	return c.buildObjectSchema(fields)
}

// converter builds JSON schemas from simple schema definitions.
type converter struct {
	types                 map[string]any
	typeCache             map[string]*extv1.JSONSchemaProps
	typeStack             map[string]bool
	requiredByDefault     bool
	errorOnUnknownMarkers bool
}

// buildObjectSchema converts a field map into an object schema with properties and required markers.
//
// This is the core conversion logic that processes each field's shorthand definition and
// determines whether the field should be required.
//
// Required field logic:
//  1. If field has explicit "required=true" marker → mark as required
//  2. If field has explicit "required=false" marker → not required
//  3. If field has a default value → not required (default fills it in)
//  4. If requiredByDefault is true → mark as required
//  5. Otherwise → not required
//
// Fields are processed in sorted order to ensure deterministic JSON Schema output.
func (c *converter) buildObjectSchema(fields map[string]any) (*extv1.JSONSchemaProps, error) {
	props := map[string]extv1.JSONSchemaProps{}
	required := []string{}

	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, name := range keys {
		field := fields[name]

		schema, requiredValue, requiredExplicit, err := c.buildFieldSchema(field)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", name, err)
		}
		if schema == nil {
			continue
		}
		props[name] = *schema
		switch {
		case requiredExplicit:
			if requiredValue {
				required = append(required, name)
			}
		case schema.Default == nil && c.requiredByDefault:
			required = append(required, name)
		}
	}

	result := &extv1.JSONSchemaProps{
		Type:       typeObject,
		Properties: props,
	}
	if len(required) > 0 {
		result.Required = required
	}
	return result, nil
}

// buildFieldSchema determines the schema for a field value that may itself be an object or shorthand string.
func (c *converter) buildFieldSchema(raw any) (*extv1.JSONSchemaProps, bool, bool, error) {
	switch typed := raw.(type) {
	case string:
		return c.schemaFromString(typed)
	case map[string]any:
		schema, err := c.buildObjectSchema(typed)
		return schema, false, false, err
	default:
		return nil, false, false, fmt.Errorf("unsupported field definition of type %T", raw)
	}
}

// schemaFromString parses shorthand schema expressions into JSON Schema.
//
// The shorthand format uses a two-part syntax: "type | constraints"
//
// Part 1 (type expression):
//   - Primitive types: "string", "integer", "number", "boolean"
//   - Array types: "[]string", "array<integer>"
//   - Map types: "map<string>", "map[string]integer"
//   - Custom types: "DatabaseConfig" (must be defined in types)
//
// Part 2 (constraint expression, optional):
//   - Validation: "minimum=0 maximum=100", "minLength=1", "pattern=^[a-z]+$"
//   - Defaults: "default=dev"
//   - Enums: "enum=dev,staging,prod"
//   - Required: "required=true" or "required=false"
//   - Documentation: "description='Port number' example=8080"
//
// Returns: (schema, requiredValue, requiredExplicit, error)
//   - requiredValue: true if field should be required
//   - requiredExplicit: true if required was explicitly set (vs. inferred)
//
// Example: "integer | minimum=1 | maximum=65535 | default=8080"
func (c *converter) schemaFromString(expr string) (*extv1.JSONSchemaProps, bool, bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, false, false, fmt.Errorf("empty schema expression")
	}

	typeExpr := expr
	constraintExpr := ""
	if idx := strings.Index(expr, "|"); idx != -1 {
		typeExpr = strings.TrimSpace(expr[:idx])
		constraintExpr = strings.TrimSpace(expr[idx+1:])
	}

	schema, err := c.schemaFromType(typeExpr)
	if err != nil {
		return nil, false, false, err
	}

	required, explicit, err := c.applyConstraints(schema, constraintExpr, schema.Type)
	if err != nil {
		return nil, false, false, err
	}
	return schema, required, explicit, nil
}

// schemaFromType resolves a type expression into a JSON schema, handling arrays, maps, and custom types.
func (c *converter) schemaFromType(typeExpr string) (*extv1.JSONSchemaProps, error) {
	switch {
	case typeExpr == typeString:
		return &extv1.JSONSchemaProps{Type: typeString}, nil
	case typeExpr == typeInteger:
		return &extv1.JSONSchemaProps{Type: typeInteger}, nil
	case typeExpr == typeNumber:
		return &extv1.JSONSchemaProps{Type: typeNumber}, nil
	case typeExpr == typeBoolean:
		return &extv1.JSONSchemaProps{Type: typeBoolean}, nil
	case typeExpr == typeObject:
		return nil, fmt.Errorf("'object' type is not allowed; use a map type (e.g., 'map<string>') for free-form objects or define a structured type with explicit properties")
	case strings.HasPrefix(typeExpr, "[]"):
		itemTypeExpr := strings.TrimSpace(typeExpr[2:])
		items, err := c.schemaFromType(itemTypeExpr)
		if err != nil {
			return nil, err
		}
		return &extv1.JSONSchemaProps{
			Type: typeArray,
			Items: &extv1.JSONSchemaPropsOrArray{
				Schema: items,
			},
		}, nil
	case strings.HasPrefix(typeExpr, "array<") && strings.HasSuffix(typeExpr, ">"):
		itemTypeExpr := strings.TrimSpace(typeExpr[len("array<") : len(typeExpr)-1])
		items, err := c.schemaFromType(itemTypeExpr)
		if err != nil {
			return nil, err
		}
		return &extv1.JSONSchemaProps{
			Type: typeArray,
			Items: &extv1.JSONSchemaPropsOrArray{
				Schema: items,
			},
		}, nil
	case strings.HasPrefix(typeExpr, "map<") && strings.HasSuffix(typeExpr, ">"):
		valueTypeExpr := strings.TrimSpace(typeExpr[len("map<") : len(typeExpr)-1])
		return c.mapSchemaFromType(valueTypeExpr)
	case strings.HasPrefix(typeExpr, "map["):
		closing := strings.Index(typeExpr, "]")
		if closing == -1 {
			return nil, fmt.Errorf("invalid map type expression %q", typeExpr)
		}
		keyTypeExpr := strings.TrimSpace(typeExpr[len("map["):closing])
		if keyTypeExpr != typeString {
			return nil, fmt.Errorf("map key type must be 'string', got %q in %q", keyTypeExpr, typeExpr)
		}
		valueTypeExpr := strings.TrimSpace(typeExpr[closing+1:])
		return c.mapSchemaFromType(valueTypeExpr)
	default:
		return c.schemaFromCustomType(typeExpr)
	}
}

// mapSchemaFromType builds the schema for map values using the provided value type expression.
func (c *converter) mapSchemaFromType(valueTypeExpr string) (*extv1.JSONSchemaProps, error) {
	valueSchema, err := c.schemaFromType(valueTypeExpr)
	if err != nil {
		return nil, err
	}

	return &extv1.JSONSchemaProps{
		Type: typeObject,
		AdditionalProperties: &extv1.JSONSchemaPropsOrBool{
			Allows: true,
			Schema: valueSchema,
		},
	}, nil
}

// schemaFromCustomType resolves user supplied type definitions while guarding against cycles and caching results.
func (c *converter) schemaFromCustomType(typeName string) (*extv1.JSONSchemaProps, error) {
	if cached, ok := c.typeCache[typeName]; ok {
		return cached.DeepCopy(), nil
	}

	if c.typeStack[typeName] {
		// Reject recursive type definitions so template authors get a clear signal when they
		// accidentally create cycles.
		return nil, fmt.Errorf("detected cyclic type reference involving %q", typeName)
	}

	raw, ok := c.types[typeName]
	if !ok {
		return nil, fmt.Errorf("unknown type %q", typeName)
	}

	c.typeStack[typeName] = true
	defer delete(c.typeStack, typeName)

	var (
		built *extv1.JSONSchemaProps
		err   error
	)

	switch typed := raw.(type) {
	case string:
		built, _, _, err = c.schemaFromString(typed)
	case map[string]any:
		built, err = c.buildObjectSchema(typed)
	default:
		err = fmt.Errorf("unsupported custom type definition for %q (type %T)", typeName, raw)
	}
	if err != nil {
		return nil, err
	}

	c.typeCache[typeName] = built
	return built.DeepCopy(), nil
}

// applyConstraints parses constraint tokens and updates the schema in place while tracking required status flags.
func (c *converter) applyConstraints(schema *extv1.JSONSchemaProps, constraintExpr, schemaType string) (bool, bool, error) {
	if strings.TrimSpace(constraintExpr) == "" {
		return false, false, nil
	}

	tokens := tokenizeConstraints(constraintExpr)
	var (
		required    bool
		hasRequired bool
	)

	// These handlers match the constraint set supported by our shorthand so examples can be lifted verbatim.
	handlers := map[string]func(string) error{
		"required": func(value string) error {
			boolVal, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid required value %q: %w", value, err)
			}
			required = boolVal
			hasRequired = true
			return nil
		},
		"default": func(value string) error {
			parsed, err := parseValueForType(value, schemaType)
			if err != nil {
				return fmt.Errorf("invalid default %q: %w", value, err)
			}
			raw, err := json.Marshal(parsed)
			if err != nil {
				return fmt.Errorf("failed to marshal default %#v: %w", parsed, err)
			}
			schema.Default = &extv1.JSON{Raw: raw}
			return nil
		},
		"enum": func(value string) error {
			values := splitAndTrim(value, ",")
			enums := make([]extv1.JSON, 0, len(values))
			for _, v := range values {
				parsed, err := parseValueForType(v, schemaType)
				if err != nil {
					return fmt.Errorf("invalid enum value %q: %w", v, err)
				}
				raw, err := json.Marshal(parsed)
				if err != nil {
					return fmt.Errorf("failed to marshal enum value %#v: %w", parsed, err)
				}
				enums = append(enums, extv1.JSON{Raw: raw})
			}
			schema.Enum = enums
			return nil
		},
		"minimum": func(value string) error {
			num, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return fmt.Errorf("invalid minimum %q: %w", value, err)
			}
			schema.Minimum = &num
			return nil
		},
		"maximum": func(value string) error {
			num, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return fmt.Errorf("invalid maximum %q: %w", value, err)
			}
			schema.Maximum = &num
			return nil
		},
		"exclusiveMinimum": func(value string) error {
			boolVal, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid exclusiveMinimum %q: %w", value, err)
			}
			schema.ExclusiveMinimum = boolVal
			return nil
		},
		"exclusiveMaximum": func(value string) error {
			boolVal, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid exclusiveMaximum %q: %w", value, err)
			}
			schema.ExclusiveMaximum = boolVal
			return nil
		},
		"minItems": func(value string) error {
			intVal, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid minItems %q: %w", value, err)
			}
			schema.MinItems = &intVal
			return nil
		},
		"maxItems": func(value string) error {
			intVal, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid maxItems %q: %w", value, err)
			}
			schema.MaxItems = &intVal
			return nil
		},
		"uniqueItems": func(value string) error {
			boolVal, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid uniqueItems %q: %w", value, err)
			}
			schema.UniqueItems = boolVal
			return nil
		},
		"minLength": func(value string) error {
			intVal, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid minLength %q: %w", value, err)
			}
			schema.MinLength = &intVal
			return nil
		},
		"maxLength": func(value string) error {
			intVal, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid maxLength %q: %w", value, err)
			}
			schema.MaxLength = &intVal
			return nil
		},
		"minProperties": func(value string) error {
			intVal, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid minProperties %q: %w", value, err)
			}
			schema.MinProperties = &intVal
			return nil
		},
		"maxProperties": func(value string) error {
			intVal, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid maxProperties %q: %w", value, err)
			}
			schema.MaxProperties = &intVal
			return nil
		},
		"multipleOf": func(value string) error {
			num, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return fmt.Errorf("invalid multipleOf %q: %w", value, err)
			}
			schema.MultipleOf = &num
			return nil
		},
		"example": func(value string) error {
			parsed, err := parseArbitraryValue(value)
			if err != nil {
				return fmt.Errorf("invalid example %q: %w", value, err)
			}
			raw, err := json.Marshal(parsed)
			if err != nil {
				return fmt.Errorf("failed to marshal example %#v: %w", parsed, err)
			}
			schema.Example = &extv1.JSON{Raw: raw}
			return nil
		},
		"nullable": func(value string) error {
			boolVal, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid nullable value %q: %w", value, err)
			}
			schema.Nullable = boolVal
			return nil
		},
	}

	setters := map[string]func(string){
		"pattern":     func(value string) { schema.Pattern = unquoteIfNeeded(value) },
		"title":       func(value string) { schema.Title = unquoteIfNeeded(value) },
		"description": func(value string) { schema.Description = unquoteIfNeeded(value) },
		"format":      func(value string) { schema.Format = unquoteIfNeeded(value) },
	}

	for _, token := range tokens {
		if !strings.Contains(token, "=") {
			continue
		}
		parts := strings.SplitN(token, "=", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		handler, ok := handlers[key]
		if !ok {
			if setter, okSetter := setters[key]; okSetter {
				setter(value)
				continue
			}
			// Unknown marker
			if c.errorOnUnknownMarkers {
				return false, false, fmt.Errorf("unknown constraint marker %q", key)
			}
			// Unknown markers are silently ignored unless errorOnUnknownMarkers is set.
			continue
		}
		if err := handler(value); err != nil {
			return false, false, err
		}
	}

	return required, hasRequired, nil
}

// parseValueForType converts a raw token into a Go value appropriate for the given schema type.
func parseValueForType(value, schemaType string) (any, error) {
	switch schemaType {
	case typeString:
		// Allow callers to write defaults like default="" (or default='v1') in the shorthand.
		// The parser stores the raw token, so we need to recognize quoted literals here and
		// unquote them back into their canonical form; otherwise default="" would render as "".
		if len(value) >= 2 {
			if value[0] == '"' && value[len(value)-1] == '"' {
				// Double-quoted strings: use Go's unquoting (matches YAML double-quote escaping)
				parsed, err := strconv.Unquote(value)
				if err == nil {
					return parsed, nil
				}
			} else if value[0] == '\'' && value[len(value)-1] == '\'' {
				// Single-quoted strings: YAML uses '' to escape a single quote
				// Strip outer quotes and replace '' with '
				inner := value[1 : len(value)-1]
				return strings.ReplaceAll(inner, "''", "'"), nil
			}
		}
		return value, nil
	case typeInteger:
		if value == "" {
			return 0, fmt.Errorf("empty integer value")
		}
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, err
		}
		return intVal, nil
	case typeNumber:
		if value == "" {
			return 0.0, fmt.Errorf("empty number value")
		}
		floatVal, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, err
		}
		return floatVal, nil
	case typeBoolean:
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return nil, err
		}
		return boolVal, nil
	case typeArray, typeObject:
		if strings.TrimSpace(value) == "" {
			if schemaType == typeArray {
				return []any{}, nil
			}
			return map[string]any{}, nil
		}
		var parsed any
		if err := json.Unmarshal([]byte(value), &parsed); err != nil {
			return nil, err
		}
		return parsed, nil
	default:
		// For custom object-like types, attempt JSON parsing; fall back to string.
		if strings.TrimSpace(value) == "" {
			return value, nil
		}
		var parsed any
		if err := json.Unmarshal([]byte(value), &parsed); err == nil {
			return parsed, nil
		}
		// Attempt numeric/bool parsing as a best effort.
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal, nil
		}
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intVal, nil
		}
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal, nil
		}
		return value, nil
	}
}

// parseArbitraryValue best-effort converts a token into JSON, bool, number, or leaves it as a string.
func parseArbitraryValue(value string) (any, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}

	if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") {
		var parsed any
		if err := json.Unmarshal([]byte(value), &parsed); err != nil {
			return nil, err
		}
		return parsed, nil
	}

	if boolVal, err := strconv.ParseBool(value); err == nil {
		return boolVal, nil
	}
	if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
		return intVal, nil
	}
	if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
		return floatVal, nil
	}
	return value, nil
}

// tokenizeConstraints splits a constraint expression into individual constraint tokens.
//
// This parser handles complex constraint values by tracking quote and bracket context:
//   - Quoted strings: "description='A long description with spaces'"
//   - JSON values: "default={\"key\": \"value\"}"
//   - Array values: "enum=['a', 'b', 'c']"
//
// Tokenization rules:
//  1. Tokens are separated by whitespace outside of quotes/brackets
//  2. Quoted content (single or double quotes) is kept intact, including whitespace
//  3. Nested brackets/braces are tracked to handle JSON/array literals
//  4. Backslash escaping is supported within quotes
//
// Example input: "default=dev description='Development environment' minLength=3"
// Example output: ["default=dev", "description='Development environment'", "minLength=3"]
func tokenizeConstraints(expr string) []string {
	var tokens []string
	var current strings.Builder

	inQuotes := false
	var quoteChar rune
	escaped := false
	bracketDepth := 0

	for _, r := range expr {
		switch {
		case inQuotes:
			current.WriteRune(r)
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == quoteChar {
				inQuotes = false
			}
		case r == '"' || r == '\'':
			inQuotes = true
			quoteChar = r
			current.WriteRune(r)
		case r == '{' || r == '[':
			bracketDepth++
			current.WriteRune(r)
		case r == '}' || r == ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
			current.WriteRune(r)
		case unicode.IsSpace(r) && bracketDepth == 0:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// splitAndTrim separates delimited lists like enums, dropping empty items along the way.
func splitAndTrim(value, sep string) []string {
	raw := strings.Split(value, sep)
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// unquoteIfNeeded removes surrounding quotes from a string value if present.
func unquoteIfNeeded(value string) string {
	if len(value) >= 2 {
		if value[0] == '"' && value[len(value)-1] == '"' {
			// Double-quoted strings: use Go's unquoting (matches YAML double-quote escaping)
			if parsed, err := strconv.Unquote(value); err == nil {
				return parsed
			}
		} else if value[0] == '\'' && value[len(value)-1] == '\'' {
			// Single-quoted strings: YAML uses '' to escape a single quote
			// Strip outer quotes and replace '' with '
			inner := value[1 : len(value)-1]
			return strings.ReplaceAll(inner, "''", "'")
		}
	}
	return value
}
