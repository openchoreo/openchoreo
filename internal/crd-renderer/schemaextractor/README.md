# Schema Extractor

This package converts compact schema shorthand syntax into Kubernetes OpenAPI v3 JSON schemas, making it easy to define configuration parameters without writing verbose schema definitions.

## Quick Example

**Input** (shorthand YAML):

```yaml
name: string
replicas: "integer | default=1"
environment: "string | enum=dev,staging,prod default=dev"
description: 'string | default=""'
```

**Output** (OpenAPI v3 JSON Schema):

```json
{
  "type": "object",
  "required": ["name"],
  "properties": {
    "name": {
      "type": "string"
    },
    "replicas": {
      "type": "integer",
      "default": 1
    },
    "environment": {
      "type": "string",
      "default": "dev",
      "enum": ["dev", "staging", "prod"]
    },
    "description": {
      "type": "string",
      "default": ""
    }
  }
}
```

## Usage

```go
import "github.com/wso2/openchoreo/internal/crd-renderer/schemaextractor"

// Define your schema using the shorthand syntax
fields := map[string]any{
    "name":        "string",
    "replicas":    "integer | default=1",
    "environment": "string | enum=dev,staging,prod default=dev",
    "description": `string | default=""`,
}

// Convert to OpenAPI v3 JSON Schema (with default options)
schema, err := schemaextractor.ExtractSchema(fields, nil)
if err != nil {
    // handle error
}

// Use the generated schema for validation, CRD generation, etc.
```

### Advanced: Custom Options

```go
// Create custom options
opts := schemaextractor.DefaultOptions()
opts.RequiredByDefault = false     // Make all fields optional unless explicitly required=true
opts.ErrorOnUnknownMarkers = true  // Fail parsing on unknown constraint markers

schema, err := schemaextractor.ExtractSchemaWithOptions(fields, nil, opts)
if err != nil {
    // handle error
}
```

## Syntax Overview

- **Primitive types** – `string`, `integer`, `number`, `boolean`.
- **Arrays** – `[]Type`, `array<Type>`, `[]map<string>`, and references to custom types such as `[]MountConfig`. Parenthesized forms like `[](map<string>)` are not supported.
- **Maps** – `map<Type>` or `map[string]Type` for free-form key-value pairs. Keys must always be `string` type.
- **Custom types** – reference a named entry declared under `definition.Types` (e.g. `[]MountConfig`).
- **Object literals** – nested field maps represented as normal YAML/JSON objects with explicitly defined properties.
- **Constraints** – append after a `|` separator with space-separated markers:

  - Valid: `string | required=true default=foo pattern=^[a-z]+$`
  - Valid: `string|required=true default=foo` (no space after first `|` is fine)
  - Invalid: `string|required=true|default=foo` (don't use `|` to separate constraints)

  The first `|` separates the type from constraints; after that, use spaces between constraint markers.
  Pipes inside quoted values are preserved (e.g., `pattern="a|b|c"` correctly sets the pattern to `a|b|c`).

- **Required by default** – fields are considered required unless they declare `default=` or `required=false`.

## Type Validation Rules

The schema extractor enforces strict type validation to ensure schemas are well-defined:

### Map Keys Must Be Strings

Map types must always use `string` keys. Non-string key types are rejected:

✅ **Valid:**
```yaml
labels: "map[string]string"
metadata: "map<string>"
config: "map[string]integer"
```

❌ **Invalid:**
```yaml
data: "map[int]string"        # Error: map key type must be 'string'
lookup: "map[number]boolean"  # Error: map key type must be 'string'
```

### The 'object' Type is Not Allowed

The generic `object` type is not supported. Instead, you must use one of these alternatives:

**For free-form key-value pairs**, use a map:
```yaml
# ✅ Use map for dynamic keys
labels: "map[string]string"
metadata: "map<string>"
```

**For structured data**, define explicit properties:
```yaml
# ✅ Use nested object with explicit properties
server:
  host: string
  port: "integer | default=8080"
  tls: boolean
```

**For custom reusable types**, define them in the types section:
```yaml
# In types section:
ServerConfig:
  host: string
  port: "integer | default=8080"
  tls: boolean

# In schema:
server: ServerConfig
```

This validation ensures:
- All object structures have well-defined schemas
- No ambiguous or unvalidated data structures
- Clear distinction between structured objects and free-form maps

## Supported Constraint Markers

- `required=true|false` - Force field to be (non-)required. Fields without this marker are required by default unless they have an explicit `default`.
- `default=<value>` - Supplies a default. Primitive values can be quoted (`default=""`, `default='v1'`) and are unquoted by the parser so they render as expected.
- `enum=a,b,c` - Enumerated values (parsed according to the field type).
- `pattern` - Regular expression pattern for string validation (JSON Schema).
- `minimum`, `maximum` - Numeric range validations (JSON Schema).
- `exclusiveMinimum`, `exclusiveMaximum` - Boolean flags for exclusive numeric bounds (JSON Schema).
- `minItems`, `maxItems` - Array length constraints.
- `uniqueItems` - Boolean flag requiring array items to be unique.
- `minLength`, `maxLength` - String length validations.
- `multipleOf` - Numeric multiplier constraint (e.g., `multipleOf=5` means value must be divisible by 5).
- `nullable=true` - Allows `null` values (JSON Schema).
- `title`, `description` - Human-readable field documentation.
- `format` - String format hint (e.g., `format=date-time`, `format=email`).
- `example` - Example value for documentation.

Unknown markers are silently ignored and won't cause parsing errors.

## Configuration Options

The schema extractor supports the following configuration options via `ExtractSchemaWithOptions`:

### `RequiredByDefault` (default: `true`)

Controls whether fields without explicit `required` or `default` markers are treated as required.

- **`true` (default)**: Fields are required unless they have a `default=` value or explicit `required=false`
- **`false`**: Fields are optional unless they have explicit `required=true`

Example:
```go
opts := schemaextractor.DefaultOptions()
opts.RequiredByDefault = false

// With this option, these fields will all be optional:
fields := map[string]any{
    "name":  "string",           // optional (no required marker)
    "age":   "integer",          // optional (no required marker)
    "email": "string | required=true", // required (explicit)
}
```

### `ErrorOnUnknownMarkers` (default: `false`)

Controls whether parsing should fail when encountering unknown constraint markers.

- **`false` (default)**: Unknown markers are silently ignored
- **`true`**: Parsing fails with an error when an unknown marker is encountered

Example:
```go
opts := schemaextractor.DefaultOptions()
opts.ErrorOnUnknownMarkers = true

// With this option, this will fail:
fields := map[string]any{
    "field": "string | customMarker=foo", // Error: unknown constraint marker "customMarker"
}
```

Use this option to catch typos in constraint markers or to enforce strict validation.

## Special Handling

**Literal handling** – `schemaextractor` keeps constraint tokens as raw strings; `parseValueForType` includes special handling for quoted primitives so that shorthands like `default=""` become actual empty strings when defaulting is applied. This ensures that empty string defaults and other quoted values work correctly when the schema is used for validation and defaulting.
