// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"os"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/context"
	"github.com/openchoreo/openchoreo/internal/template"
)

// resourceKind is a helper struct to identify resource type before full unmarshalling
type resourceKind struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

// buildRenderInputFromSample loads a multi-document YAML sample file and constructs
// a RenderInput by identifying resources by their Kind field rather than document order.
// This makes the test more resilient to YAML reordering.
func buildRenderInputFromSample(tb testing.TB, samplePath string) *RenderInput {
	tb.Helper()

	// Load sample file
	data, err := os.ReadFile(samplePath)
	if err != nil {
		tb.Fatalf("Failed to read sample file %s: %v", samplePath, err)
	}

	// Parse multi-document YAML
	docs := strings.Split(string(data), "\n---\n")

	var (
		ct             *v1alpha1.ComponentType
		traits         []v1alpha1.Trait
		component      *v1alpha1.Component
		workload       *v1alpha1.Workload
		releaseBinding *v1alpha1.ReleaseBinding
		environment    *v1alpha1.Environment
		dataplane      *v1alpha1.DataPlane
	)

	// Parse each document by identifying its kind
	for i, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		// First, identify the resource kind
		var kind resourceKind
		if err := yaml.Unmarshal([]byte(doc), &kind); err != nil {
			tb.Fatalf("Failed to parse resource kind from document %d: %v", i, err)
		}

		// Unmarshal into appropriate type based on kind
		switch kind.Kind {
		case "ComponentType":
			ct = &v1alpha1.ComponentType{}
			if err := yaml.Unmarshal([]byte(doc), ct); err != nil {
				tb.Fatalf("Failed to parse ComponentType: %v", err)
			}

		case "Trait":
			var trait v1alpha1.Trait
			if err := yaml.Unmarshal([]byte(doc), &trait); err != nil {
				tb.Fatalf("Failed to parse Trait: %v", err)
			}
			traits = append(traits, trait)

		case "Component":
			component = &v1alpha1.Component{}
			if err := yaml.Unmarshal([]byte(doc), component); err != nil {
				tb.Fatalf("Failed to parse Component: %v", err)
			}

		case "Workload":
			workload = &v1alpha1.Workload{}
			if err := yaml.Unmarshal([]byte(doc), workload); err != nil {
				tb.Fatalf("Failed to parse Workload: %v", err)
			}

		case "ReleaseBinding":
			releaseBinding = &v1alpha1.ReleaseBinding{}
			if err := yaml.Unmarshal([]byte(doc), releaseBinding); err != nil {
				tb.Fatalf("Failed to parse ReleaseBinding: %v", err)
			}
		case "Environment":
			environment = &v1alpha1.Environment{}
			if err := yaml.Unmarshal([]byte(doc), environment); err != nil {
				tb.Fatalf("Failed to parse Environment: %v", err)
			}
		case "DataPlane":
			dataplane = &v1alpha1.DataPlane{}
			if err := yaml.Unmarshal([]byte(doc), dataplane); err != nil {
				tb.Fatalf("Failed to parse DataPlane: %v", err)
			}

		default:
			tb.Logf("Skipping unknown resource kind: %s", kind.Kind)
		}
	}

	// Validate required resources and construct snapshot
	// Using explicit checks to satisfy staticcheck SA5011
	if ct == nil || component == nil || workload == nil || releaseBinding == nil {
		var missing []string
		if ct == nil {
			missing = append(missing, "ComponentType")
		}
		if component == nil {
			missing = append(missing, "Component")
		}
		if workload == nil {
			missing = append(missing, "Workload")
		}
		if releaseBinding == nil {
			missing = append(missing, "ReleaseBinding")
		}
		tb.Fatalf("Missing required resources in sample file: %v", missing)
		return nil // Never reached, but satisfies linter
	}

	// Create render input directly (all pointers guaranteed non-nil here)
	return &RenderInput{
		ComponentType:  ct,
		Component:      component,
		Traits:         traits,
		Workload:       workload,
		Environment:    environment,
		DataPlane:      dataplane,
		ReleaseBinding: releaseBinding,
		Metadata: context.MetadataContext{
			Name:            "demo-app-dev-12345678",
			Namespace:       "dp-demo-project-development-x1y2z3w4",
			ComponentName:   "demo-app",
			ComponentUID:    "a1b2c3d4-5678-90ab-cdef-1234567890ab",
			ProjectName:     "demo-project",
			ProjectUID:      "b2c3d4e5-6789-01bc-def0-234567890abc",
			DataPlaneName:   "dev-dataplane",
			DataPlaneUID:    "c3d4e5f6-7890-12cd-ef01-34567890abcd",
			EnvironmentName: "development",
			EnvironmentUID:  "d4e5f6a7-8901-23de-f012-4567890abcde",
			Labels: map[string]string{
				"openchoreo.dev/namespace":       "demo-namespace",
				"openchoreo.dev/project":         "demo-project",
				"openchoreo.dev/component":       "demo-app",
				"openchoreo.dev/environment":     "development",
				"openchoreo.dev/component-uid":   "a1b2c3d4-5678-90ab-cdef-1234567890ab",
				"openchoreo.dev/environment-uid": "d4e5f6a7-8901-23de-f012-4567890abcde",
				"openchoreo.dev/project-uid":     "b2c3d4e5-6789-01bc-def0-234567890abc",
			},
			Annotations: map[string]string{},
			PodSelectors: map[string]string{
				"openchoreo.dev/namespace":       "dp-demo-project-development-x1y2z3w4",
				"openchoreo.dev/project":         "demo-project",
				"openchoreo.dev/component":       "demo-app",
				"openchoreo.dev/environment":     "development",
				"openchoreo.dev/component-uid":   "a1b2c3d4-5678-90ab-cdef-1234567890ab",
				"openchoreo.dev/environment-uid": "d4e5f6a7-8901-23de-f012-4567890abcde",
				"openchoreo.dev/project-uid":     "b2c3d4e5-6789-01bc-def0-234567890abc",
			},
		},
	}
}

