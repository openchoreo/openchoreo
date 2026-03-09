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
	if structural.Type != "object" {
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
	if portProp.Type != "integer" {
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
	if jsonSchema.Type != "object" {
		t.Fatalf("expected type=object, got %s", jsonSchema.Type)
	}

	nameProp, ok := jsonSchema.Properties["name"]
	if !ok {
		t.Fatal("expected 'name' property")
	}
	if nameProp.Type != "string" {
		t.Fatalf("expected name type=string, got %s", nameProp.Type)
	}
	if len(jsonSchema.Required) != 1 || jsonSchema.Required[0] != "name" {
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

	// Structural path should work even with vendor extensions in input
	structural, err := OpenAPIV3ToStructural(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structural == nil {
		t.Fatal("expected non-nil structural schema")
	}
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
	if dbProp.Type != "object" {
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
	if envProp.Type != "string" {
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
	if labelsProp.Type != "object" {
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
	if replicas["type"] != "integer" {
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
	if resources["type"] != "object" {
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
						"type": "string",
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
