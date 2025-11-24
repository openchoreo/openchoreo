// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package extractor

import (
	"encoding/json"
	"strings"
	"testing"

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

	schema, err := ExtractSchema(root, types)
	if err != nil {
		t.Fatalf("ExtractSchema returned error: %v", err)
	}

	assertSchemaJSON(t, schema, expected)
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

	schema, err := ExtractSchema(root, nil)
	if err != nil {
		t.Fatalf("ExtractSchema returned error: %v", err)
	}

	assertSchemaJSON(t, schema, expected)
}

func TestConverter_EnumParsing(t *testing.T) {
	const schemaYAML = `
level: string | enum=debug,info,warn | default=info
status: 'string | enum="active","inactive","pending" | default="active"'
priority: 'integer | enum=1,2,3 | default=1'
code: 'integer | enum="1","2","3" | default="1"'
nodeVersion: 'string | default="18" enum="16,18,20,22"'
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
    "nodeVersion": {
      "type": "string",
      "default": "18",
      "enum": [
        "16",
        "18",
        "20",
        "22"
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

	schema, err := ExtractSchema(fields, nil)
	if err != nil {
		t.Fatalf("ExtractSchema returned error: %v", err)
	}

	assertSchemaJSON(t, schema, expected)
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
	// Enum values in single-quoted YAML strings (unquoted list)
	const schemaYAML = `
logLevel: 'string | enum=info,warn,error,debug | default=info'
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

	schema, err := ExtractSchema(root, types)
	if err != nil {
		t.Fatalf("ExtractSchema returned error: %v", err)
	}

	assertSchemaJSON(t, schema, expected)
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
