// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package extractor

import (
	"encoding/json"
	"strings"
	"testing"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"gopkg.in/yaml.v3"
)

func TestConverter_JSONMatchesExpected(t *testing.T) {
	const typesYAML = ``
	const schemaYAML = `
name: string
replicas: 'integer | default=1'
`
	const expected = `{
  "type": "object",
  "required": [
    "name"
  ],
  "properties": {
    "name": {
      "type": "string"
    },
    "replicas": {
      "type": "integer",
      "default": 1
    }
  }
}`

	assertConvertedSchema(t, typesYAML, schemaYAML, expected)
}

func TestConverter_ArrayDefaultParsing(t *testing.T) {
	const typesYAML = `
Item:
  name: 'string | default=default-name'
`
	const schemaYAML = `
items: '[]Item | default=[{"name":"custom"}]'
`
	const expected = `{
  "type": "object",
  "properties": {
    "items": {
      "type": "array",
      "default": [
        {
          "name": "custom"
        }
      ],
      "items": {
        "type": "object",
        "properties": {
          "name": {
            "type": "string",
            "default": "default-name"
          }
        }
      }
    }
  }
}`

	assertConvertedSchema(t, typesYAML, schemaYAML, expected)
}

func TestConverter_DefaultRequiredBehaviour(t *testing.T) {
	const typesYAML = ``
	const schemaYAML = `
mustProvide: string
hasDefault: 'integer | default=5'
optionalWithDefault: 'boolean | default=false'
`
	const expected = `{
  "type": "object",
  "required": [
    "mustProvide"
  ],
  "properties": {
    "hasDefault": {
      "type": "integer",
      "default": 5
    },
    "mustProvide": {
      "type": "string"
    },
    "optionalWithDefault": {
      "type": "boolean",
      "default": false
    }
  }
}`

	var types map[string]any
	if strings.TrimSpace(typesYAML) != "" {
		types = parseYAMLMap(t, typesYAML)
	}
	root := parseYAMLMap(t, schemaYAML)

	internalSchema, err := ExtractSchema(root, types)
	if err != nil {
		t.Fatalf("ExtractSchema returned error: %v", err)
	}

	// Convert to v1 for JSON comparison
	v1Schema := new(extv1.JSONSchemaProps)
	if err := extv1.Convert_apiextensions_JSONSchemaProps_To_v1_JSONSchemaProps(internalSchema, v1Schema, nil); err != nil {
		t.Fatalf("failed to convert schema to v1: %v", err)
	}

	assertSchemaJSON(t, v1Schema, expected)
}

func TestConverter_CustomTypeJSONMatchesExpected(t *testing.T) {
	const typesYAML = `
Resources:
  cpu: 'string | default=100m'
  memory: string
`
	const schemaYAML = `
resources: Resources
`
	const expected = `{
  "type": "object",
  "required": [
    "resources"
  ],
  "properties": {
    "resources": {
      "type": "object",
      "required": [
        "memory"
      ],
      "properties": {
        "cpu": {
          "type": "string",
          "default": "100m"
        },
        "memory": {
          "type": "string"
        }
      }
    }
  }
}`

	assertConvertedSchema(t, typesYAML, schemaYAML, expected)
}

func TestConverter_ArraySyntaxVariants(t *testing.T) {
	const typesYAML = `
Item:
  name: string
`
	const listSchema = `
items: '[]Item'
`
	const arraySchema = `
items: 'array<Item>'
`

	types := parseYAMLMap(t, typesYAML)

	list, err := ExtractSchema(parseYAMLMap(t, listSchema), types)
	if err != nil {
		t.Fatalf("ExtractSchema for []Item returned error: %v", err)
	}
	array, err := ExtractSchema(parseYAMLMap(t, arraySchema), types)
	if err != nil {
		t.Fatalf("ExtractSchema for array<Item> returned error: %v", err)
	}

	listJSON, err := json.Marshal(list)
	if err != nil {
		t.Fatalf("failed to marshal list schema: %v", err)
	}
	arrayJSON, err := json.Marshal(array)
	if err != nil {
		t.Fatalf("failed to marshal array schema: %v", err)
	}
	if string(listJSON) != string(arrayJSON) {
		t.Fatalf("expected []Item and array<Item> to produce identical schemas\nlist: %s\narray: %s", string(listJSON), string(arrayJSON))
	}
}

