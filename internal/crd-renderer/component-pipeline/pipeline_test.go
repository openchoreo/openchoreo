// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentpipeline

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/crd-renderer/component-pipeline/context"
)

func TestPipeline_Render(t *testing.T) {
	tests := []struct {
		name             string
		snapshotYAML     string
		settingsYAML     string
		wantErr          bool
		wantResourceYAML string
	}{
		{
			name: "simple component without addons",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: dev
  component:
    metadata:
      name: test-app
    spec:
      parameters:
        replicas: 2
  componentTypeDefinition:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: ${component.name}
            spec:
              replicas: ${parameters.replicas}
  workload: {}
`,
			wantResourceYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: test-app
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
  spec:
    replicas: 2
`,
			wantErr: false,
		},
		{
			name: "component with environment overrides",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: prod
  component:
    metadata:
      name: test-app
    spec:
      parameters:
        replicas: 2
  componentTypeDefinition:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: ${component.name}
            spec:
              replicas: ${parameters.replicas}
  workload: {}
`,
			settingsYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentDeployment
spec:
  overrides:
    replicas: 5
`,
			wantResourceYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: test-app
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: prod
  spec:
    replicas: 5
`,
			wantErr: false,
		},
		{
			name: "component with includeWhen",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: dev
  component:
    metadata:
      name: test-app
    spec:
      parameters:
        expose: true
  componentTypeDefinition:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: ${component.name}
        - id: service
          includeWhen: ${parameters.expose}
          template:
            apiVersion: v1
            kind: Service
            metadata:
              name: ${component.name}
  workload: {}
`,
			wantResourceYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: test-app
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
- apiVersion: v1
  kind: Service
  metadata:
    name: test-app
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
`,
			wantErr: false,
		},
		{
			name: "component with forEach",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: dev
  component:
    metadata:
      name: test-app
    spec:
      parameters:
        secrets:
          - secret1
          - secret2
  componentTypeDefinition:
    spec:
      resources:
        - id: secrets
          forEach: ${parameters.secrets}
          var: secret
          template:
            apiVersion: v1
            kind: Secret
            metadata:
              name: ${secret}
  workload: {}
`,
			wantResourceYAML: `
- apiVersion: v1
  kind: Secret
  metadata:
    name: secret1
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
- apiVersion: v1
  kind: Secret
  metadata:
    name: secret2
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
`,
			wantErr: false,
		},
		{
			name: "component with addon creates",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: dev
  component:
    metadata:
      name: test-app
    spec:
      parameters:
        replicas: 2
      addons:
        - name: mysql
          instanceName: db-1
          config:
            database: mydb
  componentTypeDefinition:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: ${component.name}
  addons:
    - metadata:
        name: mysql
      spec:
        creates:
          - template:
              apiVersion: v1
              kind: Secret
              metadata:
                name: ${addon.instanceName}-secret
              data:
                database: ${parameters.database}
  workload: {}
`,
			wantResourceYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: test-app
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
- apiVersion: v1
  kind: Secret
  metadata:
    name: db-1-secret
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
  data:
    database: mydb
`,
			wantErr: false,
		},
		{
			name: "component with addon patches",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: dev
  component:
    metadata:
      name: test-app
    spec:
      parameters: {}
      addons:
        - name: monitoring
          instanceName: mon-1
          config: {}
  componentTypeDefinition:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: app
            spec:
              template:
                spec:
                  containers:
                    - name: app
                      image: myapp:latest
  addons:
    - metadata:
        name: monitoring
      spec:
        patches:
          - target:
              kind: Deployment
              group: apps
              version: v1
            operations:
              - op: add
                path: /metadata/labels
                value:
                  monitoring: enabled
  workload: {}
`,
			wantResourceYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: app
    labels:
      monitoring: enabled
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
  spec:
    template:
      spec:
        containers:
          - name: app
            image: myapp:latest
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse snapshot
			snapshot := &v1alpha1.ComponentEnvSnapshot{}
			if err := yaml.Unmarshal([]byte(tt.snapshotYAML), snapshot); err != nil {
				t.Fatalf("Failed to parse snapshot YAML: %v", err)
			}

			// Parse settings if provided
			var settings *v1alpha1.ComponentDeployment
			if tt.settingsYAML != "" {
				settings = &v1alpha1.ComponentDeployment{}
				if err := yaml.Unmarshal([]byte(tt.settingsYAML), settings); err != nil {
					t.Fatalf("Failed to parse settings YAML: %v", err)
				}
			}

			// Create input
			input := &RenderInput{
				Snapshot:            snapshot,
				ComponentDeployment: settings,
				Metadata: context.MetadataContext{
					Name:      "test-component-dev-12345678",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"openchoreo.org/component":   "test-component",
						"openchoreo.org/environment": "dev",
					},
				},
			}

			// Create pipeline and render
			pipeline := NewPipeline()
			output, err := pipeline.Render(input)

			if (err != nil) != tt.wantErr {
				t.Errorf("Render() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.wantResourceYAML != "" {
				// Parse expected resources
				var wantResources []map[string]any
				if err := yaml.Unmarshal([]byte(tt.wantResourceYAML), &wantResources); err != nil {
					t.Fatalf("Failed to parse wantResourceYAML: %v", err)
				}

				// Compare actual vs expected
				if diff := cmp.Diff(wantResources, output.Resources); diff != "" {
					t.Errorf("Resources mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestPipeline_Options(t *testing.T) {
	tests := []struct {
		name             string
		snapshotYAML     string
		options          []Option
		wantResourceYAML string
	}{
		{
			name: "with custom labels",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: dev
  component:
    metadata:
      name: test-app
    spec:
      parameters: {}
  componentTypeDefinition:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: app
  workload: {}
`,
			options: []Option{
				WithResourceLabels(map[string]string{
					"custom": "label",
				}),
			},
			wantResourceYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: app
    labels:
      custom: label
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
`,
		},
		{
			name: "with custom annotations",
			snapshotYAML: `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: dev
  component:
    metadata:
      name: test-app
    spec:
      parameters: {}
  componentTypeDefinition:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: app
  workload: {}
`,
			options: []Option{
				WithResourceAnnotations(map[string]string{
					"custom": "annotation",
				}),
			},
			wantResourceYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: app
    labels:
      openchoreo.org/component: test-app
      openchoreo.org/environment: dev
    annotations:
      custom: annotation
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse snapshot
			snapshot := &v1alpha1.ComponentEnvSnapshot{}
			if err := yaml.Unmarshal([]byte(tt.snapshotYAML), snapshot); err != nil {
				t.Fatalf("Failed to parse snapshot YAML: %v", err)
			}

			// Create input
			input := &RenderInput{
				Snapshot: snapshot,
				Metadata: context.MetadataContext{
					Name:      "test-component-dev-12345678",
					Namespace: "test-namespace",
					Labels: map[string]string{
						"openchoreo.org/component":   "test-component",
						"openchoreo.org/environment": "dev",
					},
				},
			}

			// Create pipeline with options
			pipeline := NewPipeline(tt.options...)
			output, err := pipeline.Render(input)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			// Parse expected resources
			var wantResources []map[string]any
			if err := yaml.Unmarshal([]byte(tt.wantResourceYAML), &wantResources); err != nil {
				t.Fatalf("Failed to parse wantResourceYAML: %v", err)
			}

			// Compare actual vs expected
			if diff := cmp.Diff(wantResources, output.Resources); diff != "" {
				t.Errorf("Resources mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValidateResources(t *testing.T) {
	tests := []struct {
		name      string
		resources []map[string]any
		wantErr   bool
	}{
		{
			name: "valid resources",
			resources: []map[string]any{
				{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]any{
						"name": "test",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing apiVersion",
			resources: []map[string]any{
				{
					"kind": "Pod",
					"metadata": map[string]any{
						"name": "test",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing kind",
			resources: []map[string]any{
				{
					"apiVersion": "v1",
					"metadata": map[string]any{
						"name": "test",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing metadata.name",
			resources: []map[string]any{
				{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata":   map[string]any{},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPipeline()
			err := p.validateResources(tt.resources)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateResources() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSortResources(t *testing.T) {
	resources := []map[string]any{
		{
			"kind":       "Service",
			"apiVersion": "v1",
			"metadata": map[string]any{
				"name": "svc-b",
			},
		},
		{
			"kind":       "Deployment",
			"apiVersion": "apps/v1",
			"metadata": map[string]any{
				"name": "deploy-a",
			},
		},
		{
			"kind":       "Service",
			"apiVersion": "v1",
			"metadata": map[string]any{
				"name": "svc-a",
			},
		},
	}

	sortResources(resources)

	// Check sorted order: Deployment first, then Services sorted by name
	if resources[0]["kind"] != "Deployment" {
		t.Errorf("Expected Deployment first, got %v", resources[0]["kind"])
	}
	if resources[1]["kind"] != "Service" {
		t.Errorf("Expected Service second, got %v", resources[1]["kind"])
	}

	metadata := resources[1]["metadata"].(map[string]any)
	if metadata["name"] != "svc-a" {
		t.Errorf("Expected svc-a second, got %v", metadata["name"])
	}
}
