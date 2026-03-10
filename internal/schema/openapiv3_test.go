// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"testing"
)

func TestOpenAPIV3ToStructural_Primitives(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":    "string",
				"default": "hello",
			},
			"count": map[string]any{
				"type":    "integer",
				"minimum": float64(0),
			},
		},
	}

	structural, err := OpenAPIV3ToStructural(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structural == nil {
		t.Fatal("expected non-nil structural schema")
	}
	if structural.Type != typeObject {
		t.Fatalf("expected type=object, got %s", structural.Type)
	}
	if _, ok := structural.Properties["name"]; !ok {
		t.Fatal("expected 'name' property")
	}
	if _, ok := structural.Properties["count"]; !ok {
		t.Fatal("expected 'count' property")
	}
}

func TestOpenAPIV3ToStructural_WithRefs(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"$defs": map[string]any{
			"Port": map[string]any{
				"type":    "integer",
				"minimum": float64(1),
				"maximum": float64(65535),
				"default": float64(8080),
			},
		},
		"properties": map[string]any{
			"port": map[string]any{"$ref": "#/$defs/Port"},
		},
	}

	structural, err := OpenAPIV3ToStructural(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structural == nil {
		t.Fatal("expected non-nil structural schema")
	}

	portProp, ok := structural.Properties["port"]
	if !ok {
		t.Fatal("expected 'port' property")
	}
	if portProp.Type != typeInteger {
		t.Fatalf("expected port type=integer, got %s", portProp.Type)
	}
}

func TestOpenAPIV3ToJSONSchema_Primitives(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":      "string",
				"minLength": float64(1),
				"default":   "hello",
			},
		},
		"required": []any{"name"},
	}

	jsonSchema, err := OpenAPIV3ToJSONSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jsonSchema == nil {
		t.Fatal("expected non-nil JSON schema")
	}
	if jsonSchema.Type != typeObject {
		t.Fatalf("expected type=object, got %s", jsonSchema.Type)
	}

	nameProp, ok := jsonSchema.Properties["name"]
	if !ok {
		t.Fatal("expected 'name' property")
	}
	if nameProp.Type != typeString {
		t.Fatalf("expected name type=string, got %s", nameProp.Type)
	}
	if len(jsonSchema.Required) != 1 || jsonSchema.Required[0] != "name" { //nolint:goconst // test assertion
		t.Fatalf("expected required=[name], got %v", jsonSchema.Required)
	}
}

func TestOpenAPIV3ToJSONSchema_VendorExtensionsLost(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"x-ui-widget": "textarea",
			},
		},
	}

	// extv1.JSONSchemaProps does NOT preserve arbitrary x-* extensions.
	// Use OpenAPIV3ToResolvedSchema for API responses that need them.
	jsonSchema, err := OpenAPIV3ToJSONSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jsonSchema == nil {
		t.Fatal("expected non-nil JSON schema")
	}

	// Verify the vendor extension is not present in the output
	nameProp, ok := jsonSchema.Properties["name"]
	if !ok {
		t.Fatal("expected 'name' property")
	}
	if nameProp.Type != typeString {
		t.Fatalf("expected name type=string, got %s", nameProp.Type)
	}
	// extv1.JSONSchemaProps does not have a field for arbitrary x-* keys,
	// so they are silently dropped during JSON unmarshal.
}

func TestOpenAPIV3ToStructural_VendorExtensionsStripped(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"x-ui-widget": "textarea",
			},
		},
	}

	// Structural path should work even with vendor extensions in input.
	// stripVendorExtensions removes x-* keys before conversion so that
	// Kubernetes structural schema validation does not reject them.
	structural, err := OpenAPIV3ToStructural(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structural == nil {
		t.Fatal("expected non-nil structural schema")
	}

	// Verify the property survived and the structural schema has no extension data
	nameProp, ok := structural.Properties["name"]
	if !ok {
		t.Fatal("expected 'name' property in structural schema")
	}
	if nameProp.Type != typeString {
		t.Fatalf("expected name type=string, got %s", nameProp.Type)
	}
	// Kubernetes Structural type has no field for vendor extensions,
	// so if we reach here without error, x-ui-widget was successfully stripped.
}

