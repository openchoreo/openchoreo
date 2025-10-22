// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderer

import (
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/crd-renderer/template"
)

func TestRenderResources(t *testing.T) {
	engine := template.NewEngine()
	renderer := NewRenderer(engine)

	tests := []struct {
		name          string
		templatesYAML string
		context       map[string]any
		wantCount     int
		wantErr       bool
	}{
		{
			name: "single resource without conditions",
			templatesYAML: `
- id: deployment
  template:
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: ${component.name}
`,
			context: map[string]any{
				"component": map[string]any{
					"name": "test-app",
				},
			},
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "resource with includeWhen true",
			templatesYAML: `
- id: service
  includeWhen: ${parameters.expose}
  template:
    apiVersion: v1
    kind: Service
    metadata:
      name: test-service
`,
			context: map[string]any{
				"parameters": map[string]any{
					"expose": true,
				},
			},
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "resource with includeWhen false",
			templatesYAML: `
- id: service
  includeWhen: ${parameters.expose}
  template:
    apiVersion: v1
    kind: Service
    metadata:
      name: test-service
`,
			context: map[string]any{
				"parameters": map[string]any{
					"expose": false,
				},
			},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "resource with forEach",
			templatesYAML: `
- id: configmap
  forEach: ${parameters.configs}
  var: config
  template:
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: ${config.name}
    data:
      value: ${config.value}
`,
			context: map[string]any{
				"parameters": map[string]any{
					"configs": []any{
						map[string]any{"name": "config1", "value": "val1"},
						map[string]any{"name": "config2", "value": "val2"},
						map[string]any{"name": "config3", "value": "val3"},
					},
				},
			},
			wantCount: 3,
			wantErr:   false,
		},
		{
			name: "multiple resources mixed",
			templatesYAML: `
- id: deployment
  template:
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: app
- id: service
  includeWhen: ${parameters.expose}
  template:
    apiVersion: v1
    kind: Service
    metadata:
      name: app-svc
- id: secret
  forEach: ${parameters.secrets}
  var: secret
  template:
    apiVersion: v1
    kind: Secret
    metadata:
      name: ${secret}
`,
			context: map[string]any{
				"parameters": map[string]any{
					"expose":  true,
					"secrets": []any{"db-secret", "api-secret"},
				},
			},
			wantCount: 4, // 1 deployment + 1 service + 2 secrets
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse templates from YAML
			var templates []v1alpha1.ResourceTemplate
			if err := yaml.Unmarshal([]byte(tt.templatesYAML), &templates); err != nil {
				t.Fatalf("Failed to parse templates YAML: %v", err)
			}

			got, err := renderer.RenderResources(templates, tt.context)
			if (err != nil) != tt.wantErr {
				t.Errorf("RenderResources() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) != tt.wantCount {
				t.Errorf("RenderResources() got %d resources, want %d", len(got), tt.wantCount)
			}
		})
	}
}

