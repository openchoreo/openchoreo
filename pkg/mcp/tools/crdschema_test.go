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
