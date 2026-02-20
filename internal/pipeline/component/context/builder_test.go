// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
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
			envSettingsYAML: `
apiVersion: choreo.dev/v1alpha1
kind: ReleaseBinding
metadata:
  name: test-binding
`,
			environment: "dev",
			want: map[string]any{
				"parameters": map[string]any{
					"replicas": float64(3), // JSON numbers are float64
					"image":    "myapp:v1",
				},
				"envOverrides": map[string]any{}, // No envOverrides schema defined
				"dataplane": map[string]any{
					"publicVirtualHost": "api.example.com",
				},
				"environment": map[string]any{
					"publicVirtualHost": "api.example.com",
				},
				"metadata": map[string]any{
					"name":               "test-component-dev-12345678",
					"namespace":          "test-namespace",
					"componentName":      "test-component",
					"componentUID":       "a1b2c3d4-5678-90ab-cdef-1234567890ab",
					"componentNamespace": "test-namespace",
					"projectName":        "test-project",
					"projectUID":         "b2c3d4e5-6789-01bc-def0-234567890abc",
					"dataPlaneName":      "test-dataplane",
					"dataPlaneUID":       "c3d4e5f6-7890-12cd-ef01-34567890abcd",
					"environmentName":    "dev",
					"environmentUID":     "d4e5f6a7-8901-23de-f012-4567890abcde",
					"labels":             map[string]any{},
					"annotations":        map[string]any{},
					"podSelectors": map[string]any{
						"openchoreo.dev/component-uid": "a1b2c3d4-5678-90ab-cdef-1234567890ab",
					},
				},
				"workload": map[string]any{
					"container": map[string]any{},
					"endpoints": map[string]any{},
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
      cpu: "string | default=100m"
    envOverrides:
      replicas: "integer | default=1"
`,
			envSettingsYAML: `
apiVersion: choreo.dev/v1alpha1
kind: ReleaseBinding
metadata:
  name: test-component-prod
spec:
  componentTypeEnvOverrides:
    replicas: 5
`,
			environment: "prod",
			want: map[string]any{
				"parameters": map[string]any{
					"cpu": "100m", // From Component.Spec.Parameters
				},
				"envOverrides": map[string]any{
					"replicas": float64(5), // From ReleaseBinding.Spec.ComponentTypeEnvOverrides
				},
				"dataplane": map[string]any{
					"publicVirtualHost": "api.example.com",
				},
				"environment": map[string]any{
					"publicVirtualHost": "api.example.com",
				},
				"metadata": map[string]any{
					"name":               "test-component-dev-12345678",
					"namespace":          "test-namespace",
					"componentName":      "test-component",
					"componentUID":       "a1b2c3d4-5678-90ab-cdef-1234567890ab",
					"componentNamespace": "test-namespace",
					"projectName":        "test-project",
					"projectUID":         "b2c3d4e5-6789-01bc-def0-234567890abc",
					"dataPlaneName":      "test-dataplane",
					"dataPlaneUID":       "c3d4e5f6-7890-12cd-ef01-34567890abcd",
					"environmentName":    "prod",
					"environmentUID":     "d4e5f6a7-8901-23de-f012-4567890abcde",
					"labels":             map[string]any{},
					"annotations":        map[string]any{},
					"podSelectors": map[string]any{
						"openchoreo.dev/component-uid": "a1b2c3d4-5678-90ab-cdef-1234567890ab",
					},
				},
				"workload": map[string]any{
					"container": map[string]any{},
					"endpoints": map[string]any{},
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
  container:
    image: myapp:latest
`,
			envSettingsYAML: `
apiVersion: choreo.dev/v1alpha1
kind: ReleaseBinding
metadata:
  name: test-binding
`,
			environment: "dev",
			want: map[string]any{
				"parameters":   map[string]any{},
				"envOverrides": map[string]any{}, // No envOverrides schema defined
				"workload": map[string]any{
					"container": map[string]any{
						"image": "myapp:latest",
					},
					"endpoints": map[string]any{},
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
				"dataplane": map[string]any{
					"publicVirtualHost": "api.example.com",
				},
				"environment": map[string]any{
					"publicVirtualHost": "api.example.com",
				},
				"metadata": map[string]any{
					"name":               "test-component-dev-12345678",
					"namespace":          "test-namespace",
					"componentName":      "test-component",
					"componentUID":       "a1b2c3d4-5678-90ab-cdef-1234567890ab",
					"componentNamespace": "test-namespace",
					"projectName":        "test-project",
					"projectUID":         "b2c3d4e5-6789-01bc-def0-234567890abc",
					"dataPlaneName":      "test-dataplane",
					"dataPlaneUID":       "c3d4e5f6-7890-12cd-ef01-34567890abcd",
					"environmentName":    "dev",
					"environmentUID":     "d4e5f6a7-8901-23de-f012-4567890abcde",
					"labels":             map[string]any{},
					"annotations":        map[string]any{},
					"podSelectors": map[string]any{
						"openchoreo.dev/component-uid": "a1b2c3d4-5678-90ab-cdef-1234567890ab",
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build input from YAML
			input := &ComponentContextInput{
				DataPlane: &v1alpha1.DataPlane{
					Spec: v1alpha1.DataPlaneSpec{
						Gateway: v1alpha1.GatewaySpec{
							PublicVirtualHost: "api.example.com",
						},
					},
				},
				Environment: &v1alpha1.Environment{
					Spec: v1alpha1.EnvironmentSpec{
						DataPlaneRef: &v1alpha1.DataPlaneRef{
							Kind: v1alpha1.DataPlaneRefKindDataPlane,
							Name: "test-dataplane",
						},
					},
				},
				Metadata: MetadataContext{
					Name:               "test-component-dev-12345678",
					Namespace:          "test-namespace",
					ComponentName:      "test-component",
					ComponentUID:       "a1b2c3d4-5678-90ab-cdef-1234567890ab",
					ComponentNamespace: "test-namespace",
					ProjectName:        "test-project",
					ProjectUID:         "b2c3d4e5-6789-01bc-def0-234567890abc",
					DataPlaneName:      "test-dataplane",
					DataPlaneUID:       "c3d4e5f6-7890-12cd-ef01-34567890abcd",
					EnvironmentName:    tt.environment,
					EnvironmentUID:     "d4e5f6a7-8901-23de-f012-4567890abcde",
					Labels: func() map[string]string {
						if tt.additionalMetadata == nil {
							return map[string]string{}
						}
						return tt.additionalMetadata
					}(),
					Annotations: map[string]string{},
					PodSelectors: map[string]string{
						"openchoreo.dev/component-uid": "a1b2c3d4-5678-90ab-cdef-1234567890ab",
					},
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
				settings := &v1alpha1.ReleaseBinding{}
				if err := yaml.Unmarshal([]byte(tt.envSettingsYAML), settings); err != nil {
					t.Fatalf("Failed to parse ReleaseBinding YAML: %v", err)
				}
				input.ReleaseBinding = settings
			}

			// Parse workload and compute workload data + configurations (like pipeline would do)
			var workload *v1alpha1.Workload
			if tt.workloadYAML != "" {
				workload = &v1alpha1.Workload{}
				if err := yaml.Unmarshal([]byte(tt.workloadYAML), workload); err != nil {
					t.Fatalf("Failed to parse Workload YAML: %v", err)
				}
			}
			input.WorkloadData = ExtractWorkloadData(workload)
			input.Configurations = ExtractConfigurationsFromWorkload(nil, workload)

			got, err := BuildComponentContext(input)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildComponentContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Compare the entire result using cmp.Diff
			if diff := cmp.Diff(tt.want, got.ToMap()); diff != "" {
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
      parameters:
        database: mydb
`,
			instanceYAML: `
name: mysql-trait
instanceName: db-1
parameters:
  database: mydb
`,
			envSettingsYAML: `
apiVersion: choreo.dev/v1alpha1
kind: ReleaseBinding
metadata:
  name: test-binding
`,
			environment: "dev",
			want: map[string]any{
				"parameters": map[string]any{
					"database": "mydb",
				},
				"envOverrides": map[string]any{}, // No envOverrides schema defined
				"trait": map[string]any{
					"name":         "mysql-trait",
					"instanceName": "db-1",
				},
				"dataplane": map[string]any{
					"publicVirtualHost": "app.example.com",
					"secretStore":       "test-secret-store",
				},
				"environment": map[string]any{
					"publicVirtualHost": "app.example.com",
				},
				"metadata": map[string]any{
					"name":               "test-component-dev-12345678",
					"namespace":          "test-namespace",
					"componentName":      "test-component",
					"componentUID":       "a1b2c3d4-5678-90ab-cdef-1234567890ab",
					"componentNamespace": "test-namespace",
					"projectName":        "test-project",
					"projectUID":         "b2c3d4e5-6789-01bc-def0-234567890abc",
					"dataPlaneName":      "test-dataplane",
					"dataPlaneUID":       "c3d4e5f6-7890-12cd-ef01-34567890abcd",
					"environmentName":    "dev",
					"environmentUID":     "d4e5f6a7-8901-23de-f012-4567890abcde",
					"labels":             map[string]any{},
					"annotations":        map[string]any{},
					"podSelectors": map[string]any{
						"openchoreo.dev/component-uid": "a1b2c3d4-5678-90ab-cdef-1234567890ab",
					},
				},
				"workload": map[string]any{
					"container": map[string]any{},
					"endpoints": map[string]any{},
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
    envOverrides:
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
parameters:
  database: mydb
  size: small
`,
			envSettingsYAML: `
apiVersion: choreo.dev/v1alpha1
kind: ReleaseBinding
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
				},
				"envOverrides": map[string]any{
					"size": "large", // From ReleaseBinding.Spec.TraitOverrides
				},
				"trait": map[string]any{
					"name":         "mysql-trait",
					"instanceName": "db-1",
				},
				"dataplane": map[string]any{
					"publicVirtualHost": "app.example.com",
					"secretStore":       "test-secret-store",
				},
				"environment": map[string]any{
					"publicVirtualHost": "app.example.com",
				},
				"metadata": map[string]any{
					"name":               "test-component-dev-12345678",
					"namespace":          "test-namespace",
					"componentName":      "test-component",
					"componentUID":       "a1b2c3d4-5678-90ab-cdef-1234567890ab",
					"componentNamespace": "test-namespace",
					"projectName":        "test-project",
					"projectUID":         "b2c3d4e5-6789-01bc-def0-234567890abc",
					"dataPlaneName":      "test-dataplane",
					"dataPlaneUID":       "c3d4e5f6-7890-12cd-ef01-34567890abcd",
					"environmentName":    "dev",
					"environmentUID":     "d4e5f6a7-8901-23de-f012-4567890abcde",
					"labels":             map[string]any{},
					"annotations":        map[string]any{},
					"podSelectors": map[string]any{
						"openchoreo.dev/component-uid": "a1b2c3d4-5678-90ab-cdef-1234567890ab",
					},
				},
				"workload": map[string]any{
					"container": map[string]any{},
					"endpoints": map[string]any{},
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
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build input from YAML
			input := &TraitContextInput{
				Metadata: MetadataContext{
					Name:               "test-component-dev-12345678",
					Namespace:          "test-namespace",
					ComponentName:      "test-component",
					ComponentUID:       "a1b2c3d4-5678-90ab-cdef-1234567890ab",
					ComponentNamespace: "test-namespace",
					ProjectName:        "test-project",
					ProjectUID:         "b2c3d4e5-6789-01bc-def0-234567890abc",
					DataPlaneName:      "test-dataplane",
					DataPlaneUID:       "c3d4e5f6-7890-12cd-ef01-34567890abcd",
					EnvironmentName:    "dev",
					EnvironmentUID:     "d4e5f6a7-8901-23de-f012-4567890abcde",
					Labels: func() map[string]string {
						if tt.additionalMetadata == nil {
							return map[string]string{}
						}
						return tt.additionalMetadata
					}(),
					Annotations: map[string]string{},
					PodSelectors: map[string]string{
						"openchoreo.dev/component-uid": "a1b2c3d4-5678-90ab-cdef-1234567890ab",
					},
				},
				DataPlane: &v1alpha1.DataPlane{
					Spec: v1alpha1.DataPlaneSpec{
						Gateway: v1alpha1.GatewaySpec{
							PublicVirtualHost: "app.example.com",
						},
						SecretStoreRef: &v1alpha1.SecretStoreRef{
							Name: "test-secret-store",
						},
					},
				},
				Environment: &v1alpha1.Environment{
					Spec: v1alpha1.EnvironmentSpec{
						DataPlaneRef: &v1alpha1.DataPlaneRef{
							Kind: v1alpha1.DataPlaneRefKindDataPlane,
							Name: "test-dataplane",
						},
					},
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
				settings := &v1alpha1.ReleaseBinding{}
				if err := yaml.Unmarshal([]byte(tt.envSettingsYAML), settings); err != nil {
					t.Fatalf("Failed to parse ReleaseBinding YAML: %v", err)
				}
				input.ReleaseBinding = settings
			}

			// Compute workload data and configurations (like pipeline would do)
			// These tests don't have workloads, so both will be empty
			input.WorkloadData = ExtractWorkloadData(nil)
			input.Configurations = ExtractConfigurationsFromWorkload(nil, nil)

			traitCtx, err := BuildTraitContext(input)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildTraitContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Convert to map for comparison
			got := traitCtx.ToMap()

			// Compare the entire result using cmp.Diff
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("BuildTraitContext() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// Helper functions
