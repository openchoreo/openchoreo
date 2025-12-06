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

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
)

const (
	typeString  = "string"
	typeInteger = "integer"
	typeNumber  = "number"
	typeBoolean = "boolean"
	typeObject  = "object"
	typeArray   = "array"
)

// allowedUnknownMarkerPrefixes defines marker prefixes that are silently ignored during schema extraction.
// These markers can be used for custom annotations, documentation, or tool-specific metadata.
//
// Example:
//
//	apiKey: "string | oc:sensitive=true oc:generated=true"
//	userId: "string | oc:indexed=true"
var allowedUnknownMarkerPrefixes = []string{"oc:"}

// Options configures schema extraction behavior.
type Options struct {
	// SetAdditionalPropertiesFalse sets additionalProperties: false on all object schemas.
	// This prevents any properties not explicitly defined in the schema.
	SetAdditionalPropertiesFalse bool

	// SkipDefaultValidation disables validation of default values against their schema constraints.
	// By default (false), all defaults are validated using Kubernetes' schema validator.
	// Set to true only when you need to skip validation for performance (e.g., API endpoints
	// serving already-validated schemas). Webhooks should always use the default (false) to
	// validate defaults when schemas are created/updated.
	SkipDefaultValidation bool
}

// ExtractSchema converts a field map using shorthand schema syntax into OpenAPI v3 JSON Schema.
//
// This is the primary API for converting ComponentType/Trait schemas from the
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
//
// Schema behavior:
//   - Fields are required by default unless they have a default value
//   - Unknown markers cause errors unless they have an allowedUnknownMarkerPrefixes prefix (reserved for custom annotations)
//   - The "required" marker is not allowed (use defaults to make fields optional)
func ExtractSchema(fields map[string]any, types map[string]any, opts Options) (*apiextensions.JSONSchemaProps, error) {
	c := &converter{
		types:     types,
		typeCache: map[string]*apiextensions.JSONSchemaProps{},
		typeStack: map[string]bool{},
		opts:      opts,
	}

	return c.buildObjectSchema(fields)
}

// converter builds JSON schemas from simple schema definitions.
type converter struct {
	types     map[string]any
	typeCache map[string]*apiextensions.JSONSchemaProps
	typeStack map[string]bool
	opts      Options
}

// buildObjectSchema converts a field map into an object schema with properties and required markers.
//
// This is the core conversion logic that processes each field's shorthand definition and
// determines whether the field should be required.
//
// Required field logic:
//  1. If field has a default value → not required (default fills it in)
//  2. Otherwise → required
//
// The "required" marker is not allowed - use defaults to make fields optional.
//
// Special handling for $default key:
//   - If the field map contains a $default key, it specifies a default value for the object itself
//   - The $default key is removed from the fields before processing other properties
//   - The default value must be valid JSON and must satisfy the object's schema
//
// Fields are processed in sorted order to ensure deterministic JSON Schema output.
func (c *converter) buildObjectSchema(fields map[string]any) (*apiextensions.JSONSchemaProps, error) {
	// Check for and extract $default key before processing other fields
	var objectDefault any
	var hasObjectDefault bool
	if defaultVal, ok := fields["$default"]; ok {
		objectDefault = defaultVal
		hasObjectDefault = true

		// Create a new map without the $default key
		fieldsWithoutDefault := make(map[string]any, len(fields)-1)
		for k, v := range fields {
			if k != "$default" {
				fieldsWithoutDefault[k] = v
			}
		}
		fields = fieldsWithoutDefault
	}

	props := map[string]apiextensions.JSONSchemaProps{}
	required := []string{}

	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, name := range keys {
		field := fields[name]

		schema, err := c.buildFieldSchema(field)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", name, err)
		}
		if schema == nil {
			continue
		}
		props[name] = *schema

		// Field is required unless it has a default value
		if schema.Default == nil {
			required = append(required, name)
		}
	}

	result := &apiextensions.JSONSchemaProps{
		Type:       typeObject,
		Properties: props,
	}
	if len(required) > 0 {
		result.Required = required
	}

	// Set additionalProperties: false if configured
	if c.opts.SetAdditionalPropertiesFalse {
		result.AdditionalProperties = &apiextensions.JSONSchemaPropsOrBool{
			Allows: false,
		}
	}

	// Apply object-level default if specified
	if hasObjectDefault {
		if err := c.applyObjectDefault(result, objectDefault); err != nil {
			return nil, fmt.Errorf("invalid $default: %w", err)
		}
	}

	return result, nil
}

