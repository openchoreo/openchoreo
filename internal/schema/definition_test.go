// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	repoURLPicker      = "RepoUrlPicker"
	defaultCPUQuantity = "100m"
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
	return &v1alpha1.SchemaSection{OpenAPIV3Schema: raw}
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
	if portProp.Type != typeInteger {
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
	if envProp.Type != typeString {
		t.Fatalf("expected env type=string, got %s", envProp.Type)
	}
}

func TestSectionToJSONSchema_ShorthandSchema(t *testing.T) {
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
	if jsonSchema.Type != typeObject {
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
	if jsonSchema.Type != typeObject {
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

	// Verify nested defaults applied through $ref (ResourceRequirements -> ResourceQuantity)
	resMap, ok := result["resources"].(map[string]any)
	if !ok {
		t.Fatal("expected 'resources' to be a map after defaulting")
	}
	limitsMap, ok := resMap["limits"].(map[string]any)
	if !ok {
		t.Fatal("expected 'resources.limits' to be a map after defaulting")
	}
	if limitsMap["cpu"] != defaultCPUQuantity {
		t.Errorf("expected resources.limits.cpu default='100m', got %v", limitsMap["cpu"])
	}
	if limitsMap["memory"] != "256Mi" {
		t.Errorf("expected resources.limits.memory default='256Mi', got %v", limitsMap["memory"])
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
	if jsonSchema.Type != typeObject {
		t.Fatalf("expected type=object, got %s", jsonSchema.Type)
	}

	// Verify required fields preserved
	if !slices.Contains(jsonSchema.Required, "image") {
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
					"ui:field": repoURLPicker,
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
	if jsonSchema.Type != typeObject {
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
	if resources["cpu"] != defaultCPUQuantity {
		t.Errorf("expected resources.cpu=100m, got %v", resources["cpu"])
	}

	// Step 2: Validate the defaulted values
	if err := ValidateWithJSONSchema(result, jsonSchema); err != nil {
		t.Fatalf("expected defaulted values to pass validation, got: %v", err)
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
	if portProp.Type != typeInteger {
		t.Fatalf("expected port type=integer, got %s", portProp.Type)
	}
}

func TestSectionToRawJSONSchema_OpenAPIV3_PreservesVendorExtensions(t *testing.T) {
	rawYAML := `
type: object
properties:
  replicas:
    type: integer
    default: 1
    x-openchoreo-backstage-portal:
      ui:field: RepoUrlPicker
      ui:options:
        allowedHosts:
          - github.com
  imagePullPolicy:
    type: string
    default: IfNotPresent
    x-openchoreo-pull-portal:
      ui:field: RepoUrlPicker
`

	section := &v1alpha1.SchemaSection{
		OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(rawYAML)},
	}

	result, err := SectionToRawJSONSchema(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	props := result["properties"].(map[string]any)

	// x-openchoreo-backstage-portal preserved
	replicas := props["replicas"].(map[string]any)
	ext, ok := replicas["x-openchoreo-backstage-portal"].(map[string]any)
	if !ok {
		t.Fatal("expected x-openchoreo-backstage-portal on replicas")
	}
	if ext["ui:field"] != repoURLPicker {
		t.Fatalf("expected ui:field=RepoUrlPicker, got %v", ext["ui:field"])
	}

	// x-openchoreo-pull-portal preserved
	ipp := props["imagePullPolicy"].(map[string]any)
	ext2, ok := ipp["x-openchoreo-pull-portal"].(map[string]any)
	if !ok {
		t.Fatal("expected x-openchoreo-pull-portal on imagePullPolicy")
	}
	if ext2["ui:field"] != repoURLPicker {
		t.Fatalf("expected ui:field=RepoUrlPicker, got %v", ext2["ui:field"])
	}
}

func TestSectionToRawJSONSchema_OpenAPIV3_RefWithVendorExtension(t *testing.T) {
	rawYAML := `
type: object
$defs:
  ResourceRequirements:
    type: object
    properties:
      cpu:
        type: string
        default: "100m"
    default: {}
properties:
  resources:
    $ref: "#/$defs/ResourceRequirements"
    x-openchoreo-resources-portal:
      ui:field: ResourcePicker
`

	section := &v1alpha1.SchemaSection{
		OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(rawYAML)},
	}

	result, err := SectionToRawJSONSchema(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// $defs removed
	if _, ok := result["$defs"]; ok {
		t.Fatal("expected $defs to be removed")
	}

	props := result["properties"].(map[string]any)
	resources := props["resources"].(map[string]any)

	// $ref resolved
	if resources["type"] != typeObject {
		t.Fatalf("expected type=object from resolved $ref, got %v", resources["type"])
	}

	// Vendor extension preserved as sibling
	ext, ok := resources["x-openchoreo-resources-portal"].(map[string]any)
	if !ok {
		t.Fatal("expected x-openchoreo-resources-portal on resources")
	}
	if ext["ui:field"] != "ResourcePicker" {
		t.Fatalf("expected ui:field=ResourcePicker, got %v", ext["ui:field"])
	}
}

func TestSectionToRawJSONSchema_NilSection(t *testing.T) {
	result, err := SectionToRawJSONSchema(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["type"] != typeObject {
		t.Fatalf("expected type=object for nil section, got %v", result["type"])
	}
}

func TestSectionToRawJSONSchema_ShorthandSchema(t *testing.T) {
	rawYAML := `
replicas: "integer | default=1"
name: "string"
`
	section := &v1alpha1.SchemaSection{
		OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(rawYAML)},
	}

	result, err := SectionToRawJSONSchema(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["type"] != typeObject {
		t.Fatalf("expected type=object, got %v", result["type"])
	}
	props, ok := result["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	if _, ok := props["replicas"]; !ok {
		t.Fatal("expected 'replicas' property")
	}
	if _, ok := props["name"]; !ok {
		t.Fatal("expected 'name' property")
	}
}

func TestResolveSectionToStructural_BothSchemasSet(t *testing.T) {
	// When openAPIV3Schema is set, it is used directly.
	section := &v1alpha1.SchemaSection{
		OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{
			"type": "object",
			"properties": {
				"image": {"type": "string", "default": "nginx"}
			}
		}`)},
	}

	structural, err := ResolveSectionToStructural(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structural == nil {
		t.Fatal("expected non-nil structural")
	}

	if _, ok := structural.Properties["image"]; !ok {
		t.Fatal("expected 'image' property from openAPIV3Schema")
	}
}

func TestSectionToJSONSchema_ComplexNestedOpenAPIV3(t *testing.T) {
	section := makeSchemaSection(true, map[string]any{
		"type": "object",
		"$defs": map[string]any{
			"Credentials": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"username": map[string]any{"type": "string"},
					"password": map[string]any{"type": "string"},
				},
				"required": []any{"username", "password"},
			},
		},
		"properties": map[string]any{
			"database": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"host": map[string]any{
						"type":        "string",
						"description": "Database hostname",
					},
					"port": map[string]any{
						"type":    "integer",
						"default": float64(5432),
						"minimum": float64(1),
						"maximum": float64(65535),
					},
					"credentials": map[string]any{
						"$ref": "#/$defs/Credentials",
					},
					"options": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"ssl": map[string]any{
								"type":    "boolean",
								"default": true,
							},
							"poolSize": map[string]any{
								"type":    "integer",
								"default": float64(10),
								"minimum": float64(1),
							},
						},
					},
				},
				"required": []any{"host", "credentials"},
			},
		},
		"required": []any{"database"},
	})

	jsonSchema, err := SectionToJSONSchema(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jsonSchema == nil {
		t.Fatal("expected non-nil jsonSchema")
	}

	// Top-level required
	if len(jsonSchema.Required) != 1 || jsonSchema.Required[0] != "database" {
		t.Fatalf("expected required=[database], got %v", jsonSchema.Required)
	}

	// Database property
	dbProp, ok := jsonSchema.Properties["database"]
	if !ok {
		t.Fatal("expected 'database' property")
	}
	if dbProp.Type != typeObject {
		t.Fatalf("expected database type=object, got %s", dbProp.Type)
	}

	// Database.host description
	hostProp, ok := dbProp.Properties["host"]
	if !ok {
		t.Fatal("expected 'host' property in database")
	}
	if hostProp.Description != "Database hostname" {
		t.Fatalf("expected host description preserved, got %q", hostProp.Description)
	}

	// $ref resolved: credentials should have username/password
	credsProp, ok := dbProp.Properties["credentials"]
	if !ok {
		t.Fatal("expected 'credentials' property in database")
	}
	if _, ok := credsProp.Properties["username"]; !ok {
		t.Fatal("expected 'username' in credentials after $ref resolution")
	}
	if _, ok := credsProp.Properties["password"]; !ok {
		t.Fatal("expected 'password' in credentials after $ref resolution")
	}

	// Nested options
	optionsProp, ok := dbProp.Properties["options"]
	if !ok {
		t.Fatal("expected 'options' property in database")
	}
	if _, ok := optionsProp.Properties["ssl"]; !ok {
		t.Fatal("expected 'ssl' in options")
	}
}

func TestOpenAPIV3_EndToEnd_DefaultsThenValidation(t *testing.T) {
	// Full end-to-end: openAPIV3Schema with defaults applied, then validated
	section := makeSchemaSection(true, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"replicas": map[string]any{
				"type":    "integer",
				"default": float64(3),
				"minimum": float64(1),
				"maximum": float64(100),
			},
			"name": map[string]any{
				"type":      "string",
				"minLength": float64(1),
			},
			"config": map[string]any{
				"type":    "object",
				"default": map[string]any{},
				"properties": map[string]any{
					"timeout": map[string]any{
						"type":    "integer",
						"default": float64(30),
						"minimum": float64(1),
					},
					"retries": map[string]any{
						"type":    "integer",
						"default": float64(3),
						"minimum": float64(0),
					},
				},
			},
		},
		"required": []any{"name"},
	})

	structural, jsonSchema, err := ResolveSectionToBundle(section)
	if err != nil {
		t.Fatalf("ResolveSectionToBundle error: %v", err)
	}

	// Apply defaults to partial values
	values := map[string]any{"name": "my-service"}
	result := ApplyDefaults(values, structural)

	// Verify defaults were applied
	if result["replicas"] != int64(3) {
		t.Errorf("expected replicas=3, got %v", result["replicas"])
	}
	config, ok := result["config"].(map[string]any)
	if !ok {
		t.Fatalf("expected config to be map, got %T", result["config"])
	}
	if config["timeout"] != int64(30) {
		t.Errorf("expected config.timeout=30, got %v", config["timeout"])
	}
	if config["retries"] != int64(3) {
		t.Errorf("expected config.retries=3, got %v", config["retries"])
	}

	// Validate the defaulted values should pass
	if err := ValidateWithJSONSchema(result, jsonSchema); err != nil {
		t.Fatalf("expected defaulted values to pass validation, got: %v", err)
	}
}

func TestValidateWithJSONSchema_OpenAPIV3_WrongType(t *testing.T) {
	section := makeSchemaSection(true, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"replicas": map[string]any{"type": "integer"},
			"name":     map[string]any{"type": "string"},
		},
	})

	jsonSchema, err := SectionToJSONSchema(section)
	if err != nil {
		t.Fatalf("SectionToJSONSchema error: %v", err)
	}

	// Wrong type: string instead of integer
	wrongType := map[string]any{
		"replicas": "not-a-number",
		"name":     "test",
	}
	if err := ValidateWithJSONSchema(wrongType, jsonSchema); err == nil {
		t.Fatal("expected error for wrong type (string instead of integer)")
	}
}

func TestValidateWithJSONSchema_OpenAPIV3_ExtraFields(t *testing.T) {
	section := makeSchemaSection(true, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
		"additionalProperties": false,
	})

	jsonSchema, err := SectionToJSONSchema(section)
	if err != nil {
		t.Fatalf("SectionToJSONSchema error: %v", err)
	}

	// Extra field when additionalProperties=false
	extraField := map[string]any{
		"name":         "test",
		"unknownField": "unexpected",
	}
	if err := ValidateWithJSONSchema(extraField, jsonSchema); err == nil {
		t.Fatal("expected error for extra field with additionalProperties=false")
	}
}

func TestValidateWithJSONSchema_OpenAPIV3_MissingRequired(t *testing.T) {
	section := makeSchemaSection(true, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":  map[string]any{"type": "string"},
			"image": map[string]any{"type": "string"},
		},
		"required": []any{"name", "image"},
	})

	jsonSchema, err := SectionToJSONSchema(section)
	if err != nil {
		t.Fatalf("SectionToJSONSchema error: %v", err)
	}

	// Missing both required fields
	empty := map[string]any{}
	err = ValidateWithJSONSchema(empty, jsonSchema)
	if err == nil {
		t.Fatal("expected error for missing required fields")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Fatalf("expected error to mention 'name', got: %v", err)
	}
	if !strings.Contains(err.Error(), "image") {
		t.Fatalf("expected error to mention 'image', got: %v", err)
	}

	// Missing one required field
	partial := map[string]any{"name": "test"}
	err = ValidateWithJSONSchema(partial, jsonSchema)
	if err == nil {
		t.Fatal("expected error for missing required field 'image'")
	}
	if !strings.Contains(err.Error(), "image") {
		t.Fatalf("expected error to mention 'image', got: %v", err)
	}
}

func TestSectionToRawJSONSchema_DeeplyNestedVendorExtensions(t *testing.T) {
	rawYAML := `
type: object
properties:
  config:
    type: object
    x-section: main
    properties:
      database:
        type: object
        x-category: storage
        properties:
          host:
            type: string
            x-widget: text-input
            x-validation:
              pattern: "^[a-z]+$"
              message: "Only lowercase letters"
`
	section := &v1alpha1.SchemaSection{
		OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(rawYAML)},
	}

	result, err := SectionToRawJSONSchema(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Level 1
	props := result["properties"].(map[string]any)
	config := props["config"].(map[string]any)
	if config["x-section"] != "main" {
		t.Fatalf("expected x-section=main, got %v", config["x-section"])
	}

	// Level 2
	configProps := config["properties"].(map[string]any)
	db := configProps["database"].(map[string]any)
	if db["x-category"] != "storage" {
		t.Fatalf("expected x-category=storage, got %v", db["x-category"])
	}

	// Level 3
	dbProps := db["properties"].(map[string]any)
	host := dbProps["host"].(map[string]any)
	if host["x-widget"] != "text-input" {
		t.Fatalf("expected x-widget=text-input, got %v", host["x-widget"])
	}
	xVal, ok := host["x-validation"].(map[string]any)
	if !ok {
		t.Fatal("expected x-validation on host")
	}
	if xVal["pattern"] != "^[a-z]+$" {
		t.Fatalf("expected x-validation.pattern=^[a-z]+$, got %v", xVal["pattern"])
	}
}

func TestValidateWithJSONSchema_NilSchema(t *testing.T) {
	err := ValidateWithJSONSchema(map[string]any{"name": "test"}, nil)
	if err == nil {
		t.Fatal("expected error for nil schema")
	}
	if !strings.Contains(err.Error(), "schema is nil") {
		t.Fatalf("expected 'schema is nil' error, got: %v", err)
	}
}

func TestResolveSectionToStructural_EmptyRaw(t *testing.T) {
	// SchemaSection with empty raw data should return nil
	section := &v1alpha1.SchemaSection{
		OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte{}},
	}

	structural, err := ResolveSectionToStructural(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if structural != nil {
		t.Fatal("expected nil structural for empty raw data")
	}
}

func TestSectionToRawJSONSchema_EmptyOpenAPIV3(t *testing.T) {
	// OpenAPIV3Schema with just "type: object" and no properties
	section := makeSchemaSection(true, map[string]any{
		"type": "object",
	})

	result, err := SectionToRawJSONSchema(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["type"] != typeObject {
		t.Fatalf("expected type=object, got %v", result["type"])
	}
}

func TestSectionToJSONSchema_BothSchemasSet(t *testing.T) {
	// When both are set, openAPIV3Schema should take priority
	section := &v1alpha1.SchemaSection{
		OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{
			"type": "object",
			"properties": {
				"name": {"type": "string"}
			}
		}`)},
	}

	jsonSchema, err := SectionToJSONSchema(section)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := jsonSchema.Properties["name"]; !ok {
		t.Fatal("expected 'name' from openAPIV3Schema")
	}
}

func TestMergeFieldMaps_EmptyInput(t *testing.T) {
	result := mergeFieldMaps(nil)
	if len(result) != 0 {
		t.Fatalf("expected empty map, got %v", result)
	}

	result = mergeFieldMaps([]map[string]any{})
	if len(result) != 0 {
		t.Fatalf("expected empty map, got %v", result)
	}
}

func TestMergeFieldMaps_NestedMerge(t *testing.T) {
	maps := []map[string]any{
		{"db": map[string]any{"host": "localhost"}},
		{"db": map[string]any{"port": "5432"}, "name": "test"},
	}

	result := mergeFieldMaps(maps)
	db, ok := result["db"].(map[string]any)
	if !ok {
		t.Fatal("expected db to be map")
	}
	if db["host"] != "localhost" {
		t.Fatalf("expected db.host=localhost, got %v", db["host"])
	}
	if db["port"] != "5432" {
		t.Fatalf("expected db.port=5432, got %v", db["port"])
	}
	if result["name"] != "test" {
		t.Fatalf("expected name=test, got %v", result["name"])
	}
}
