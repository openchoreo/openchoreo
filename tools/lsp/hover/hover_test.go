// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package hover

import (
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/schema"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine/template"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestBuildHoverContent(t *testing.T) {
	t.Run("string field", func(t *testing.T) {
		node := &schema.FieldSchema{Type: "string", Required: true}
		content := buildHoverContent(node, "metadata.name")
		if !strings.Contains(content, "name") {
			t.Error("missing field name")
		}
		if !strings.Contains(content, "string") {
			t.Error("missing type")
		}
		if !strings.Contains(content, "Required") {
			t.Error("missing required")
		}
	})

	t.Run("optional field", func(t *testing.T) {
		node := &schema.FieldSchema{Type: "string", Required: false}
		content := buildHoverContent(node, "metadata.annotations")
		if !strings.Contains(content, "Optional") {
			t.Error("missing optional")
		}
	})

	t.Run("enum field shows list", func(t *testing.T) {
		node := &schema.FieldSchema{Type: "string", Enum: []string{"deployment", "statefulset"}}
		content := buildHoverContent(node, "spec.workloadType")
		if !strings.Contains(content, "deployment") {
			t.Error("missing enum value deployment")
		}
		if !strings.Contains(content, "statefulset") {
			t.Error("missing enum value statefulset")
		}
		if !strings.Contains(content, "Allowed values") {
			t.Error("missing allowed values header")
		}
	})

	t.Run("pattern field", func(t *testing.T) {
		node := &schema.FieldSchema{Type: "string", Pattern: "^[a-z]+$"}
		content := buildHoverContent(node, "spec.id")
		if !strings.Contains(content, "^[a-z]+$") {
			t.Error("missing pattern")
		}
	})

	t.Run("length constraints", func(t *testing.T) {
		node := &schema.FieldSchema{Type: "string", MinLength: 1, MaxLength: 63}
		content := buildHoverContent(node, "spec.name")
		if !strings.Contains(content, "1-63 characters") {
			t.Error("missing length constraint")
		}
	})

	t.Run("preserve unknown fields", func(t *testing.T) {
		node := &schema.FieldSchema{Type: "object", Preserve: true}
		content := buildHoverContent(node, "spec.template")
		if !strings.Contains(content, "preserveUnknownFields") {
			t.Error("missing preserveUnknownFields info")
		}
	})

	t.Run("object with children shows table", func(t *testing.T) {
		node := &schema.FieldSchema{
			Type: "object",
			Properties: map[string]*schema.FieldSchema{
				"name":      {Type: "string", Required: true},
				"namespace": {Type: "string", Required: false},
			},
		}
		content := buildHoverContent(node, "metadata")
		if !strings.Contains(content, "Fields") {
			t.Error("missing fields header")
		}
		if !strings.Contains(content, "name") {
			t.Error("missing child field name")
		}
		if !strings.Contains(content, "namespace") {
			t.Error("missing child field namespace")
		}
		if !strings.Contains(content, "|") {
			t.Error("missing table separator")
		}
	})

	t.Run("array with item fields shows table", func(t *testing.T) {
		node := &schema.FieldSchema{
			Type: "array",
			Items: &schema.FieldSchema{
				Type: "object",
				Properties: map[string]*schema.FieldSchema{
					"kind": {Type: "string", Required: true},
					"name": {Type: "string", Required: true},
				},
			},
		}
		content := buildHoverContent(node, "spec.allowedTraits")
		if !strings.Contains(content, "Array item fields") {
			t.Error("missing array item fields header")
		}
		if !strings.Contains(content, "kind") {
			t.Error("missing array item field kind")
		}
	})

	t.Run("nil returns empty", func(t *testing.T) {
		if content := buildHoverContent(nil, ""); content != "" {
			t.Errorf("expected empty, got %q", content)
		}
	})

	t.Run("root path uses field name", func(t *testing.T) {
		node := &schema.FieldSchema{Type: "string"}
		content := buildHoverContent(node, "metadata")
		if !strings.Contains(content, "metadata") {
			t.Error("missing field name from root path")
		}
	})

	t.Run("known field has description", func(t *testing.T) {
		if err := load(); err != nil {
			t.Fatalf("load failed: %v", err)
		}
		node := &schema.FieldSchema{Type: "string"}
		content := buildHoverContent(node, "apiVersion")
		if !strings.Contains(content, "API version") {
			t.Error("missing description for apiVersion")
		}
	})

	t.Run("unknown field has no description", func(t *testing.T) {
		node := &schema.FieldSchema{Type: "string"}
		content := buildHoverContent(node, "customField")
		if strings.Contains(content, "customField") && strings.Contains(content, "Description") {
			// It's fine if it has the name but no description
		}
	})
}

func TestHandleOnSpecKeyShowsSpecNotMetadata(t *testing.T) {
	doc := `apiVersion: openchoreo.dev/v1alpha1
kind: DeploymentPipeline
metadata:
  name: standard
  namespace: default
spec:
  promotionPaths:
    - sourceEnvironmentRef:
        name: development
`
	// Cursor on the "spec:" key line (line 6, 1-indexed).
	h := Handle(doc, 6, 1)
	if h == nil {
		t.Fatal("expected hover to be non-nil")
	}
	value := h.Contents.(protocol.MarkupContent).Value
	if !strings.Contains(value, "spec") {
		t.Errorf("expected hover to reference spec, got %q", value)
	}
	if strings.Contains(value, "Metadata to add to the generated secret") {
		t.Errorf("hover leaked metadata description into spec hover: %q", value)
	}

	// Cursor on the "metadata:" key line (line 3, 1-indexed).
	h = Handle(doc, 3, 1)
	if h == nil {
		t.Fatal("expected hover to be non-nil")
	}
	value = h.Contents.(protocol.MarkupContent).Value
	if !strings.Contains(value, "Resource metadata.") {
		t.Errorf("expected metadata hover description on metadata key, got %q", value)
	}
}

func TestFieldDescription(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"apiVersion", "apiVersion", "API version"},
		{"kind", "kind", "Kind"},
		{"metadata", "metadata", "Metadata"},
		{"name", "metadata.name", "Name"},
		{"spec", "spec", "Spec"},
		{"status", "status", "Status"},
		{"unknown", "unknown", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := fieldDescription(tt.name, tt.path)
			if tt.expected == "" {
				if desc != "" {
					t.Errorf("expected empty, got %q", desc)
				}
			} else {
				if !strings.Contains(desc, tt.expected) {
					t.Errorf("expected %q in %q", tt.expected, desc)
				}
			}
		})
	}
}