func TestConverter_ArrayOfMaps(t *testing.T) {
	const schemaYAML = `
tags: '[]map<string> | default=[]'
`
	const expected = `{
  "type": "object",
  "properties": {
    "tags": {
      "type": "array",
      "default": [],
      "items": {
        "type": "object",
        "additionalProperties": {
          "type": "string"
        }
      }
    }
  }
}`

	assertConvertedSchema(t, "", schemaYAML, expected)
}

func TestConverter_MapBracketSyntaxValidation(t *testing.T) {
	const schemaYAML = `
labels: 'map[string]string'
metadata: 'map[string]number'
`
	const expected = `{
  "type": "object",
  "required": [
    "labels",
    "metadata"
  ],
  "properties": {
    "labels": {
      "type": "object",
      "additionalProperties": {
        "type": "string"
      }
    },
    "metadata": {
      "type": "object",
      "additionalProperties": {
        "type": "number"
      }
    }
  }
}`

	assertConvertedSchema(t, "", schemaYAML, expected)
}

func TestConverter_ParenthesizedArraySyntaxRejected(t *testing.T) {
	const schemaYAML = `
tags: "[](map<string>)"
`

	_, err := ExtractSchema(parseYAMLMap(t, schemaYAML), nil)
	if err == nil {
		t.Fatalf("expected error for unsupported syntax [](map<string>)")
	}
}

// Error case tests

