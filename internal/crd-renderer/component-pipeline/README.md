# Component Pipeline

The component pipeline is responsible for rendering Kubernetes resource manifests from OpenChoreo component definitions.

## Overview

The pipeline takes a `ComponentEnvSnapshot` (which contains a component, its type definition, workload, and addons) and produces fully resolved Kubernetes resources ready to be applied to a cluster.

### Workflow

```
Input: ComponentEnvSnapshot + ComponentDeployment + Metadata
  ↓
  1. Build context (parameters + overrides + defaults)
  ↓
  2. Render base resources (from ComponentTypeDefinition)
  ↓
  3. Process addons (creates + patches)
  ↓
  4. Post-process (labels, annotations, validation)
  ↓
Output: []map[string]any (Kubernetes resource manifests)
```

## Usage

### Controller Usage (Recommended)

**DO**: Create **one pipeline instance** and share it across all reconciliations.

- Add `Pipeline *componentpipeline.Pipeline` field to your reconciler
- Initialize once during controller setup
- Reuse in every reconciliation

**Recommendation**: One pipeline per controller. Creating more doesn't provide benefits since the cache is shared.

**DON'T**: Create a new pipeline per reconciliation.

**Performance impact of creating new pipeline per render:**

- +1.42ms slower per render
- +1.16MB extra memory per render
- +18,672 extra allocations per render

## Thread Safety

### The Pipeline is Fully Thread-Safe

You can safely use a **single pipeline instance** across multiple goroutines (concurrent reconciliations).

**Why it's safe:**

1. **CEL Environment Cache** - Protected by `sync.Mutex` - all cache operations are synchronized
2. **Immutable CEL Environments** - Cached environments are never modified after creation, safe to share
3. **Stateless Rendering** - Each `Render()` call uses only local variables, no shared mutable state
4. **Read-Only Options** - Options are set once during initialization, then only read

Multiple goroutines can call `pipeline.Render()` concurrently without any coordination.

**Note**: You can even share a single pipeline instance across multiple controllers if desired, though one per controller is typical.

## Caching & Performance

The pipeline uses 2-level LRU caching for CEL environments and compiled programs. Caches are based on context structure (variable names), not values, enabling high reuse across all component types.

**Key insight**: All components share the same context structure, so a single pipeline instance effectively caches compiled programs for the entire system.

**Performance**: Warm cache renders complete in ~268μs vs ~1.69ms cold (6.3x faster).

See [BENCHMARKS.md](./BENCHMARKS.md) for detailed performance analysis, cache effectiveness metrics, and optimization guidance.

## Configuration Options

Available options:

- `WithValidation(bool)` - Enable/disable resource validation (default: enabled). Validation failures always cause rendering to fail.
- `WithResourceLabels(map[string]string)` - Add custom labels to all resources
- `WithResourceAnnotations(map[string]string)` - Add custom annotations to all resources

**Note**: If you need different options for different renders, create multiple pipeline instances (e.g., one with validation disabled for testing).

## Testing

### Run All Tests

```bash
go test ./internal/crd-renderer/component-pipeline/...
```

### Run Benchmarks

```bash
# Run all benchmarks
go test -bench=. -benchmem ./internal/crd-renderer/component-pipeline/

# Compare shared vs new-per-render
go test -bench="BenchmarkPipeline_RenderWithRealSample" -benchmem

# Generate CPU profile
go test -bench=BenchmarkPipeline_RenderWithRealSample -cpuprofile=cpu.prof
go tool pprof cpu.prof
```

See [BENCHMARKS.md](./BENCHMARKS.md) for detailed performance analysis.

## Architecture

### Component Structure

```
component-pipeline/
├── pipeline.go          # Main pipeline orchestration
├── types.go            # Input/output types
├── options.go          # Configuration options
├── context/            # Context building (parameters, overrides, defaults)
│   ├── builder.go      # Entry points
│   ├── component.go    # Component context
│   └── addon.go        # Addon context
├── renderer/           # Base resource rendering
│   └── renderer.go     # Template evaluation for ComponentTypeDefinition
└── addon/              # Addon processing
    └── processor.go    # Addon creates and patches
```

### Dependencies

- **template**: CEL-based template engine for expression evaluation
- **patch**: JSON Patch/JSON Merge Patch for addon patching
- **schema**: Schema definition and defaults application
- **schemaextractor**: Schema extraction from type definitions

## Common Patterns

### Building Metadata Context

The controller is responsible for computing resource names and namespaces. The `MetadataContext` should include:

- `Name` - Base resource name (e.g., "my-app-dev-12345678")
- `Namespace` - Target namespace (e.g., "dp-org-project-dev-x1y2z3w4")
- `Labels` - Common labels for all resources
- `PodSelectors` - Selectors for pod identity (used in Deployments, Services, etc.)

### Error Handling

Check `err` from `pipeline.Render()` for rendering errors.

The pipeline validates that all rendered resources have required fields (`apiVersion`, `kind`, `metadata.name`). Resources missing these fields will cause rendering to fail with an error.

### Output Resources

The rendered resources are independent copies. You can safely modify them without affecting subsequent renders.

## Related Documentation

- [BENCHMARKS.md](./BENCHMARKS.md) - Performance analysis and benchmarks
- [Template Engine](../template/README.md) - CEL expression evaluation
- [Patch Package](../patch/README.md) - JSON Patch operations
- [Schema Package](../schema/README.md) - Schema validation and defaults