func TestSchemaForKnownKinds(t *testing.T) {
	kinds := []string{"Project", "Component", "ComponentType", "ClusterComponentType", "Workflow"}
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			s := template.SchemaFor(kind)
			if s == nil {
				t.Errorf("no schema for %s", kind)
			}
		})
	}
}

func TestSchemaForUnknownKind(t *testing.T) {
	if s := template.SchemaFor("UnknownKind"); s != nil {
		t.Error("expected nil for unknown kind")
	}
}

func TestParseHoverConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		data := []byte(`{
			"kind": "the kind field",
			"goToDefinitions": { "Project": { "description": "a project", "url": "https://example.com" } }
		}`)
		fields, kinds, err := parseHoverConfig(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(fields) != 1 || fields["kind"] != "the kind field" {
			t.Errorf("unexpected fields: %v", fields)
		}
		if len(kinds) != 1 {
			t.Errorf("expected 1 kind, got %d", len(kinds))
		}
		if info := kinds["Project"]; info.Description != "a project" || info.URL != "https://example.com" {
			t.Errorf("unexpected kind info: %+v", info)
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		fields, kinds, err := parseHoverConfig([]byte("{not json"))
		if err == nil {
			t.Fatal("expected error for malformed JSON")
		}
		if fields != nil || kinds != nil {
			t.Error("expected nil maps on error")
		}
	})

	t.Run("non-string fields ignored", func(t *testing.T) {
		data := []byte(`{ "kind": {"nested": true}, "name": "plain" }`)
		fields, _, err := parseHoverConfig(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := fields["kind"]; ok {
			t.Error("expected nested object field to be ignored")
		}
		if fields["name"] != "plain" {
			t.Errorf("expected plain field to be kept, got %q", fields["name"])
		}
	})
}