func TestConverter_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		typesYAML   string
		schemaYAML  string
		expectError string
	}{
		{
			name: "empty schema expression",
			schemaYAML: `
field: ""
`,
			expectError: "empty schema expression",
		},
		{
			name: "unknown custom type",
			schemaYAML: `
field: UnknownType
`,
			expectError: "unknown type",
		},
		{
			name: "cyclic type reference",
			typesYAML: `
TypeA: TypeB
TypeB: TypeA
`,
			schemaYAML: `
field: TypeA
`,
			expectError: "cyclic type reference",
		},
		{
			name: "invalid map syntax",
			schemaYAML: `
field: "map["
`,
			expectError: "invalid map type expression",
		},
		{
			name: "empty array type",
			schemaYAML: `
field: "[]"
`,
			expectError: "unknown type",
		},
		{
			name: "empty map type",
			schemaYAML: `
field: "map<>"
`,
			expectError: "unknown type",
		},
		{
			name: "required marker not allowed",
			schemaYAML: `
field: "string | required=true"
`,
			expectError: "marker \"required\" is not allowed",
		},
		{
			name: "invalid minLength value",
			schemaYAML: `
field: "string | minLength=notanumber"
`,
			expectError: "invalid minLength",
		},
		{
			name: "invalid maxLength value",
			schemaYAML: `
field: "string | maxLength=abc"
`,
			expectError: "invalid maxLength",
		},
		{
			name: "invalid minimum value",
			schemaYAML: `
field: "number | minimum=notanumber"
`,
			expectError: "invalid minimum",
		},
		{
			name: "invalid maximum value",
			schemaYAML: `
field: "number | maximum=xyz"
`,
			expectError: "invalid maximum",
		},
		{
			name: "invalid multipleOf value",
			schemaYAML: `
field: "number | multipleOf=abc"
`,
			expectError: "invalid multipleOf",
		},
		{
			name: "invalid minItems value",
			schemaYAML: `
field: "[]string | minItems=notanumber"
`,
			expectError: "invalid minItems",
		},
		{
			name: "invalid maxItems value",
			schemaYAML: `
field: "[]string | maxItems=xyz"
`,
			expectError: "invalid maxItems",
		},
		{
			name: "invalid uniqueItems value",
			schemaYAML: `
field: "[]string | uniqueItems=notabool"
`,
			expectError: "invalid uniqueItems",
		},
		{
			name: "invalid exclusiveMinimum value",
			schemaYAML: `
field: "number | exclusiveMinimum=notabool"
`,
			expectError: "invalid exclusiveMinimum",
		},
		{
			name: "invalid exclusiveMaximum value",
			schemaYAML: `
field: "number | exclusiveMaximum=xyz"
`,
			expectError: "invalid exclusiveMaximum",
		},
		{
			name: "invalid integer default",
			schemaYAML: `
field: "integer | default=notanumber"
`,
			expectError: "invalid default",
		},
		{
			name: "invalid number default",
			schemaYAML: `
field: "number | default=xyz"
`,
			expectError: "invalid default",
		},
		{
			name: "invalid boolean default",
			schemaYAML: `
field: "boolean | default=notabool"
`,
			expectError: "invalid default",
		},
		{
			name: "invalid integer enum",
			schemaYAML: `
field: "integer | enum=a,b,c"
`,
			expectError: "invalid enum value",
		},
		{
			name: "invalid number enum",
			schemaYAML: `
field: "number | enum=x,y,z"
`,
			expectError: "invalid enum value",
		},
		{
			name: "empty integer default",
			schemaYAML: `
field: "integer | default="
`,
			expectError: "empty integer value",
		},
		{
			name: "empty number default",
			schemaYAML: `
field: "number | default="
`,
			expectError: "empty number value",
		},
		{
			name: "map with non-string key type (int)",
			schemaYAML: `
field: "map[int]string"
`,
			expectError: "map key type must be 'string'",
		},
		{
			name: "map with non-string key type (number)",
			schemaYAML: `
field: "map[number]boolean"
`,
			expectError: "map key type must be 'string'",
		},
		{
			name: "map with non-string key type (integer)",
			schemaYAML: `
field: "map[integer]string"
`,
			expectError: "map key type must be 'string'",
		},
		{
			name: "object type not allowed as field",
			schemaYAML: `
field: "object"
`,
			expectError: "'object' type is not allowed",
		},
		{
			name: "object type not allowed in map values",
			schemaYAML: `
field: "map[string]object"
`,
			expectError: "'object' type is not allowed",
		},
		{
			name: "object type not allowed in map values (angle bracket syntax)",
			schemaYAML: `
field: "map<object>"
`,
			expectError: "'object' type is not allowed",
		},
		{
			name: "object type not allowed in array items",
			schemaYAML: `
field: "[]object"
`,
			expectError: "'object' type is not allowed",
		},
		{
			name: "object type not allowed in array items (array syntax)",
			schemaYAML: `
field: "array<object>"
`,
			expectError: "'object' type is not allowed",
		},
		{
			name: "object type not allowed in nested structures",
			schemaYAML: `
field: "map[string][]object"
`,
			expectError: "'object' type is not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var types map[string]any
			if strings.TrimSpace(tt.typesYAML) != "" {
				types = parseYAMLMap(t, tt.typesYAML)
			}
			fields := parseYAMLMap(t, tt.schemaYAML)

			_, err := ExtractSchema(fields, types)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.expectError)
			}
			if !strings.Contains(err.Error(), tt.expectError) {
				t.Fatalf("expected error containing %q, got: %v", tt.expectError, err)
			}
		})
	}
}

func TestConverter_CombinedConstraintsSpacing(t *testing.T) {
	const schemaYAML = `
field: string | default=foo pattern=^[a-z]+$ minLength=3
`
	const expected = `{
  "type": "object",
  "properties": {
    "field": {
      "type": "string",
      "default": "foo",
      "minLength": 3,
      "pattern": "^[a-z]+$"
    }
  }
}`

	root := parseYAMLMap(t, schemaYAML)

	internalSchema, err := ExtractSchema(root, nil)
	if err != nil {
		t.Fatalf("ExtractSchema returned error: %v", err)
	}

	// Convert to v1 for JSON comparison
	v1Schema := new(extv1.JSONSchemaProps)
	if err := extv1.Convert_apiextensions_JSONSchemaProps_To_v1_JSONSchemaProps(internalSchema, v1Schema, nil); err != nil {
		t.Fatalf("failed to convert schema to v1: %v", err)
	}

	assertSchemaJSON(t, v1Schema, expected)
}

