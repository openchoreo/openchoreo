// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package trait

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

func TestApplyTraitCreates(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	tests := []struct {
		name              string
		baseResourcesYAML string
		traitYAML         string
		context           map[string]any
		wantResourcesYAML string
		wantErr           bool
	}{
		{
			name:              "single create template",
			baseResourcesYAML: `[]`,
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: db-trait
spec:
  creates:
    - template:
        apiVersion: v1
        kind: Secret
        metadata:
          name: ${trait.instanceName}-secret
        data:
          key: ${parameters.secretValue}
`,
			context: map[string]any{
				"trait": map[string]any{
					"instanceName": "db-1",
				},
				"parameters": map[string]any{
					"secretValue": "myvalue",
				},
			},
			wantResourcesYAML: `
- apiVersion: v1
  kind: Secret
  metadata:
    name: db-1-secret
  data:
    key: myvalue
`,
			wantErr: false,
		},
		{
			name: "multiple create templates",
			baseResourcesYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: app
`,
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: config-trait
spec:
  creates:
    - template:
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: config
    - template:
        apiVersion: v1
        kind: Secret
        metadata:
          name: secret
`,
			context: map[string]any{},
			wantResourcesYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: app
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: config
- apiVersion: v1
  kind: Secret
  metadata:
    name: secret
`,
			wantErr: false,
		},
		{
			name:              "create with omit()",
			baseResourcesYAML: `[]`,
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: config-trait
spec:
  creates:
    - template:
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: config
          annotations: ${oc_omit()}
`,
			context: map[string]any{},
			wantResourcesYAML: `
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: config
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse base resources
			var baseResources []map[string]any
			if err := yaml.Unmarshal([]byte(tt.baseResourcesYAML), &baseResources); err != nil {
				t.Fatalf("Failed to parse base resources YAML: %v", err)
			}

			// Parse trait
			var trait v1alpha1.Trait
			if err := yaml.Unmarshal([]byte(tt.traitYAML), &trait); err != nil {
				t.Fatalf("Failed to parse trait YAML: %v", err)
			}

			// Parse expected resources
			var wantResources []map[string]any
			if err := yaml.Unmarshal([]byte(tt.wantResourcesYAML), &wantResources); err != nil {
				t.Fatalf("Failed to parse expected resources YAML: %v", err)
			}

			got, err := processor.ApplyTraitCreates(baseResources, &trait, tt.context)
			if (err != nil) != tt.wantErr {
				t.Errorf("ApplyTraitCreates() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got) != len(wantResources) {
					t.Errorf("ApplyTraitCreates() got %d resources, want %d", len(got), len(wantResources))
				}

				// Compare resources
				if diff := cmp.Diff(wantResources, got); diff != "" {
					t.Errorf("ApplyTraitCreates() mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestApplyTraitPatches(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	tests := []struct {
		name              string
		resourcesYAML     string
		traitYAML         string
		context           map[string]any
		wantResourcesYAML string
		wantErr           bool
	}{
		{
			name: "simple add operation",
			resourcesYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: app
  spec:
    template:
      spec:
        containers:
          - name: app
            image: myapp:latest
`,
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: label-trait
spec:
  patches:
    - target:
        kind: Deployment
        version: v1
        group: apps
      operations:
        - op: add
          path: /metadata/labels
          value:
            managed-by: trait
`,
			context: map[string]any{},
			wantResourcesYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: app
    labels:
      managed-by: trait
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
			name: "replace operation with CEL",
			resourcesYAML: `
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: config
  data:
    key: oldvalue
`,
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: config-trait
spec:
  patches:
    - target:
        kind: ConfigMap
        version: v1
      operations:
        - op: replace
          path: /data/key
          value: ${parameters.newValue}
`,
			context: map[string]any{
				"parameters": map[string]any{
					"newValue": "updated",
				},
			},
			wantResourcesYAML: `
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: config
  data:
    key: updated
`,
			wantErr: false,
		},
		{
			name: "patch with forEach",
			resourcesYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: app
  spec:
    template:
      spec:
        containers:
          - name: app
            image: myapp:latest
            volumeMounts: []
`,
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: volume-trait
spec:
  patches:
    - forEach: ${parameters.volumes}
      var: volume
      target:
        kind: Deployment
        version: v1
        group: apps
      operations:
        - op: add
          path: /spec/template/spec/containers/0/volumeMounts/-
          value:
            name: ${volume.name}
            mountPath: ${volume.path}
`,
			context: map[string]any{
				"parameters": map[string]any{
					"volumes": []any{
						map[string]any{"name": "vol1", "path": "/data1"},
						map[string]any{"name": "vol2", "path": "/data2"},
					},
				},
			},
			wantResourcesYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: app
  spec:
    template:
      spec:
        containers:
          - name: app
            image: myapp:latest
            volumeMounts:
              - name: vol1
                mountPath: /data1
              - name: vol2
                mountPath: /data2
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse resources
			var resources []map[string]any
			if err := yaml.Unmarshal([]byte(tt.resourcesYAML), &resources); err != nil {
				t.Fatalf("Failed to parse resources YAML: %v", err)
			}

			// Parse trait
			var trait v1alpha1.Trait
			if err := yaml.Unmarshal([]byte(tt.traitYAML), &trait); err != nil {
				t.Fatalf("Failed to parse trait YAML: %v", err)
			}

			// Parse expected resources
			var wantResources []map[string]any
			if err := yaml.Unmarshal([]byte(tt.wantResourcesYAML), &wantResources); err != nil {
				t.Fatalf("Failed to parse expected resources YAML: %v", err)
			}

			// Make a copy of resources since patches modify in place
			resourcesCopy := make([]map[string]any, len(resources))
			for i, r := range resources {
				resourcesCopy[i] = deepCopy(r)
			}

			err := processor.ApplyTraitPatches(resourcesCopy, &trait, tt.context)
			if (err != nil) != tt.wantErr {
				t.Errorf("ApplyTraitPatches() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				// Compare resources
				if diff := cmp.Diff(wantResources, resourcesCopy); diff != "" {
					t.Errorf("ApplyTraitPatches() mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestProcessTraits(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	tests := []struct {
		name              string
		resourcesYAML     string
		traitYAML         string
		context           map[string]any
		wantResourcesYAML string
		wantErr           bool
	}{
		{
			name: "trait with creates and patches",
			resourcesYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: app
`,
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: full-trait
spec:
  creates:
    - template:
        apiVersion: v1
        kind: Secret
        metadata:
          name: db-secret
  patches:
    - target:
        kind: Deployment
        version: v1
        group: apps
      operations:
        - op: add
          path: /metadata/labels
          value:
            trait: enabled
`,
			context: map[string]any{},
			wantResourcesYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: app
    labels:
      trait: enabled
- apiVersion: v1
  kind: Secret
  metadata:
    name: db-secret
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse resources
			var resources []map[string]any
			if err := yaml.Unmarshal([]byte(tt.resourcesYAML), &resources); err != nil {
				t.Fatalf("Failed to parse resources YAML: %v", err)
			}

			// Parse trait
			var trait v1alpha1.Trait
			if err := yaml.Unmarshal([]byte(tt.traitYAML), &trait); err != nil {
				t.Fatalf("Failed to parse trait YAML: %v", err)
			}

			// Parse expected resources
			var wantResources []map[string]any
			if err := yaml.Unmarshal([]byte(tt.wantResourcesYAML), &wantResources); err != nil {
				t.Fatalf("Failed to parse expected resources YAML: %v", err)
			}

			got, err := processor.ProcessTraits(resources, &trait, tt.context)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProcessTraits() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got) != len(wantResources) {
					t.Errorf("ProcessTraits() got %d resources, want %d", len(got), len(wantResources))
				}

				// Compare resources
				if diff := cmp.Diff(wantResources, got); diff != "" {
					t.Errorf("ProcessTraits() mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestFindTargetResources(t *testing.T) {
	t.Parallel()

	resourcesYAML := `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: web
- apiVersion: apps/v1
  kind: StatefulSet
  metadata:
    name: database
- apiVersion: v1
  kind: Service
  metadata:
    name: web-svc
- apiVersion: batch/v1
  kind: Job
  metadata:
    name: migration
`

	var resources []map[string]any
	if err := yaml.Unmarshal([]byte(resourcesYAML), &resources); err != nil {
		t.Fatalf("Failed to parse resources YAML: %v", err)
	}

	tests := []struct {
		name      string
		target    TargetSpec
		wantCount int
		wantNames []string
	}{
		{
			name: "match by kind only",
			target: TargetSpec{
				Kind: "Deployment",
			},
			wantCount: 1,
			wantNames: []string{"web"},
		},
		{
			name: "match by group and version",
			target: TargetSpec{
				Group:   "apps",
				Version: "v1",
			},
			wantCount: 2,
			wantNames: []string{"web", "database"},
		},
		{
			name: "match by kind and group",
			target: TargetSpec{
				Kind:  "Job",
				Group: "batch",
			},
			wantCount: 1,
			wantNames: []string{"migration"},
		},
		{
			name: "match core API (empty group)",
			target: TargetSpec{
				Group:   "",
				Version: "v1",
				Kind:    "Service",
			},
			wantCount: 1,
			wantNames: []string{"web-svc"},
		},
		{
			name:      "match all (empty target)",
			target:    TargetSpec{},
			wantCount: 4,
			wantNames: []string{"web", "database", "web-svc", "migration"},
		},
		{
			name: "no matches",
			target: TargetSpec{
				Kind: "NonExistent",
			},
			wantCount: 0,
			wantNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			matches := FindTargetResources(resources, tt.target)

			if len(matches) != tt.wantCount {
				t.Errorf("FindTargetResources() returned %d resources, want %d", len(matches), tt.wantCount)
			}

			gotNames := make([]string, len(matches))
			for i, match := range matches {
				metadata := match["metadata"].(map[string]any)
				gotNames[i] = metadata["name"].(string)
			}

			if diff := cmp.Diff(tt.wantNames, gotNames); diff != "" {
				t.Errorf("Resource names mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// Helper functions

func deepCopy(m map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range m {
		result[k] = deepCopyValue(v)
	}
	return result
}

func deepCopyValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return deepCopy(val)
	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = deepCopyValue(item)
		}
		return result
	default:
		return val
	}
}
