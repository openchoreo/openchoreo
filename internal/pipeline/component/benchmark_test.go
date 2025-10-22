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
		ctd        *v1alpha1.ComponentTypeDefinition
		addons     []v1alpha1.Addon
		component  *v1alpha1.Component
		workload   *v1alpha1.Workload
		deployment *v1alpha1.ComponentDeployment
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
		case "ComponentTypeDefinition":
			ctd = &v1alpha1.ComponentTypeDefinition{}
			if err := yaml.Unmarshal([]byte(doc), ctd); err != nil {
				tb.Fatalf("Failed to parse ComponentTypeDefinition: %v", err)
			}

		case "Addon":
			var addon v1alpha1.Addon
			if err := yaml.Unmarshal([]byte(doc), &addon); err != nil {
				tb.Fatalf("Failed to parse Addon: %v", err)
			}
			addons = append(addons, addon)

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

		default:
			tb.Logf("Skipping unknown resource kind: %s", kind.Kind)
		}
	}

	// Validate required resources and construct snapshot
	// Using explicit checks to satisfy staticcheck SA5011
	if ctd == nil || component == nil || workload == nil || deployment == nil {
		var missing []string
		if ctd == nil {
			missing = append(missing, "ComponentTypeDefinition")
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
			Environment:             deployment.Spec.Environment,
			Component:               *component,
			ComponentTypeDefinition: *ctd,
			Workload:                *workload,
			Addons:                  addons,
		},
	}

	// Create render input
	return &RenderInput{
		ComponentTypeDefinition: &snapshot.Spec.ComponentTypeDefinition,
		Component:               &snapshot.Spec.Component,
		Addons:                  snapshot.Spec.Addons,
		Workload:                &snapshot.Spec.Workload,
		Environment:             snapshot.Spec.Environment,
		ComponentDeployment:     deployment,
		Metadata: context.MetadataContext{
			Name:      "demo-app-dev-12345678",
			Namespace: "dp-demo-project-development-x1y2z3w4",
			Labels: map[string]string{
				"openchoreo.org/component":   "demo-app",
				"openchoreo.org/environment": "development",
				"openchoreo.org/project":     "demo-project",
			},
			PodSelectors: map[string]string{
				"openchoreo.org/component-id": "demo-app-12345678",
				"openchoreo.org/environment":  "development",
			},
		},
	}
}

// BenchmarkPipeline_RenderWithRealSample benchmarks the full pipeline using the
// realistic sample from samples/component-with-addons/component-with-addons.yaml
//
// This benchmark measures:
// - Template engine cache effectiveness (CEL environment caching)
// - Full pipeline performance with addons, patches, and creates
// - Memory allocations in the hot path
//
// Run with:
//
//	go test -bench=BenchmarkPipeline_RenderWithRealSample -benchmem
//	go test -bench=BenchmarkPipeline_RenderWithRealSample -benchmem -cpuprofile=cpu.prof
func BenchmarkPipeline_RenderWithRealSample(b *testing.B) {
	// Load sample and build render input
	samplePath := "../../../samples/component-with-addons/component-with-addons.yaml"
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

	// Expected: 2 base resources (Deployment, Service) + 1 addon create (PVC) = 3 resources
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
	samplePath := "../../../samples/component-with-addons/component-with-addons.yaml"
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

// BenchmarkPipeline_RenderSimple benchmarks a minimal pipeline without addons
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
  componentTypeDefinition:
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

	snapshot := &v1alpha1.ComponentEnvSnapshot{}
	if err := yaml.Unmarshal([]byte(snapshotYAML), snapshot); err != nil {
		b.Fatalf("Failed to parse snapshot: %v", err)
	}

	input := &RenderInput{
		ComponentTypeDefinition: &snapshot.Spec.ComponentTypeDefinition,
		Component:               &snapshot.Spec.Component,
		Addons:                  snapshot.Spec.Addons,
		Workload:                &snapshot.Spec.Workload,
		Environment:             snapshot.Spec.Environment,
		Metadata: context.MetadataContext{
			Name:      "test-app-dev-12345678",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"openchoreo.org/component": "test-app",
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
  componentTypeDefinition:
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

	snapshot := &v1alpha1.ComponentEnvSnapshot{}
	if err := yaml.Unmarshal([]byte(snapshotYAML), snapshot); err != nil {
		b.Fatalf("Failed to parse snapshot: %v", err)
	}

	input := &RenderInput{
		ComponentTypeDefinition: &snapshot.Spec.ComponentTypeDefinition,
		Component:               &snapshot.Spec.Component,
		Addons:                  snapshot.Spec.Addons,
		Workload:                &snapshot.Spec.Workload,
		Environment:             snapshot.Spec.Environment,
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