func TestOpenAPIV3ToStructuralAndJSONSchema(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"replicas": map[string]any{
				"type":    "integer",
				"default": float64(1),
			},
			"image": map[string]any{
				"type": "string",
			},
		},
		"required": []any{"image"},
	}

	structural, jsonSchema, err := OpenAPIV3ToStructuralAndJSONSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structural == nil {
		t.Fatal("expected non-nil structural schema")
	}
	if jsonSchema == nil {
		t.Fatal("expected non-nil JSON schema")
	}

	// Verify structural
	if _, ok := structural.Properties["replicas"]; !ok {
		t.Fatal("structural: expected 'replicas' property")
	}

	// Verify JSON schema
	if _, ok := jsonSchema.Properties["image"]; !ok {
		t.Fatal("jsonSchema: expected 'image' property")
	}
	if len(jsonSchema.Required) != 1 || jsonSchema.Required[0] != "image" {
		t.Fatalf("expected required=[image], got %v", jsonSchema.Required)
	}
}

func TestOpenAPIV3ToStructural_NestedObject(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"database": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"host": map[string]any{"type": "string"},
					"port": map[string]any{"type": "integer", "default": float64(5432)},
				},
				"required": []any{"host"},
			},
		},
	}

	structural, err := OpenAPIV3ToStructural(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dbProp, ok := structural.Properties["database"]
	if !ok {
		t.Fatal("expected 'database' property")
	}
	if dbProp.Type != typeObject {
		t.Fatalf("expected database type=object, got %s", dbProp.Type)
	}
	if _, ok := dbProp.Properties["host"]; !ok {
		t.Fatal("expected 'host' property in database")
	}
}

func TestOpenAPIV3ToStructural_ArrayItems(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tags": map[string]any{
				"type":     "array",
				"items":    map[string]any{"type": "string"},
				"minItems": float64(1),
			},
		},
	}

	structural, err := OpenAPIV3ToStructural(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tagsProp, ok := structural.Properties["tags"]
	if !ok {
		t.Fatal("expected 'tags' property")
	}
	if tagsProp.Type != "array" {
		t.Fatalf("expected tags type=array, got %s", tagsProp.Type)
	}
}

func TestOpenAPIV3ToStructural_Enum(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"env": map[string]any{
				"type":    "string",
				"enum":    []any{"dev", "staging", "prod"},
				"default": "dev",
			},
		},
	}

	structural, err := OpenAPIV3ToStructural(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	envProp, ok := structural.Properties["env"]
	if !ok {
		t.Fatal("expected 'env' property")
	}
	if envProp.Type != typeString {
		t.Fatalf("expected env type=string, got %s", envProp.Type)
	}
}

func TestOpenAPIV3ToStructural_MapType(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"labels": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
			},
		},
	}

	structural, err := OpenAPIV3ToStructural(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	labelsProp, ok := structural.Properties["labels"]
	if !ok {
		t.Fatal("expected 'labels' property")
	}
	if labelsProp.Type != typeObject {
		t.Fatalf("expected labels type=object, got %s", labelsProp.Type)
	}
}

