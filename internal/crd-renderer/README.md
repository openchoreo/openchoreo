# CRD Renderer

The `crd-renderer` package provides the core rendering infrastructure for OpenChoreo's ComponentTypeDefinitions. It transforms declarative component specifications into fully-resolved Kubernetes resource manifests by combining schemas, templates, patches, and CEL-based evaluation.

## Package Structure

```
internal/crd-renderer/
├── component-pipeline/    # Main orchestration pipeline
│   ├── addon/            # Addon processing (creates and patches)
│   ├── context/          # CEL evaluation context building
│   └── renderer/         # Base resource rendering
├── schema/               # Structural schema and defaulting
├── schemaextractor/      # Shorthand schema syntax parser
├── template/             # CEL-based template engine
└── patch/                # JSON Patch with filtering extensions
```

## Package Overview

### component-pipeline

The main orchestration layer that ties all rendering components together. It implements the complete workflow for transforming a `ComponentEnvSnapshot` and `EnvSettings` into rendered Kubernetes manifests.

**Key responsibilities:**
- Build CEL evaluation contexts from component parameters, environment overrides, and defaults
- Render base resources from ComponentTypeDefinition templates
- Process addons (resource creation and patching)
- Post-process resources (validation, labels, annotations, sorting)

**Entry point:** `Pipeline.Render(input *RenderInput) (*RenderOutput, error)`

### schema

Aggregates multiple schema definitions from different sources (parameters, environment overrides, addon inputs) and provides utilities for schema conversion and defaulting.

**Key functions:**
- `ToJSONSchema(def Definition) (*extv1.JSONSchemaProps, error)` – merges multiple field maps and converts to OpenAPI v3 JSON Schema using schemaextractor
- `ToStructural(def Definition) (*apiextschema.Structural, error)` – converts JSON Schema to Kubernetes Structural Schema format
- `ApplyDefaults(target map[string]any, structural *apiextschema.Structural) map[string]any` – applies defaults using Kubernetes defaulting algorithm

### schemaextractor

Parses compact schema shorthand syntax and converts it to OpenAPI v3 JSON Schema. This makes defining configuration parameters simple without writing verbose schema definitions.

**Example shorthand:**
```yaml
replicas: 'integer | default=1 | minimum=0'
environment: 'string | enum=dev,staging,prod | default=dev'
```

See [schemaextractor/README.md](./schemaextractor/README.md) for detailed syntax and usage.

### template

A CEL-backed template engine that evaluates expressions embedded in YAML/JSON structures. Supports inline expressions, dynamic map keys, and nested structures.

**Key Features:**
- **Standalone expressions** - Return native types (strings, numbers, booleans, lists, maps)
- **Interpolated expressions** - String concatenation with automatic type coercion
- **Dynamic map keys** - Computed key names (must evaluate to strings)
- **Helper functions** - `omit()`, `merge()`, `sanitizeK8sResourceName()`
- **CEL extensions** - Strings, encoders, math, lists, sets
- **Performance caching** - Two-level cache for environments and compiled programs

**Example:**
```yaml
metadata:
  name: ${metadata.name}
  labels: ${metadata.labels}
spec:
  replicas: ${parameters.replicas}
  image: "myapp:${parameters.version}"
```

See [template/README.md](./template/README.md) for detailed syntax, examples, and usage patterns.

### patch

Implements JSON Patch (RFC 6902) with OpenChoreo-specific extensions for resource manipulation.

**Extensions:**
- Array filtering: `/spec/containers[?(@.name=='app')]/env/-`
- Automatic parent creation: creates intermediate maps/arrays as needed
- `mergeShallow` operation: overlays top-level keys while preserving siblings
- CEL-based targeting: select resources using `where` filters

See [patch/README.md](./patch/README.md) for detailed operations and examples.

## Rendering Workflow

The complete rendering pipeline follows this workflow:

```
ComponentEnvSnapshot + EnvSettings
          ↓
    [1. Build Context]
          ↓
    CEL Evaluation Context
    (parameters + overrides + defaults)
          ↓
    [2. Render Base Resources]
          ↓
    Template Engine → Base Resources
          ↓
    [3. Process Addons]
          ↓
    Addon Creates → New Resources
    Addon Patches → Modified Resources
          ↓
    [4. Post-Process]
          ↓
    Validation, Labels, Annotations
          ↓
    Rendered Kubernetes Manifests
```

### Step-by-Step

1. **Build Context** (`context` package)
   - Merge component parameters from snapshot
   - Apply environment-specific overrides from settings
   - Extract schema from ComponentTypeDefinition
   - Apply defaults using structural schema
   - Build final CEL evaluation context

2. **Render Base Resources** (`renderer` + `template`)
   - Evaluate resource templates from ComponentTypeDefinition.Spec.Resources
   - Use template engine to resolve CEL expressions
   - Produce initial set of Kubernetes resources

3. **Process Addons** (`addon` package)
   - Execute addon "creates" to generate additional resources
   - Execute addon "patches" to modify existing resources
   - Both use the template engine and patch engine

4. **Post-Process** (`component-pipeline`)
   - Validate resources
   - Add standard labels and annotations
   - Sort resources for deterministic output
   - Return final manifest set

## Usage Example

```go
import (
    "github.com/openchoreo/openchoreo/internal/crd-renderer/component-pipeline"
)

// Create pipeline
pipeline := componentpipeline.NewPipeline(
    componentpipeline.WithValidation(true),
)

// Prepare input
input := &componentpipeline.RenderInput{
    Snapshot: componentEnvSnapshot,  // From Component + ComponentTypeDefinition
    Settings: envSettings,           // Environment-specific overrides
    Metadata: metadata,              // Labels, annotations, etc.
}

// Render
output, err := pipeline.Render(input)
if err != nil {
    // handle error
}

// Use rendered resources
for _, resource := range output.Resources {
    // Apply to cluster, save to file, etc.
}
```

## Testing

Each package includes comprehensive tests:

```bash
# Test entire crd-renderer
go test ./internal/crd-renderer/...

# Test specific packages
go test ./internal/crd-renderer/schemaextractor
go test ./internal/crd-renderer/schema
go test ./internal/crd-renderer/template
go test ./internal/crd-renderer/patch
go test ./internal/crd-renderer/component-pipeline
```
