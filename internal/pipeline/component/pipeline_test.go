// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/context"
)

// testSnapshot is a test-only struct for parsing legacy ComponentEnvSnapshot YAML in tests
type testSnapshot struct {
	Spec struct {
		Component     v1alpha1.Component     `json:"component"`
		ComponentType v1alpha1.ComponentType `json:"componentType"`
		Workload      v1alpha1.Workload      `json:"workload"`
		Traits        []v1alpha1.Trait       `json:"traits,omitempty"`
	} `json:"spec"`
}

// loadTestDataFile loads a file from the testdata directory
func loadTestDataFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", path))
	if err != nil {
		t.Fatalf("Failed to read testdata file %s: %v", path, err)
	}
	return string(data)
}

func TestPipeline_Render(t *testing.T) {
	devEnvironmentYAML := `
    apiVersion: openchoreo.dev/v1alpha1
    kind: Environment
    metadata:
      name: dev
      namespace: test-namespace
    spec:
      dataPlaneRef: dev-dataplane
      isProduction: false
      gateway:
        dnsPrefix: dev
        security:
          remoteJwks:
            uri: https://auth.example.com/.well-known/jwks.json`
	devDataplaneYAML := `
    apiVersion: openchoreo.dev/v1alpha1
    kind: DataPlane
    metadata:
      name: dev-dataplane
      namespace: test-namespace
    spec:
      kubernetesCluster:
        name: development-cluster
        credentials:
          apiServerURL: https://k8s-api.example.com:6443
          caCert: LS0tLS1CRUdJTi
          clientCert: LS0tLS1CRUdJTi
          clientKey: LS0tLS1CRUdJTi
      registry:
        prefix: docker.io/myorg
        secretRef: registry-credentials
      gateway:
        publicVirtualHost: api.example.com
        organizationVirtualHost: internal.example.com
      observer:
        url: https://observer.example.com
        authentication:
          basicAuth:
            username: admin
            password: secretpassword
      secretStoreRef:
        name: dev-vault-store`
	tests := []struct {
		name                 string
		snapshotYAML         string
		settingsYAML         string
		wantErr              bool
		wantResourceYAML     string
		environmentYAML      string
		dataplaneYAML        string
		secretReferencesYAML string
	}{
		{
			name: "simple component without traits",
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
  componentType:
    spec:
      schema:
        parameters:
          replicas: "integer"
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: ${metadata.name}
            spec:
              replicas: ${parameters.replicas}
  workload: {}
`,
			environmentYAML: devEnvironmentYAML,
			dataplaneYAML:   devDataplaneYAML,
			wantResourceYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: test-component-dev-12345678
    labels:
      openchoreo.dev/component: test-app
      openchoreo.dev/environment: dev
      openchoreo.dev/project: test-project
  spec:
    replicas: 2
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
  componentType:
    spec:
      schema:
        parameters:
          expose: "boolean"
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: ${metadata.name}
        - id: service
          includeWhen: ${parameters.expose}
          template:
            apiVersion: v1
            kind: Service
            metadata:
              name: ${metadata.name}
  workload: {}
`,
			environmentYAML: devEnvironmentYAML,
			dataplaneYAML:   devDataplaneYAML,
			wantResourceYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: test-component-dev-12345678
    labels:
      openchoreo.dev/component: test-app
      openchoreo.dev/environment: dev
      openchoreo.dev/project: test-project
- apiVersion: v1
  kind: Service
  metadata:
    name: test-component-dev-12345678
    labels:
      openchoreo.dev/component: test-app
      openchoreo.dev/environment: dev
      openchoreo.dev/project: test-project
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
  componentType:
    spec:
      schema:
        parameters:
          secrets: "[]string"
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
			environmentYAML: devEnvironmentYAML,
			dataplaneYAML:   devDataplaneYAML,
			wantResourceYAML: `
- apiVersion: v1
  kind: Secret
  metadata:
    name: secret1
    labels:
      openchoreo.dev/component: test-app
      openchoreo.dev/environment: dev
      openchoreo.dev/project: test-project
- apiVersion: v1
  kind: Secret
  metadata:
    name: secret2
    labels:
      openchoreo.dev/component: test-app
      openchoreo.dev/environment: dev
      openchoreo.dev/project: test-project
`,
			wantErr: false,
		},
		{
			name: "component with trait creates",
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
      traits:
        - name: mysql
          instanceName: db-1
          parameters:
            database: mydb
  componentType:
    spec:
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: ${metadata.name}
  traits:
    - metadata:
        name: mysql
      spec:
        schema:
          parameters:
            database: "string"
        creates:
          - template:
              apiVersion: v1
              kind: Secret
              metadata:
                name: ${trait.instanceName}-secret
              data:
                database: ${parameters.database}
  workload: {}
`,
			environmentYAML: devEnvironmentYAML,
			dataplaneYAML:   devDataplaneYAML,
			wantResourceYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: test-component-dev-12345678
    labels:
      openchoreo.dev/component: test-app
      openchoreo.dev/environment: dev
      openchoreo.dev/project: test-project
- apiVersion: v1
  kind: Secret
  metadata:
    name: db-1-secret
    labels:
      openchoreo.dev/component: test-app
      openchoreo.dev/environment: dev
      openchoreo.dev/project: test-project
  data:
    database: mydb
`,
			wantErr: false,
		},
		{
			name: "component with trait patches",
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
      traits:
        - name: monitoring
          instanceName: mon-1
          config: {}
  componentType:
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
  traits:
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
			environmentYAML: devEnvironmentYAML,
			dataplaneYAML:   devDataplaneYAML,
			wantResourceYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: app
    labels:
      monitoring: enabled
      openchoreo.dev/component: test-app
      openchoreo.dev/environment: dev
      openchoreo.dev/project: test-project
  spec:
    template:
      spec:
        containers:
          - name: app
            image: myapp:latest
`,
			wantErr: false,
		},
		{
			name:                 "component with configurations and secrets",
			snapshotYAML:         loadTestDataFile(t, "configurations-and-secrets/snapshot.yaml"),
			settingsYAML:         loadTestDataFile(t, "configurations-and-secrets/settings.yaml"),
			environmentYAML:      devEnvironmentYAML,
			dataplaneYAML:        devDataplaneYAML,
			secretReferencesYAML: loadTestDataFile(t, "configurations-and-secrets/secret-references.yaml"),
			wantResourceYAML:     loadTestDataFile(t, "configurations-and-secrets/expected-resources.yaml"),
			wantErr:              false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse snapshot (using test-only struct for legacy YAML format)
			snapshot := &testSnapshot{}
			if err := yaml.Unmarshal([]byte(tt.snapshotYAML), snapshot); err != nil {
				t.Fatalf("Failed to parse snapshot YAML: %v", err)
			}

			// Parse settings if provided
			var settings *v1alpha1.ReleaseBinding
			if tt.settingsYAML != "" {
				settings = &v1alpha1.ReleaseBinding{}
				if err := yaml.Unmarshal([]byte(tt.settingsYAML), settings); err != nil {
					t.Fatalf("Failed to parse settings YAML: %v", err)
				}
			}

			// Parse environment
			var environment *v1alpha1.Environment
			if tt.environmentYAML != "" {
				environment = &v1alpha1.Environment{}
				if err := yaml.Unmarshal([]byte(tt.environmentYAML), environment); err != nil {
					t.Fatalf("Failed to parse environment YAML: %v", err)
				}
			}

			// Parse dataplane
			var dataplane *v1alpha1.DataPlane
			if tt.dataplaneYAML != "" {
				dataplane = &v1alpha1.DataPlane{}
				if err := yaml.Unmarshal([]byte(tt.dataplaneYAML), dataplane); err != nil {
					t.Fatalf("Failed to parse dataplane YAML: %v", err)
				}
			}

			// Parse secret references if provided
			var secretReferences map[string]*v1alpha1.SecretReference
			if tt.secretReferencesYAML != "" {
				var refs []v1alpha1.SecretReference
				if err := yaml.Unmarshal([]byte(tt.secretReferencesYAML), &refs); err != nil {
					t.Fatalf("Failed to parse secretReferences YAML: %v", err)
				}
				secretReferences = make(map[string]*v1alpha1.SecretReference)
				for i := range refs {
					secretReferences[refs[i].Name] = &refs[i]
				}
			}

			// Create input
			input := &RenderInput{
				ComponentType:    &snapshot.Spec.ComponentType,
				Component:        &snapshot.Spec.Component,
				Traits:           snapshot.Spec.Traits,
				Workload:         &snapshot.Spec.Workload,
				Environment:      environment,
				DataPlane:        dataplane,
				ReleaseBinding:   settings,
				SecretReferences: secretReferences,
				Metadata: context.MetadataContext{
					Name:            "test-component-dev-12345678",
					Namespace:       "test-namespace",
					ComponentName:   "test-app",
					ComponentUID:    "a1b2c3d4-5678-90ab-cdef-1234567890ab",
					ProjectName:     "test-project",
					ProjectUID:      "b2c3d4e5-6789-01bc-def0-234567890abc",
					DataPlaneName:   "dev-dataplane",
					DataPlaneUID:    "c3d4e5f6-7890-12cd-ef01-34567890abcd",
					EnvironmentName: "dev",
					EnvironmentUID:  "d4e5f6a7-8901-23de-f012-4567890abcde",
					Labels: map[string]string{
						"openchoreo.dev/component":   "test-component",
						"openchoreo.dev/environment": "dev",
					},
					Annotations: map[string]string{},
					PodSelectors: map[string]string{
						"openchoreo.dev/component-uid": "a1b2c3d4-5678-90ab-cdef-1234567890ab",
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

				if diff := cmp.Diff(wantResources, output.Resources, sortAnySlicesByName()); diff != "" {
					t.Errorf("Resources mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestPipeline_Options(t *testing.T) {
	devEnvironmentYAML := `
    apiVersion: openchoreo.dev/v1alpha1
    kind: Environment
    metadata:
      name: dev
      namespace: test-namespace
    spec:
      dataPlaneRef: dev-dataplane
      isProduction: false
      gateway:
        dnsPrefix: dev
        security:
          remoteJwks:
            uri: https://auth.example.com/.well-known/jwks.json`
	devDataplaneYAML := `
    apiVersion: openchoreo.dev/v1alpha1
    kind: DataPlane
    metadata:
      name: dev-dataplane
      namespace: test-namespace
    spec:
      kubernetesCluster:
        name: development-cluster
        credentials:
          apiServerURL: https://k8s-api.example.com:6443
          caCert: LS0tLS1CRUdJTi
          clientCert: LS0tLS1CRUdJTi
          clientKey: LS0tLS1CRUdJTi
      registry:
        prefix: docker.io/myorg
        secretRef: registry-credentials
      gateway:
        publicVirtualHost: api.example.com
        organizationVirtualHost: internal.example.com
      observer:
        url: https://observer.example.com
        authentication:
          basicAuth:
            username: admin
            password: secretpassword`
	tests := []struct {
		name             string
		snapshotYAML     string
		options          []Option
		wantResourceYAML string
		environmentYAML  string
		dataplaneYAML    string
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
  componentType:
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
			environmentYAML: devEnvironmentYAML,
			dataplaneYAML:   devDataplaneYAML,
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
      openchoreo.dev/component: test-app
      openchoreo.dev/environment: dev
      openchoreo.dev/project: test-project
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
  componentType:
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
			environmentYAML: devEnvironmentYAML,
			dataplaneYAML:   devDataplaneYAML,
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
      openchoreo.dev/component: test-app
      openchoreo.dev/environment: dev
      openchoreo.dev/project: test-project
    annotations:
      custom: annotation
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse snapshot (using test-only struct for legacy YAML format)
			snapshot := &testSnapshot{}
			if err := yaml.Unmarshal([]byte(tt.snapshotYAML), snapshot); err != nil {
				t.Fatalf("Failed to parse snapshot YAML: %v", err)
			}

			// Parse environment
			var environment *v1alpha1.Environment
			if tt.environmentYAML != "" {
				environment = &v1alpha1.Environment{}
				if err := yaml.Unmarshal([]byte(tt.environmentYAML), environment); err != nil {
					t.Fatalf("Failed to parse environment YAML: %v", err)
				}
			}

			// Parse dataplane
			var dataplane *v1alpha1.DataPlane
			if tt.dataplaneYAML != "" {
				dataplane = &v1alpha1.DataPlane{}
				if err := yaml.Unmarshal([]byte(tt.dataplaneYAML), dataplane); err != nil {
					t.Fatalf("Failed to parse dataplane YAML: %v", err)
				}
			}

			// Create input
			input := &RenderInput{
				ComponentType: &snapshot.Spec.ComponentType,
				Component:     &snapshot.Spec.Component,
				Traits:        snapshot.Spec.Traits,
				Workload:      &snapshot.Spec.Workload,
				Environment:   environment,
				DataPlane:     dataplane,
				Metadata: context.MetadataContext{
					Name:            "test-component-dev-12345678",
					Namespace:       "test-namespace",
					ComponentName:   "test-app",
					ComponentUID:    "a1b2c3d4-5678-90ab-cdef-1234567890ab",
					ProjectName:     "test-project",
					ProjectUID:      "b2c3d4e5-6789-01bc-def0-234567890abc",
					DataPlaneName:   "dev-dataplane",
					DataPlaneUID:    "c3d4e5f6-7890-12cd-ef01-34567890abcd",
					EnvironmentName: "dev",
					EnvironmentUID:  "d4e5f6a7-8901-23de-f012-4567890abcde",
					Labels: map[string]string{
						"openchoreo.dev/component":   "test-component",
						"openchoreo.dev/environment": "dev",
					},
					Annotations: map[string]string{},
					PodSelectors: map[string]string{
						"openchoreo.dev/component-uid": "a1b2c3d4-5678-90ab-cdef-1234567890ab",
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

// compareByKey compares two items by their key field ("name" or "secretKey").
// Returns true if i should come before j in sorted order.
func compareByKey(i, j any, getKey func(any) (string, bool)) bool {
	ki, iok := getKey(i)
	kj, jok := getKey(j)

	// Both missing keys -> preserve original order
	if !iok && !jok {
		return false
	}
	// i missing, j has -> j should come before i => return false
	if !iok && jok {
		return false
	}
	// i has, j missing -> i should come before j
	if iok && !jok {
		return true
	}
	// Both have keys -> compare lexicographically
	return ki < kj
}

// sortAnySlicesByName returns a cmp.Transformer to handle []any slices that contain maps with "name" or "secretKey" field.
func sortAnySlicesByName() cmp.Option {
	return cmp.Transformer("SortAnySlicesByName", func(in []any) []any {
		// Check if this is a slice of maps with "name" or "secretKey" field
		if len(in) == 0 {
			return in
		}

		firstMap, ok := in[0].(map[string]any)
		if !ok {
			return in
		}

		if _, hasName := firstMap["name"]; !hasName {
			if _, hasSecretKey := firstMap["secretKey"]; !hasSecretKey {
				return in
			}
		}

		// Helper to extract key from an any element (map[string]any)
		getKeyAny := func(x any) (string, bool) {
			m, ok := x.(map[string]any)
			if !ok {
				return "", false
			}
			if v, ok := m["name"].(string); ok && v != "" {
				return v, true
			}
			if v, ok := m["secretKey"].(string); ok && v != "" {
				return v, true
			}
			return "", false
		}

		// Create a copy and sort by key
		out := make([]any, len(in))
		copy(out, in)
		sort.SliceStable(out, func(i, j int) bool {
			return compareByKey(out[i], out[j], getKeyAny)
		})
		return out
	})
}