func TestOpenAPIV3ToStructural_EmptySchema(t *testing.T) {
	schema := map[string]any{
		"type": "object",
	}

	structural, err := OpenAPIV3ToStructural(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structural == nil {
		t.Fatal("expected non-nil structural schema")
	}
}

func TestStripVendorExtensions(t *testing.T) {
	schema := map[string]any{
		"type":        "object",
		"x-ui-widget": "form",
		"properties": map[string]any{
			"name": map[string]any{
				"type":      "string",
				"x-display": "hidden",
			},
		},
	}

	result := stripVendorExtensions(schema)

	if _, ok := result["x-ui-widget"]; ok {
		t.Fatal("expected x-ui-widget to be stripped")
	}
	if _, ok := result["type"]; !ok {
		t.Fatal("expected type to be preserved")
	}

	props := result["properties"].(map[string]any)
	name := props["name"].(map[string]any)
	if _, ok := name["x-display"]; ok {
		t.Fatal("expected x-display to be stripped from nested property")
	}
	if _, ok := name["type"]; !ok {
		t.Fatal("expected type to be preserved in nested property")
	}
}

func TestStripVendorExtensions_Nil(t *testing.T) {
	result := stripVendorExtensions(nil)
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestOpenAPIV3ToStructural_ArrayOfObjects(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"containers": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":  map[string]any{"type": "string"},
						"image": map[string]any{"type": "string"},
						"port":  map[string]any{"type": "integer", "default": float64(8080)},
					},
					"required": []any{"name", "image"},
				},
			},
		},
	}

	structural, err := OpenAPIV3ToStructural(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	containersProp, ok := structural.Properties["containers"]
	if !ok {
		t.Fatal("expected 'containers' property")
	}
	if containersProp.Type != "array" {
		t.Fatalf("expected containers type=array, got %s", containersProp.Type)
	}
	if containersProp.Items == nil {
		t.Fatal("expected items schema on containers")
	}
	if _, ok := containersProp.Items.Properties["name"]; !ok {
		t.Fatal("expected 'name' property in container item schema")
	}
	if _, ok := containersProp.Items.Properties["port"]; !ok {
		t.Fatal("expected 'port' property in container item schema")
	}
}

func TestOpenAPIV3ToStructural_BooleanAndNumber(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"enabled": map[string]any{
				"type":    "boolean",
				"default": true,
			},
			"threshold": map[string]any{
				"type":    "number",
				"default": 0.75,
				"minimum": float64(0),
				"maximum": float64(1),
			},
		},
	}

	structural, err := OpenAPIV3ToStructural(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	enabledProp, ok := structural.Properties["enabled"]
	if !ok {
		t.Fatal("expected 'enabled' property")
	}
	if enabledProp.Type != "boolean" {
		t.Fatalf("expected enabled type=boolean, got %s", enabledProp.Type)
	}

	thresholdProp, ok := structural.Properties["threshold"]
	if !ok {
		t.Fatal("expected 'threshold' property")
	}
	if thresholdProp.Type != "number" {
		t.Fatalf("expected threshold type=number, got %s", thresholdProp.Type)
	}
}

func TestOpenAPIV3ToStructural_PatternAndFormat(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"email": map[string]any{
				"type":   "string",
				"format": "email",
			},
			"version": map[string]any{
				"type":    "string",
				"pattern": `^v\d+\.\d+\.\d+$`,
				"default": "v1.0.0",
			},
		},
	}

	structural, err := OpenAPIV3ToStructural(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := structural.Properties["email"]; !ok {
		t.Fatal("expected 'email' property")
	}
	if _, ok := structural.Properties["version"]; !ok {
		t.Fatal("expected 'version' property")
	}
}

func TestOpenAPIV3ToStructural_NestedDefaultsThroughRefs(t *testing.T) {
	// Each nested level needs its own default:{} for K8s defaulting to create it.
	schema := map[string]any{
		"type": "object",
		"$defs": map[string]any{
			"ResourceQuantity": map[string]any{
				"type":    "object",
				"default": map[string]any{},
				"properties": map[string]any{
					"cpu":    map[string]any{"type": "string", "default": "100m"},
					"memory": map[string]any{"type": "string", "default": "256Mi"},
				},
			},
			"ResourceRequirements": map[string]any{
				"type":    "object",
				"default": map[string]any{},
				"properties": map[string]any{
					"requests": map[string]any{"$ref": "#/$defs/ResourceQuantity"},
					"limits":   map[string]any{"$ref": "#/$defs/ResourceQuantity"},
				},
			},
		},
		"properties": map[string]any{
			"resources": map[string]any{"$ref": "#/$defs/ResourceRequirements"},
		},
	}

	structural, err := OpenAPIV3ToStructural(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Apply defaults and verify nested defaults through $ref resolution
	values := map[string]any{}
	result := ApplyDefaults(values, structural)

	resources, ok := result["resources"].(map[string]any)
	if !ok {
		t.Fatalf("expected resources to be map, got %T", result["resources"])
	}

	// Each level has default:{}, so the nested tree gets created
	requests, ok := resources["requests"].(map[string]any)
	if !ok {
		t.Fatalf("expected resources.requests to be map, got %T", resources["requests"])
	}
	if requests["cpu"] != "100m" {
		t.Errorf("expected resources.requests.cpu=100m, got %v", requests["cpu"])
	}
	if requests["memory"] != "256Mi" {
		t.Errorf("expected resources.requests.memory=256Mi, got %v", requests["memory"])
	}
}

func TestOpenAPIV3_DefaultsApplied(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"replicas": map[string]any{
				"type":    "integer",
				"default": float64(3),
			},
			"name": map[string]any{
				"type": "string",
			},
		},
	}

	structural, err := OpenAPIV3ToStructural(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Apply defaults to empty map
	values := map[string]any{}
	result := ApplyDefaults(values, structural)

	if result["replicas"] != int64(3) {
		t.Fatalf("expected replicas default=3, got %v (type: %T)", result["replicas"], result["replicas"])
	}
}