// applyObjectDefault validates and applies a $default value to an object schema.
//
// Validation rules:
//  1. The default value must be parseable as a JSON object
//  2. The default value must satisfy the schema (types, constraints, required fields, etc.)
//
// This uses Kubernetes' built-in schema validation to ensure the default value
// satisfies all schema constraints including types, required fields, enums, patterns,
// min/max values, and nested structures.
func (c *converter) applyObjectDefault(schema *apiextensions.JSONSchemaProps, defaultValue any) error {
	var defaultMap map[string]any

	// Handle the default value based on its type
	switch v := defaultValue.(type) {
	case map[string]any:
		// Direct YAML map - use as-is
		defaultMap = v
	case string:
		// JSON string - parse it
		parsed, err := parseValueForType(v, typeObject)
		if err != nil {
			return fmt.Errorf("value is not a valid object: %w", err)
		}
		var ok bool
		defaultMap, ok = parsed.(map[string]any)
		if !ok {
			return fmt.Errorf("value must be an object, got %T", parsed)
		}
	default:
		return fmt.Errorf("value must be an object or JSON string, got %T", defaultValue)
	}

	// Set the default value on the schema
	var defaultJSON apiextensions.JSON = defaultMap
	schema.Default = &defaultJSON

	// Validate default value against schema unless explicitly skipped
	if !c.opts.SkipDefaultValidation {
		if err := c.validateDefault(schema, defaultJSON); err != nil {
			return fmt.Errorf("default value does not satisfy schema: %w", err)
		}
	}

	return nil
}

// validateDefault validates a default value against its schema constraints using Kubernetes' validator.
//
// This ensures that default values satisfy all schema constraints including:
//   - Type constraints
//   - Numeric constraints (minimum, maximum, multipleOf)
//   - String constraints (minLength, maxLength, pattern)
//   - Array constraints (minItems, maxItems, uniqueItems)
//   - Enum values
//
// For complex types (objects, arrays), this performs deep validation of nested structures.
func (c *converter) validateDefault(schema *apiextensions.JSONSchemaProps, defaultValue apiextensions.JSON) error {
	validator, _, err := validation.NewSchemaValidator(schema)
	if err != nil {
		return fmt.Errorf("failed to create schema validator: %w", err)
	}

	result := validator.Validate(defaultValue)
	if !result.IsValid() {
		return fmt.Errorf("default value does not satisfy schema constraints: %v", result.Errors)
	}

	return nil
}

