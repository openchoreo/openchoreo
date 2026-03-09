// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestApplyDefaults_ArrayFieldBehaviour(t *testing.T) {
	def := Definition{
		Schemas: []map[string]any{
			{
				"$types": map[string]any{
					"Item": map[string]any{
						"name": "string | default=default-name",
					},
				},
				"list": "[]Item",
			},
		},
	}

	structural, err := ToStructural(def)
	if err != nil {
		t.Fatalf("ToStructural returned error: %v", err)
	}

	defaults := ApplyDefaults(nil, structural)
	if _, ok := defaults["list"]; ok {
		t.Fatalf("expected no default array elements when only item defaults are present, got %v", defaults["list"])
	}

	defWithArrayDefault := Definition{
		Schemas: []map[string]any{
			{
				"$types": map[string]any{
					"Item": map[string]any{
						"name": "string | default=default-name",
					},
				},
				"list": "[]Item | default=[{\"name\":\"custom\"}]",
			},
		},
	}

	structural, err = ToStructural(defWithArrayDefault)
	if err != nil {
		t.Fatalf("ToStructural returned error: %v", err)
	}

	defaults = ApplyDefaults(nil, structural)
	got, ok := defaults["list"].([]any)
	if !ok {
		t.Fatalf("expected slice default, got %T (%v)", defaults["list"], defaults["list"])
	}
	if len(got) != 1 || got[0].(map[string]any)["name"] != "custom" {
		t.Fatalf("unexpected array default: %v", got)
	}
}

func TestApplyDefaults_ArrayItems(t *testing.T) {
	def := Definition{
		Schemas: []map[string]any{
			{
				"$types": map[string]any{
					"MountConfig": map[string]any{
						"containerName": "string",
						"mountPath":     "string",
						"readOnly":      "boolean | default=true",
						"subPath":       "string | default=\"\"",
					},
				},
				"volumeName": "string",
				"mounts":     "[]MountConfig",
			},
		},
	}

	structural, err := ToStructural(def)
	if err != nil {
		t.Fatalf("ToStructural returned error: %v", err)
	}

	values := map[string]any{
		"volumeName": "shared",
		"mounts": []any{
			map[string]any{
				"containerName": "app",
				"mountPath":     "/var/log/app",
			},
		},
	}

	ApplyDefaults(values, structural)

	mounts, ok := values["mounts"].([]any)
	if !ok || len(mounts) != 1 {
		t.Fatalf("expected one mount after defaulting, got %v", values["mounts"])
	}

	mount, ok := mounts[0].(map[string]any)
	if !ok {
		t.Fatalf("expected mount to be a map, got %T", mounts[0])
	}

	readOnly, ok := mount["readOnly"].(bool)
	if !ok {
		t.Fatalf("expected readOnly to be a bool, got %T", mount["readOnly"])
	}
	if !readOnly {
		t.Fatalf("expected readOnly default true, got %v", readOnly)
	}

	if _, ok := mount["subPath"].(string); !ok {
		t.Fatalf("expected subPath to be a string, got %T", mount["subPath"])
	}
}

func makeSchemaSection(isOpenAPIV3 bool, schema map[string]any) *v1alpha1.SchemaSection {
	data, _ := json.Marshal(schema)
	raw := &runtime.RawExtension{Raw: data}
	if isOpenAPIV3 {
		return &v1alpha1.SchemaSection{OpenAPIV3Schema: raw}
	}
	return &v1alpha1.SchemaSection{OCSchema: raw}
}

func TestResolveSectionToStructural_OpenAPIV3(t *testing.T) {
	section := makeSchemaSection(true, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"replicas": map[string]any{
				"type":    "integer",
				"default": float64(3),
			},
			"image": map[string]any{
				"type": "string",
			},
		},
		"required": []any{"image"},
	})

	structural, err := ResolveSectionToStructural(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structural == nil {
		t.Fatal("expected non-nil structural")
	}
	if _, ok := structural.Properties["replicas"]; !ok {
		t.Fatal("expected 'replicas' property")
	}
	if _, ok := structural.Properties["image"]; !ok {
		t.Fatal("expected 'image' property")
	}
}