func TestOpenAPIV3ToResolvedSchema_PreservesVendorExtensions(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"replicas": map[string]any{
				"type":    "integer",
				"default": float64(1),
				"x-openchoreo-backstage-portal": map[string]any{
					"ui:field": "RepoUrlPicker",
					"ui:options": map[string]any{
						"allowedHosts": []any{"github.com"},
					},
				},
			},
			"imagePullPolicy": map[string]any{
				"type":    "string",
				"default": "IfNotPresent",
				"x-openchoreo-pull-portal": map[string]any{
					"ui:field": "RepoUrlPicker",
				},
			},
		},
	}

	resolved, err := OpenAPIV3ToResolvedSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	props := resolved["properties"].(map[string]any)

	// Verify x-openchoreo-backstage-portal is preserved on replicas
	replicas := props["replicas"].(map[string]any)
	ext, ok := replicas["x-openchoreo-backstage-portal"].(map[string]any)
	if !ok {
		t.Fatal("expected x-openchoreo-backstage-portal on replicas")
	}
	if ext["ui:field"] != "RepoUrlPicker" {
		t.Fatalf("expected ui:field=RepoUrlPicker, got %v", ext["ui:field"])
	}

	// Verify x-openchoreo-pull-portal is preserved on imagePullPolicy
	ipp := props["imagePullPolicy"].(map[string]any)
	ext2, ok := ipp["x-openchoreo-pull-portal"].(map[string]any)
	if !ok {
		t.Fatal("expected x-openchoreo-pull-portal on imagePullPolicy")
	}
	if ext2["ui:field"] != "RepoUrlPicker" {
		t.Fatalf("expected ui:field=RepoUrlPicker, got %v", ext2["ui:field"])
	}

	// Standard fields should also be present
	if replicas["type"] != typeInteger {
		t.Fatalf("expected type=integer, got %v", replicas["type"])
	}
}

func TestOpenAPIV3ToResolvedSchema_VendorExtensionsWithRefSiblings(t *testing.T) {
	// Tests the real-world case: $ref alongside x-* extensions (sibling keys)
	schema := map[string]any{
		"type": "object",
		"$defs": map[string]any{
			"ResourceRequirements": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cpu":    map[string]any{"type": "string", "default": "100m"},
					"memory": map[string]any{"type": "string", "default": "256Mi"},
				},
				"default": map[string]any{},
			},
		},
		"properties": map[string]any{
			"resources": map[string]any{
				"$ref": "#/$defs/ResourceRequirements",
				"x-openchoreo-resources-portal": map[string]any{
					"ui:field": "ResourcePicker",
				},
			},
		},
	}

	resolved, err := OpenAPIV3ToResolvedSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// $defs should be removed
	if _, ok := resolved["$defs"]; ok {
		t.Fatal("expected $defs to be removed after resolution")
	}

	props := resolved["properties"].(map[string]any)
	resources := props["resources"].(map[string]any)

	// $ref should be resolved — type and properties from the definition
	if resources["type"] != typeObject {
		t.Fatalf("expected type=object from resolved $ref, got %v", resources["type"])
	}
	resProp := resources["properties"].(map[string]any)
	if _, ok := resProp["cpu"]; !ok {
		t.Fatal("expected 'cpu' property from resolved $ref")
	}

	// x-* sibling should be preserved after $ref resolution
	ext, ok := resources["x-openchoreo-resources-portal"].(map[string]any)
	if !ok {
		t.Fatal("expected x-openchoreo-resources-portal preserved as $ref sibling")
	}
	if ext["ui:field"] != "ResourcePicker" {
		t.Fatalf("expected ui:field=ResourcePicker, got %v", ext["ui:field"])
	}
}