// BenchmarkPipeline_RenderWithRealSample benchmarks the full pipeline using the
// realistic sample from samples/component-with-traits/component-with-traits.yaml
//
// This benchmark measures:
// - Template engine cache effectiveness (CEL environment caching)
// - Full pipeline performance with traits, patches, and creates
// - Memory allocations in the hot path
//
// Run with:
//
//	go test -bench=BenchmarkPipeline_RenderWithRealSample -benchmem
//	go test -bench=BenchmarkPipeline_RenderWithRealSample -benchmem -cpuprofile=cpu.prof
func BenchmarkPipeline_RenderWithRealSample(b *testing.B) {
	// Load sample and build render input
	samplePath := "./testdata/component-with-traits.yaml"
	input := buildRenderInputFromSample(b, samplePath)

	// To test with no caching:
	// engine := template.NewEngineWithOptions(template.DisableCache())
	// pipeline := NewPipeline(WithTemplateEngine(engine))

	// To test with env cache only:
	// engine := template.NewEngineWithOptions(template.DisableProgramCacheOnly())
	// pipeline := NewPipeline(WithTemplateEngine(engine))

	// Default: full caching
	pipeline := NewPipeline()

	// Verify it works before benchmarking
	output, err := pipeline.Render(input)
	if err != nil {
		b.Fatalf("Pipeline render failed: %v", err)
	}
	if len(output.Resources) == 0 {
		b.Fatal("Expected resources to be rendered, got 0")
	}

	// Expected: 2 base resources (Deployment, Service) + 1 trait create (PVC) = 3 resources
	expectedResources := 3
	if len(output.Resources) != expectedResources {
		b.Logf("Resources rendered: %d (expected %d)", len(output.Resources), expectedResources)
	}

	// Reset timer to exclude setup
	b.ResetTimer()

	// Run benchmark
	for b.Loop() {
		_, err := pipeline.Render(input)
		if err != nil {
			b.Fatalf("Pipeline render failed: %v", err)
		}
	}
}