// buildFieldSchema determines the schema for a field value that may itself be an object or shorthand string.
func (c *converter) buildFieldSchema(raw any) (*apiextensions.JSONSchemaProps, error) {
	switch typed := raw.(type) {
	case string:
		return c.schemaFromString(typed)
	case map[string]any:
		return c.buildObjectSchema(typed)
	default:
		return nil, fmt.Errorf("unsupported field definition of type %T", raw)
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
//   - Defaults: "default=dev" or "default={}" for objects
//   - Enums: "enum=dev,staging,prod"
//   - Documentation: "description='Port number' example=8080"
//   - Custom annotations: "oc_sensitive=true" (with oc_ prefix)
//
// Note: The "required" marker is not allowed. Fields are required unless they have a default.
//
// Example: "integer | minimum=1 | maximum=65535 | default=8080"
func (c *converter) schemaFromString(expr string) (*apiextensions.JSONSchemaProps, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("empty schema expression")
	}

	typeExpr := expr
	constraintExpr := ""
	if idx := strings.Index(expr, "|"); idx != -1 {
		typeExpr = strings.TrimSpace(expr[:idx])
		constraintExpr = strings.TrimSpace(expr[idx+1:])
	}

	schema, err := c.schemaFromType(typeExpr)
	if err != nil {
		return nil, err
	}

	if err := c.applyConstraints(schema, constraintExpr, schema.Type); err != nil {
		return nil, err
	}
	return schema, nil
}

// schemaFromType resolves a type expression into a JSON schema, handling arrays, maps, and custom types.
func (c *converter) schemaFromType(typeExpr string) (*apiextensions.JSONSchemaProps, error) {
	switch {
	case typeExpr == typeString:
		return &apiextensions.JSONSchemaProps{Type: typeString}, nil
	case typeExpr == typeInteger:
		return &apiextensions.JSONSchemaProps{Type: typeInteger}, nil
	case typeExpr == typeNumber:
		return &apiextensions.JSONSchemaProps{Type: typeNumber}, nil
	case typeExpr == typeBoolean:
		return &apiextensions.JSONSchemaProps{Type: typeBoolean}, nil
	case typeExpr == typeObject:
		return nil, fmt.Errorf("'object' type is not allowed; use a map type (e.g., 'map<string>') for free-form objects or define a structured type with explicit properties")
	case strings.HasPrefix(typeExpr, "[]"):
		itemTypeExpr := strings.TrimSpace(typeExpr[2:])
		items, err := c.schemaFromType(itemTypeExpr)
		if err != nil {
			return nil, err
		}
		return &apiextensions.JSONSchemaProps{
			Type: typeArray,
			Items: &apiextensions.JSONSchemaPropsOrArray{
				Schema: items,
			},
		}, nil
	case strings.HasPrefix(typeExpr, "array<") && strings.HasSuffix(typeExpr, ">"):
		itemTypeExpr := strings.TrimSpace(typeExpr[len("array<") : len(typeExpr)-1])
		items, err := c.schemaFromType(itemTypeExpr)
		if err != nil {
			return nil, err
		}
		return &apiextensions.JSONSchemaProps{
			Type: typeArray,
			Items: &apiextensions.JSONSchemaPropsOrArray{
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
func (c *converter) mapSchemaFromType(valueTypeExpr string) (*apiextensions.JSONSchemaProps, error) {
	valueSchema, err := c.schemaFromType(valueTypeExpr)
	if err != nil {
		return nil, err
	}

	return &apiextensions.JSONSchemaProps{
		Type: typeObject,
		AdditionalProperties: &apiextensions.JSONSchemaPropsOrBool{
			Allows: true,
			Schema: valueSchema,
		},
	}, nil
}

// schemaFromCustomType resolves user supplied type definitions while guarding against cycles and caching results.
func (c *converter) schemaFromCustomType(typeName string) (*apiextensions.JSONSchemaProps, error) {
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
		built *apiextensions.JSONSchemaProps
		err   error
	)

	switch typed := raw.(type) {
	case string:
		built, err = c.schemaFromString(typed)
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

// applyConstraints parses constraint tokens and updates the schema in place.
// The "required" marker is not allowed - use defaults to make fields optional.
// Unknown markers cause errors unless they have an "oc_" prefix (reserved for annotations).
func (c *converter) applyConstraints(schema *apiextensions.JSONSchemaProps, constraintExpr, schemaType string) error {
	if strings.TrimSpace(constraintExpr) == "" {
		return nil
	}

	tokens := tokenizeConstraints(constraintExpr)

	// These handlers match the constraint set supported by our shorthand so examples can be lifted verbatim.
	handlers := c.buildConstraintHandlers(schema, schemaType)
	setters := c.buildConstraintSetters(schema)

	for _, token := range tokens {
		if !strings.Contains(token, "=") {
			// Token without '=' - check if it's just a separator or an allowed marker
			trimmedToken := strings.TrimSpace(token)
			// Skip pure pipe separators (used for readability: "enum=a,b | default=x")
			if trimmedToken == "|" {
				continue
			}
			if hasAllowedPrefix(trimmedToken, allowedUnknownMarkerPrefixes) {
				// Silently ignore markers with allowed prefixes (they're for annotations/metadata)
				continue
			}
			// Unknown marker without value - likely a typo
			return fmt.Errorf("constraint marker %q is missing a value (should be in format 'key=value')", trimmedToken)
		}
		parts := strings.SplitN(token, "=", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Reject "required" marker - use defaults to make fields optional
		if key == "required" {
			return fmt.Errorf("marker %q is not allowed - use default values to make fields optional", key)
		}

		handler, ok := handlers[key]
		if !ok {
			if setter, okSetter := setters[key]; okSetter {
				setter(value)
				continue
			}
			// Unknown marker - allow if it has an allowed prefix (reserved for annotations)
			if hasAllowedPrefix(key, allowedUnknownMarkerPrefixes) {
				// Silently ignore markers with allowed prefixes (they're for annotations/metadata)
				continue
			}
			return fmt.Errorf("unknown constraint marker %q", key)
		}
		if err := handler(value); err != nil {
			return err
		}
	}

	// Validate default value against schema constraints unless explicitly skipped
	if !c.opts.SkipDefaultValidation && schema.Default != nil {
		if err := c.validateDefault(schema, *schema.Default); err != nil {
			return fmt.Errorf("invalid default value: %w", err)
		}
	}

	return nil
}

// buildConstraintHandlers creates the map of constraint handlers for schema validation.
func (c *converter) buildConstraintHandlers(schema *apiextensions.JSONSchemaProps, schemaType string) map[string]func(string) error {
	return map[string]func(string) error{
		"default": func(value string) error {
			parsed, err := parseValueForType(value, schemaType)
			if err != nil {
				return fmt.Errorf("invalid default %q: %w", value, err)
			}
			var defaultJSON apiextensions.JSON = parsed
			schema.Default = &defaultJSON
			return nil
		},
		"enum": func(value string) error {
			values := splitRespectingQuotes(value, ",")
			enums := make([]apiextensions.JSON, 0, len(values))
			for _, v := range values {
				parsed, err := parseValueForType(v, schemaType)
				if err != nil {
					return fmt.Errorf("invalid enum value %q: %w", v, err)
				}
				enums = append(enums, parsed)
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
			var exampleJSON apiextensions.JSON = parsed
			schema.Example = &exampleJSON
			return nil
		},
	}
}

// buildConstraintSetters creates the map of simple constraint setters.
func (c *converter) buildConstraintSetters(schema *apiextensions.JSONSchemaProps) map[string]func(string) {
	return map[string]func(string){
		"pattern":     func(value string) { schema.Pattern = unquoteIfNeeded(value) },
		"title":       func(value string) { schema.Title = unquoteIfNeeded(value) },
		"description": func(value string) { schema.Description = unquoteIfNeeded(value) },
		"format":      func(value string) { schema.Format = unquoteIfNeeded(value) },
	}
}

// parseValueForType converts a raw token into a Go value appropriate for the given schema type.
func parseValueForType(value, schemaType string) (any, error) {
	switch schemaType {
	case typeString:
		return unquoteIfNeeded(value), nil
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
			return nil, fmt.Errorf("empty %s value", schemaType)
		}
		var parsed any
		if err := json.Unmarshal([]byte(value), &parsed); err != nil {
			return nil, err
		}
		return parsed, nil
	default:
		return nil, fmt.Errorf("unsupported schema type %q", schemaType)
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
//   - Quoted enum values: "enum="a,b,c"
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

// splitRespectingQuotes splits a string by the separator, but respects quoted strings.
// Commas inside quoted strings are not treated as separators.
// For example: `"value1","value with, comma","value3"` splits into 3 values, not 4.
func splitRespectingQuotes(value, sep string) []string {
	var result []string
	var current strings.Builder
	inQuotes := false
	escaped := false

	for i := 0; i < len(value); i++ {
		char := value[i]

		switch {
		case escaped:
			// Previous character was a backslash, this character is escaped
			current.WriteByte(char)
			escaped = false
		case char == '\\' && inQuotes:
			// Backslash inside quotes - next character is escaped
			current.WriteByte(char)
			escaped = true
		case char == '"':
			// Toggle quote state
			current.WriteByte(char)
			inQuotes = !inQuotes
		case char == sep[0] && !inQuotes:
			// Separator outside quotes - split here
			trimmed := strings.TrimSpace(current.String())
			if trimmed != "" {
				result = append(result, trimmed)
			}
			current.Reset()
		default:
			current.WriteByte(char)
		}
	}

	// Add the last item
	trimmed := strings.TrimSpace(current.String())
	if trimmed != "" {
		result = append(result, trimmed)
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

// hasAllowedPrefix checks if a marker key has one of the allowed prefixes.
// This is used to silently ignore custom annotation markers.
func hasAllowedPrefix(key string, allowedPrefixes []string) bool {
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}