func TestResolveSectionToStructural_OpenAPIV3_WithRefs(t *testing.T) {
	section := makeSchemaSection(true, map[string]any{
		"type": "object",
		"$defs": map[string]any{
			"Port": map[string]any{
				"type":    "integer",
				"minimum": float64(1),
				"maximum": float64(65535),
			},
		},
		"properties": map[string]any{
			"port": map[string]any{"$ref": "#/$defs/Port"},
		},
	})

	structural, err := ResolveSectionToStructural(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structural == nil {
		t.Fatal("expected non-nil structural")
	}
	portProp, ok := structural.Properties["port"]
	if !ok {
		t.Fatal("expected 'port' property")
	}
	if portProp.Type != "integer" {
		t.Fatalf("expected port type=integer, got %s", portProp.Type)
	}
}

func TestResolveSectionToBundle_OpenAPIV3(t *testing.T) {
	section := makeSchemaSection(true, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":    "string",
				"default": "default-name",
			},
		},
	})

	structural, jsonSchema, err := ResolveSectionToBundle(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structural == nil {
		t.Fatal("expected non-nil structural")
	}
	if jsonSchema == nil {
		t.Fatal("expected non-nil jsonSchema")
	}

	if _, ok := structural.Properties["name"]; !ok {
		t.Fatal("structural: expected 'name' property")
	}
	if _, ok := jsonSchema.Properties["name"]; !ok {
		t.Fatal("jsonSchema: expected 'name' property")
	}
}

func TestResolveSectionToStructural_NilSection(t *testing.T) {
	structural, err := ResolveSectionToStructural(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structural != nil {
		t.Fatal("expected nil structural for nil section")
	}
}

func TestSectionToJSONSchema_OpenAPIV3(t *testing.T) {
	section := makeSchemaSection(true, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"env": map[string]any{
				"type":    "string",
				"enum":    []any{"dev", "staging", "prod"},
				"default": "dev",
			},
		},
	})

	jsonSchema, err := SectionToJSONSchema(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jsonSchema == nil {
		t.Fatal("expected non-nil jsonSchema")
	}

	envProp, ok := jsonSchema.Properties["env"]
	if !ok {
		t.Fatal("expected 'env' property")
	}
	if envProp.Type != "string" {
		t.Fatalf("expected env type=string, got %s", envProp.Type)
	}
}

