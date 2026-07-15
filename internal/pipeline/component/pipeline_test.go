// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/context"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/renderer"
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

// resourceKindName returns a "Kind/name" label for a rendered resource, for test messages.
func resourceKindName(resource map[string]any) string {
	kind, _ := resource["kind"].(string)
	name := ""
	if meta, ok := resource["metadata"].(map[string]any); ok {
		name, _ = meta["name"].(string)
	}
	return fmt.Sprintf("%s/%s", kind, name)
}

func TestPipeline_Render(t *testing.T) {
	devEnvironmentYAML := `
    apiVersion: openchoreo.dev/v1alpha1
    kind: Environment
    metadata:
      name: dev
      namespace: test-namespace
    spec:
      dataPlaneRef:
        kind: DataPlane
        name: dev-dataplane
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
        ingress:
          external:
            http:
              port: 80
              host: api.example.com
            https:
              port: 443
              host: api.example.com
          internal:
            http:
              port: 80
              host: internal.example.com
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
		wantMetadata         *RenderMetadata
		wantTargetPlanes     []string
	}{
		{
			name:         "simple component without traits",
			wantMetadata: &RenderMetadata{ResourceCount: 1, BaseResourceCount: 1, TraitCount: 0, TraitResourceCount: 0, Warnings: []string{}},
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
      parameters:
        openAPIV3Schema:
          type: object
          properties:
            replicas:
              type: integer
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
      openchoreo.dev/component-uid: a1b2c3d4-5678-90ab-cdef-1234567890ab
      openchoreo.dev/environment: dev
      openchoreo.dev/environment-uid: d4e5f6a7-8901-23de-f012-4567890abcde
      openchoreo.dev/namespace: test-namespace
      openchoreo.dev/project: test-project
      openchoreo.dev/project-uid: b2c3d4e5-6789-01bc-def0-234567890abc
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
      parameters:
        openAPIV3Schema:
          type: object
          properties:
            expose:
              type: boolean
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
      openchoreo.dev/component-uid: a1b2c3d4-5678-90ab-cdef-1234567890ab
      openchoreo.dev/environment: dev
      openchoreo.dev/environment-uid: d4e5f6a7-8901-23de-f012-4567890abcde
      openchoreo.dev/namespace: test-namespace
      openchoreo.dev/project: test-project
      openchoreo.dev/project-uid: b2c3d4e5-6789-01bc-def0-234567890abc
- apiVersion: v1
  kind: Service
  metadata:
    name: test-component-dev-12345678
    labels:
      openchoreo.dev/component: test-app
      openchoreo.dev/component-uid: a1b2c3d4-5678-90ab-cdef-1234567890ab
      openchoreo.dev/environment: dev
      openchoreo.dev/environment-uid: d4e5f6a7-8901-23de-f012-4567890abcde
      openchoreo.dev/namespace: test-namespace
      openchoreo.dev/project: test-project
      openchoreo.dev/project-uid: b2c3d4e5-6789-01bc-def0-234567890abc
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
      parameters:
        openAPIV3Schema:
          type: object
          properties:
            secrets:
              type: array
              items:
                type: string
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
      openchoreo.dev/component-uid: a1b2c3d4-5678-90ab-cdef-1234567890ab
      openchoreo.dev/environment: dev
      openchoreo.dev/environment-uid: d4e5f6a7-8901-23de-f012-4567890abcde
      openchoreo.dev/namespace: test-namespace
      openchoreo.dev/project: test-project
      openchoreo.dev/project-uid: b2c3d4e5-6789-01bc-def0-234567890abc
- apiVersion: v1
  kind: Secret
  metadata:
    name: secret2
    labels:
      openchoreo.dev/component: test-app
      openchoreo.dev/component-uid: a1b2c3d4-5678-90ab-cdef-1234567890ab
      openchoreo.dev/environment: dev
      openchoreo.dev/environment-uid: d4e5f6a7-8901-23de-f012-4567890abcde
      openchoreo.dev/namespace: test-namespace
      openchoreo.dev/project: test-project
      openchoreo.dev/project-uid: b2c3d4e5-6789-01bc-def0-234567890abc
`,
			wantErr: false,
		},
		{
			name:             "component with trait creates",
			wantMetadata:     &RenderMetadata{ResourceCount: 2, BaseResourceCount: 1, TraitCount: 1, TraitResourceCount: 1, Warnings: []string{}},
			wantTargetPlanes: []string{v1alpha1.TargetPlaneDataPlane, v1alpha1.TargetPlaneObservabilityPlane},
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
        parameters:
          openAPIV3Schema:
            type: object
            properties:
              database:
                type: string
        creates:
          - targetPlane: observabilityplane
            template:
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
      openchoreo.dev/component-uid: a1b2c3d4-5678-90ab-cdef-1234567890ab
      openchoreo.dev/environment: dev
      openchoreo.dev/environment-uid: d4e5f6a7-8901-23de-f012-4567890abcde
      openchoreo.dev/namespace: test-namespace
      openchoreo.dev/project: test-project
      openchoreo.dev/project-uid: b2c3d4e5-6789-01bc-def0-234567890abc
- apiVersion: v1
  kind: Secret
  metadata:
    name: db-1-secret
    labels:
      openchoreo.dev/component: test-app
      openchoreo.dev/component-uid: a1b2c3d4-5678-90ab-cdef-1234567890ab
      openchoreo.dev/environment: dev
      openchoreo.dev/environment-uid: d4e5f6a7-8901-23de-f012-4567890abcde
      openchoreo.dev/namespace: test-namespace
      openchoreo.dev/project: test-project
      openchoreo.dev/project-uid: b2c3d4e5-6789-01bc-def0-234567890abc
  data:
    database: mydb
`,
			wantErr: false,
		},
		{
			name:         "component with trait patches",
			wantMetadata: &RenderMetadata{ResourceCount: 1, BaseResourceCount: 1, TraitCount: 1, TraitResourceCount: 0, Warnings: []string{}},
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
      openchoreo.dev/component-uid: a1b2c3d4-5678-90ab-cdef-1234567890ab
      openchoreo.dev/environment: dev
      openchoreo.dev/environment-uid: d4e5f6a7-8901-23de-f012-4567890abcde
      openchoreo.dev/namespace: test-namespace
      openchoreo.dev/project: test-project
      openchoreo.dev/project-uid: b2c3d4e5-6789-01bc-def0-234567890abc
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
			name: "embedded trait creates new resources",
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
        mountPath: /var/data
  componentType:
    spec:
      parameters:
        openAPIV3Schema:
          type: object
          properties:
            mountPath:
              type: string
      traits:
        - name: storage
          instanceName: app-storage
          parameters:
            mountPath: ${parameters.mountPath}
            volumeName: app-data
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: ${metadata.name}
  traits:
    - metadata:
        name: storage
      spec:
        parameters:
          openAPIV3Schema:
            type: object
            properties:
              mountPath:
                type: string
              volumeName:
                type: string
        creates:
          - template:
              apiVersion: v1
              kind: PersistentVolumeClaim
              metadata:
                name: ${parameters.volumeName}
              spec:
                accessModes:
                  - ReadWriteOnce
                resources:
                  requests:
                    storage: 5Gi
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
      openchoreo.dev/component-uid: a1b2c3d4-5678-90ab-cdef-1234567890ab
      openchoreo.dev/environment: dev
      openchoreo.dev/environment-uid: d4e5f6a7-8901-23de-f012-4567890abcde
      openchoreo.dev/namespace: test-namespace
      openchoreo.dev/project: test-project
      openchoreo.dev/project-uid: b2c3d4e5-6789-01bc-def0-234567890abc
- apiVersion: v1
  kind: PersistentVolumeClaim
  metadata:
    name: app-data
    labels:
      openchoreo.dev/component: test-app
      openchoreo.dev/component-uid: a1b2c3d4-5678-90ab-cdef-1234567890ab
      openchoreo.dev/environment: dev
      openchoreo.dev/environment-uid: d4e5f6a7-8901-23de-f012-4567890abcde
      openchoreo.dev/namespace: test-namespace
      openchoreo.dev/project: test-project
      openchoreo.dev/project-uid: b2c3d4e5-6789-01bc-def0-234567890abc
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
			name: "embedded trait patches base resources",
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
      traits:
        - name: monitoring
          instanceName: embedded-mon
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
      openchoreo.dev/component-uid: a1b2c3d4-5678-90ab-cdef-1234567890ab
      openchoreo.dev/environment: dev
      openchoreo.dev/environment-uid: d4e5f6a7-8901-23de-f012-4567890abcde
      openchoreo.dev/namespace: test-namespace
      openchoreo.dev/project: test-project
      openchoreo.dev/project-uid: b2c3d4e5-6789-01bc-def0-234567890abc
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
			name: "both embedded and component-level traits",
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
        database: mydb
      traits:
        - name: mysql
          instanceName: db-1
          parameters:
            database: mydb
  componentType:
    spec:
      parameters:
        openAPIV3Schema:
          type: object
          properties:
            database:
              type: string
      traits:
        - name: monitoring
          instanceName: embedded-mon
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: ${metadata.name}
  traits:
    - metadata:
        name: monitoring
      spec:
        creates:
          - template:
              apiVersion: v1
              kind: ConfigMap
              metadata:
                name: monitoring-config
              data:
                enabled: "true"
    - metadata:
        name: mysql
      spec:
        parameters:
          openAPIV3Schema:
            type: object
            properties:
              database:
                type: string
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
- apiVersion: v1
  kind: ConfigMap
  metadata:
    name: monitoring-config
    labels:
      openchoreo.dev/component: test-app
      openchoreo.dev/component-uid: a1b2c3d4-5678-90ab-cdef-1234567890ab
      openchoreo.dev/environment: dev
      openchoreo.dev/environment-uid: d4e5f6a7-8901-23de-f012-4567890abcde
      openchoreo.dev/namespace: test-namespace
      openchoreo.dev/project: test-project
      openchoreo.dev/project-uid: b2c3d4e5-6789-01bc-def0-234567890abc
  data:
    enabled: "true"
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: test-component-dev-12345678
    labels:
      openchoreo.dev/component: test-app
      openchoreo.dev/component-uid: a1b2c3d4-5678-90ab-cdef-1234567890ab
      openchoreo.dev/environment: dev
      openchoreo.dev/environment-uid: d4e5f6a7-8901-23de-f012-4567890abcde
      openchoreo.dev/namespace: test-namespace
      openchoreo.dev/project: test-project
      openchoreo.dev/project-uid: b2c3d4e5-6789-01bc-def0-234567890abc
- apiVersion: v1
  kind: Secret
  metadata:
    name: db-1-secret
    labels:
      openchoreo.dev/component: test-app
      openchoreo.dev/component-uid: a1b2c3d4-5678-90ab-cdef-1234567890ab
      openchoreo.dev/environment: dev
      openchoreo.dev/environment-uid: d4e5f6a7-8901-23de-f012-4567890abcde
      openchoreo.dev/namespace: test-namespace
      openchoreo.dev/project: test-project
      openchoreo.dev/project-uid: b2c3d4e5-6789-01bc-def0-234567890abc
  data:
    database: mydb
`,
			wantErr: false,
		},
		{
			name: "embedded trait with CEL bindings resolved from component parameters",
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
        appPort: 8080
  componentType:
    spec:
      parameters:
        openAPIV3Schema:
          type: object
          properties:
            appPort:
              type: integer
      traits:
        - name: service-exposure
          instanceName: expose-1
          parameters:
            port: ${parameters.appPort}
            protocol: TCP
      resources:
        - id: deployment
          template:
            apiVersion: apps/v1
            kind: Deployment
            metadata:
              name: ${metadata.name}
  traits:
    - metadata:
        name: service-exposure
      spec:
        parameters:
          openAPIV3Schema:
            type: object
            properties:
              port:
                type: integer
              protocol:
                type: string
                default: TCP
        creates:
          - template:
              apiVersion: v1
              kind: Service
              metadata:
                name: ${metadata.name}
              spec:
                ports:
                  - port: ${parameters.port}
                    protocol: ${parameters.protocol}
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
      openchoreo.dev/component-uid: a1b2c3d4-5678-90ab-cdef-1234567890ab
      openchoreo.dev/environment: dev
      openchoreo.dev/environment-uid: d4e5f6a7-8901-23de-f012-4567890abcde
      openchoreo.dev/namespace: test-namespace
      openchoreo.dev/project: test-project
      openchoreo.dev/project-uid: b2c3d4e5-6789-01bc-def0-234567890abc
- apiVersion: v1
  kind: Service
  metadata:
    name: test-component-dev-12345678
    labels:
      openchoreo.dev/component: test-app
      openchoreo.dev/component-uid: a1b2c3d4-5678-90ab-cdef-1234567890ab
      openchoreo.dev/environment: dev
      openchoreo.dev/environment-uid: d4e5f6a7-8901-23de-f012-4567890abcde
      openchoreo.dev/namespace: test-namespace
      openchoreo.dev/project: test-project
      openchoreo.dev/project-uid: b2c3d4e5-6789-01bc-def0-234567890abc
  spec:
    ports:
      - port: 8080
        protocol: TCP
`,
			wantErr: false,
		},
		{
			name:         "component with configurations and secrets",
			wantMetadata: &RenderMetadata{ResourceCount: 7, BaseResourceCount: 7, TraitCount: 0, TraitResourceCount: 0, Warnings: []string{}},
			wantTargetPlanes: []string{
				v1alpha1.TargetPlaneDataPlane, v1alpha1.TargetPlaneDataPlane, v1alpha1.TargetPlaneDataPlane,
				v1alpha1.TargetPlaneDataPlane,
				v1alpha1.TargetPlaneDataPlane, v1alpha1.TargetPlaneDataPlane, v1alpha1.TargetPlaneDataPlane,
			},
			snapshotYAML:         loadTestDataFile(t, "configurations-and-secrets/snapshot.yaml"),
			settingsYAML:         loadTestDataFile(t, "configurations-and-secrets/settings.yaml"),
			environmentYAML:      devEnvironmentYAML,
			dataplaneYAML:        devDataplaneYAML,
			secretReferencesYAML: loadTestDataFile(t, "configurations-and-secrets/secret-references.yaml"),
			wantResourceYAML:     loadTestDataFile(t, "configurations-and-secrets/expected-resources.yaml"),
			wantErr:              false,
		},
		{
			name:                 "component with configurations using configFiles helper",
			snapshotYAML:         loadTestDataFile(t, "configurations-and-secrets/snapshot-with-config-helpers.yaml"),
			settingsYAML:         loadTestDataFile(t, "configurations-and-secrets/settings.yaml"),
			environmentYAML:      devEnvironmentYAML,
			dataplaneYAML:        devDataplaneYAML,
			secretReferencesYAML: loadTestDataFile(t, "configurations-and-secrets/secret-references.yaml"),
			wantResourceYAML:     loadTestDataFile(t, "configurations-and-secrets/expected-resources-with-config-helpers.yaml"),
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
					Name:               "test-component-dev-12345678",
					Namespace:          "test-namespace",
					ComponentName:      "test-app",
					ComponentUID:       "a1b2c3d4-5678-90ab-cdef-1234567890ab",
					ComponentNamespace: "test-namespace",
					ProjectName:        "test-project",
					ProjectUID:         "b2c3d4e5-6789-01bc-def0-234567890abc",
					DataPlaneName:      "dev-dataplane",
					DataPlaneUID:       "c3d4e5f6-7890-12cd-ef01-34567890abcd",
					EnvironmentName:    "dev",
					EnvironmentUID:     "d4e5f6a7-8901-23de-f012-4567890abcde",
					Labels: map[string]string{
						"openchoreo.dev/namespace":       "test-namespace",
						"openchoreo.dev/project":         "test-project",
						"openchoreo.dev/component":       "test-app",
						"openchoreo.dev/environment":     "dev",
						"openchoreo.dev/component-uid":   "a1b2c3d4-5678-90ab-cdef-1234567890ab",
						"openchoreo.dev/environment-uid": "d4e5f6a7-8901-23de-f012-4567890abcde",
						"openchoreo.dev/project-uid":     "b2c3d4e5-6789-01bc-def0-234567890abc",
					},
					Annotations: map[string]string{},
					PodSelectors: map[string]string{
						"openchoreo.dev/namespace":       "test-namespace",
						"openchoreo.dev/project":         "test-project",
						"openchoreo.dev/component":       "test-app",
						"openchoreo.dev/environment":     "dev",
						"openchoreo.dev/component-uid":   "a1b2c3d4-5678-90ab-cdef-1234567890ab",
						"openchoreo.dev/environment-uid": "d4e5f6a7-8901-23de-f012-4567890abcde",
						"openchoreo.dev/project-uid":     "b2c3d4e5-6789-01bc-def0-234567890abc",
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

			if tt.wantErr {
				return
			}
			if output == nil {
				t.Fatalf("Render() returned nil output without an error")
			}

			if tt.wantMetadata != nil {
				if diff := cmp.Diff(tt.wantMetadata, output.Metadata); diff != "" {
					t.Errorf("Metadata mismatch (-want +got):\n%s", diff)
				}
			}

			if tt.wantTargetPlanes != nil {
				if len(tt.wantTargetPlanes) != len(output.Resources) {
					t.Fatalf("wantTargetPlanes length %d != rendered resource count %d",
						len(tt.wantTargetPlanes), len(output.Resources))
				}
				for i, rr := range output.Resources {
					if rr.TargetPlane != tt.wantTargetPlanes[i] {
						t.Errorf("resource[%d] (%s) targetPlane = %q, want %q",
							i, resourceKindName(rr.Resource), rr.TargetPlane, tt.wantTargetPlanes[i])
					}
				}
			}

			if tt.wantResourceYAML != "" {
				// Parse expected resources
				var wantResources []map[string]any
				if err := yaml.Unmarshal([]byte(tt.wantResourceYAML), &wantResources); err != nil {
					t.Fatalf("Failed to parse wantResourceYAML: %v", err)
				}

				// Extract just the Resource field from RenderedResource
				actualResources := make([]map[string]any, len(output.Resources))
				for i, rr := range output.Resources {
					actualResources[i] = rr.Resource
				}

				actualYAML, err := yaml.Marshal(actualResources)
				if err != nil {
					t.Fatalf("Failed to marshal actual resources: %v", err)
				}
				var normalizedActualResources []map[string]any
				if err := yaml.Unmarshal(actualYAML, &normalizedActualResources); err != nil {
					t.Fatalf("Failed to unmarshal normalized actual resources: %v", err)
				}

				if diff := cmp.Diff(wantResources, normalizedActualResources, sortAnySlicesByName()); diff != "" {
					t.Errorf("Resources mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestValidateResources(t *testing.T) {
	tests := []struct {
		name      string
		resources []renderer.RenderedResource
		wantErr   bool
	}{
		{
			name: "valid resources",
			resources: []renderer.RenderedResource{
				{
					Resource: map[string]any{
						"apiVersion": "v1",
						"kind":       "Pod",
						"metadata": map[string]any{
							"name": "test",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing apiVersion",
			resources: []renderer.RenderedResource{
				{
					Resource: map[string]any{
						"kind": "Pod",
						"metadata": map[string]any{
							"name": "test",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing kind",
			resources: []renderer.RenderedResource{
				{
					Resource: map[string]any{
						"apiVersion": "v1",
						"metadata": map[string]any{
							"name": "test",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing metadata.name",
			resources: []renderer.RenderedResource{
				{
					Resource: map[string]any{
						"apiVersion": "v1",
						"kind":       "Pod",
						"metadata":   map[string]any{},
					},
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

func TestPipeline_SchemaValidation(t *testing.T) {
	baseMetadata := context.MetadataContext{
		Name:               "test",
		Namespace:          "ns",
		ComponentName:      "app",
		ComponentUID:       "uid1",
		ComponentNamespace: "ns",
		ProjectName:        "proj",
		ProjectUID:         "uid2",
		DataPlaneName:      "dp",
		DataPlaneUID:       "uid3",
		EnvironmentName:    "dev",
		EnvironmentUID:     "uid4",
		Labels:             map[string]string{},
		Annotations:        map[string]string{},
		PodSelectors:       map[string]string{"k": "v"},
	}

	tests := []struct {
		name               string
		componentTypeYAML  string
		componentYAML      string
		traitsYAML         string
		releaseBindingYAML string
		wantErrMsg         string
	}{
		{
			name: "component parameters missing required field",
			componentTypeYAML: `
spec:
  parameters:
    openAPIV3Schema:
      type: object
      required: [replicas]
      properties:
        replicas:
          type: integer
  resources:
    - id: deployment
      template: {apiVersion: v1, kind: Pod, metadata: {name: x}}
`,
			componentYAML: `spec: {parameters: {}}`,
			wantErrMsg:    "parameters validation failed",
		},
		{
			name: "component environmentConfigs missing required field",
			componentTypeYAML: `
spec:
  environmentConfigs:
    openAPIV3Schema:
      type: object
      required: [logLevel]
      properties:
        logLevel:
          type: string
  resources:
    - id: deployment
      template: {apiVersion: v1, kind: Pod, metadata: {name: x}}
`,
			componentYAML:      `spec: {}`,
			releaseBindingYAML: `spec: {componentTypeEnvironmentConfigs: {}}`,
			wantErrMsg:         "environmentConfigs validation failed",
		},
		{
			name: "trait parameters missing required field",
			componentTypeYAML: `
spec:
  resources:
    - id: deployment
      template: {apiVersion: v1, kind: Pod, metadata: {name: x}}
`,
			componentYAML: `
spec:
  traits:
    - name: storage
      instanceName: vol1
      parameters: {}
`,
			traitsYAML: `
- metadata: {name: storage}
  spec:
    parameters:
      openAPIV3Schema:
        type: object
        required: [size]
        properties:
          size:
            type: string
`,
			wantErrMsg: "parameters validation failed",
		},
		{
			name: "trait environmentConfigs missing required field",
			componentTypeYAML: `
spec:
  resources:
    - id: deployment
      template: {apiVersion: v1, kind: Pod, metadata: {name: x}}
`,
			componentYAML: `
spec:
  traits:
    - name: storage
      instanceName: vol1
`,
			traitsYAML: `
- metadata: {name: storage}
  spec:
    environmentConfigs:
      openAPIV3Schema:
        type: object
        required: [storageClass]
        properties:
          storageClass:
            type: string
`,
			releaseBindingYAML: `spec: {traitEnvironmentConfigs: {vol1: {}}}`,
			wantErrMsg:         "environmentConfigs validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var componentType v1alpha1.ComponentType
			if err := yaml.Unmarshal([]byte(tt.componentTypeYAML), &componentType); err != nil {
				t.Fatalf("Failed to parse componentType: %v", err)
			}

			var component v1alpha1.Component
			if err := yaml.Unmarshal([]byte(tt.componentYAML), &component); err != nil {
				t.Fatalf("Failed to parse component: %v", err)
			}

			var traits []v1alpha1.Trait
			if tt.traitsYAML != "" {
				if err := yaml.Unmarshal([]byte(tt.traitsYAML), &traits); err != nil {
					t.Fatalf("Failed to parse traits: %v", err)
				}
			}

			var releaseBinding *v1alpha1.ReleaseBinding
			if tt.releaseBindingYAML != "" {
				releaseBinding = &v1alpha1.ReleaseBinding{}
				if err := yaml.Unmarshal([]byte(tt.releaseBindingYAML), releaseBinding); err != nil {
					t.Fatalf("Failed to parse releaseBinding: %v", err)
				}
			}

			input := &RenderInput{
				ComponentType:  &componentType,
				Component:      &component,
				Traits:         traits,
				Workload:       &v1alpha1.Workload{},
				Environment:    &v1alpha1.Environment{},
				DataPlane:      &v1alpha1.DataPlane{},
				ReleaseBinding: releaseBinding,
				Metadata:       baseMetadata,
			}

			_, err := NewPipeline().Render(input)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErrMsg)
			} else {
				t.Log(err.Error())
			}
		})
	}
}

func TestPipeline_ValidationRules(t *testing.T) {
	baseMetadata := context.MetadataContext{
		Name:               "test",
		Namespace:          "ns",
		ComponentName:      "app",
		ComponentUID:       "uid1",
		ComponentNamespace: "ns",
		ProjectName:        "proj",
		ProjectUID:         "uid2",
		DataPlaneName:      "dp",
		DataPlaneUID:       "uid3",
		EnvironmentName:    "dev",
		EnvironmentUID:     "uid4",
		Labels:             map[string]string{},
		Annotations:        map[string]string{},
		PodSelectors:       map[string]string{"k": "v"},
	}

	tests := []struct {
		name              string
		componentTypeYAML string
		componentYAML     string
		traitsYAML        string
		wantErr           bool
		wantErrMsgs       []string
	}{
		{
			name: "component type validation rule passes",
			componentTypeYAML: `
spec:
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        replicas:
          type: integer
          default: 1
  validations:
    - rule: "${parameters.replicas > 0}"
      message: "replicas must be positive"
  resources:
    - id: deployment
      template: {apiVersion: apps/v1, kind: Deployment, metadata: {name: x}}
`,
			componentYAML: `spec: {parameters: {replicas: 3}}`,
			wantErr:       false,
		},
		{
			name: "component type validation rule fails with context",
			componentTypeYAML: `
spec:
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        replicas:
          type: integer
          default: 1
  validations:
    - rule: "${parameters.replicas > 5}"
      message: "replicas must be greater than 5"
  resources:
    - id: deployment
      template: {apiVersion: apps/v1, kind: Deployment, metadata: {name: x}}
`,
			componentYAML: `spec: {parameters: {replicas: 3}}`,
			wantErr:       true,
			wantErrMsgs: []string{
				"component type validation failed",
				"rule[0]",
				"evaluated to false",
				"replicas must be greater than 5",
			},
		},
		{
			name: "trait validation rule passes",
			componentTypeYAML: `
spec:
  resources:
    - id: deployment
      template: {apiVersion: apps/v1, kind: Deployment, metadata: {name: x}}
`,
			componentYAML: `
spec:
  traits:
    - name: storage
      instanceName: vol1
      parameters:
        size: 10
`,
			traitsYAML: `
- metadata: {name: storage}
  spec:
    parameters:
      openAPIV3Schema:
        type: object
        properties:
          size:
            type: integer
            default: 1
    validations:
      - rule: "${parameters.size > 0}"
        message: "size must be positive"
    creates:
      - template: {apiVersion: v1, kind: ConfigMap, metadata: {name: cfg}}
`,
			wantErr: false,
		},
		{
			name: "trait validation rule fails with context",
			componentTypeYAML: `
spec:
  resources:
    - id: deployment
      template: {apiVersion: apps/v1, kind: Deployment, metadata: {name: x}}
`,
			componentYAML: `
spec:
  traits:
    - name: storage
      instanceName: vol1
      parameters:
        size: 0
`,
			traitsYAML: `
- metadata: {name: storage}
  spec:
    parameters:
      openAPIV3Schema:
        type: object
        properties:
          size:
            type: integer
            default: 1
    validations:
      - rule: "${parameters.size > 0}"
        message: "size must be positive"
    creates:
      - template: {apiVersion: v1, kind: ConfigMap, metadata: {name: cfg}}
`,
			wantErr: true,
			wantErrMsgs: []string{
				"trait storage/vol1 validation failed",
				"rule[0]",
				"evaluated to false",
				"size must be positive",
			},
		},
		{
			name: "multiple validation rules all evaluated",
			componentTypeYAML: `
spec:
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        replicas:
          type: integer
          default: 1
        name:
          type: string
          default: app
  validations:
    - rule: "${parameters.replicas > 10}"
      message: "replicas too low"
    - rule: "${parameters.name != 'app'}"
      message: "name must not be app"
  resources:
    - id: deployment
      template: {apiVersion: apps/v1, kind: Deployment, metadata: {name: x}}
`,
			componentYAML: `spec: {parameters: {replicas: 1, name: app}}`,
			wantErr:       true,
			wantErrMsgs: []string{
				"rule[0]",
				"replicas too low",
				"rule[1]",
				"name must not be app",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var componentType v1alpha1.ComponentType
			if err := yaml.Unmarshal([]byte(tt.componentTypeYAML), &componentType); err != nil {
				t.Fatalf("Failed to parse componentType: %v", err)
			}

			var component v1alpha1.Component
			if err := yaml.Unmarshal([]byte(tt.componentYAML), &component); err != nil {
				t.Fatalf("Failed to parse component: %v", err)
			}

			var traits []v1alpha1.Trait
			if tt.traitsYAML != "" {
				if err := yaml.Unmarshal([]byte(tt.traitsYAML), &traits); err != nil {
					t.Fatalf("Failed to parse traits: %v", err)
				}
			}

			input := &RenderInput{
				ComponentType: &componentType,
				Component:     &component,
				Traits:        traits,
				Workload:      &v1alpha1.Workload{},
				Environment:   &v1alpha1.Environment{},
				DataPlane:     &v1alpha1.DataPlane{},
				Metadata:      baseMetadata,
			}

			_, err := NewPipeline().Render(input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				for _, msg := range tt.wantErrMsgs {
					if !strings.Contains(err.Error(), msg) {
						t.Errorf("error %q should contain %q", err.Error(), msg)
					}
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
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

func TestPipeline_DPResourceHashAnnotation(t *testing.T) {
	tests := []struct {
		name             string
		workloadType     string
		resources        []renderer.RenderedResource
		wantAnnotation   bool
		wantHashNotEmpty bool
	}{
		{
			name:         "deployment with configmap gets hash annotation",
			workloadType: "deployment",
			resources: []renderer.RenderedResource{
				{
					Resource: map[string]any{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata":   map[string]any{"name": "app"},
						"spec": map[string]any{
							"template": map[string]any{
								"metadata": map[string]any{},
								"spec":     map[string]any{},
							},
						},
					},
					TargetPlane: "dataplane",
				},
				{
					Resource: map[string]any{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]any{"name": "config"},
						"data":       map[string]any{"key": "value"},
					},
					TargetPlane: "dataplane",
				},
			},
			wantAnnotation:   true,
			wantHashNotEmpty: true,
		},
		{
			name:         "statefulset with secret gets hash annotation",
			workloadType: "statefulset",
			resources: []renderer.RenderedResource{
				{
					Resource: map[string]any{
						"apiVersion": "apps/v1",
						"kind":       "StatefulSet",
						"metadata":   map[string]any{"name": "app"},
						"spec": map[string]any{
							"template": map[string]any{
								"metadata": map[string]any{},
								"spec":     map[string]any{},
							},
						},
					},
					TargetPlane: "dataplane",
				},
				{
					Resource: map[string]any{
						"apiVersion": "v1",
						"kind":       "Secret",
						"metadata":   map[string]any{"name": "secret"},
						"data":       map[string]any{"password": "secret123"},
					},
					TargetPlane: "dataplane",
				},
			},
			wantAnnotation:   true,
			wantHashNotEmpty: true,
		},
		{
			name:         "deployment without non-workload resources has no annotation",
			workloadType: "deployment",
			resources: []renderer.RenderedResource{
				{
					Resource: map[string]any{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata":   map[string]any{"name": "app"},
						"spec": map[string]any{
							"template": map[string]any{
								"metadata": map[string]any{},
								"spec":     map[string]any{},
							},
						},
					},
					TargetPlane: "dataplane",
				},
			},
			wantAnnotation: false,
		},
		{
			name:         "cronjob workload type is skipped",
			workloadType: "cronjob",
			resources: []renderer.RenderedResource{
				{
					Resource: map[string]any{
						"apiVersion": "batch/v1",
						"kind":       "CronJob",
						"metadata":   map[string]any{"name": "job"},
						"spec": map[string]any{
							"jobTemplate": map[string]any{
								"spec": map[string]any{
									"template": map[string]any{
										"metadata": map[string]any{},
										"spec":     map[string]any{},
									},
								},
							},
						},
					},
					TargetPlane: "dataplane",
				},
				{
					Resource: map[string]any{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]any{"name": "config"},
						"data":       map[string]any{"key": "value"},
					},
					TargetPlane: "dataplane",
				},
			},
			wantAnnotation: false,
		},
		{
			name:         "observabilityplane resources are excluded from hash",
			workloadType: "deployment",
			resources: []renderer.RenderedResource{
				{
					Resource: map[string]any{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata":   map[string]any{"name": "app"},
						"spec": map[string]any{
							"template": map[string]any{
								"metadata": map[string]any{},
								"spec":     map[string]any{},
							},
						},
					},
					TargetPlane: "dataplane",
				},
				{
					Resource: map[string]any{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]any{"name": "metrics-config"},
						"data":       map[string]any{"scrape": "true"},
					},
					TargetPlane: "observabilityplane",
				},
			},
			wantAnnotation: false,
		},
		{
			name:         "deployment with service gets hash annotation",
			workloadType: "deployment",
			resources: []renderer.RenderedResource{
				{
					Resource: map[string]any{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata":   map[string]any{"name": "app"},
						"spec": map[string]any{
							"template": map[string]any{
								"metadata": map[string]any{},
								"spec":     map[string]any{},
							},
						},
					},
					TargetPlane: "dataplane",
				},
				{
					Resource: map[string]any{
						"apiVersion": "v1",
						"kind":       "Service",
						"metadata":   map[string]any{"name": "svc"},
						"spec":       map[string]any{"ports": []any{map[string]any{"port": 80}}},
					},
					TargetPlane: "dataplane",
				},
			},
			wantAnnotation:   true,
			wantHashNotEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &RenderInput{
				ComponentType: &v1alpha1.ComponentType{
					Spec: v1alpha1.ComponentTypeSpec{
						WorkloadType: tt.workloadType,
					},
				},
			}

			p := NewPipeline()
			err := p.addDPResourceHashAnnotation(tt.resources, input)
			if err != nil {
				t.Fatalf("addDPResourceHashAnnotation() error = %v", err)
			}

			// Find workload resource based on workload type
			var workloadResource map[string]any
			for _, rr := range tt.resources {
				kind, _ := rr.Resource["kind"].(string)
				if (tt.workloadType == "deployment" && kind == "Deployment") ||
					(tt.workloadType == "statefulset" && kind == "StatefulSet") {
					workloadResource = rr.Resource
					break
				}
			}

			if workloadResource == nil {
				if tt.wantAnnotation {
					t.Fatal("expected workload resource not found")
				}
				return
			}

			spec, _ := workloadResource["spec"].(map[string]any)
			template, _ := spec["template"].(map[string]any)
			templateMeta, _ := template["metadata"].(map[string]any)
			annotations, _ := templateMeta["annotations"].(map[string]any)

			hashValue, hasAnnotation := annotations["openchoreo.dev/dp-resource-hash"].(string)

			if tt.wantAnnotation && !hasAnnotation {
				t.Error("expected hash annotation but not found")
			}
			if !tt.wantAnnotation && hasAnnotation {
				t.Errorf("unexpected hash annotation found: %s", hashValue)
			}
			if tt.wantHashNotEmpty && hashValue == "" {
				t.Error("expected non-empty hash value")
			}
		})
	}
}

func TestHashDeterminism(t *testing.T) {
	resources := []renderer.RenderedResource{
		{
			Resource: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]any{"name": "config"},
				"data":       map[string]any{"key1": "value1", "key2": "value2"},
			},
			TargetPlane: "dataplane",
		},
	}

	// Compute hash multiple times
	var hashes []string
	for i := 0; i < 5; i++ {
		hashContent := make([]map[string]any, 0, len(resources))
		for _, rr := range resources {
			hashContent = append(hashContent, extractContentExcludingMetadata(rr.Resource))
		}
		hashes = append(hashes, computeTestHash(hashContent))
	}

	// All hashes should be identical
	for i := 1; i < len(hashes); i++ {
		if hashes[i] != hashes[0] {
			t.Errorf("hash not deterministic: %s != %s", hashes[i], hashes[0])
		}
	}
}

func TestHashOrderIndependence(t *testing.T) {
	// Create resources in different orders but with same content
	configMap := renderer.RenderedResource{
		Resource: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"name": "config", "namespace": "default"},
			"data":       map[string]any{"key": "value"},
		},
		TargetPlane: "dataplane",
	}
	secret := renderer.RenderedResource{
		Resource: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata":   map[string]any{"name": "secret", "namespace": "default"},
			"data":       map[string]any{"password": "secret123"},
		},
		TargetPlane: "dataplane",
	}
	service := renderer.RenderedResource{
		Resource: map[string]any{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata":   map[string]any{"name": "svc", "namespace": "default"},
			"spec":       map[string]any{"ports": []any{map[string]any{"port": 80}}},
		},
		TargetPlane: "dataplane",
	}
	deployment := renderer.RenderedResource{
		Resource: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]any{"name": "app", "namespace": "default"},
			"spec": map[string]any{
				"template": map[string]any{
					"metadata": map[string]any{},
					"spec":     map[string]any{},
				},
			},
		},
		TargetPlane: "dataplane",
	}

	// Order 1: ConfigMap, Secret, Service, Deployment
	resources1 := []renderer.RenderedResource{configMap, secret, service, deployment}

	// Order 2: Service, Deployment, ConfigMap, Secret (different order)
	resources2 := []renderer.RenderedResource{service, deployment, configMap, secret}

	// Order 3: Secret, Service, ConfigMap, Deployment (another different order)
	resources3 := []renderer.RenderedResource{secret, service, configMap, deployment}

	input := &RenderInput{
		ComponentType: &v1alpha1.ComponentType{
			Spec: v1alpha1.ComponentTypeSpec{
				WorkloadType: "deployment",
			},
		},
	}

	p := NewPipeline()

	// Compute hash for each order
	if err := p.addDPResourceHashAnnotation(resources1, input); err != nil {
		t.Fatalf("failed for order 1: %v", err)
	}
	hash1 := getHashFromDeployment(resources1)

	if err := p.addDPResourceHashAnnotation(resources2, input); err != nil {
		t.Fatalf("failed for order 2: %v", err)
	}
	hash2 := getHashFromDeployment(resources2)

	if err := p.addDPResourceHashAnnotation(resources3, input); err != nil {
		t.Fatalf("failed for order 3: %v", err)
	}
	hash3 := getHashFromDeployment(resources3)

	// All hashes should be identical regardless of resource order
	if hash1 != hash2 {
		t.Errorf("hash changed with different resource order: %s != %s", hash1, hash2)
	}
	if hash1 != hash3 {
		t.Errorf("hash changed with different resource order: %s != %s", hash1, hash3)
	}
}

func getHashFromDeployment(resources []renderer.RenderedResource) string {
	for _, rr := range resources {
		kind, _ := rr.Resource["kind"].(string)
		if kind == "Deployment" {
			spec := rr.Resource["spec"].(map[string]any)
			template := spec["template"].(map[string]any)
			templateMeta := template["metadata"].(map[string]any)
			annotations, _ := templateMeta["annotations"].(map[string]any)
			if annotations != nil {
				return annotations["openchoreo.dev/dp-resource-hash"].(string)
			}
		}
	}
	return ""
}

func TestHashChangesWithContent(t *testing.T) {
	content1 := []map[string]any{{"data": map[string]any{"key": "value1"}}}
	content2 := []map[string]any{{"data": map[string]any{"key": "value2"}}}

	hash1 := computeTestHash(content1)
	hash2 := computeTestHash(content2)

	if hash1 == hash2 {
		t.Error("different content should produce different hashes")
	}
}

func TestExtractContentExcludingMetadata(t *testing.T) {
	resource := map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":      "test",
			"namespace": "default",
			"labels":    map[string]any{"app": "test"},
		},
		"data": map[string]any{"key": "value"},
	}

	result := extractContentExcludingMetadata(resource)

	// Should not have metadata
	if _, ok := result["metadata"]; ok {
		t.Error("metadata should be excluded")
	}

	// Should have other fields
	if result["apiVersion"] != "v1" {
		t.Error("apiVersion should be preserved")
	}
	if result["kind"] != "ConfigMap" {
		t.Error("kind should be preserved")
	}
	if result["data"] == nil {
		t.Error("data should be preserved")
	}
}

func TestIsMainWorkloadKind(t *testing.T) {
	tests := []struct {
		kind         string
		workloadType string
		want         bool
	}{
		{"Deployment", "deployment", true},
		{"StatefulSet", "statefulset", true},
		{"Deployment", "statefulset", false},
		{"StatefulSet", "deployment", false},
		{"CronJob", "cronjob", false},
		{"Job", "job", false},
		{"ConfigMap", "deployment", false},
	}

	for _, tt := range tests {
		t.Run(tt.kind+"_"+tt.workloadType, func(t *testing.T) {
			got := isMainWorkloadKind(tt.kind, tt.workloadType)
			if got != tt.want {
				t.Errorf("isMainWorkloadKind(%q, %q) = %v, want %v", tt.kind, tt.workloadType, got, tt.want)
			}
		})
	}
}

// computeTestHash is a helper to compute hash for testing
func computeTestHash(content []map[string]any) string {
	// Import the hash package indirectly through the pipeline
	// We use the same algorithm as the production code
	p := NewPipeline()
	resources := make([]renderer.RenderedResource, 0, 1+len(content))
	resources = append(resources, renderer.RenderedResource{
		Resource: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]any{"name": "test"},
			"spec": map[string]any{
				"template": map[string]any{
					"metadata": map[string]any{},
				},
			},
		},
		TargetPlane: "dataplane",
	})

	// Add test content as additional resources
	for _, c := range content {
		resources = append(resources, renderer.RenderedResource{
			Resource: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]any{"name": "test-config"},
				"data":       c["data"],
			},
			TargetPlane: "dataplane",
		})
	}

	input := &RenderInput{
		ComponentType: &v1alpha1.ComponentType{
			Spec: v1alpha1.ComponentTypeSpec{
				WorkloadType: "deployment",
			},
		},
	}

	_ = p.addDPResourceHashAnnotation(resources, input)

	// Extract the hash from the deployment
	spec := resources[0].Resource["spec"].(map[string]any)
	template := spec["template"].(map[string]any)
	templateMeta := template["metadata"].(map[string]any)
	annotations := templateMeta["annotations"].(map[string]any)

	return annotations["openchoreo.dev/dp-resource-hash"].(string)
}

func TestUsesEndpointResources(t *testing.T) {
	withTemplate := func(raw string) *RenderInput {
		return &RenderInput{
			ComponentType: &v1alpha1.ComponentType{
				Spec: v1alpha1.ComponentTypeSpec{
					Resources: []v1alpha1.ResourceTemplate{
						{ID: "r", Template: &runtime.RawExtension{Raw: []byte(raw)}},
					},
				},
			},
		}
	}

	t.Run("absent when no template references the macro", func(t *testing.T) {
		in := withTemplate(`{"spec": {"port": "${endpoint.value.port}"}}`)
		if usesEndpointResources(in) {
			t.Fatal("expected false when macro is not used")
		}
	})

	t.Run("detected in a component type resource template", func(t *testing.T) {
		in := withTemplate(`{"spec": {"m": "${workload.toEndpointResources()[endpoint.key]}"}}`)
		if !usesEndpointResources(in) {
			t.Fatal("expected true when macro is used in a resource template")
		}
	})

	t.Run("detected in a trait", func(t *testing.T) {
		in := withTemplate(`{"spec": {}}`)
		in.Traits = []v1alpha1.Trait{{
			Spec: v1alpha1.TraitSpec{
				Creates: []v1alpha1.TraitCreate{
					{Template: &runtime.RawExtension{Raw: []byte(`{"x": "${workload.toEndpointResources()}"}`)}},
				},
			},
		}}
		if !usesEndpointResources(in) {
			t.Fatal("expected true when macro is used in a trait")
		}
	})
}

// postRenderTestMetadata returns a minimal MetadataContext for the post/pre-render
// validation pipeline tests below.
func postRenderTestMetadata() context.MetadataContext {
	return context.MetadataContext{
		Name:               "test",
		Namespace:          "ns",
		ComponentName:      "app",
		ComponentUID:       "uid1",
		ComponentNamespace: "ns",
		ProjectName:        "proj",
		ProjectUID:         "uid2",
		DataPlaneName:      "dp",
		DataPlaneUID:       "uid3",
		EnvironmentName:    "dev",
		EnvironmentUID:     "uid4",
		Labels:             map[string]string{},
		Annotations:        map[string]string{},
		PodSelectors:       map[string]string{"k": "v"},
	}
}

// renderPipelineYAML builds a RenderInput from the given YAML fragments and runs the pipeline.
func renderPipelineYAML(t *testing.T, componentTypeYAML, componentYAML, traitsYAML string) (*RenderOutput, error) {
	t.Helper()
	var componentType v1alpha1.ComponentType
	if err := yaml.Unmarshal([]byte(componentTypeYAML), &componentType); err != nil {
		t.Fatalf("Failed to parse componentType: %v", err)
	}
	var component v1alpha1.Component
	if err := yaml.Unmarshal([]byte(componentYAML), &component); err != nil {
		t.Fatalf("Failed to parse component: %v", err)
	}
	var traits []v1alpha1.Trait
	if traitsYAML != "" {
		if err := yaml.Unmarshal([]byte(traitsYAML), &traits); err != nil {
			t.Fatalf("Failed to parse traits: %v", err)
		}
	}
	input := &RenderInput{
		ComponentType: &componentType,
		Component:     &component,
		Traits:        traits,
		Workload:      &v1alpha1.Workload{},
		Environment:   &v1alpha1.Environment{},
		DataPlane:     &v1alpha1.DataPlane{},
		Metadata:      postRenderTestMetadata(),
	}
	return NewPipeline().Render(input)
}

// renderWithTraitPostValidation renders a ComponentType that emits a single Deployment
// with the given replicas, plus one component-level trait carrying a post-render validation.
func renderWithTraitPostValidation(t *testing.T, replicas int, rule, message string, mustMatch *bool, when string) (*RenderOutput, error) {
	t.Helper()
	ct := fmt.Sprintf(`
spec:
  resources:
    - id: deployment
      template: {apiVersion: apps/v1, kind: Deployment, metadata: {name: web}, spec: {replicas: %d}}
`, replicas)
	comp := `
spec:
  traits:
    - name: post
      instanceName: p1
`
	var b strings.Builder
	b.WriteString("- metadata: {name: post}\n")
	b.WriteString("  spec:\n")
	b.WriteString("    postRenderValidations:\n")
	b.WriteString("      - target:\n")
	b.WriteString("          group: apps\n")
	b.WriteString("          version: v1\n")
	b.WriteString("          kind: Deployment\n")
	if mustMatch != nil {
		b.WriteString(fmt.Sprintf("          mustMatch: %t\n", *mustMatch))
	}
	if when != "" {
		b.WriteString(fmt.Sprintf("        when: %q\n", when))
	}
	b.WriteString(fmt.Sprintf("        rule: %q\n", rule))
	b.WriteString(fmt.Sprintf("        message: %q\n", message))
	return renderPipelineYAML(t, ct, comp, b.String())
}

// renderWithRemoveThenPostValidation renders a ComponentType emitting a Deployment, then
// a trait that removes it, then a second trait whose post-render validation targets the
// (now missing) Deployment with default mustMatch=true.
func renderWithRemoveThenPostValidation(t *testing.T) (*RenderOutput, error) {
	t.Helper()
	ct := `
spec:
  resources:
    - id: deployment
      template: {apiVersion: apps/v1, kind: Deployment, metadata: {name: web}, spec: {replicas: 1}}
`
	comp := `
spec:
  traits:
    - name: remover
      instanceName: r1
    - name: post
      instanceName: p1
`
	traits := `
- metadata: {name: remover}
  spec:
    removes:
      - target: {group: apps, version: v1, kind: Deployment}
- metadata: {name: post}
  spec:
    postRenderValidations:
      - target: {group: apps, version: v1, kind: Deployment}
        rule: "${resource.spec.replicas == 1}"
        message: "deployment must survive"
`
	return renderPipelineYAML(t, ct, comp, traits)
}

// renderWithTraitPreRenderValidation renders a ComponentType emitting a Deployment plus a
// trait using preRenderValidations (the non-deprecated alias) with the given rule.
func renderWithTraitPreRenderValidation(t *testing.T, rule, message string) (*RenderOutput, error) {
	t.Helper()
	ct := `
spec:
  resources:
    - id: deployment
      template: {apiVersion: apps/v1, kind: Deployment, metadata: {name: web}, spec: {replicas: 1}}
`
	comp := `
spec:
  traits:
    - name: pre
      instanceName: p1
`
	traits := fmt.Sprintf(`
- metadata: {name: pre}
  spec:
    preRenderValidations:
      - rule: %q
        message: %q
`, rule, message)
	return renderPipelineYAML(t, ct, comp, traits)
}

func TestRender_PostRenderValidationPasses(t *testing.T) {
	out, err := renderWithTraitPostValidation(t, 1,
		"${resource.spec.replicas == 1}", "must be single replica", nil, "")
	if err != nil {
		t.Fatalf("expected render to pass, got %v", err)
	}
	if out == nil || len(out.Resources) == 0 {
		t.Fatalf("expected rendered resources")
	}
}

func TestRender_PostRenderValidationFailsBlocksRender(t *testing.T) {
	_, err := renderWithTraitPostValidation(t, 3,
		"${resource.spec.replicas == 1}", "must be single replica", nil, "")
	if err == nil || !strings.Contains(err.Error(), "must be single replica") {
		t.Fatalf("expected post-render validation failure, got %v", err)
	}
	if !strings.Contains(err.Error(), "post-render validation") {
		t.Fatalf("expected error wrapped as post-render validation, got %v", err)
	}
}

func TestRender_PostRenderMustMatchAfterRemove(t *testing.T) {
	_, err := renderWithRemoveThenPostValidation(t)
	if err == nil || !strings.Contains(err.Error(), "no resource matched target") {
		t.Fatalf("expected mustMatch failure after remove, got %v", err)
	}
}

func TestRender_PreRenderValidationsAliasHonored(t *testing.T) {
	_, err := renderWithTraitPreRenderValidation(t, "${1 == 2}", "pre-render boom")
	if err == nil || !strings.Contains(err.Error(), "pre-render boom") {
		t.Fatalf("expected pre-render validation failure, got %v", err)
	}
}

// renderWithForEachPostValidation renders a ComponentType emitting two HTTPRoutes ("a","b")
// and attaches a trait whose forEach post-render validation iterates parameters.routes
// (a, b, gone); the "gone" iteration selects no resource → default mustMatch fails.
func renderWithForEachPostValidation(t *testing.T) (*RenderOutput, error) {
	t.Helper()
	ct := `
spec:
  resources:
    - id: route-a
      template: {apiVersion: gateway.networking.k8s.io/v1, kind: HTTPRoute, metadata: {name: a}, spec: {rules: [{}]}}
    - id: route-b
      template: {apiVersion: gateway.networking.k8s.io/v1, kind: HTTPRoute, metadata: {name: b}, spec: {rules: [{}]}}
`
	comp := `
spec:
  traits:
    - name: routecheck
      instanceName: rc
      parameters:
        routes:
          - name: a
          - name: b
          - name: gone
`
	traits := `
- metadata: {name: routecheck}
  spec:
    parameters:
      openAPIV3Schema:
        type: object
        properties:
          routes:
            type: array
            items:
              type: object
              properties:
                name: {type: string}
    postRenderValidations:
      - forEach: "${parameters.routes}"
        var: route
        target:
          group: gateway.networking.k8s.io
          version: v1
          kind: HTTPRoute
          where: "${resource.metadata.name == route.name}"
        rule: "${resource.spec.rules.size() > 0}"
        message: "route ${route.name} lost its rules"
`
	return renderPipelineYAML(t, ct, comp, traits)
}

func TestRender_PostRenderForEachPerItemMustMatch(t *testing.T) {
	_, err := renderWithForEachPostValidation(t)
	if err == nil || !strings.Contains(err.Error(), "no resource matched target") {
		t.Fatalf("expected forEach per-item mustMatch failure for the missing route, got %v", err)
	}
}

// renderWithEmbeddedTraitPostValidation attaches a post-render validation to an EMBEDDED
// trait (ComponentType.Spec.Traits, collected at pipeline.go:232-238) rather than a
// component-level trait — the branch no other post-render test exercises. The embedded
// trait binds expectedName from the component context via CEL, and the rule compares the
// rendered Deployment's name against it, so a passing render pins that the embedded trait's
// own resolved context (not the component context) is what the validation evaluates against.
func renderWithEmbeddedTraitPostValidation(t *testing.T, appName string) (*RenderOutput, error) {
	t.Helper()
	ct := `
spec:
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        appName:
          type: string
  traits:
    - name: post
      instanceName: e1
      parameters:
        expectedName: ${parameters.appName}
  resources:
    - id: deployment
      template: {apiVersion: apps/v1, kind: Deployment, metadata: {name: web}, spec: {replicas: 1}}
`
	comp := fmt.Sprintf(`
spec:
  parameters:
    appName: %q
`, appName)
	traits := `
- metadata: {name: post}
  spec:
    parameters:
      openAPIV3Schema:
        type: object
        properties:
          expectedName:
            type: string
    postRenderValidations:
      - target: {group: apps, version: v1, kind: Deployment}
        rule: "${resource.metadata.name == parameters.expectedName}"
        message: "deployment name must match expectedName"
`
	return renderPipelineYAML(t, ct, comp, traits)
}

func TestRender_EmbeddedTraitPostRenderValidationFailsBlocksRender(t *testing.T) {
	// A failing post-render validation on an EMBEDDED trait must block the render. This is
	// the only test that exercises the embedded collection branch (pipeline.go:232-238); the
	// others attach the trait via Component.Spec.Traits (pipeline.go:289-295).
	_, err := renderWithEmbeddedTraitPostValidation(t, "mismatch")
	if err == nil {
		t.Fatalf("expected embedded-trait post-render failure, got nil")
	}
	if !strings.Contains(err.Error(), "post-render validation") {
		t.Fatalf("expected error wrapped as post-render validation, got %v", err)
	}
	if !strings.Contains(err.Error(), "deployment name must match expectedName") {
		t.Fatalf("expected the rule message in the error, got %v", err)
	}
	// The embedded trait's label (name/instanceName) must appear — proving the embedded
	// branch, not the component-level one, produced the pending validation.
	if !strings.Contains(err.Error(), "post/e1") {
		t.Fatalf("expected embedded trait label 'post/e1' in the error, got %v", err)
	}

	// Passing variant: when the CEL-bound expectedName matches the rendered Deployment name,
	// the render succeeds — pinning that expectedName was resolved from the embedded trait's
	// own context rather than left unresolved or bound against the wrong context.
	if _, err := renderWithEmbeddedTraitPostValidation(t, "web"); err != nil {
		t.Fatalf("expected embedded-trait post-render to pass when name matches, got %v", err)
	}
}

// renderWithComponentTypePostValidation renders a ComponentType that emits a single Deployment
// with the given replicas and carries the post-render validation directly on the ComponentType
// (no traits).
func renderWithComponentTypePostValidation(t *testing.T, replicas int, rule, message string, mustMatch *bool, when string) (*RenderOutput, error) {
	t.Helper()
	var b strings.Builder
	b.WriteString("spec:\n")
	b.WriteString("  postRenderValidations:\n")
	b.WriteString("    - target:\n")
	b.WriteString("        group: apps\n")
	b.WriteString("        version: v1\n")
	b.WriteString("        kind: Deployment\n")
	if mustMatch != nil {
		b.WriteString(fmt.Sprintf("        mustMatch: %t\n", *mustMatch))
	}
	if when != "" {
		b.WriteString(fmt.Sprintf("      when: %q\n", when))
	}
	b.WriteString(fmt.Sprintf("      rule: %q\n", rule))
	b.WriteString(fmt.Sprintf("      message: %q\n", message))
	b.WriteString("  resources:\n")
	b.WriteString("    - id: deployment\n")
	b.WriteString(fmt.Sprintf("      template: {apiVersion: apps/v1, kind: Deployment, metadata: {name: web}, spec: {replicas: %d}}\n", replicas))
	return renderPipelineYAML(t, b.String(), "spec: {}", "")
}

// renderWithComponentTypePostValidationAndTraitMutation renders a ComponentType that emits a
// Deployment with replicas 1 and asserts (via its own post-render validation) replicas == 1,
// then attaches a trait that patches replicas to 3. Because ComponentType post-render runs
// after all traits, the validation must observe the mutated value and fail.
func renderWithComponentTypePostValidationAndTraitMutation(t *testing.T) (*RenderOutput, error) {
	t.Helper()
	ct := `
spec:
  postRenderValidations:
    - target: {group: apps, version: v1, kind: Deployment}
      rule: "${resource.spec.replicas == 1}"
      message: "a trait changed replicas"
  resources:
    - id: deployment
      template: {apiVersion: apps/v1, kind: Deployment, metadata: {name: web}, spec: {replicas: 1}}
`
	comp := `
spec:
  traits:
    - name: bump
      instanceName: b1
`
	traits := `
- metadata: {name: bump}
  spec:
    patches:
      - target: {group: apps, version: v1, kind: Deployment}
        operations:
          - {op: replace, path: /spec/replicas, value: 3}
`
	return renderPipelineYAML(t, ct, comp, traits)
}

// renderWithComponentTypePreRenderValidation renders a ComponentType emitting a Deployment
// and carrying a preRenderValidations rule (the non-deprecated alias) on the ComponentType.
func renderWithComponentTypePreRenderValidation(t *testing.T, rule, message string) (*RenderOutput, error) {
	t.Helper()
	ct := fmt.Sprintf(`
spec:
  preRenderValidations:
    - rule: %q
      message: %q
  resources:
    - id: deployment
      template: {apiVersion: apps/v1, kind: Deployment, metadata: {name: web}, spec: {replicas: 1}}
`, rule, message)
	return renderPipelineYAML(t, ct, "spec: {}", "")
}

func TestRender_ComponentTypePostRenderValidationPasses(t *testing.T) {
	out, err := renderWithComponentTypePostValidation(t, 1,
		"${resource.spec.replicas == 1}", "must be single replica", nil, "")
	if err != nil {
		t.Fatalf("expected render to pass, got %v", err)
	}
	if out == nil || len(out.Resources) == 0 {
		t.Fatalf("expected rendered resources")
	}
}

func TestRender_ComponentTypePostRenderValidationFailsBlocksRender(t *testing.T) {
	_, err := renderWithComponentTypePostValidation(t, 3,
		"${resource.spec.replicas == 1}", "must be single replica", nil, "")
	if err == nil || !strings.Contains(err.Error(), "must be single replica") {
		t.Fatalf("expected post-render validation failure, got %v", err)
	}
	if !strings.Contains(err.Error(), "post-render validation") {
		t.Fatalf("expected error wrapped as post-render validation, got %v", err)
	}
}

func TestRender_ComponentTypePostRenderRunsAfterTraits(t *testing.T) {
	_, err := renderWithComponentTypePostValidationAndTraitMutation(t)
	if err == nil || !strings.Contains(err.Error(), "post-render validation") {
		t.Fatalf("expected CT post-render to observe the trait mutation and fail, got %v", err)
	}
	if !strings.Contains(err.Error(), "a trait changed replicas") {
		t.Fatalf("expected CT post-render message, got %v", err)
	}
}

func TestRender_ComponentTypePreRenderValidationsAliasHonored(t *testing.T) {
	_, err := renderWithComponentTypePreRenderValidation(t, "${1 == 2}", "pre-render boom")
	if err == nil || !strings.Contains(err.Error(), "pre-render boom") {
		t.Fatalf("expected pre-render validation failure, got %v", err)
	}
}