// BenchmarkPipeline_RenderWithRealSample_NewPipelinePerRender benchmarks the old approach
// of creating a new pipeline instance for every render (cold cache every time).
//
// This simulates the BEFORE state where the controller created a new pipeline per reconciliation.
// Compare this with BenchmarkPipeline_RenderWithRealSample to see the benefit of sharing
// a single pipeline instance.
//
// Run with:
//
//	go test -bench="BenchmarkPipeline_RenderWithRealSample" -benchmem
func BenchmarkPipeline_RenderWithRealSample_NewPipelinePerRender(b *testing.B) {
	// Load sample and build render input
	samplePath := "./testdata/component-with-traits.yaml"
	input := buildRenderInputFromSample(b, samplePath)

	// Verify it works before benchmarking
	pipeline := NewPipeline()
	output, err := pipeline.Render(input)
	if err != nil {
		b.Fatalf("Pipeline render failed: %v", err)
	}
	if len(output.Resources) == 0 {
		b.Fatal("Expected resources to be rendered, got 0")
	}

	// Reset timer to exclude setup
	b.ResetTimer()

	// Run benchmark - create NEW pipeline for each iteration
	// This simulates the old controller behavior (cold cache every time)
	for b.Loop() {
		pipeline := NewPipeline() // ← NEW INSTANCE per iteration
		_, err := pipeline.Render(input)
		if err != nil {
			b.Fatalf("Pipeline render failed: %v", err)
		}
	}
}

// BenchmarkPipeline_RenderSimple benchmarks a minimal pipeline without traits
// to establish a baseline for comparison.
func BenchmarkPipeline_RenderSimple(b *testing.B) {
	componentYAML := `
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: test-app
spec:
  parameters:
    replicas: 2
    port: 8080
`

	componentTypeYAML := `
apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: test-type
spec:
  workloadType: deployment
  schema:
    parameters:
      replicas: "integer | default=1"
      port: "integer | default=8080"
  resources:
    - id: deployment
      template:
        apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: ${metadata.name}
          namespace: ${metadata.namespace}
        spec:
          replicas: ${parameters.replicas}
          template:
            spec:
              containers:
                - name: app
                  ports:
                    - containerPort: ${parameters.port}
    - id: service
      template:
        apiVersion: v1
        kind: Service
        metadata:
          name: ${metadata.name}
          namespace: ${metadata.namespace}
        spec:
          ports:
            - port: 80
              targetPort: ${parameters.port}
`

	workloadYAML := `
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: test-workload
spec: {}
`

	environmentYAML := `
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
        uri: https://auth.example.com/.well-known/jwks.json
`

	dataplaneYAML := `
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
`

	component := &v1alpha1.Component{}
	if err := yaml.Unmarshal([]byte(componentYAML), component); err != nil {
		b.Fatalf("Failed to parse component: %v", err)
	}

	componentType := &v1alpha1.ComponentType{}
	if err := yaml.Unmarshal([]byte(componentTypeYAML), componentType); err != nil {
		b.Fatalf("Failed to parse component type: %v", err)
	}

	workload := &v1alpha1.Workload{}
	if err := yaml.Unmarshal([]byte(workloadYAML), workload); err != nil {
		b.Fatalf("Failed to parse workload: %v", err)
	}

	environment := v1alpha1.Environment{}
	if err := yaml.Unmarshal([]byte(environmentYAML), &environment); err != nil {
		b.Fatalf("Failed to parse environment: %v", err)
	}

	dataplane := v1alpha1.DataPlane{}
	if err := yaml.Unmarshal([]byte(dataplaneYAML), &dataplane); err != nil {
		b.Fatalf("Failed to parse dataplane: %v", err)
	}

	input := &RenderInput{
		ComponentType: componentType,
		Component:     component,
		Traits:        []v1alpha1.Trait{},
		Workload:      workload,
		Environment:   &environment,
		DataPlane:     &dataplane,
		Metadata: context.MetadataContext{
			Name:            "test-app-dev-12345678",
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
				"openchoreo.dev/component": "test-app",
			},
			Annotations: map[string]string{},
			PodSelectors: map[string]string{
				"openchoreo.dev/component-uid": "a1b2c3d4-5678-90ab-cdef-1234567890ab",
			},
		},
	}

	pipeline := NewPipeline()

	// Verify it works
	_, err := pipeline.Render(input)
	if err != nil {
		b.Fatalf("Pipeline render failed: %v", err)
	}

	b.ResetTimer()

	for b.Loop() {
		_, err := pipeline.Render(input)
		if err != nil {
			b.Fatalf("Pipeline render failed: %v", err)
		}
	}
}