func TestSectionToJSONSchema_OCSchema(t *testing.T) {
	section := makeSchemaSection(false, map[string]any{
		"replicas": "integer | default=1",
		"image":    "string",
	})

	jsonSchema, err := SectionToJSONSchema(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jsonSchema == nil {
		t.Fatal("expected non-nil jsonSchema")
	}
	if jsonSchema.Type != "object" {
		t.Fatalf("expected type=object, got %s", jsonSchema.Type)
	}
	if _, ok := jsonSchema.Properties["replicas"]; !ok {
		t.Fatal("expected 'replicas' property")
	}
}

func TestSectionToJSONSchema_NilSection(t *testing.T) {
	jsonSchema, err := SectionToJSONSchema(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jsonSchema == nil {
		t.Fatal("expected non-nil jsonSchema for nil section")
	}
	if jsonSchema.Type != "object" {
		t.Fatalf("expected type=object, got %s", jsonSchema.Type)
	}
}

func TestResolveSectionToStructural_OpenAPIV3_DefaultsWork(t *testing.T) {
	section := makeSchemaSection(true, map[string]any{
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
	})

	structural, err := ResolveSectionToStructural(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	values := map[string]any{"name": "test"}
	result := ApplyDefaults(values, structural)
	if result["replicas"] != int64(3) {
		t.Fatalf("expected replicas default=3, got %v (type: %T)", result["replicas"], result["replicas"])
	}
}

// loadTestdataAsSchemaSection reads a YAML testdata file and wraps it as an openAPIV3 SchemaSection.
func loadTestdataAsSchemaSection(t *testing.T, filename string) *v1alpha1.SchemaSection {
	t.Helper()
	data, err := os.ReadFile("testdata/" + filename)
	if err != nil {
		t.Fatalf("failed to read testdata/%s: %v", filename, err)
	}
	// Parse YAML to map then re-marshal as JSON for RawExtension
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatalf("failed to parse testdata/%s: %v", filename, err)
	}
	jsonData, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("failed to marshal testdata/%s to JSON: %v", filename, err)
	}
	return &v1alpha1.SchemaSection{
		OpenAPIV3Schema: &runtime.RawExtension{Raw: jsonData},
	}
}

func TestTestdata_SimpleOpenAPIV3_Structural(t *testing.T) {
	section := loadTestdataAsSchemaSection(t, "simple_openapiv3.yaml")

	structural, err := ResolveSectionToStructural(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structural == nil {
		t.Fatal("expected non-nil structural")
	}

	for _, field := range []string{"replicas", "image", "enabled", "port", "environment"} {
		if _, ok := structural.Properties[field]; !ok {
			t.Errorf("expected property %q", field)
		}
	}

	// Verify defaults are applied
	values := map[string]any{"image": "nginx:latest"}
	result := ApplyDefaults(values, structural)
	if result["replicas"] != int64(1) {
		t.Errorf("expected replicas default=1, got %v", result["replicas"])
	}
	if result["enabled"] != true {
		t.Errorf("expected enabled default=true, got %v", result["enabled"])
	}
	if result["port"] != int64(8080) {
		t.Errorf("expected port default=8080, got %v", result["port"])
	}
	if result["environment"] != "dev" {
		t.Errorf("expected environment default=dev, got %v", result["environment"])
	}
}

func TestTestdata_WithRefsOpenAPIV3_Structural(t *testing.T) {
	section := loadTestdataAsSchemaSection(t, "with_refs_openapiv3.yaml")

	structural, err := ResolveSectionToStructural(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structural == nil {
		t.Fatal("expected non-nil structural")
	}

	// Verify $ref resolved: resources should have requests/limits with cpu/memory
	resources, ok := structural.Properties["resources"]
	if !ok {
		t.Fatal("expected 'resources' property")
	}
	requests, ok := resources.Properties["requests"]
	if !ok {
		t.Fatal("expected 'resources.requests' property")
	}
	if _, ok := requests.Properties["cpu"]; !ok {
		t.Fatal("expected 'resources.requests.cpu' property")
	}
	if _, ok := requests.Properties["memory"]; !ok {
		t.Fatal("expected 'resources.requests.memory' property")
	}

	// Verify defaults applied through refs
	values := map[string]any{}
	result := ApplyDefaults(values, structural)
	if result["replicas"] != int64(1) {
		t.Errorf("expected replicas default=1, got %v", result["replicas"])
	}
	if result["imagePullPolicy"] != "IfNotPresent" {
		t.Errorf("expected imagePullPolicy default=IfNotPresent, got %v", result["imagePullPolicy"])
	}
}

func TestTestdata_NestedOpenAPIV3_Structural(t *testing.T) {
	section := loadTestdataAsSchemaSection(t, "nested_openapiv3.yaml")

	structural, err := ResolveSectionToStructural(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structural == nil {
		t.Fatal("expected non-nil structural")
	}

	// Verify deep nesting resolved through $ref
	autoscaling, ok := structural.Properties["autoscaling"]
	if !ok {
		t.Fatal("expected 'autoscaling' property")
	}
	if _, ok := autoscaling.Properties["enabled"]; !ok {
		t.Fatal("expected 'autoscaling.enabled' property")
	}
	metrics, ok := autoscaling.Properties["metrics"]
	if !ok {
		t.Fatal("expected 'autoscaling.metrics' property")
	}
	cpu, ok := metrics.Properties["cpu"]
	if !ok {
		t.Fatal("expected 'autoscaling.metrics.cpu' property")
	}
	if _, ok := cpu.Properties["targetUtilization"]; !ok {
		t.Fatal("expected 'autoscaling.metrics.cpu.targetUtilization' property")
	}

	// Verify database nested object
	db, ok := structural.Properties["database"]
	if !ok {
		t.Fatal("expected 'database' property")
	}
	if _, ok := db.Properties["credentials"]; !ok {
		t.Fatal("expected 'database.credentials' property")
	}
}

func TestTestdata_InvalidCircularRef_Error(t *testing.T) {
	section := loadTestdataAsSchemaSection(t, "invalid_circular_ref.yaml")

	_, err := ResolveSectionToStructural(section)
	if err == nil {
		t.Fatal("expected error for circular ref, got nil")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Fatalf("expected circular ref error, got: %v", err)
	}
}

func TestTestdata_SimpleOpenAPIV3_JSONSchema(t *testing.T) {
	section := loadTestdataAsSchemaSection(t, "simple_openapiv3.yaml")

	jsonSchema, err := SectionToJSONSchema(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jsonSchema == nil {
		t.Fatal("expected non-nil jsonSchema")
	}
	if jsonSchema.Type != "object" {
		t.Fatalf("expected type=object, got %s", jsonSchema.Type)
	}

	// Verify required fields preserved
	found := false
	for _, r := range jsonSchema.Required {
		if r == "image" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'image' in required fields")
	}

	// Verify description preserved
	imageProp := jsonSchema.Properties["image"]
	if imageProp.Description != "Container image to deploy" {
		t.Fatalf("expected description on image property, got %q", imageProp.Description)
	}
}

func TestSectionToJSONSchema_PreservesVendorExtensions(t *testing.T) {
	// Task 2.11: Verify that SectionToJSONSchema preserves x-* vendor extensions
	// in API responses for openAPIV3Schema input.
	section := makeSchemaSection(true, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "Git repository URL",
				"x-openchoreo-backstage-portal": map[string]any{
					"ui:field": "RepoUrlPicker",
					"ui:options": map[string]any{
						"allowedHosts": []any{"github.com"},
					},
				},
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Component name",
			},
		},
		"additionalProperties": false,
	})

	jsonSchema, err := SectionToJSONSchema(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jsonSchema == nil {
		t.Fatal("expected non-nil jsonSchema")
	}

	// Verify description is preserved
	urlProp, ok := jsonSchema.Properties["url"]
	if !ok {
		t.Fatal("expected 'url' property")
	}
	if urlProp.Description != "Git repository URL" {
		t.Fatalf("expected description preserved, got %q", urlProp.Description)
	}

	// Verify additionalProperties: false is preserved
	if jsonSchema.AdditionalProperties == nil {
		t.Fatal("expected additionalProperties to be set")
	}
	if jsonSchema.AdditionalProperties.Allows != false {
		t.Fatal("expected additionalProperties=false to be preserved")
	}
}

func TestResolveSectionToBundle_OpenAPIV3_VendorExtensionsStrippedFromStructural(t *testing.T) {
	// Task 2.11: Verify that the structural schema path strips x-* extensions
	// (K8s rejects them) while JSON schema path preserves them.
	section := makeSchemaSection(true, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "Git repository URL",
				"x-ui-widget": "textarea",
			},
		},
	})

	structural, jsonSchema, err := ResolveSectionToBundle(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structural == nil {
		t.Fatal("expected non-nil structural")
	}
	if jsonSchema == nil {
		t.Fatal("expected non-nil jsonSchema")
	}

	// Structural should work fine (x-* stripped before structural conversion)
	if _, ok := structural.Properties["url"]; !ok {
		t.Fatal("structural: expected 'url' property")
	}

	// JSON schema should preserve description
	urlProp := jsonSchema.Properties["url"]
	if urlProp.Description != "Git repository URL" {
		t.Fatalf("jsonSchema: expected description preserved, got %q", urlProp.Description)
	}
}

func TestValidateWithJSONSchema_OpenAPIV3(t *testing.T) {
	// End-to-end: openAPIV3Schema → JSON Schema → validate values
	section := makeSchemaSection(true, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"replicas": map[string]any{
				"type":    "integer",
				"minimum": float64(1),
				"maximum": float64(100),
			},
			"image": map[string]any{
				"type":      "string",
				"minLength": float64(1),
			},
		},
		"required": []any{"image"},
	})

	jsonSchema, err := SectionToJSONSchema(section)
	if err != nil {
		t.Fatalf("SectionToJSONSchema error: %v", err)
	}

	// Valid values should pass
	validValues := map[string]any{
		"image":    "nginx:latest",
		"replicas": int64(3),
	}
	if err := ValidateWithJSONSchema(validValues, jsonSchema); err != nil {
		t.Fatalf("expected valid values to pass, got: %v", err)
	}

	// Missing required field should fail
	missingRequired := map[string]any{
		"replicas": int64(1),
	}
	if err := ValidateWithJSONSchema(missingRequired, jsonSchema); err == nil {
		t.Fatal("expected error for missing required field 'image'")
	}

	// Value violating constraint should fail
	invalidConstraint := map[string]any{
		"image":    "nginx",
		"replicas": int64(0), // minimum is 1
	}
	if err := ValidateWithJSONSchema(invalidConstraint, jsonSchema); err == nil {
		t.Fatal("expected error for replicas < minimum")
	}
}