func TestConverter_EnumParsing(t *testing.T) {
	const schemaYAML = `
level: string | enum=debug,info,warn | default=info
status: 'string | enum="active","inactive","pending" | default="active"'
priority: 'integer | enum=1,2,3 | default=1'
code: 'integer | enum="1","2","3" | default="1"'
`
	const expected = `{
  "type": "object",
  "properties": {
    "code": {
      "type": "integer",
      "default": 1,
      "enum": [
        1,
        2,
        3
      ]
    },
    "level": {
      "type": "string",
      "default": "info",
      "enum": [
        "debug",
        "info",
        "warn"
      ]
    },
    "priority": {
      "type": "integer",
      "default": 1,
      "enum": [
        1,
        2,
        3
      ]
    },
    "status": {
      "type": "string",
      "default": "active",
      "enum": [
        "active",
        "inactive",
        "pending"
      ]
    }
  }
}`

	assertConvertedSchema(t, "", schemaYAML, expected)
}

func TestConverter_SpaceSeparatedConstraintsNoSpaceAfterPipe(t *testing.T) {
	const schemaYAML = `
field1: string|default=foo
field2: string|enum=a,b,c default=b
`
	const expected = `{
  "type": "object",
  "properties": {
    "field1": {
      "type": "string",
      "default": "foo"
    },
    "field2": {
      "type": "string",
      "default": "b",
      "enum": [
        "a",
        "b",
        "c"
      ]
    }
  }
}`

	assertConvertedSchema(t, "", schemaYAML, expected)
}

func TestConverter_PipeInsideQuotes(t *testing.T) {
	const schemaYAML = `
pattern: 'string | pattern="a|b|c" default="x|y"'
`
	const expected = `{
  "type": "object",
  "properties": {
    "pattern": {
      "type": "string",
      "default": "x|y",
      "pattern": "a|b|c"
    }
  }
}`

	assertConvertedSchema(t, "", schemaYAML, expected)
}

func TestConverter_RequiredUnlessDefault(t *testing.T) {
	// Fields without defaults are required; fields with defaults are optional
	const schemaYAML = `
field1: string
field2: integer
field3: 'string | default=foo'
`
	const expected = `{
  "type": "object",
  "required": [
    "field1",
    "field2"
  ],
  "properties": {
    "field1": {
      "type": "string"
    },
    "field2": {
      "type": "integer"
    },
    "field3": {
      "type": "string",
      "default": "foo"
    }
  }
}`

	fields := parseYAMLMap(t, schemaYAML)

	internalSchema, err := ExtractSchema(fields, nil)
	if err != nil {
		t.Fatalf("ExtractSchema returned error: %v", err)
	}

	// Convert to v1 for JSON comparison
	v1Schema := new(extv1.JSONSchemaProps)
	if err := extv1.Convert_apiextensions_JSONSchemaProps_To_v1_JSONSchemaProps(internalSchema, v1Schema, nil); err != nil {
		t.Fatalf("failed to convert schema to v1: %v", err)
	}

	assertSchemaJSON(t, v1Schema, expected)
}

func TestConverter_ErrorOnUnknownMarkers(t *testing.T) {
	const schemaYAML = `
field: 'string | unknownMarker=foo'
`

	fields := parseYAMLMap(t, schemaYAML)

	// Unknown markers (without oc: prefix) should error
	_, err := ExtractSchema(fields, nil)
	if err == nil {
		t.Fatal("expected error with unknown marker")
	}
	if !strings.Contains(err.Error(), "unknown constraint marker") {
		t.Fatalf("expected error about unknown marker, got: %v", err)
	}
}