func TestOpenAPIV3ToResolvedSchema_NestedVendorExtensions(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"database": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"host": map[string]any{
						"type":            "string",
						"x-openchoreo-ui": map[string]any{"widget": "text"},
					},
				},
				"x-openchoreo-section": "advanced",
			},
		},
		"x-openchoreo-form-layout": "tabs",
	}

	resolved, err := OpenAPIV3ToResolvedSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Top-level extension
	if resolved["x-openchoreo-form-layout"] != "tabs" {
		t.Fatalf("expected top-level x-openchoreo-form-layout=tabs, got %v", resolved["x-openchoreo-form-layout"])
	}

	// Nested object-level extension
	props := resolved["properties"].(map[string]any)
	db := props["database"].(map[string]any)
	if db["x-openchoreo-section"] != "advanced" {
		t.Fatalf("expected x-openchoreo-section=advanced, got %v", db["x-openchoreo-section"])
	}

	// Deeply nested property-level extension
	dbProps := db["properties"].(map[string]any)
	host := dbProps["host"].(map[string]any)
	hostExt, ok := host["x-openchoreo-ui"].(map[string]any)
	if !ok {
		t.Fatal("expected x-openchoreo-ui on host")
	}
	if hostExt["widget"] != "text" {
		t.Fatalf("expected widget=text, got %v", hostExt["widget"])
	}
}

func TestOpenAPIV3ToResolvedSchema_DoesNotMutateInput(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"$defs": map[string]any{
			"Port": map[string]any{"type": "integer", "default": float64(8080)},
		},
		"properties": map[string]any{
			"port": map[string]any{
				"$ref":            "#/$defs/Port",
				"x-openchoreo-ui": "port-picker",
			},
		},
	}

	_, err := OpenAPIV3ToResolvedSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Original should still have $defs and $ref
	if _, ok := schema["$defs"]; !ok {
		t.Fatal("input schema was mutated: $defs removed")
	}
	port := schema["properties"].(map[string]any)["port"].(map[string]any)
	if _, ok := port["$ref"]; !ok {
		t.Fatal("input schema was mutated: $ref removed")
	}
}

func TestOpenAPIV3ToStructural_RequiredFieldValidation(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":  map[string]any{"type": "string"},
			"image": map[string]any{"type": "string"},
			"port":  map[string]any{"type": "integer", "default": float64(8080)},
		},
		"required": []any{"name", "image"},
	}

	structural, err := OpenAPIV3ToStructural(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structural == nil {
		t.Fatal("expected non-nil structural schema")
	}

	// Structural schema should have the required fields tracked via ValueValidation
	if structural.ValueValidation == nil {
		t.Fatal("expected ValueValidation to be set for required fields")
	}
	requiredFields := structural.ValueValidation.Required
	if len(requiredFields) != 2 {
		t.Fatalf("expected 2 required fields, got %d", len(requiredFields))
	}

	// Verify both required fields are present
	requiredMap := map[string]bool{}
	for _, r := range requiredFields {
		requiredMap[r] = true
	}
	if !requiredMap["name"] {
		t.Fatal("expected 'name' in required fields")
	}
	if !requiredMap["image"] {
		t.Fatal("expected 'image' in required fields")
	}
}

