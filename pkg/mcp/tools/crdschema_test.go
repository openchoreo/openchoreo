// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"testing"
)

func TestComponentTypeCreationSchema(t *testing.T) {
	schema, err := ComponentTypeCreationSchema()
	if err != nil {
		t.Fatalf("ComponentTypeCreationSchema() error: %v", err)
	}
	assertSpecSchema(t, schema, "ComponentType")
}

func TestClusterComponentTypeCreationSchema(t *testing.T) {
	schema, err := ClusterComponentTypeCreationSchema()
	if err != nil {
		t.Fatalf("ClusterComponentTypeCreationSchema() error: %v", err)
	}
	assertSpecSchema(t, schema, "ClusterComponentType")
}

func TestTraitCreationSchema(t *testing.T) {
	schema, err := TraitCreationSchema()
	if err != nil {
		t.Fatalf("TraitCreationSchema() error: %v", err)
	}
	assertTraitSpecSchema(t, schema, "Trait")
}

func TestClusterTraitCreationSchema(t *testing.T) {
	schema, err := ClusterTraitCreationSchema()
	if err != nil {
		t.Fatalf("ClusterTraitCreationSchema() error: %v", err)
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("ClusterTrait schema missing 'properties'")
	}
	// ClusterTrait has the same shape as Trait minus 'validations' (per skills/skills/CLAUDE.md notes).
	for _, field := range []string{"creates", "patches", "parameters", "environmentConfigs"} {
		if _, exists := props[field]; !exists {
			t.Errorf("ClusterTrait schema missing optional field %q", field)
		}
	}
}

func TestWorkflowCreationSchema(t *testing.T) {
	schema, err := WorkflowCreationSchema()
	if err != nil {
		t.Fatalf("WorkflowCreationSchema() error: %v", err)
	}
	if _, ok := schema["properties"].(map[string]any); !ok {
		t.Fatalf("Workflow schema missing 'properties'")
	}
}

func TestClusterWorkflowCreationSchema(t *testing.T) {
	schema, err := ClusterWorkflowCreationSchema()
	if err != nil {
		t.Fatalf("ClusterWorkflowCreationSchema() error: %v", err)
	}
	if _, ok := schema["properties"].(map[string]any); !ok {
		t.Fatalf("ClusterWorkflow schema missing 'properties'")
	}
}

func assertTraitSpecSchema(t *testing.T, schema map[string]any, kind string) {
	t.Helper()

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("%s schema missing 'properties'", kind)
	}

	optionalFields := []string{"creates", "patches", "parameters", "environmentConfigs", "validations"}
	for _, field := range optionalFields {
		if _, exists := props[field]; !exists {
			t.Errorf("%s schema missing optional field %q", kind, field)
		}
	}
}

func assertSpecSchema(t *testing.T, schema map[string]any, kind string) {
	t.Helper()

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("%s schema missing 'properties'", kind)
	}

	requiredFields := []string{"workloadType", "resources"}
	for _, field := range requiredFields {
		if _, exists := props[field]; !exists {
			t.Errorf("%s schema missing required field %q", kind, field)
		}
	}

	optionalFields := []string{"parameters", "environmentConfigs", "allowedWorkflows",
		"traits", "allowedTraits", "validations"}
	for _, field := range optionalFields {
		if _, exists := props[field]; !exists {
			t.Errorf("%s schema missing optional field %q", kind, field)
		}
	}
}