func TestConverter_OcPrefixMarkersIgnored(t *testing.T) {
	// Markers with oc: prefix should be silently ignored
	const schemaYAML = `
field: 'string | oc:custom=foo oc:annotation=bar'
`

	fields := parseYAMLMap(t, schemaYAML)

	schema, err := ExtractSchema(fields, nil)
	if err != nil {
		t.Fatalf("ExtractSchema with oc: prefixed markers should not error, got: %v", err)
	}
	if schema == nil {
		t.Fatal("expected valid schema")
	}
}

func TestConverter_SingleQuotedStringsWithDoubleQuotes(t *testing.T) {
	// Single-quoted YAML strings can contain unescaped double quotes
	// Common in JSONPath expressions, filters, and query strings
	const schemaYAML = `
jsonPath: 'string | default=''.status.conditions[?(@.type=="Ready")].status'''
`
	const expected = `{
  "type": "object",
  "properties": {
    "jsonPath": {
      "type": "string",
      "default": ".status.conditions[?(@.type==\"Ready\")].status"
    }
  }
}`

	assertConvertedSchema(t, "", schemaYAML, expected)
}

func TestConverter_SingleQuotedStringsWithEscapedQuotes(t *testing.T) {
	// In YAML single-quoted strings, '' escapes a single quote
	// Common in descriptions, labels, and annotations
	const schemaYAML = `
description: 'string | default=''User''''s preferred timezone'''
`
	const expected = `{
  "type": "object",
  "properties": {
    "description": {
      "type": "string",
      "default": "User's preferred timezone"
    }
  }
}`

	assertConvertedSchema(t, "", schemaYAML, expected)
}

func TestConverter_SingleQuotedEnumWithSpecialChars(t *testing.T) {
	// Enum values with quotes in configuration contexts
	const schemaYAML = `
logLevel: 'string | enum=''info'',''warn'',''error'',''debug'' | default=''info'''
`
	const expected = `{
  "type": "object",
  "properties": {
    "logLevel": {
      "type": "string",
      "default": "info",
      "enum": [
        "info",
        "warn",
        "error",
        "debug"
      ]
    }
  }
}`

	assertConvertedSchema(t, "", schemaYAML, expected)
}

func TestConverter_DoubleQuotedStringsWithEscaping(t *testing.T) {
	// Double-quoted strings use backslash escaping
	// Common for regex patterns and escape sequences
	const schemaYAML = `
pattern: "string | default=\"^[a-z]+\\\\d{3}$\""
`
	const expected = `{
  "type": "object",
  "properties": {
    "pattern": {
      "type": "string",
      "default": "^[a-z]+\\d{3}$"
    }
  }
}`

	assertConvertedSchema(t, "", schemaYAML, expected)
}

func assertSchemaJSON(t *testing.T, schema any, expected string) {
	t.Helper()

	actualBytes, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal schema: %v", err)
	}

	if string(actualBytes) != expected {
		t.Fatalf("schema JSON mismatch\nexpected:\n%s\nactual:\n%s", expected, string(actualBytes))
	}
}

func assertConvertedSchema(t *testing.T, typesYAML, schemaYAML, expected string) {
	t.Helper()

	var types map[string]any
	if strings.TrimSpace(typesYAML) != "" {
		types = parseYAMLMap(t, typesYAML)
	}
	root := parseYAMLMap(t, schemaYAML)

	internalSchema, err := ExtractSchema(root, types)
	if err != nil {
		t.Fatalf("ExtractSchema returned error: %v", err)
	}

	// Convert to v1 for JSON comparison (v1 has JSON tags for proper serialization)
	v1Schema := new(extv1.JSONSchemaProps)
	if err := extv1.Convert_apiextensions_JSONSchemaProps_To_v1_JSONSchemaProps(internalSchema, v1Schema, nil); err != nil {
		t.Fatalf("failed to convert schema to v1: %v", err)
	}

	assertSchemaJSON(t, v1Schema, expected)
}

func parseYAMLMap(t *testing.T, doc string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := yaml.Unmarshal([]byte(doc), &out); err != nil {
		t.Fatalf("failed to parse yaml: %v", err)
	}
	return out
}