func TestOpenAPIV3ToResolvedSchema_DeeplyNestedVendorExtensions(t *testing.T) {
	// Vendor extensions nested 3+ levels deep should all be preserved
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"config": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"database": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"host": map[string]any{
								"type": "string",
								"x-level3-extension": map[string]any{
									"widget": "text",
									"nested": map[string]any{
										"x-level4-marker": true,
									},
								},
							},
						},
						"x-level2-extension": "database-section",
					},
				},
				"x-level1-extension": "config-section",
			},
		},
		"x-top-level": "root",
	}

	resolved, err := OpenAPIV3ToResolvedSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Top level
	if resolved["x-top-level"] != "root" {
		t.Fatalf("expected x-top-level=root, got %v", resolved["x-top-level"])
	}

	// Level 1
	props := resolved["properties"].(map[string]any)
	config := props["config"].(map[string]any)
	if config["x-level1-extension"] != "config-section" {
		t.Fatalf("expected x-level1-extension=config-section, got %v", config["x-level1-extension"])
	}

	// Level 2
	configProps := config["properties"].(map[string]any)
	db := configProps["database"].(map[string]any)
	if db["x-level2-extension"] != "database-section" {
		t.Fatalf("expected x-level2-extension=database-section, got %v", db["x-level2-extension"])
	}

	// Level 3
	dbProps := db["properties"].(map[string]any)
	host := dbProps["host"].(map[string]any)
	ext, ok := host["x-level3-extension"].(map[string]any)
	if !ok {
		t.Fatal("expected x-level3-extension on host")
	}
	if ext["widget"] != "text" {
		t.Fatalf("expected widget=text at level 3, got %v", ext["widget"])
	}
}

func TestOpenAPIV3ToStructural_OnlyAdditionalProperties(t *testing.T) {
	// Schema with additionalProperties but no properties — a pure map type
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": map[string]any{"type": "string"},
	}

	structural, err := OpenAPIV3ToStructural(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structural == nil {
		t.Fatal("expected non-nil structural schema")
	}
	if structural.Type != typeObject {
		t.Fatalf("expected type=object, got %s", structural.Type)
	}
	if structural.AdditionalProperties == nil || structural.AdditionalProperties.Structural == nil {
		t.Fatal("expected additionalProperties to be set")
	}
	if structural.AdditionalProperties.Structural.Type != typeString {
		t.Fatalf("expected additionalProperties type=string, got %s",
			structural.AdditionalProperties.Structural.Type)
	}
}

func TestOpenAPIV3ToJSONSchema_DescriptionPreserved(t *testing.T) {
	schema := map[string]any{
		"type":        "object",
		"description": "Top-level schema description",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "The name of the component",
			},
			"port": map[string]any{
				"type":        "integer",
				"description": "The port number to listen on",
			},
		},
	}

	jsonSchema, err := OpenAPIV3ToJSONSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if jsonSchema.Description != "Top-level schema description" {
		t.Fatalf("expected top-level description preserved, got %q", jsonSchema.Description)
	}

	nameProp := jsonSchema.Properties["name"]
	if nameProp.Description != "The name of the component" {
		t.Fatalf("expected name description preserved, got %q", nameProp.Description)
	}

	portProp := jsonSchema.Properties["port"]
	if portProp.Description != "The port number to listen on" {
		t.Fatalf("expected port description preserved, got %q", portProp.Description)
	}
}

func TestOpenAPIV3ToResolvedSchema_NoVendorExtensions(t *testing.T) {
	// Schema with no vendor extensions should pass through unchanged (minus $defs)
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":    "string",
				"default": "hello",
			},
			"count": map[string]any{
				"type":    "integer",
				"minimum": float64(0),
			},
		},
		"required": []any{"name"},
	}

	resolved, err := OpenAPIV3ToResolvedSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved["type"] != typeObject {
		t.Fatalf("expected type=object, got %v", resolved["type"])
	}

	props := resolved["properties"].(map[string]any)
	name := props["name"].(map[string]any)
	if name["type"] != typeString {
		t.Fatalf("expected name type=string, got %v", name["type"])
	}
	if name["default"] != "hello" {
		t.Fatalf("expected name default=hello, got %v", name["default"])
	}

	req := resolved["required"].([]any)
	if len(req) != 1 || req[0] != "name" {
		t.Fatalf("expected required=[name], got %v", req)
	}
}