func TestShouldInclude(t *testing.T) {
	engine := template.NewEngine()
	renderer := NewRenderer(engine)

	tests := []struct {
		name     string
		template v1alpha1.ResourceTemplate
		context  map[string]any
		want     bool
		wantErr  bool
	}{
		{
			name: "no includeWhen - defaults to true",
			template: v1alpha1.ResourceTemplate{
				ID:          "test",
				IncludeWhen: "",
			},
			context: map[string]any{},
			want:    true,
			wantErr: false,
		},
		{
			name: "includeWhen evaluates to true",
			template: v1alpha1.ResourceTemplate{
				ID:          "test",
				IncludeWhen: "${enabled}",
			},
			context: map[string]any{
				"enabled": true,
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "includeWhen evaluates to false",
			template: v1alpha1.ResourceTemplate{
				ID:          "test",
				IncludeWhen: "${enabled}",
			},
			context: map[string]any{
				"enabled": false,
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "includeWhen with complex expression",
			template: v1alpha1.ResourceTemplate{
				ID:          "test",
				IncludeWhen: "${parameters.replicas > 1}",
			},
			context: map[string]any{
				"parameters": map[string]any{
					"replicas": 3,
				},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "includeWhen with missing data - gracefully returns false",
			template: v1alpha1.ResourceTemplate{
				ID:          "test",
				IncludeWhen: "${nonexistent.field}",
			},
			context: map[string]any{},
			want:    false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := renderer.shouldInclude(tt.template, tt.context)
			if (err != nil) != tt.wantErr {
				t.Errorf("shouldInclude() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("shouldInclude() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenderWithForEach(t *testing.T) {
	engine := template.NewEngine()
	renderer := NewRenderer(engine)

	tests := []struct {
		name         string
		templateYAML string
		context      map[string]any
		wantCount    int
		wantErr      bool
	}{
		{
			name: "forEach with default var name",
			templateYAML: `
id: test
forEach: ${items}
template:
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: ${item}
`,
			context: map[string]any{
				"items": []any{"item1", "item2", "item3"},
			},
			wantCount: 3,
			wantErr:   false,
		},
		{
			name: "forEach with custom var name",
			templateYAML: `
id: test
forEach: ${configs}
var: config
template:
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: ${config.name}
  data:
    value: ${config.value}
`,
			context: map[string]any{
				"configs": []any{
					map[string]any{"name": "cfg1", "value": "val1"},
					map[string]any{"name": "cfg2", "value": "val2"},
				},
			},
			wantCount: 2,
			wantErr:   false,
		},
		{
			name: "forEach with empty array",
			templateYAML: `
id: test
forEach: ${items}
template:
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: test
`,
			context: map[string]any{
				"items": []any{},
			},
			wantCount: 0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse template from YAML
			var template v1alpha1.ResourceTemplate
			if err := yaml.Unmarshal([]byte(tt.templateYAML), &template); err != nil {
				t.Fatalf("Failed to parse template YAML: %v", err)
			}

			got, err := renderer.renderWithForEach(template, tt.context)
			if (err != nil) != tt.wantErr {
				t.Errorf("renderWithForEach() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) != tt.wantCount {
				t.Errorf("renderWithForEach() got %d resources, want %d", len(got), tt.wantCount)
			}
		})
	}
}

func TestRenderSingleResource(t *testing.T) {
	engine := template.NewEngine()
	renderer := NewRenderer(engine)

	tests := []struct {
		name         string
		templateYAML string
		context      map[string]any
		wantErr      bool
		checkFn      func(*testing.T, map[string]any)
	}{
		{
			name: "basic resource rendering",
			templateYAML: `
id: test
template:
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: ${name}
  data:
    key: ${value}
`,
			context: map[string]any{
				"name":  "my-config",
				"value": "my-value",
			},
			wantErr: false,
			checkFn: func(t *testing.T, resource map[string]any) {
				if resource["kind"] != "ConfigMap" {
					t.Errorf("Expected kind=ConfigMap, got %v", resource["kind"])
				}
				metadata := resource["metadata"].(map[string]any)
				if metadata["name"] != "my-config" {
					t.Errorf("Expected name=my-config, got %v", metadata["name"])
				}
				data := resource["data"].(map[string]any)
				if data["key"] != "my-value" {
					t.Errorf("Expected key=my-value, got %v", data["key"])
				}
			},
		},
		{
			name: "resource with omit()",
			templateYAML: `
id: test
template:
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: test
    annotations: ${omit()}
`,
			context: map[string]any{},
			wantErr: false,
			checkFn: func(t *testing.T, resource map[string]any) {
				metadata := resource["metadata"].(map[string]any)
				if _, hasAnnotations := metadata["annotations"]; hasAnnotations {
					t.Errorf("Expected annotations to be omitted")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse template from YAML
			var template v1alpha1.ResourceTemplate
			if err := yaml.Unmarshal([]byte(tt.templateYAML), &template); err != nil {
				t.Fatalf("Failed to parse template YAML: %v", err)
			}

			got, err := renderer.renderSingleResource(template, tt.context)
			if (err != nil) != tt.wantErr {
				t.Errorf("renderSingleResource() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.checkFn != nil {
				tt.checkFn(t, got)
			}
		})
	}
}

func TestValidateResource(t *testing.T) {
	tests := []struct {
		name       string
		resource   map[string]any
		resourceID string
		wantErr    bool
	}{
		{
			name: "valid resource",
			resource: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "test",
				},
			},
			resourceID: "test",
			wantErr:    false,
		},
		{
			name: "missing kind",
			resource: map[string]any{
				"apiVersion": "v1",
				"metadata": map[string]any{
					"name": "test",
				},
			},
			resourceID: "test",
			wantErr:    true,
		},
		{
			name: "missing apiVersion",
			resource: map[string]any{
				"kind": "ConfigMap",
				"metadata": map[string]any{
					"name": "test",
				},
			},
			resourceID: "test",
			wantErr:    true,
		},
		{
			name: "missing metadata",
			resource: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
			},
			resourceID: "test",
			wantErr:    true,
		},
		{
			name: "missing metadata.name",
			resource: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]any{},
			},
			resourceID: "test",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResource(tt.resource, tt.resourceID)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateResource() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