func TestSplitRespectingQuotes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple values without quotes",
			input:    "a,b,c",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "quoted values without commas",
			input:    `"value1","value2","value3"`,
			expected: []string{`"value1"`, `"value2"`, `"value3"`},
		},
		{
			name:     "quoted values with commas inside",
			input:    `"lastname, firstname","firstname lastname","last, first, middle"`,
			expected: []string{`"lastname, firstname"`, `"firstname lastname"`, `"last, first, middle"`},
		},
		{
			name:     "mixed quoted and unquoted",
			input:    `simple,"with space","with, comma"`,
			expected: []string{"simple", `"with space"`, `"with, comma"`},
		},
		{
			name:     "values with escaped quotes",
			input:    `"value with \"quotes\"","simple"`,
			expected: []string{`"value with \"quotes\""`, `"simple"`},
		},
		{
			name:     "complex case with commas and quotes",
			input:    `"pending","in-progress","user said: \"hello, world\""`,
			expected: []string{`"pending"`, `"in-progress"`, `"user said: \"hello, world\""`},
		},
		{
			name:     "empty values filtered out",
			input:    `a,,b,  ,c`,
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "values with spaces around commas",
			input:    `"value1" , "value2" , "value3"`,
			expected: []string{`"value1"`, `"value2"`, `"value3"`},
		},
		{
			name:     "complex combination",
			input:    `"simple","with spaces","with, comma","with \"quotes\"","combo: \"a, b\""`,
			expected: []string{`"simple"`, `"with spaces"`, `"with, comma"`, `"with \"quotes\""`, `"combo: \"a, b\""`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitRespectingQuotes(tt.input, ",")
			if len(result) != len(tt.expected) {
				t.Fatalf("length mismatch: expected %d values, got %d\nexpected: %v\ngot: %v",
					len(tt.expected), len(result), tt.expected, result)
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("value %d mismatch: expected %q, got %q", i, tt.expected[i], result[i])
				}
			}
		})
	}
}

func TestConverter_InlineObjectWithDollarDefaultEmpty(t *testing.T) {
	const typesYAML = ``
	const schemaYAML = `
monitoring:
  $default: {}
  enabled: 'boolean | default=false'
  port: 'integer | default=9090'
`
	const expected = `{
  "type": "object",
  "properties": {
    "monitoring": {
      "type": "object",
      "default": {},
      "properties": {
        "enabled": {
          "type": "boolean",
          "default": false
        },
        "port": {
          "type": "integer",
          "default": 9090
        }
      }
    }
  }
}`

	assertConvertedSchema(t, typesYAML, schemaYAML, expected)
}

func TestConverter_InlineObjectWithDollarDefaultValues(t *testing.T) {
	const typesYAML = ``
	const schemaYAML = `
database:
  $default:
    host: "localhost"
    port: 5432
  host: string
  port: 'integer | default=5432'
`
	const expected = `{
  "type": "object",
  "properties": {
    "database": {
      "type": "object",
      "default": {
        "host": "localhost",
        "port": 5432
      },
      "required": [
        "host"
      ],
      "properties": {
        "host": {
          "type": "string"
        },
        "port": {
          "type": "integer",
          "default": 5432
        }
      }
    }
  }
}`

	assertConvertedSchema(t, typesYAML, schemaYAML, expected)
}

func TestConverter_MultipleInlineObjectsWithDollarDefault(t *testing.T) {
	const typesYAML = ``
	const schemaYAML = `
resources:
  requests:
    $default: {}
    cpu: 'string | default=100m'
    memory: 'string | default=256Mi'
  limits:
    $default: {}
    cpu: 'string | default=1000m'
    memory: 'string | default=1Gi'
`
	const expected = `{
  "type": "object",
  "required": [
    "resources"
  ],
  "properties": {
    "resources": {
      "type": "object",
      "properties": {
        "limits": {
          "type": "object",
          "default": {},
          "properties": {
            "cpu": {
              "type": "string",
              "default": "1000m"
            },
            "memory": {
              "type": "string",
              "default": "1Gi"
            }
          }
        },
        "requests": {
          "type": "object",
          "default": {},
          "properties": {
            "cpu": {
              "type": "string",
              "default": "100m"
            },
            "memory": {
              "type": "string",
              "default": "256Mi"
            }
          }
        }
      }
    }
  }
}`

	assertConvertedSchema(t, typesYAML, schemaYAML, expected)
}

