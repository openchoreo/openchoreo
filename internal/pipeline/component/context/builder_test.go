// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestBuildComponentContext(t *testing.T) {
	tests := []struct {
		name               string
		componentYAML      string
		componentTypeYAML  string
		envSettingsYAML    string
		workloadYAML       string
		environment        string
		additionalMetadata map[string]string
		want               map[string]any
		wantErr            bool
	}{
		{
			name: "basic component with parameters",
			componentYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Component
metadata:
  name: test-component
  namespace: default
spec:
  type: service
  parameters:
    replicas: 3
    image: myapp:v1
`,
			componentTypeYAML: `
apiVersion: choreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: service
spec:
  schema:
    parameters:
      replicas: "integer | default=1"
      image: "string"
`,
			environment: "dev",
			want: map[string]any{
				"parameters": map[string]any{
					"replicas": float64(3), // JSON numbers are float64
					"image":    "myapp:v1",
				},
				"component": map[string]any{
					"name":      "test-component",
					"namespace": "default",
				},
				"environment": map[string]any{
					"name":  "dev",
					"vhost": "api.example.com",
				},
				"metadata": map[string]any{
					"name":      "test-component-dev-12345678",
					"namespace": "test-namespace",
				},
			},
			wantErr: false,
		},
		{
			name: "component with environment overrides",
			componentYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Component
metadata:
  name: test-component
spec:
  type: service
  parameters:
    replicas: 3
    cpu: "100m"
`,
			componentTypeYAML: `
apiVersion: choreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: service
spec:
  schema:
    parameters:
      replicas: "integer | default=1"
      cpu: "string | default=100m"
`,
			envSettingsYAML: `
apiVersion: choreo.dev/v1alpha1
kind: ComponentDeployment
metadata:
  name: test-component-prod
spec:
  overrides:
    replicas: 5
`,
			environment: "prod",
			want: map[string]any{
				"parameters": map[string]any{
					"replicas": float64(5), // Override applied
					"cpu":      "100m",     // Base value preserved
				},
				"component": map[string]any{
					"name": "test-component",
				},
				"environment": map[string]any{
					"name":  "prod",
					"vhost": "api.example.com",
				},
				"metadata": map[string]any{
					"name":      "test-component-dev-12345678",
					"namespace": "test-namespace",
				},
			},
			wantErr: false,
		},
		{
			name: "component with workload",
			componentYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Component
metadata:
  name: test-component
spec:
  type: service
  parameters: {}
`,
			componentTypeYAML: `
apiVersion: choreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: service
spec:
  schema:
    parameters: {}
`,
			workloadYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Workload
metadata:
  name: test-workload
spec:
  containers:
    app:
      image: myapp:latest
`,
			environment: "dev",
			want: map[string]any{
				"parameters": map[string]any{},
				"component": map[string]any{
					"name": "test-component",
				},
				"workload": map[string]any{
					"name": "test-workload",
					"containers": map[string]any{
						"app": map[string]any{
							"image": "myapp:latest",
						},
					},
				},
				"configurations": map[string]any{
					"configs": map[string]any{
						"envs":  []any{},
						"files": []any{},
					},
					"secrets": map[string]any{
						"envs":  []any{},
						"files": []any{},
					},
				},
				"environment": map[string]any{
					"name":  "dev",
					"vhost": "api.example.com",
				},
				"metadata": map[string]any{
					"name":      "test-component-dev-12345678",
					"namespace": "test-namespace",
				},
			},
			wantErr: false,
		},
		{
			name: "nil component type",
			componentYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Component
metadata:
  name: test-component
spec:
  type: service
`,
			componentTypeYAML: "", // Empty to test nil
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build input from YAML
			input := &ComponentContextInput{
				Environment: EnvironmentContext{
					Name:        tt.environment,
					VirtualHost: "api.example.com",
				},
				Metadata: MetadataContext{
					Name:      "test-component-dev-12345678",
					Namespace: "test-namespace",
					Labels:    tt.additionalMetadata,
				},
			}

			// Parse component
			if tt.componentYAML != "" {
				comp := &v1alpha1.Component{}
				if err := yaml.Unmarshal([]byte(tt.componentYAML), comp); err != nil {
					t.Fatalf("Failed to parse component YAML: %v", err)
				}
				input.Component = comp
			}

			// Parse component type
			if tt.componentTypeYAML != "" {
				ct := &v1alpha1.ComponentType{}
				if err := yaml.Unmarshal([]byte(tt.componentTypeYAML), ct); err != nil {
					t.Fatalf("Failed to parse ComponentType YAML: %v", err)
				}
				input.ComponentType = ct
			}

			// Parse env settings
			if tt.envSettingsYAML != "" {
				settings := &v1alpha1.ComponentDeployment{}
				if err := yaml.Unmarshal([]byte(tt.envSettingsYAML), settings); err != nil {
					t.Fatalf("Failed to parse ComponentDeployment YAML: %v", err)
				}
				input.ComponentDeployment = settings
			}

			// Parse workload
			if tt.workloadYAML != "" {
				workload := &v1alpha1.Workload{}
				if err := yaml.Unmarshal([]byte(tt.workloadYAML), workload); err != nil {
					t.Fatalf("Failed to parse Workload YAML: %v", err)
				}
				input.Workload = workload
			}

			got, err := BuildComponentContext(input)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildComponentContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Compare the entire result using cmp.Diff
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("BuildComponentContext() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildTraitContext(t *testing.T) {
	tests := []struct {
		name               string
		traitYAML          string
		componentYAML      string
		instanceYAML       string
		envSettingsYAML    string
		environment        string
		additionalMetadata map[string]string
		want               map[string]any
		wantErr            bool
	}{
		{
			name: "basic trait with parameters",
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: mysql-trait
spec:
  schema:
    parameters:
      database: "string"
`,
			componentYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Component
metadata:
  name: test-component
spec:
  type: service
  traits:
    - name: mysql-trait
      instanceName: db-1
      config:
        database: mydb
`,
			instanceYAML: `
name: mysql-trait
instanceName: db-1
config:
  database: mydb
`,
			environment: "dev",
			want: map[string]any{
				"parameters": map[string]any{
					"database": "mydb",
				},
				"trait": map[string]any{
					"name":         "mysql-trait",
					"instanceName": "db-1",
				},
				"component": map[string]any{
					"name": "test-component",
				},
				"environment": map[string]any{
					"name":  "dev",
					"vhost": "api.example.com",
				},
				"metadata": map[string]any{
					"name":      "test-component-dev-12345678",
					"namespace": "test-namespace",
				},
			},
			wantErr: false,
		},
		{
			name: "trait with environment overrides",
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: mysql-trait
spec:
  schema:
    parameters:
      database: "string"
      size: "string | default=small"
`,
			componentYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Component
metadata:
  name: test-component
spec:
  type: service
`,
			instanceYAML: `
name: mysql-trait
instanceName: db-1
config:
  database: mydb
  size: small
`,
			envSettingsYAML: `
apiVersion: choreo.dev/v1alpha1
kind: ComponentDeployment
metadata:
  name: test-component-prod
spec:
  traitOverrides:
    db-1:
      size: large
`,
			environment: "prod",
			want: map[string]any{
				"parameters": map[string]any{
					"database": "mydb",
					"size":     "large", // Override applied
				},
				"trait": map[string]any{
					"name":         "mysql-trait",
					"instanceName": "db-1",
				},
				"component": map[string]any{
					"name": "test-component",
				},
				"environment": map[string]any{
					"name":  "prod",
					"vhost": "api.example.com",
				},
				"metadata": map[string]any{
					"name":      "test-component-dev-12345678",
					"namespace": "test-namespace",
				},
			},
			wantErr: false,
		},
		{
			name: "nil trait input",
			componentYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Component
metadata:
  name: test-component
spec:
  type: service
`,
			traitYAML: "", // Empty to test nil
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build input from YAML
			input := &TraitContextInput{
				Environment: EnvironmentContext{
					Name:        tt.environment,
					VirtualHost: "api.example.com",
				},
				Metadata: MetadataContext{
					Name:      "test-component-dev-12345678",
					Namespace: "test-namespace",
					Labels:    tt.additionalMetadata,
				},
			}

			// Parse trait
			if tt.traitYAML != "" {
				trait := &v1alpha1.Trait{}
				if err := yaml.Unmarshal([]byte(tt.traitYAML), trait); err != nil {
					t.Fatalf("Failed to parse Trait YAML: %v", err)
				}
				input.Trait = trait
			}

			// Parse component
			if tt.componentYAML != "" {
				comp := &v1alpha1.Component{}
				if err := yaml.Unmarshal([]byte(tt.componentYAML), comp); err != nil {
					t.Fatalf("Failed to parse Component YAML: %v", err)
				}
				input.Component = comp
			}

			// Parse trait instance
			if tt.instanceYAML != "" {
				instance := v1alpha1.ComponentTrait{}
				if err := yaml.Unmarshal([]byte(tt.instanceYAML), &instance); err != nil {
					t.Fatalf("Failed to parse trait instance YAML: %v", err)
				}
				input.Instance = instance
			}

			// Parse env settings
			if tt.envSettingsYAML != "" {
				settings := &v1alpha1.ComponentDeployment{}
				if err := yaml.Unmarshal([]byte(tt.envSettingsYAML), settings); err != nil {
					t.Fatalf("Failed to parse ComponentDeployment YAML: %v", err)
				}
				input.ComponentDeployment = settings
			}

			got, err := BuildTraitContext(input)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildTraitContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Compare the entire result using cmp.Diff
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("BuildTraitContext() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDeepMerge(t *testing.T) {
	tests := []struct {
		name     string
		base     map[string]any
		override map[string]any
		want     map[string]any
	}{
		{
			name: "simple merge",
			base: map[string]any{
				"a": 1,
				"b": 2,
			},
			override: map[string]any{
				"b": 3,
				"c": 4,
			},
			want: map[string]any{
				"a": 1,
				"b": 3,
				"c": 4,
			},
		},
		{
			name: "nested merge",
			base: map[string]any{
				"config": map[string]any{
					"replicas": 1,
					"cpu":      "100m",
				},
			},
			override: map[string]any{
				"config": map[string]any{
					"replicas": 3,
				},
			},
			want: map[string]any{
				"config": map[string]any{
					"replicas": 3,
					"cpu":      "100m",
				},
			},
		},
		{
			name: "override with different type",
			base: map[string]any{
				"value": "string",
			},
			override: map[string]any{
				"value": 123,
			},
			want: map[string]any{
				"value": 123,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deepMerge(tt.base, tt.override)

			// Convert to JSON for easy comparison
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tt.want)

			if string(gotJSON) != string(wantJSON) {
				t.Errorf("deepMerge() = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

// Helper functions