// BenchmarkPipeline_RenderWithForEach benchmarks forEach iteration performance
// which is affected by context cloning.
func BenchmarkPipeline_RenderWithForEach(b *testing.B) {
	componentYAML := `
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: test-app
spec:
  parameters:
    envVars:
      - name: VAR1
        value: value1
      - name: VAR2
        value: value2
      - name: VAR3
        value: value3
      - name: VAR4
        value: value4
      - name: VAR5
        value: value5
`

	componentTypeYAML := `
apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: test-type
spec:
  workloadType: deployment
  schema:
    types:
      EnvVar:
        name: string
        value: string
    parameters:
      envVars: "[]EnvVar"
  resources:
    - id: configmaps
      forEach: ${parameters.envVars}
      var: env
      template:
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: ${metadata.name}-${env.name}
        data:
          value: ${env.value}
`

	workloadYAML := `
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: test-workload
spec: {}
`

	environmentYAML := `
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
        uri: https://auth.example.com/.well-known/jwks.json
`

	dataplaneYAML := `
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
`

	component := &v1alpha1.Component{}
	if err := yaml.Unmarshal([]byte(componentYAML), component); err != nil {
		b.Fatalf("Failed to parse component: %v", err)
	}

	componentType := &v1alpha1.ComponentType{}
	if err := yaml.Unmarshal([]byte(componentTypeYAML), componentType); err != nil {
		b.Fatalf("Failed to parse component type: %v", err)
	}

	workload := &v1alpha1.Workload{}
	if err := yaml.Unmarshal([]byte(workloadYAML), workload); err != nil {
		b.Fatalf("Failed to parse workload: %v", err)
	}

	environment := v1alpha1.Environment{}
	if err := yaml.Unmarshal([]byte(environmentYAML), &environment); err != nil {
		b.Fatalf("Failed to parse environment: %v", err)
	}

	dataplane := v1alpha1.DataPlane{}
	if err := yaml.Unmarshal([]byte(dataplaneYAML), &dataplane); err != nil {
		b.Fatalf("Failed to parse dataplane: %v", err)
	}

	input := &RenderInput{
		ComponentType: componentType,
		Component:     component,
		Traits:        []v1alpha1.Trait{},
		Workload:      workload,
		Environment:   &environment,
		DataPlane:     &dataplane,
		Metadata: context.MetadataContext{
			Name:            "test-app-dev-12345678",
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

	pipeline := NewPipeline()

	b.ResetTimer()

	for b.Loop() {
		_, err := pipeline.Render(input)
		if err != nil {
			b.Fatalf("Pipeline render failed: %v", err)
		}
	}
}

// WithTemplateEngine is an option to set a custom template engine for benchmarking.
// Use this to test different caching strategies:
//
// Example - Benchmark with no caching:
//
//	func BenchmarkPipeline_RenderWithRealSample(b *testing.B) {
//	    // ... setup code ...
//	    engine := template.NewEngineWithOptions(template.DisableCache())
//	    pipeline := NewPipeline(WithTemplateEngine(engine))
//	    // ... benchmark code ...
//	}
//
// Example - Benchmark with only env cache (no program cache):
//
//	engine := template.NewEngineWithOptions(template.DisableProgramCacheOnly())
//	pipeline := NewPipeline(WithTemplateEngine(engine))
func WithTemplateEngine(engine *template.Engine) Option {
	return func(p *Pipeline) {
		p.templateEngine = engine
	}
}
