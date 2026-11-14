// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"encoding/json"
	"strconv"
)

// FieldSchema holds validation rules for a single field from the JSON schema
// definition emitted by the json-schema-generator. It is shared by every
// rule-engine template package.
type FieldSchema struct {
	Type       string                  // valueString, valueObject, "array", valueInteger, "boolean"
	Required   bool                    // whether the field must be present
	Properties map[string]*FieldSchema // nested fields (for objects)
	Items      *FieldSchema            // element schema (for arrays)
	Enum       []string                // allowed enum values
	Pattern    string                  // regex pattern
	MinLength  int                     // minimum string length
	MaxLength  int                     // maximum string length
	Preserve   bool                    // preserve unknown fields (skip children validation)
	Default    string                  // display/sample value used by autocompletion
}

// Parse builds a FieldSchema tree from the given JSON schema definition.
func Parse(data []byte) (*FieldSchema, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return buildSchema(raw), nil
}

// buildSchema recursively converts a JSON schema node into a FieldSchema.
func buildSchema(m map[string]interface{}) *FieldSchema {
	fs := &FieldSchema{}

	if v, ok := m["type"].(string); ok {
		fs.Type = v
	}
	if v, ok := m["required"].(bool); ok {
		fs.Required = v
	}
	if v, ok := m["preserveUnknownFields"].(bool); ok {
		fs.Preserve = v
	}
	if v, ok := m["pattern"].(string); ok {
		fs.Pattern = v
	}
	if dv, ok := m["default"]; ok {
		fs.Default = normalizeDefault(dv)
	}
	if v, ok := m["minLength"].(float64); ok {
		fs.MinLength = int(v)
	}
	if v, ok := m["maxLength"].(float64); ok {
		fs.MaxLength = int(v)
	}
	if v, ok := m["enum"].([]interface{}); ok {
		for _, e := range v {
			if s, ok := e.(string); ok {
				fs.Enum = append(fs.Enum, s)
			}
		}
	}
	if v, ok := m["items"].(map[string]interface{}); ok {
		fs.Items = buildSchema(v)
	}
	if v, ok := m["properties"].(map[string]interface{}); ok {
		fs.Properties = make(map[string]*FieldSchema)
		for key, val := range v {
			if child, ok := val.(map[string]interface{}); ok {
				fs.Properties[key] = buildSchema(child)
			}
		}
	}
	return fs
}

// normalizeDefault converts a JSON "default" value (which may be a string,
// number, or boolean) into its string form so it can be injected into the
// generated YAML sample regardless of the leaf data type.
func normalizeDefault(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return ""
	}
}

// FindSchema returns the schema definition for a dot-separated field path
// (e.g., "spec.componentType.name") from the root schema, or nil if any part
// of the path does not exist. Arrays are transparent: a path segment that
// follows an array refers to a property of its element schema (sequence items
// have no index in a path, e.g. "spec.roleMappings.roleRef.name").
func FindSchema(root *FieldSchema, path string) *FieldSchema {
	if root == nil || path == "" {
		return root
	}
	current := root
	for _, part := range splitPath(path) {
		if current == nil {
			return nil
		}
		if current.Type == valueArray {
			current = current.Items
		}
		if current == nil {
			return nil
		}
		current = current.Properties[part]
	}
	return current
}

func splitPath(path string) []string {
	parts := make([]string, 0, 8)
	start := 0
	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == '.' {
			parts = append(parts, path[start:i])
			start = i + 1
		}
	}
	return parts
}
