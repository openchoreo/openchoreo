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
		ct          *v1alpha1.ComponentType
		traits      []v1alpha1.Trait
		component   *v1alpha1.Component
		workload    *v1alpha1.Workload
		deployment  *v1alpha1.ComponentDeployment
		environment *v1alpha1.Environment
		dataplane   *v1alpha1.DataPlane
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

		case "ComponentDeployment":
			deployment = &v1alpha1.ComponentDeployment{}
			if err := yaml.Unmarshal([]byte(doc), deployment); err != nil {
				tb.Fatalf("Failed to parse ComponentDeployment: %v", err)
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
	if ct == nil || component == nil || workload == nil || deployment == nil {
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
		if deployment == nil {
			missing = append(missing, "ComponentDeployment")
		}
		tb.Fatalf("Missing required resources in sample file: %v", missing)
		return nil // Never reached, but satisfies linter
	}

	// Build ComponentEnvSnapshot (all pointers guaranteed non-nil here)
	snapshot := &v1alpha1.ComponentEnvSnapshot{
		Spec: v1alpha1.ComponentEnvSnapshotSpec{
			Environment:   deployment.Spec.Environment,
			Component:     *component,
			ComponentType: *ct,
			Workload:      *workload,
			Traits:        traits,
		},
	}

	// Create render input
	return &RenderInput{
		ComponentType:       &snapshot.Spec.ComponentType,
		Component:           &snapshot.Spec.Component,
		Traits:              snapshot.Spec.Traits,
		Workload:            &snapshot.Spec.Workload,
		Environment:         environment,
		DataPlane:           dataplane,
		ComponentDeployment: deployment,
		Metadata: context.MetadataContext{
			Name:            "demo-app-dev-12345678",
			Namespace:       "dp-demo-project-development-x1y2z3w4",
			ComponentName:   "demo-app",
			EnvironmentName: "development",
			ProjectName:     "demo-project",
			Labels: map[string]string{
				"openchoreo.dev/component":   "demo-app",
				"openchoreo.dev/environment": "development",
				"openchoreo.dev/project":     "demo-project",
			},
			PodSelectors: map[string]string{
				"openchoreo.dev/component-id": "demo-app-12345678",
				"openchoreo.dev/environment":  "development",
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
	for i := 0; i < b.N; i++ {
		_, err := pipeline.Render(input)
		if err != nil {
			b.Fatalf("Pipeline render failed on iteration %d: %v", i, err)
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
	for i := 0; i < b.N; i++ {
		pipeline := NewPipeline() // ← NEW INSTANCE per iteration
		_, err := pipeline.Render(input)
		if err != nil {
			b.Fatalf("Pipeline render failed on iteration %d: %v", i, err)
		}
	}
}

// BenchmarkPipeline_RenderSimple benchmarks a minimal pipeline without traits
// to establish a baseline for comparison.
func BenchmarkPipeline_RenderSimple(b *testing.B) {
	snapshotYAML := `
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
        port: 8080
  componentType:
    spec:
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
  workload: {}
`

	environmentYAML := `
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

	snapshot := &v1alpha1.ComponentEnvSnapshot{}
	if err := yaml.Unmarshal([]byte(snapshotYAML), snapshot); err != nil {
		b.Fatalf("Failed to parse snapshot: %v", err)
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
		ComponentType: &snapshot.Spec.ComponentType,
		Component:     &snapshot.Spec.Component,
		Traits:        snapshot.Spec.Traits,
		Workload:      &snapshot.Spec.Workload,
		Environment:   &environment,
		DataPlane:     &dataplane,
		Metadata: context.MetadataContext{
			Name:      "test-app-dev-12345678",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"openchoreo.dev/component": "test-app",
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

	for i := 0; i < b.N; i++ {
		_, err := pipeline.Render(input)
		if err != nil {
			b.Fatalf("Pipeline render failed: %v", err)
		}
	}
}

// BenchmarkPipeline_RenderWithForEach benchmarks forEach iteration performance
// which is affected by context cloning.
func BenchmarkPipeline_RenderWithForEach(b *testing.B) {
	snapshotYAML := `
apiVersion: core.choreo.dev/v1alpha1
kind: ComponentEnvSnapshot
spec:
  environment: dev
  component:
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
  componentType:
    spec:
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
  workload: {}
`

	environmentYAML := `
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

	snapshot := &v1alpha1.ComponentEnvSnapshot{}
	if err := yaml.Unmarshal([]byte(snapshotYAML), snapshot); err != nil {
		b.Fatalf("Failed to parse snapshot: %v", err)
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
		ComponentType: &snapshot.Spec.ComponentType,
		Component:     &snapshot.Spec.Component,
		Traits:        snapshot.Spec.Traits,
		Workload:      &snapshot.Spec.Workload,
		Environment:   &environment,
		DataPlane:     &dataplane,
		Metadata: context.MetadataContext{
			Name:      "test-app-dev-12345678",
			Namespace: "test-namespace",
		},
	}

	pipeline := NewPipeline()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
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
