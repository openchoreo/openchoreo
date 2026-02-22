// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gopkg.in/yaml.v3"

	corev1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/schema"
)

// remarshal converts src to dst via JSON marshaling/unmarshaling.
func remarshal(src, dst interface{}) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

// Test suite for Generator using fixtures
func TestGenerator_BasicTypes(t *testing.T) {
	runFixtureTest(t, "basic_types")
}

func TestGenerator_ArrayScaffolding(t *testing.T) {
	runFixtureTest(t, "array_scaffolding")
}

func TestGenerator_WithTraits(t *testing.T) {
	runFixtureTest(t, "with_traits")
}

func TestGenerator_WithWorkflow(t *testing.T) {
	runFixtureTest(t, "with_workflow")
}

func TestGenerator_MinimalComments_False(t *testing.T) {
	runFixtureTest(t, "minimal_comments_false")
}

func TestGenerator_MinimalComments_True(t *testing.T) {
	runFixtureTest(t, "minimal_comments_true")
}

func TestGenerator_ValidationAndTypes(t *testing.T) {
	runFixtureTest(t, "validation_and_types")
}

func TestGenerator_ObjectDefaults(t *testing.T) {
	runFixtureTest(t, "object_defaults")
}

func TestGenerator_MapScaffolding(t *testing.T) {
	runFixtureTest(t, "map_scaffolding")
}

func TestGenerator_CollectionShapes(t *testing.T) {
	runFixtureTest(t, "collection_shapes")
}

func TestGenerator_EscapingQuoting(t *testing.T) {
	runFixtureTest(t, "escaping_quoting")
}

// runFixtureTest loads input/want YAML files and runs the generator test.
// Input files use --- to separate YAML documents:
// - First document: Options (no apiVersion/kind)
// - Subsequent documents: CRDs identified by kind field
func runFixtureTest(t *testing.T, testName string) {
	t.Helper()

	// Load input file
	inputPath := filepath.Join("testdata", testName+"_input.yaml")
	inputBytes, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("failed to read input file %s: %v", inputPath, err)
	}

	// Parse multi-document YAML
	decoder := yaml.NewDecoder(bytes.NewReader(inputBytes))

	// First document is Options
	var opts Options
	err = decoder.Decode(&opts)
	if err != nil {
		t.Fatalf("failed to decode options: %v", err)
	}

	// Parse remaining documents and identify by kind
	var componentType *corev1alpha1.ComponentType
	var traits []*corev1alpha1.Trait
	var workflow *corev1alpha1.Workflow

	for {
		var doc map[string]interface{}
		err := decoder.Decode(&doc)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break // End of documents
			}
			t.Fatalf("failed to decode YAML document: %v", err)
		}

		kind, ok := doc["kind"].(string)
		if !ok {
			continue
		}

		switch kind {
		case "ComponentType":
			componentType = &corev1alpha1.ComponentType{}
			err = remarshal(doc, componentType)
			if err != nil {
				t.Fatalf("failed to convert ComponentType: %v", err)
			}

		case "Trait":
			trait := &corev1alpha1.Trait{}
			err = remarshal(doc, trait)
			if err != nil {
				t.Fatalf("failed to convert Trait: %v", err)
			}
			traits = append(traits, trait)

		case "Workflow":
			workflow = &corev1alpha1.Workflow{}
			err = remarshal(doc, workflow)
			if err != nil {
				t.Fatalf("failed to convert Workflow: %v", err)
			}
		}
	}

	if componentType == nil {
		t.Fatal("ComponentType not found in input file")
	}

	// Create generator
	generator, err := NewGenerator(componentType, traits, workflow, &opts)
	if err != nil {
		t.Fatalf("NewGenerator() failed: %v", err)
	}

	// Generate output
	generated, err := generator.Generate()
	if err != nil {
		t.Fatalf("generator.Generate() failed: %v", err)
	}

	// Load expected output file
	wantPath := filepath.Join("testdata", testName+"_want.yaml")
	wantBytes, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("failed to read want file %s: %v", wantPath, err)
	}

	// Compare exact string (preserves comments, works with deterministic output)
	want := string(wantBytes)
	if diff := cmp.Diff(want, generated); diff != "" {
		t.Errorf("generated output mismatch (-want +got):\n%s", diff)
	}

	// Validate generated output against schemas
	validateGeneratedOutput(t, generated, componentType, traits, workflow)
}

// validateGeneratedOutput parses the generated YAML and validates each section against its schema.
func validateGeneratedOutput(t *testing.T, generated string, componentType *corev1alpha1.ComponentType, traits []*corev1alpha1.Trait, workflow *corev1alpha1.Workflow) {
	t.Helper()

	// Parse generated YAML into a map structure
	var component map[string]any
	if err := yaml.Unmarshal([]byte(generated), &component); err != nil {
		t.Fatalf("failed to parse generated output: %v", err)
	}

	spec, ok := component["spec"].(map[string]any)
	if !ok {
		t.Fatal("generated output missing spec section")
	}

	// Validate component parameters against ComponentType schema
	if params, ok := spec["parameters"].(map[string]any); ok {
		validateComponentParameters(t, params, componentType)
	}

	// Validate trait parameters
	if traitsSection, ok := spec["traits"].([]any); ok {
		validateTraitParameters(t, traitsSection, traits)
	}

	// Validate workflow parameters
	if workflowSection, ok := spec["workflow"].(map[string]any); ok && workflow != nil {
		validateWorkflowParameters(t, workflowSection, workflow)
	}
}