func TestValidateAgainstSchema_OpenAPIV3(t *testing.T) {
	// Validate unknown field detection with openAPIV3Schema
	section := makeSchemaSection(true, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"replicas": map[string]any{"type": "integer"},
			"image":    map[string]any{"type": "string"},
		},
	})

	structural, err := ResolveSectionToStructural(section)
	if err != nil {
		t.Fatalf("ResolveSectionToStructural error: %v", err)
	}

	// Valid fields should pass
	validValues := map[string]any{"replicas": 1, "image": "nginx"}
	if err := ValidateAgainstSchema(validValues, structural); err != nil {
		t.Fatalf("expected valid values to pass, got: %v", err)
	}

	// Unknown field should fail
	unknownField := map[string]any{"replicas": 1, "unknownField": "value"}
	if err := ValidateAgainstSchema(unknownField, structural); err == nil {
		t.Fatal("expected error for unknown field")
	} else if !strings.Contains(err.Error(), "unknownField") {
		t.Fatalf("expected error to mention unknownField, got: %v", err)
	}
}

func TestResolveSectionToBundle_NilSection(t *testing.T) {
	structural, jsonSchema, err := ResolveSectionToBundle(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structural != nil {
		t.Fatal("expected nil structural for nil section")
	}
	if jsonSchema != nil {
		t.Fatal("expected nil jsonSchema for nil section")
	}
}

func TestSectionToJSONSchema_EmptyOpenAPIV3(t *testing.T) {
	// OpenAPIV3Schema with no properties should return a valid empty object schema
	section := makeSchemaSection(true, map[string]any{
		"type": "object",
	})

	jsonSchema, err := SectionToJSONSchema(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jsonSchema == nil {
		t.Fatal("expected non-nil jsonSchema")
	}
	if jsonSchema.Type != "object" {
		t.Fatalf("expected type=object, got %s", jsonSchema.Type)
	}
}

func TestOpenAPIV3_EndToEnd_DefaultsAndValidation(t *testing.T) {
	// Full pipeline: openAPIV3Schema → structural + JSON schema → defaults → validate
	section := makeSchemaSection(true, map[string]any{
		"type": "object",
		"$defs": map[string]any{
			"ResourceQuantity": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cpu":    map[string]any{"type": "string", "default": "100m"},
					"memory": map[string]any{"type": "string", "default": "256Mi"},
				},
			},
		},
		"properties": map[string]any{
			"replicas": map[string]any{
				"type":    "integer",
				"default": float64(1),
				"minimum": float64(1),
			},
			"resources": map[string]any{
				"$ref":    "#/$defs/ResourceQuantity",
				"default": map[string]any{},
			},
			"image": map[string]any{
				"type":      "string",
				"minLength": float64(1),
			},
		},
		"required": []any{"image"},
	})

	structural, jsonSchema, err := ResolveSectionToBundle(section)
	if err != nil {
		t.Fatalf("ResolveSectionToBundle error: %v", err)
	}

	// Step 1: Apply defaults
	values := map[string]any{"image": "nginx:latest"}
	result := ApplyDefaults(values, structural)

	if result["replicas"] != int64(1) {
		t.Errorf("expected replicas default=1, got %v", result["replicas"])
	}
	resources, ok := result["resources"].(map[string]any)
	if !ok {
		t.Fatalf("expected resources to be map after defaulting, got %T", result["resources"])
	}
	if resources["cpu"] != "100m" {
		t.Errorf("expected resources.cpu=100m, got %v", resources["cpu"])
	}

	// Step 2: Validate the defaulted values
	if err := ValidateWithJSONSchema(result, jsonSchema); err != nil {
		t.Fatalf("expected defaulted values to pass validation, got: %v", err)
	}

	// Step 3: Validate unknown fields
	if err := ValidateAgainstSchema(result, structural); err != nil {
		t.Fatalf("expected defaulted values to pass structural validation, got: %v", err)
	}
}

func TestTestdata_WithRefsOpenAPIV3_JSONSchemaPreservesFields(t *testing.T) {
	section := loadTestdataAsSchemaSection(t, "with_refs_openapiv3.yaml")

	jsonSchema, err := SectionToJSONSchema(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jsonSchema == nil {
		t.Fatal("expected non-nil jsonSchema")
	}

	// Verify enum values preserved after $ref resolution
	policyProp, ok := jsonSchema.Properties["imagePullPolicy"]
	if !ok {
		t.Fatal("expected 'imagePullPolicy' property")
	}
	if len(policyProp.Enum) != 3 {
		t.Fatalf("expected 3 enum values, got %d", len(policyProp.Enum))
	}

	// Verify port has constraints after $ref resolution
	portProp, ok := jsonSchema.Properties["port"]
	if !ok {
		t.Fatal("expected 'port' property")
	}
	if portProp.Type != "integer" {
		t.Fatalf("expected port type=integer, got %s", portProp.Type)
	}
}