func TestOpenAPIV3ToJSONSchema_EmptyDefs(t *testing.T) {
	// Schema with an empty $defs section should not cause errors
	schema := map[string]any{
		"type":  "object",
		"$defs": map[string]any{},
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}

	jsonSchema, err := OpenAPIV3ToJSONSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jsonSchema == nil {
		t.Fatal("expected non-nil JSON schema")
	}
	if _, ok := jsonSchema.Properties["name"]; !ok {
		t.Fatal("expected 'name' property")
	}
}

func TestOpenAPIV3ToStructural_FormatField(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"email": map[string]any{
				"type":   "string",
				"format": "email",
			},
			"created": map[string]any{
				"type":   "string",
				"format": "date-time",
			},
			"age": map[string]any{
				"type":   "integer",
				"format": "int32",
			},
		},
	}

	structural, err := OpenAPIV3ToStructural(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structural == nil {
		t.Fatal("expected non-nil structural schema")
	}

	emailProp := structural.Properties["email"]
	if emailProp.Type != typeString {
		t.Fatalf("expected email type=string, got %s", emailProp.Type)
	}

	createdProp := structural.Properties["created"]
	if createdProp.Type != typeString {
		t.Fatalf("expected created type=string, got %s", createdProp.Type)
	}

	ageProp := structural.Properties["age"]
	if ageProp.Type != typeInteger {
		t.Fatalf("expected age type=integer, got %s", ageProp.Type)
	}
}

func TestStripVendorExtensions_NestedInArray(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"allOf": []any{
			map[string]any{
				"type":       "object",
				"x-internal": true,
				"properties": map[string]any{
					"a": map[string]any{
						"type":     "string",
						"x-hidden": true,
					},
				},
			},
		},
	}

	result := stripVendorExtensions(schema)

	allOf := result["allOf"].([]any)
	first := allOf[0].(map[string]any)
	if _, ok := first["x-internal"]; ok {
		t.Fatal("expected x-internal to be stripped from allOf element")
	}
	if first["type"] != typeObject {
		t.Fatal("expected type to be preserved in allOf element")
	}

	props := first["properties"].(map[string]any)
	a := props["a"].(map[string]any)
	if _, ok := a["x-hidden"]; ok {
		t.Fatal("expected x-hidden to be stripped from nested property in array")
	}
}

func TestOpenAPIV3ToStructuralAndJSONSchema_WithVendorExtensions(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Component name",
				"x-ui-hint":   "Enter the component name",
			},
		},
	}

	structural, jsonSchema, err := OpenAPIV3ToStructuralAndJSONSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Structural should work (x-* stripped internally)
	if structural == nil {
		t.Fatal("expected non-nil structural schema")
	}
	if _, ok := structural.Properties["name"]; !ok {
		t.Fatal("structural: expected 'name' property")
	}

	// JSON schema should preserve standard fields
	if jsonSchema == nil {
		t.Fatal("expected non-nil JSON schema")
	}
	nameProp := jsonSchema.Properties["name"]
	if nameProp.Description != "Component name" {
		t.Fatalf("expected description preserved in JSON schema, got %q", nameProp.Description)
	}
}