// validateComponentParameters validates component parameters against the ComponentType schema.
func validateComponentParameters(t *testing.T, params map[string]any, componentType *corev1alpha1.ComponentType) {
	t.Helper()

	if componentType.Spec.Schema.Parameters == nil {
		return
	}

	// Convert schema to JSONSchema
	paramsMap, err := rawExtensionToMap(componentType.Spec.Schema.Parameters)
	if err != nil {
		t.Fatalf("failed to convert ComponentType parameters: %v", err)
	}

	var typesMap map[string]any
	if componentType.Spec.Schema.Types != nil {
		typesMap, err = rawExtensionToMap(componentType.Spec.Schema.Types)
		if err != nil {
			t.Fatalf("failed to convert ComponentType types: %v", err)
		}
	}

	def := schema.Definition{
		Schemas: []map[string]any{paramsMap},
		Types:   typesMap,
	}

	jsonSchema, err := schema.ToJSONSchema(def)
	if err != nil {
		t.Fatalf("failed to convert to JSON schema: %v", err)
	}

	// Validate and filter out placeholder-related errors
	if err := schema.ValidateWithJSONSchema(params, jsonSchema); err != nil {
		if realErrors := filterPlaceholderErrors(err); realErrors != "" {
			t.Errorf("component parameters validation failed: %s", realErrors)
		}
	}
}

// validateTraitParameters validates each trait's parameters against its schema.
func validateTraitParameters(t *testing.T, traitsSection []any, traits []*corev1alpha1.Trait) {
	t.Helper()

	// Build a map of trait name -> trait for lookup
	traitMap := make(map[string]*corev1alpha1.Trait)
	for _, trait := range traits {
		traitMap[trait.Name] = trait
	}

	for _, item := range traitsSection {
		traitInstance, ok := item.(map[string]any)
		if !ok {
			continue
		}

		traitName, _ := traitInstance["name"].(string)
		trait, exists := traitMap[traitName]
		if !exists || trait.Spec.Schema.Parameters == nil {
			continue
		}

		params, ok := traitInstance["parameters"].(map[string]any)
		if !ok {
			continue
		}

		// Convert trait schema to JSONSchema
		paramsMap, err := rawExtensionToMap(trait.Spec.Schema.Parameters)
		if err != nil {
			t.Fatalf("failed to convert Trait %s parameters: %v", traitName, err)
		}

		var typesMap map[string]any
		if trait.Spec.Schema.Types != nil {
			typesMap, err = rawExtensionToMap(trait.Spec.Schema.Types)
			if err != nil {
				t.Fatalf("failed to convert Trait %s types: %v", traitName, err)
			}
		}

		def := schema.Definition{
			Schemas: []map[string]any{paramsMap},
			Types:   typesMap,
		}

		jsonSchema, err := schema.ToJSONSchema(def)
		if err != nil {
			t.Fatalf("failed to convert Trait %s to JSON schema: %v", traitName, err)
		}

		if err := schema.ValidateWithJSONSchema(params, jsonSchema); err != nil {
			if realErrors := filterPlaceholderErrors(err); realErrors != "" {
				t.Errorf("trait %s parameters validation failed: %s", traitName, realErrors)
			}
		}
	}
}

// validateWorkflowParameters validates workflow parameters against the Workflow schema.
func validateWorkflowParameters(t *testing.T, workflowSection map[string]any, workflow *corev1alpha1.Workflow) {
	t.Helper()

	if workflow.Spec.Schema.Parameters == nil {
		return
	}

	params, ok := workflowSection["parameters"].(map[string]any)
	if !ok {
		return
	}

	// Convert workflow schema to JSONSchema
	paramsMap, err := rawExtensionToMap(workflow.Spec.Schema.Parameters)
	if err != nil {
		t.Fatalf("failed to convert Workflow parameters: %v", err)
	}

	var typesMap map[string]any
	if workflow.Spec.Schema.Types != nil {
		typesMap, err = rawExtensionToMap(workflow.Spec.Schema.Types)
		if err != nil {
			t.Fatalf("failed to convert Workflow types: %v", err)
		}
	}

	def := schema.Definition{
		Schemas: []map[string]any{paramsMap},
		Types:   typesMap,
	}

	jsonSchema, err := schema.ToJSONSchema(def)
	if err != nil {
		t.Fatalf("failed to convert Workflow to JSON schema: %v", err)
	}

	if err := schema.ValidateWithJSONSchema(params, jsonSchema); err != nil {
		if realErrors := filterPlaceholderErrors(err); realErrors != "" {
			t.Errorf("workflow parameters validation failed: %s", realErrors)
		}
	}
}

// filterPlaceholderErrors filters out validation errors related to placeholder values.
// The following error types are filtered:
// - Errors mentioning "<TODO_" (placeholders for users to fill in)
// - Type mismatch errors for string values (placeholders are always strings regardless of expected type)
// - Pattern validation errors (placeholders never match patterns)
// Returns the remaining real errors as a string, or empty string if all errors were filtered.
func filterPlaceholderErrors(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()
	// Split by semicolon (multiple validation errors are joined with "; ")
	parts := strings.Split(errStr, "; ")

	realErrors := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// Skip errors that mention placeholder values directly
		if strings.Contains(part, "<TODO_") {
			continue
		}
		// Skip type mismatch errors caused by string placeholders
		// e.g., "port in body must be of type integer: \"string\""
		if strings.Contains(part, `: "string"`) {
			continue
		}
		// Skip pattern validation errors (placeholders like <TODO_X> never match patterns)
		// e.g., "username in body should match '^[a-z][a-z0-9_]*$'"
		if strings.Contains(part, "should match '") {
			continue
		}
		realErrors = append(realErrors, part)
	}

	return strings.Join(realErrors, "; ")
}
