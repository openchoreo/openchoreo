// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package trait

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/renderer"
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
		{
			name:              "create with includeWhen true - resource created",
			baseResourcesYAML: `[]`,
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: conditional-trait
spec:
  creates:
    - includeWhen: ${parameters.enabled}
      template:
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: conditional-config
`,
			context: map[string]any{
				"parameters": map[string]any{
					"enabled": true,
				},
			},
			wantResourcesYAML: `
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: conditional-config
`,
			wantErr: false,
		},
		{
			name:              "create with includeWhen false - resource skipped",
			baseResourcesYAML: `[]`,
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: conditional-trait
spec:
  creates:
    - includeWhen: ${parameters.enabled}
      template:
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: conditional-config
`,
			context: map[string]any{
				"parameters": map[string]any{
					"enabled": false,
				},
			},
			wantResourcesYAML: `[]`,
			wantErr:           false,
		},
		{
			name:              "create with includeWhen missing data - returns error",
			baseResourcesYAML: `[]`,
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: conditional-trait
spec:
  creates:
    - includeWhen: ${parameters.nonexistent}
      template:
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: conditional-config
`,
			context: map[string]any{
				"parameters": map[string]any{},
			},
			wantResourcesYAML: `[]`,
			wantErr:           true,
		},
		{
			name:              "create with forEach over list",
			baseResourcesYAML: `[]`,
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: volume-trait
spec:
  creates:
    - forEach: ${parameters.volumes}
      var: vol
      template:
        apiVersion: v1
        kind: PersistentVolumeClaim
        metadata:
          name: ${vol.name}
        spec:
          accessModes:
            - ReadWriteOnce
          resources:
            requests:
              storage: ${vol.size}
`,
			context: map[string]any{
				"parameters": map[string]any{
					"volumes": []any{
						map[string]any{"name": "data-vol", "size": "10Gi"},
						map[string]any{"name": "cache-vol", "size": "5Gi"},
					},
				},
			},
			wantResourcesYAML: `
- apiVersion: v1
  kind: PersistentVolumeClaim
  metadata:
    name: data-vol
  spec:
    accessModes:
      - ReadWriteOnce
    resources:
      requests:
        storage: 10Gi
- apiVersion: v1
  kind: PersistentVolumeClaim
  metadata:
    name: cache-vol
  spec:
    accessModes:
      - ReadWriteOnce
    resources:
      requests:
        storage: 5Gi
`,
			wantErr: false,
		},
		{
			name:              "create with forEach over map - deterministic order",
			baseResourcesYAML: `[]`,
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: config-trait
spec:
  creates:
    - forEach: ${parameters.configs}
      var: cfg
      template:
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: ${cfg.key}-config
        data:
          value: ${cfg.value}
`,
			context: map[string]any{
				"parameters": map[string]any{
					"configs": map[string]any{
						"zebra": "z-value",
						"alpha": "a-value",
					},
				},
			},
			wantResourcesYAML: `
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: alpha-config
  data:
    value: a-value
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: zebra-config
  data:
    value: z-value
`,
			wantErr: false,
		},
		{
			name:              "create with forEach over empty list",
			baseResourcesYAML: `[]`,
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: empty-trait
spec:
  creates:
    - forEach: ${parameters.items}
      var: item
      template:
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: ${item}
`,
			context: map[string]any{
				"parameters": map[string]any{
					"items": []any{},
				},
			},
			wantResourcesYAML: `[]`,
			wantErr:           false,
		},
		{
			name:              "create with includeWhen and forEach combined - includeWhen controls entire forEach block",
			baseResourcesYAML: `[]`,
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: conditional-forEach-trait
spec:
  creates:
    - includeWhen: ${parameters.enabled}
      forEach: ${parameters.volumes}
      var: vol
      template:
        apiVersion: v1
        kind: PersistentVolumeClaim
        metadata:
          name: ${vol.name}
        spec:
          resources:
            requests:
              storage: ${vol.size}
`,
			context: map[string]any{
				"parameters": map[string]any{
					"enabled": false,
					"volumes": []any{
						map[string]any{"name": "vol1", "size": "10Gi"},
						map[string]any{"name": "vol2", "size": "5Gi"},
					},
				},
			},
			wantResourcesYAML: `[]`,
			wantErr:           false,
		},
		{
			name:              "create with forEach using default var name 'item'",
			baseResourcesYAML: `[]`,
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: default-var-trait
spec:
  creates:
    - forEach: ${parameters.names}
      template:
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: ${item}-config
`,
			context: map[string]any{
				"parameters": map[string]any{
					"names": []any{"first", "second"},
				},
			},
			wantResourcesYAML: `
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: first-config
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: second-config
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse base resources
			var baseResourceMaps []map[string]any
			if err := yaml.Unmarshal([]byte(tt.baseResourcesYAML), &baseResourceMaps); err != nil {
				t.Fatalf("Failed to parse base resources YAML: %v", err)
			}

			baseResources := toRenderedResources(baseResourceMaps)

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

				gotResources := extractResources(got)

				// Compare resources
				if diff := cmp.Diff(wantResources, gotResources); diff != "" {
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
		{
			name: "patch with target where clause",
			resourcesYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: web-app
    labels:
      monitoring: enabled
  spec:
    replicas: 3
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: worker
    labels:
      monitoring: disabled
  spec:
    replicas: 2
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: api
    labels:
      monitoring: enabled
  spec:
    replicas: 5
`,
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: monitoring-trait
spec:
  patches:
    - target:
        kind: Deployment
        version: v1
        group: apps
        where: ${resource.metadata.labels.monitoring == "enabled"}
      operations:
        - op: add
          path: /metadata/annotations
          value:
            prometheus.io/scrape: "true"
            prometheus.io/port: "9090"
`,
			context: map[string]any{},
			wantResourcesYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: web-app
    labels:
      monitoring: enabled
    annotations:
      prometheus.io/scrape: "true"
      prometheus.io/port: "9090"
  spec:
    replicas: 3
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: worker
    labels:
      monitoring: disabled
  spec:
    replicas: 2
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: api
    labels:
      monitoring: enabled
    annotations:
      prometheus.io/scrape: "true"
      prometheus.io/port: "9090"
  spec:
    replicas: 5
`,
			wantErr: false,
		},
		{
			name: "patch with forEach and where combined",
			resourcesYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: web-app
    labels:
      tier: frontend
  spec:
    template:
      spec:
        containers:
          - name: app
            image: myapp:latest
            ports: []
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: worker
    labels:
      tier: backend
  spec:
    template:
      spec:
        containers:
          - name: app
            image: myapp:latest
            ports: []
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: api
    labels:
      tier: frontend
  spec:
    template:
      spec:
        containers:
          - name: app
            image: myapp:latest
            ports: []
`,
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: port-trait
spec:
  patches:
    - forEach: ${parameters.exposedPorts}
      var: port
      target:
        kind: Deployment
        version: v1
        group: apps
        where: ${resource.metadata.labels.tier == "frontend"}
      operations:
        - op: add
          path: /spec/template/spec/containers/0/ports/-
          value:
            containerPort: ${port.number}
            name: ${port.name}
`,
			context: map[string]any{
				"parameters": map[string]any{
					"exposedPorts": []any{
						map[string]any{"number": float64(8080), "name": "http"},
						map[string]any{"number": float64(9090), "name": "metrics"},
					},
				},
			},
			wantResourcesYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: web-app
    labels:
      tier: frontend
  spec:
    template:
      spec:
        containers:
          - name: app
            image: myapp:latest
            ports:
              - containerPort: 8080
                name: http
              - containerPort: 9090
                name: metrics
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: worker
    labels:
      tier: backend
  spec:
    template:
      spec:
        containers:
          - name: app
            image: myapp:latest
            ports: []
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: api
    labels:
      tier: frontend
  spec:
    template:
      spec:
        containers:
          - name: app
            image: myapp:latest
            ports:
              - containerPort: 8080
                name: http
              - containerPort: 9090
                name: metrics
`,
			wantErr: false,
		},
		{
			name: "patch with forEach over map - add env vars",
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
            env: []
`,
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: env-injector
spec:
  patches:
    - forEach: ${parameters.envVars}
      var: env
      target:
        kind: Deployment
        version: v1
        group: apps
      operations:
        - op: add
          path: /spec/template/spec/containers/0/env/-
          value:
            name: ${env.key}
            value: ${env.value}
`,
			context: map[string]any{
				"parameters": map[string]any{
					"envVars": map[string]any{
						"DB_HOST": "localhost",
						"API_KEY": "secret123",
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
            env:
              - name: API_KEY
                value: secret123
              - name: DB_HOST
                value: localhost
`,
			wantErr: false,
		},
		{
			name: "patch with forEach over empty map",
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
            env: []
`,
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: env-injector
spec:
  patches:
    - forEach: ${parameters.envVars}
      var: env
      target:
        kind: Deployment
        version: v1
        group: apps
      operations:
        - op: add
          path: /spec/template/spec/containers/0/env/-
          value:
            name: ${env.key}
            value: ${env.value}
`,
			context: map[string]any{
				"parameters": map[string]any{
					"envVars": map[string]any{},
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
            env: []
`,
			wantErr: false,
		},
		{
			name: "patch with forEach over map - deterministic order",
			resourcesYAML: `
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: config
  data: {}
`,
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: data-injector
spec:
  patches:
    - forEach: ${parameters.settings}
      var: setting
      target:
        kind: ConfigMap
        version: v1
      operations:
        - op: add
          path: /data/${setting.key}
          value: ${setting.value}
`,
			context: map[string]any{
				"parameters": map[string]any{
					"settings": map[string]any{
						"zebra": "z-value",
						"alpha": "a-value",
						"beta":  "b-value",
					},
				},
			},
			wantResourcesYAML: `
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: config
  data:
    alpha: a-value
    beta: b-value
    zebra: z-value
`,
			wantErr: false,
		},
		{
			name: "patch with CEL expression as entire value",
			resourcesYAML: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: app
  spec:
    template:
      spec:
        containers:
          - name: main
            image: myapp:latest
`,
			traitYAML: `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: config-trait
spec:
  patches:
    - forEach: ${parameters.containers}
      var: container
      target:
        kind: Deployment
        version: v1
        group: apps
      operations:
        - op: add
          path: /spec/template/spec/containers/[?(@.name=='${container.key}')]/envFrom
          value: ${parameters.envFromList}
`,
			context: map[string]any{
				"parameters": map[string]any{
					"containers": map[string]any{
						"main": map[string]any{},
					},
					"envFromList": []any{
						map[string]any{"configMapRef": map[string]any{"name": "my-config"}},
						map[string]any{"secretRef": map[string]any{"name": "my-secret"}},
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
          - name: main
            image: myapp:latest
            envFrom:
              - configMapRef:
                  name: my-config
              - secretRef:
                  name: my-secret
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse resources
			var resourceMaps []map[string]any
			if err := yaml.Unmarshal([]byte(tt.resourcesYAML), &resourceMaps); err != nil {
				t.Fatalf("Failed to parse resources YAML: %v", err)
			}

			resources := toRenderedResources(resourceMaps)

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

			err := processor.ApplyTraitPatches(resources, &trait, tt.context)
			if (err != nil) != tt.wantErr {
				t.Errorf("ApplyTraitPatches() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				gotResources := extractResources(resources)

				// Compare resources
				if diff := cmp.Diff(wantResources, gotResources); diff != "" {
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
			var resourceMaps []map[string]any
			if err := yaml.Unmarshal([]byte(tt.resourcesYAML), &resourceMaps); err != nil {
				t.Fatalf("Failed to parse resources YAML: %v", err)
			}

			resources := toRenderedResources(resourceMaps)

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

				gotResources := extractResources(got)

				// Compare resources
				if diff := cmp.Diff(wantResources, gotResources); diff != "" {
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

	var resourceMaps []map[string]any
	if err := yaml.Unmarshal([]byte(resourcesYAML), &resourceMaps); err != nil {
		t.Fatalf("Failed to parse resources YAML: %v", err)
	}

	// Create resources with mixed target planes
	resources := []renderer.RenderedResource{
		{Resource: deepCopy(resourceMaps[0]), TargetPlane: v1alpha1.TargetPlaneDataPlane},          // web deployment
		{Resource: deepCopy(resourceMaps[1]), TargetPlane: v1alpha1.TargetPlaneDataPlane},          // database statefulset
		{Resource: deepCopy(resourceMaps[2]), TargetPlane: v1alpha1.TargetPlaneDataPlane},          // web-svc service
		{Resource: deepCopy(resourceMaps[3]), TargetPlane: v1alpha1.TargetPlaneObservabilityPlane}, // migration job (observability)
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
		{
			name: "match by targetPlane only - dataplane",
			target: TargetSpec{
				TargetPlane: v1alpha1.TargetPlaneDataPlane,
			},
			wantCount: 3,
			wantNames: []string{"web", "database", "web-svc"},
		},
		{
			name: "match by targetPlane only - observabilityplane",
			target: TargetSpec{
				TargetPlane: v1alpha1.TargetPlaneObservabilityPlane,
			},
			wantCount: 1,
			wantNames: []string{"migration"},
		},
		{
			name: "match by targetPlane and kind",
			target: TargetSpec{
				TargetPlane: v1alpha1.TargetPlaneDataPlane,
				Kind:        "Deployment",
			},
			wantCount: 1,
			wantNames: []string{"web"},
		},
		{
			name: "match by targetPlane and kind - no match (wrong plane)",
			target: TargetSpec{
				TargetPlane: v1alpha1.TargetPlaneObservabilityPlane,
				Kind:        "Deployment",
			},
			wantCount: 0,
			wantNames: []string{},
		},
		{
			name: "match by targetPlane, group, and version",
			target: TargetSpec{
				TargetPlane: v1alpha1.TargetPlaneDataPlane,
				Group:       "apps",
				Version:     "v1",
			},
			wantCount: 2,
			wantNames: []string{"web", "database"},
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
			for i, rr := range matches {
				metadata := rr.Resource["metadata"].(map[string]any)
				gotNames[i] = metadata["name"].(string)
			}

			if diff := cmp.Diff(tt.wantNames, gotNames); diff != "" {
				t.Errorf("Resource names mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// singleDeploymentResourceYAML is a common YAML snippet used across multiple tests.
const singleDeploymentResourceYAML = `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: app
`

func TestApplyTraitCreates_ForEach(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	traitYAML := `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: sidecar-trait
spec:
  creates:
    - forEach: ${parameters.sidecars}
      var: sidecar
      template:
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: ${sidecar.name}-config
        data:
          port: ${sidecar.port}
`
	var trait v1alpha1.Trait
	require.NoError(t, yaml.Unmarshal([]byte(traitYAML), &trait))

	ctx := map[string]any{
		"parameters": map[string]any{
			"sidecars": []any{
				map[string]any{"name": "envoy", "port": "9901"},
				map[string]any{"name": "fluentd", "port": "24224"},
				map[string]any{"name": "otel", "port": "4317"},
			},
		},
	}

	got, err := processor.ApplyTraitCreates(nil, &trait, ctx)
	require.NoError(t, err)
	assert.Len(t, got, 3)

	names := make([]string, len(got))
	ports := make([]string, len(got))
	for i, rr := range got {
		metadata := rr.Resource["metadata"].(map[string]any)
		names[i] = metadata["name"].(string)
		data := rr.Resource["data"].(map[string]any)
		ports[i] = data["port"].(string)
	}
	assert.Equal(t, []string{"envoy-config", "fluentd-config", "otel-config"}, names)
	assert.Equal(t, []string{"9901", "24224", "4317"}, ports)
}

func TestApplyTraitCreates_ForEachError(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	traitYAML := `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: broken-trait
spec:
  creates:
    - forEach: ${nonexistent.field}
      var: item
      template:
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: ${item}
`
	var trait v1alpha1.Trait
	require.NoError(t, yaml.Unmarshal([]byte(traitYAML), &trait))

	ctx := map[string]any{}

	_, err := processor.ApplyTraitCreates(nil, &trait, ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forEach")
}

func TestApplyTraitCreates_RenderError(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	traitYAML := `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: bad-template-trait
spec:
  creates:
    - template:
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: ${nonexistent.deeply.nested.field}
`
	var trait v1alpha1.Trait
	require.NoError(t, yaml.Unmarshal([]byte(traitYAML), &trait))

	ctx := map[string]any{}

	_, err := processor.ApplyTraitCreates(nil, &trait, ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "render")
}

func TestApplyTraitCreates_TargetPlane(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	traitYAML := `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: obs-trait
spec:
  creates:
    - targetPlane: observabilityplane
      template:
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: metrics-config
    - targetPlane: dataplane
      template:
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: app-config
`
	var trait v1alpha1.Trait
	require.NoError(t, yaml.Unmarshal([]byte(traitYAML), &trait))

	ctx := map[string]any{}

	got, err := processor.ApplyTraitCreates(nil, &trait, ctx)
	require.NoError(t, err)
	require.Len(t, got, 2)

	assert.Equal(t, "metrics-config", got[0].Resource["metadata"].(map[string]any)["name"])
	assert.Equal(t, v1alpha1.TargetPlaneObservabilityPlane, got[0].TargetPlane)
	assert.Equal(t, "app-config", got[1].Resource["metadata"].(map[string]any)["name"])
	assert.Equal(t, v1alpha1.TargetPlaneDataPlane, got[1].TargetPlane)
}

func TestApplyTraitPatches_ForEach(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	resourcesYAML := `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: app
    labels: {}
`
	var resourceMaps []map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(resourcesYAML), &resourceMaps))
	resources := toRenderedResources(resourceMaps)

	traitYAML := `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: label-trait
spec:
  patches:
    - forEach: ${parameters.labels}
      var: lbl
      target:
        kind: Deployment
        version: v1
        group: apps
      operations:
        - op: add
          path: /metadata/labels/${lbl.key}
          value: ${lbl.value}
`
	var trait v1alpha1.Trait
	require.NoError(t, yaml.Unmarshal([]byte(traitYAML), &trait))

	ctx := map[string]any{
		"parameters": map[string]any{
			"labels": []any{
				map[string]any{"key": "env", "value": "prod"},
				map[string]any{"key": "team", "value": "platform"},
			},
		},
	}

	err := processor.ApplyTraitPatches(resources, &trait, ctx)
	require.NoError(t, err)

	labels := resources[0].Resource["metadata"].(map[string]any)["labels"].(map[string]any)
	assert.Equal(t, "prod", labels["env"])
	assert.Equal(t, "platform", labels["team"])
}

func TestApplyTraitPatches_ForEachError(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	resourcesYAML := singleDeploymentResourceYAML
	var resourceMaps []map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(resourcesYAML), &resourceMaps))
	resources := toRenderedResources(resourceMaps)

	traitYAML := `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: broken-trait
spec:
  patches:
    - forEach: ${nonexistent.field}
      var: item
      target:
        kind: Deployment
        version: v1
        group: apps
      operations:
        - op: add
          path: /metadata/labels/key
          value: val
`
	var trait v1alpha1.Trait
	require.NoError(t, yaml.Unmarshal([]byte(traitYAML), &trait))

	ctx := map[string]any{}

	err := processor.ApplyTraitPatches(resources, &trait, ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forEach")
}

func TestApplyTraitPatches_WhereClause(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	resourcesYAML := `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: web
    labels:
      tier: frontend
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: api
    labels:
      tier: backend
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: admin
    labels:
      tier: frontend
`
	var resourceMaps []map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(resourcesYAML), &resourceMaps))
	resources := toRenderedResources(resourceMaps)

	traitYAML := `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: frontend-trait
spec:
  patches:
    - target:
        kind: Deployment
        version: v1
        group: apps
        where: ${resource.metadata.labels.tier == "frontend"}
      operations:
        - op: add
          path: /metadata/annotations
          value:
            ingress: "true"
`
	var trait v1alpha1.Trait
	require.NoError(t, yaml.Unmarshal([]byte(traitYAML), &trait))

	ctx := map[string]any{}

	err := processor.ApplyTraitPatches(resources, &trait, ctx)
	require.NoError(t, err)

	// "web" and "admin" should have annotations, "api" should not
	webMeta := resources[0].Resource["metadata"].(map[string]any)
	assert.Equal(t, "true", webMeta["annotations"].(map[string]any)["ingress"])

	apiMeta := resources[1].Resource["metadata"].(map[string]any)
	_, hasAnnotations := apiMeta["annotations"]
	assert.False(t, hasAnnotations, "backend deployment should not have annotations")

	adminMeta := resources[2].Resource["metadata"].(map[string]any)
	assert.Equal(t, "true", adminMeta["annotations"].(map[string]any)["ingress"])
}

func TestApplyTraitPatches_WhereClauseError(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	resourcesYAML := singleDeploymentResourceYAML
	var resourceMaps []map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(resourcesYAML), &resourceMaps))
	resources := toRenderedResources(resourceMaps)

	traitYAML := `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: broken-where-trait
spec:
  patches:
    - target:
        kind: Deployment
        version: v1
        group: apps
        where: ${nonexistent.deeply.nested}
      operations:
        - op: add
          path: /metadata/labels/key
          value: val
`
	var trait v1alpha1.Trait
	require.NoError(t, yaml.Unmarshal([]byte(traitYAML), &trait))

	ctx := map[string]any{}

	err := processor.ApplyTraitPatches(resources, &trait, ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "where clause")
}

func TestApplyTraitPatches_WhereClauseNonBoolean(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	resourcesYAML := `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: app
    labels:
      tier: frontend
`
	var resourceMaps []map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(resourcesYAML), &resourceMaps))
	resources := toRenderedResources(resourceMaps)

	// where clause evaluates to a string instead of a boolean
	traitYAML := `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: nonbool-where-trait
spec:
  patches:
    - target:
        kind: Deployment
        version: v1
        group: apps
        where: ${resource.metadata.labels.tier}
      operations:
        - op: add
          path: /metadata/labels/key
          value: val
`
	var trait v1alpha1.Trait
	require.NoError(t, yaml.Unmarshal([]byte(traitYAML), &trait))

	ctx := map[string]any{}

	err := processor.ApplyTraitPatches(resources, &trait, ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boolean")
}

func TestApplyTraitPatches_WhereFiltersAll(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	resourcesYAML := `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: app
    labels:
      tier: backend
`
	var resourceMaps []map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(resourcesYAML), &resourceMaps))
	resources := toRenderedResources(resourceMaps)

	// Snapshot the resource before patching to verify it is truly unmodified
	before := deepCopy(resources[0].Resource)

	// where clause matches nothing (looking for frontend, only backend exists)
	traitYAML := `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: no-match-trait
spec:
  patches:
    - target:
        kind: Deployment
        version: v1
        group: apps
        where: ${resource.metadata.labels.tier == "frontend"}
      operations:
        - op: add
          path: /metadata/annotations
          value:
            patched: "true"
`
	var trait v1alpha1.Trait
	require.NoError(t, yaml.Unmarshal([]byte(traitYAML), &trait))

	ctx := map[string]any{}

	err := processor.ApplyTraitPatches(resources, &trait, ctx)
	require.NoError(t, err)

	// Resource must be completely unmodified
	if diff := cmp.Diff(before, resources[0].Resource); diff != "" {
		t.Errorf("resource was modified despite where filtering all targets (-before +after):\n%s", diff)
	}
}

func TestApplyTraitPatches_WherePreservesResourceBinding(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	resourcesYAML := `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: app
    labels:
      tier: frontend
`
	var resourceMaps []map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(resourcesYAML), &resourceMaps))
	resources := toRenderedResources(resourceMaps)

	traitYAML := `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: where-trait
spec:
  patches:
    - target:
        kind: Deployment
        version: v1
        group: apps
        where: ${resource.metadata.labels.tier == "frontend"}
      operations:
        - op: add
          path: /metadata/annotations
          value:
            patched: "true"
`
	var trait v1alpha1.Trait
	require.NoError(t, yaml.Unmarshal([]byte(traitYAML), &trait))

	// Set up a context that already has a "resource" key to verify it's preserved
	existingResourceValue := map[string]any{"existing": "data"}
	ctx := map[string]any{
		"resource": existingResourceValue,
	}

	err := processor.ApplyTraitPatches(resources, &trait, ctx)
	require.NoError(t, err)

	// Verify the original "resource" binding is preserved/restored after filtering
	assert.Equal(t, existingResourceValue, ctx["resource"], "original resource binding should be preserved after where clause evaluation")
}

func TestApplyTraitPatches_NoMatchingResources(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	resourcesYAML := `
- apiVersion: v1
  kind: Service
  metadata:
    name: my-svc
`
	var resourceMaps []map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(resourcesYAML), &resourceMaps))
	resources := toRenderedResources(resourceMaps)

	// Patch targets Deployment but only a Service exists
	traitYAML := `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: noop-trait
spec:
  patches:
    - target:
        kind: Deployment
        version: v1
        group: apps
      operations:
        - op: add
          path: /metadata/labels/key
          value: val
`
	var trait v1alpha1.Trait
	require.NoError(t, yaml.Unmarshal([]byte(traitYAML), &trait))

	ctx := map[string]any{}

	before := deepCopy(resources[0].Resource)

	err := processor.ApplyTraitPatches(resources, &trait, ctx)
	require.NoError(t, err, "patching with no matching resources should be a no-op")

	if diff := cmp.Diff(before, resources[0].Resource); diff != "" {
		t.Errorf("resource was modified despite no matching targets (-before +after):\n%s", diff)
	}
}

func TestApplyTraitPatches_PatchApplyError(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	resourcesYAML := `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: my-deploy
`
	var resourceMaps []map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(resourcesYAML), &resourceMaps))
	resources := toRenderedResources(resourceMaps)

	// Use "replace" on a nonexistent path to trigger an error in patch application
	traitYAML := `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: bad-patch-trait
spec:
  patches:
    - target:
        kind: Deployment
        version: v1
        group: apps
      operations:
        - op: replace
          path: /spec/nonexistent/deeply/nested/path
          value: broken
`
	var trait v1alpha1.Trait
	require.NoError(t, yaml.Unmarshal([]byte(traitYAML), &trait))

	ctx := map[string]any{}

	err := processor.ApplyTraitPatches(resources, &trait, ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Deployment/my-deploy")
}

func TestRenderOperations_PathError(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	resourcesYAML := singleDeploymentResourceYAML
	var resourceMaps []map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(resourcesYAML), &resourceMaps))
	resources := toRenderedResources(resourceMaps)

	// Path references a nonexistent variable
	traitYAML := `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: bad-path-trait
spec:
  patches:
    - target:
        kind: Deployment
        version: v1
        group: apps
      operations:
        - op: add
          path: /metadata/labels/${nonexistent.var}
          value: something
`
	var trait v1alpha1.Trait
	require.NoError(t, yaml.Unmarshal([]byte(traitYAML), &trait))

	ctx := map[string]any{}

	err := processor.ApplyTraitPatches(resources, &trait, ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path")
}

func TestRenderOperations_NonStringPath(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	resourcesYAML := singleDeploymentResourceYAML
	var resourceMaps []map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(resourcesYAML), &resourceMaps))
	resources := toRenderedResources(resourceMaps)

	// Path evaluates to an integer instead of a string
	traitYAML := `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: int-path-trait
spec:
  patches:
    - target:
        kind: Deployment
        version: v1
        group: apps
      operations:
        - op: add
          path: ${parameters.pathVal}
          value: something
`
	var trait v1alpha1.Trait
	require.NoError(t, yaml.Unmarshal([]byte(traitYAML), &trait))

	ctx := map[string]any{
		"parameters": map[string]any{
			"pathVal": 42,
		},
	}

	err := processor.ApplyTraitPatches(resources, &trait, ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "string")
}

func TestRenderOperations_ValueUnmarshalError(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	resourcesYAML := singleDeploymentResourceYAML
	var resourceMaps []map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(resourcesYAML), &resourceMaps))
	resources := toRenderedResources(resourceMaps)

	// Construct a trait programmatically with invalid Value.Raw bytes
	trait := v1alpha1.Trait{}
	trait.Name = "bad-value-trait"
	trait.Spec.Patches = []v1alpha1.TraitPatch{
		{
			Target: v1alpha1.PatchTarget{
				Kind:    "Deployment",
				Version: "v1",
				Group:   "apps",
			},
			Operations: []v1alpha1.JSONPatchOperation{
				{
					Op:    "add",
					Path:  "/metadata/labels/key",
					Value: &runtime.RawExtension{Raw: []byte("not valid json {{{")},
				},
			},
		},
	}

	ctx := map[string]any{}

	err := processor.ApplyTraitPatches(resources, &trait, ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestRenderOperations_ValueRenderError(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	resourcesYAML := singleDeploymentResourceYAML
	var resourceMaps []map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(resourcesYAML), &resourceMaps))
	resources := toRenderedResources(resourceMaps)

	// Construct a trait with a value containing invalid CEL
	valueJSON, err := json.Marshal("${nonexistent.deeply.nested}")
	require.NoError(t, err)

	trait := v1alpha1.Trait{}
	trait.Name = "bad-cel-trait"
	trait.Spec.Patches = []v1alpha1.TraitPatch{
		{
			Target: v1alpha1.PatchTarget{
				Kind:    "Deployment",
				Version: "v1",
				Group:   "apps",
			},
			Operations: []v1alpha1.JSONPatchOperation{
				{
					Op:    "add",
					Path:  "/metadata/labels/key",
					Value: &runtime.RawExtension{Raw: valueJSON},
				},
			},
		},
	}

	ctx := map[string]any{}

	err = processor.ApplyTraitPatches(resources, &trait, ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "render value")
}

func TestProcessTraits_CreatesError(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	resourcesYAML := singleDeploymentResourceYAML
	var resourceMaps []map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(resourcesYAML), &resourceMaps))
	resources := toRenderedResources(resourceMaps)

	// Create a trait with a broken creates template (references nonexistent variable)
	traitYAML := `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: broken-creates-trait
spec:
  creates:
    - template:
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: ${nonexistent.deeply.nested}
`
	var trait v1alpha1.Trait
	require.NoError(t, yaml.Unmarshal([]byte(traitYAML), &trait))

	ctx := map[string]any{}

	_, err := processor.ProcessTraits(resources, &trait, ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "render")
}

func TestProcessTraits_PatchesError(t *testing.T) {
	engine := template.NewEngine()
	processor := NewProcessor(engine)

	resourcesYAML := singleDeploymentResourceYAML
	var resourceMaps []map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(resourcesYAML), &resourceMaps))
	resources := toRenderedResources(resourceMaps)

	// Create a trait with valid creates but broken patches (references nonexistent variable in path)
	traitYAML := `
apiVersion: choreo.dev/v1alpha1
kind: Trait
metadata:
  name: broken-patches-trait
spec:
  patches:
    - target:
        kind: Deployment
        version: v1
        group: apps
      operations:
        - op: add
          path: /metadata/labels/${nonexistent.field}
          value: something
`
	var trait v1alpha1.Trait
	require.NoError(t, yaml.Unmarshal([]byte(traitYAML), &trait))

	ctx := map[string]any{}

	_, err := processor.ProcessTraits(resources, &trait, ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path")
}

// Helper functions

func toRenderedResources(resourceMaps []map[string]any) []renderer.RenderedResource {
	resources := make([]renderer.RenderedResource, len(resourceMaps))
	for i, r := range resourceMaps {
		resources[i] = renderer.RenderedResource{
			Resource:    deepCopy(r), // Always deep copy for test isolation
			TargetPlane: v1alpha1.TargetPlaneDataPlane,
		}
	}
	return resources
}

func extractResources(rendered []renderer.RenderedResource) []map[string]any {
	resources := make([]map[string]any, len(rendered))
	for i, rr := range rendered {
		resources[i] = rr.Resource
	}
	return resources
}

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
