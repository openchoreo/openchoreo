// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"fmt"
	"sort"

	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/defaulting"

	"github.com/openchoreo/openchoreo/internal/clone"
	"github.com/openchoreo/openchoreo/internal/schema/extractor"
)

// Definition represents a schematized object assembled from one or more field maps.
type Definition struct {
	Types   map[string]any
	Schemas []map[string]any
}

// ToJSONSchema converts a schema definition into an OpenAPI v3 JSON schema.
//
// This is the primary entry point for converting ComponentTypeDefinition and Addon schemas
// from the shorthand format into standard JSON Schema that can be used for validation.
//
// Process:
//  1. Merge all schema maps (parameters, envOverrides, addon config) into one
//  2. Convert from shorthand syntax to full JSON Schema via extractor
//  3. Sort required fields for deterministic output
//
// Example input (shorthand):
//
//	schemas: [{replicas: "integer | default=1"}, {environment: "string"}]
//
// Example output (JSON Schema):
//
//	{type: "object", properties: {replicas: {type: "integer", default: 1}, environment: {type: "string"}}}
func ToJSONSchema(def Definition) (*extv1.JSONSchemaProps, error) {
	merged := mergeFieldMaps(def.Schemas)
	if len(merged) == 0 {
		return &extv1.JSONSchemaProps{
			Type:       "object",
			Properties: map[string]extv1.JSONSchemaProps{},
		}, nil
	}

	jsonSchema, err := extractor.ExtractSchema(merged, def.Types)
	if err != nil {
		return nil, fmt.Errorf("failed to convert schema to OpenAPI: %w", err)
	}

	sortRequiredFields(jsonSchema)
	return jsonSchema, nil
}

// ToStructural converts the definition into a Kubernetes structural schema.
//
// Structural schemas are a stricter variant of JSON Schema used by Kubernetes for:
//  1. Server-side validation of CRD instances
//  2. Defaulting field values based on schema defaults
//  3. Pruning unknown fields during admission
//
// The conversion process:
//  1. Convert to standard JSON Schema (OpenAPI v3)
//  2. Convert from v1 to internal API version
//  3. Validate and convert to structural format (enforces additional constraints)
//
// Structural schema constraints include:
//   - All objects must have explicit type: "object"
//   - All arrays must specify item schema
//   - No x-kubernetes-* extensions in certain contexts
//
// This is primarily used with ApplyDefaults to populate default values.
func ToStructural(def Definition) (*apiextschema.Structural, error) {
	jsonSchemaV1, err := ToJSONSchema(def)
	if err != nil {
		return nil, err
	}

	internal := new(apiext.JSONSchemaProps)
	if err := extv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(jsonSchemaV1, internal, nil); err != nil {
		return nil, fmt.Errorf("failed to convert schema: %w", err)
	}

	structural, err := apiextschema.NewStructural(internal)
	if err != nil {
		return nil, fmt.Errorf("failed to build structural schema: %w", err)
	}
	return structural, nil
}

// ApplyDefaults applies schema default values to a target object using Kubernetes defaulting logic.
//
// This function walks the structural schema and target object in parallel, filling in default
// values for missing fields. The defaulting algorithm follows the same rules as Kubernetes
// API server defaulting for CustomResourceDefinitions.
//
// Behavior:
//   - Missing fields with defaults are added
//   - Existing fields are not overwritten (even if they differ from the default)
//   - Nested objects are defaulted recursively
//   - Array items are not defaulted (Kubernetes limitation)
//
// The target map is modified in place and also returned for convenience.
//
// Example:
//
//	Schema: {replicas: {type: "integer", default: 1}, image: {type: "string"}}
//	Input:  {image: "nginx"}
//	Output: {image: "nginx", replicas: 1}
func ApplyDefaults(target map[string]any, structural *apiextschema.Structural) map[string]any {
	if structural == nil {
		return target
	}
	if target == nil {
		target = map[string]any{}
	}
	defaulting.Default(target, structural)
	return target
}

// mergeFieldMaps combines multiple schema maps into a single unified schema.
//
// ComponentTypeDefinitions separate schemas into logical groups:
//   - schema.parameters: Component-level configuration
//   - schema.envOverrides: Environment-specific overrides
//   - schema.addonConfig: Addon instance configuration (for Addons)
//
// This function merges them using deep merge semantics so that:
//  1. Nested objects are merged recursively
//  2. Later schemas can extend or override earlier ones
//  3. The result matches how templates access these values (all under ${spec.*})
//
// Example:
//
//	Input: [{port: "integer"}, {replicas: "integer"}]
//	Output: {port: "integer", replicas: "integer"}
func mergeFieldMaps(maps []map[string]any) map[string]any {
	result := map[string]any{}
	for _, fields := range maps {
		mergeInto(result, fields)
	}
	return result
}

// mergeInto recursively merges src into dst, modifying dst in place.
//
// Merge behavior:
//   - If src value is a map and dst has a map at that key: recursively merge
//   - Otherwise: overwrite dst's value with a deep copy of src's value
//   - All values are deep copied to avoid shared references
//
// This ensures that nested object schemas are properly combined rather than replaced.
//
// Example:
//
//	dst: {db: {host: "string"}}
//	src: {db: {port: "integer"}, replicas: "integer"}
//	Result: {db: {host: "string", port: "integer"}, replicas: "integer"}
func mergeInto(dst map[string]any, src map[string]any) {
	if src == nil {
		return
	}
	if dst == nil {
		// should not happen, but guard anyway
		return
	}
	for k, v := range src {
		if vMap, ok := v.(map[string]any); ok {
			existing, ok := dst[k].(map[string]any)
			if !ok {
				dst[k] = clone.DeepCopy(vMap)
				continue
			}
			mergeInto(existing, vMap)
			continue
		}
		dst[k] = clone.DeepCopy(v)
	}
}

// sortRequiredFields recursively sorts the 'required' arrays in a JSON schema.
//
// This ensures deterministic output across multiple runs, which is important for:
//  1. Minimizing git diffs when schemas change
//  2. CLI/UI schema generators that rely on consistent ordering
//  3. Testing (comparing schemas requires consistent output)
//
// The function traverses the entire schema tree, sorting required fields in:
//   - Top-level objects
//   - Nested object properties
//   - Array item schemas
//   - Additional properties schemas
//
// Note: This modifies the schema in place, including nested properties.
func sortRequiredFields(schema *extv1.JSONSchemaProps) {
	if schema == nil {
		return
	}
	if len(schema.Required) > 0 {
		// Keep output deterministic for CLI/UI generators and to minimize diffs when definitions change.
		sort.Strings(schema.Required)
	}
	if schema.Properties != nil {
		for key := range schema.Properties {
			prop := schema.Properties[key]
			sortRequiredFields(&prop)
			schema.Properties[key] = prop
		}
	}
	if schema.Items != nil && schema.Items.Schema != nil {
		sortRequiredFields(schema.Items.Schema)
	}
	if schema.AdditionalProperties != nil && schema.AdditionalProperties.Schema != nil {
		sortRequiredFields(schema.AdditionalProperties.Schema)
	}
}