func TestStripVendorExtensions_PreservesXKubernetes(t *testing.T) {
	schema := map[string]any{
		"type":                                 "object",
		"x-ui-widget":                          "form",
		"x-kubernetes-preserve-unknown-fields": true,
		"properties": map[string]any{
			"data": map[string]any{
				"type":                  "object",
				"x-openchoreo-ui":       "custom",
				"x-kubernetes-map-type": "granular",
				"additionalProperties":  map[string]any{"type": "string"},
			},
			"items": map[string]any{
				"type":                       "array",
				"x-kubernetes-list-type":     "map",
				"x-kubernetes-list-map-keys": []any{"name"},
				"x-custom-display":           "hidden",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{"type": "string"},
					},
				},
			},
		},
	}

	result := stripVendorExtensions(schema)

	// Custom vendor extensions should be stripped
	if _, ok := result["x-ui-widget"]; ok {
		t.Fatal("expected x-ui-widget to be stripped")
	}

	// x-kubernetes-* should be preserved at top level
	if v, ok := result["x-kubernetes-preserve-unknown-fields"]; !ok || v != true {
		t.Fatal("expected x-kubernetes-preserve-unknown-fields to be preserved")
	}

	props := result["properties"].(map[string]any)

	// Nested: custom stripped, x-kubernetes-* preserved
	data := props["data"].(map[string]any)
	if _, ok := data["x-openchoreo-ui"]; ok {
		t.Fatal("expected x-openchoreo-ui to be stripped")
	}
	if v, ok := data["x-kubernetes-map-type"]; !ok || v != "granular" {
		t.Fatal("expected x-kubernetes-map-type to be preserved")
	}

	items := props["items"].(map[string]any)
	if _, ok := items["x-custom-display"]; ok {
		t.Fatal("expected x-custom-display to be stripped")
	}
	if v, ok := items["x-kubernetes-list-type"]; !ok || v != "map" {
		t.Fatal("expected x-kubernetes-list-type to be preserved")
	}
	if _, ok := items["x-kubernetes-list-map-keys"]; !ok {
		t.Fatal("expected x-kubernetes-list-map-keys to be preserved")
	}
}

func TestOpenAPIV3ToStructural_XKubernetesPreserveUnknownFields(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"metadata": map[string]any{
				"type":                                 "object",
				"x-kubernetes-preserve-unknown-fields": true,
			},
			"items": map[string]any{
				"type":                       "array",
				"x-kubernetes-list-type":     "map",
				"x-kubernetes-list-map-keys": []any{"name"},
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{"type": "string"},
					},
				},
			},
			"labels": map[string]any{
				"type":                  "object",
				"x-kubernetes-map-type": "granular",
				"additionalProperties":  map[string]any{"type": "string"},
			},
		},
	}

	structural, err := OpenAPIV3ToStructural(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// x-kubernetes-preserve-unknown-fields should survive into structural schema
	metaProp := structural.Properties["metadata"]
	if metaProp.XPreserveUnknownFields != true {
		t.Fatal("expected x-kubernetes-preserve-unknown-fields=true on metadata")
	}

	// x-kubernetes-list-type should survive
	itemsProp := structural.Properties["items"]
	if itemsProp.XListType == nil || *itemsProp.XListType != "map" {
		t.Fatal("expected x-kubernetes-list-type=map on items")
	}
	if len(itemsProp.XListMapKeys) != 1 || itemsProp.XListMapKeys[0] != "name" {
		t.Fatalf("expected x-kubernetes-list-map-keys=[name], got %v", itemsProp.XListMapKeys)
	}

	// x-kubernetes-map-type should survive
	labelsProp := structural.Properties["labels"]
	if labelsProp.XMapType == nil || *labelsProp.XMapType != "granular" {
		t.Fatal("expected x-kubernetes-map-type=granular on labels")
	}
}

func TestOpenAPIV3ToJSONSchema_RequiredFieldsSorted(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"z_field": map[string]any{"type": "string"},
			"a_field": map[string]any{"type": "string"},
			"m_field": map[string]any{"type": "string"},
		},
		"required": []any{"z_field", "a_field", "m_field"},
	}

	jsonSchema, err := OpenAPIV3ToJSONSchema(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Required fields should be sorted for deterministic output
	if len(jsonSchema.Required) != 3 {
		t.Fatalf("expected 3 required fields, got %d", len(jsonSchema.Required))
	}
	if jsonSchema.Required[0] != "a_field" || jsonSchema.Required[1] != "m_field" || jsonSchema.Required[2] != "z_field" {
		t.Fatalf("expected sorted required=[a_field, m_field, z_field], got %v", jsonSchema.Required)
	}
}