func TestConverter_DollarDefaultWithJSONString(t *testing.T) {
	const typesYAML = ``
	const schemaYAML = `
config:
  $default: '{"enabled":true,"count":5}'
  enabled: 'boolean | default=false'
  count: 'integer | default=1'
`
	const expected = `{
  "type": "object",
  "properties": {
    "config": {
      "type": "object",
      "default": {
        "count": 5,
        "enabled": true
      },
      "properties": {
        "count": {
          "type": "integer",
          "default": 1
        },
        "enabled": {
          "type": "boolean",
          "default": false
        }
      }
    }
  }
}`

	assertConvertedSchema(t, typesYAML, schemaYAML, expected)
}

func TestConverter_NestedObjectWithDollarDefault(t *testing.T) {
	const typesYAML = ``
	const schemaYAML = `
server:
  $default: {}
  host: 'string | default=localhost'
  tls:
    $default: {}
    enabled: 'boolean | default=false'
    port: 'integer | default=443'
`
	const expected = `{
  "type": "object",
  "properties": {
    "server": {
      "type": "object",
      "default": {},
      "properties": {
        "host": {
          "type": "string",
          "default": "localhost"
        },
        "tls": {
          "type": "object",
          "default": {},
          "properties": {
            "enabled": {
              "type": "boolean",
              "default": false
            },
            "port": {
              "type": "integer",
              "default": 443
            }
          }
        }
      }
    }
  }
}`

	assertConvertedSchema(t, typesYAML, schemaYAML, expected)
}

func TestConverter_TypeWithDollarDefault(t *testing.T) {
	const typesYAML = `
Resources:
  $default:
    cpu: "100m"
    memory: "256Mi"
  cpu: string
  memory: string
`
	const schemaYAML = `
defaultResources: Resources
customResources: 'Resources | default={"cpu":"500m","memory":"1Gi"}'
`
	const expected = `{
  "type": "object",
  "properties": {
    "customResources": {
      "type": "object",
      "default": {
        "cpu": "500m",
        "memory": "1Gi"
      },
      "required": [
        "cpu",
        "memory"
      ],
      "properties": {
        "cpu": {
          "type": "string"
        },
        "memory": {
          "type": "string"
        }
      }
    },
    "defaultResources": {
      "type": "object",
      "default": {
        "cpu": "100m",
        "memory": "256Mi"
      },
      "required": [
        "cpu",
        "memory"
      ],
      "properties": {
        "cpu": {
          "type": "string"
        },
        "memory": {
          "type": "string"
        }
      }
    }
  }
}`

	assertConvertedSchema(t, typesYAML, schemaYAML, expected)
}

func TestConverter_DollarDefaultErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		schemaYAML  string
		expectError string
	}{
		{
			name: "empty default missing required field",
			schemaYAML: `
cache:
  $default: {}
  host: string
  port: 'integer | default=6379'
`,
			expectError: "is required",
		},
		{
			name: "default with invalid type",
			schemaYAML: `
config:
  $default: "not an object"
  enabled: boolean
`,
			expectError: "value is not a valid object",
		},
		{
			name: "default missing multiple required fields",
			schemaYAML: `
database:
  $default:
    port: 5432
  host: string
  name: string
  port: 'integer | default=5432'
`,
			expectError: "is required",
		},
		{
			name: "default with non-map value",
			schemaYAML: `
items:
  $default: []
  name: string
`,
			expectError: "value must be an object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := parseYAMLMap(t, tt.schemaYAML)

			_, err := ExtractSchema(fields, nil)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.expectError)
			}
			if !strings.Contains(err.Error(), tt.expectError) {
				t.Fatalf("expected error containing %q, got: %v", tt.expectError, err)
			}
		})
	}
}
