# Component Pipeline Benchmarks

This document explains the benchmark suite for the component rendering pipeline and how to interpret the results.

## Running Benchmarks

```bash
# Run all benchmarks
go test -bench=. -benchmem ./internal/crd-renderer/component-pipeline/

# Run specific benchmark
go test -bench=BenchmarkPipeline_RenderWithRealSample -benchmem

# Generate CPU profile
go test -bench=BenchmarkPipeline_RenderWithRealSample -cpuprofile=cpu.prof
go tool pprof cpu.prof

# Generate memory profile
go test -bench=BenchmarkPipeline_RenderWithRealSample -memprofile=mem.prof
go tool pprof mem.prof
```

## Benchmark Results

**Last Updated**: 2025-11-01

**Environment**: Apple M4 Pro, Go 1.23+, darwin/arm64

```
BenchmarkPipeline_RenderWithRealSample-14                         5121     208426 ns/op     376801 B/op     3448 allocs/op
BenchmarkPipeline_RenderWithRealSample_NewPipelinePerRender-14     739    1609488 ns/op    2052846 B/op    30083 allocs/op
BenchmarkPipeline_RenderSimple-14                                61321      20327 ns/op      33382 B/op      394 allocs/op
BenchmarkPipeline_RenderWithForEach-14                           36991      32770 ns/op      49341 B/op      597 allocs/op
```

### Performance Characteristics

| Benchmark           | Time    | Memory  | Allocations | Throughput |
| ------------------- | ------- | ------- | ----------- | ---------- |
| RealSample (shared) | 208 μs  | 377 KB  | 3,448       | 4,798/sec  |
| NewPipeline (cold)  | 1.61 ms | 2.05 MB | 30,083      | 621/sec    |
| Simple              | 20.3 μs | 33.4 KB | 394         | 49,195/sec |
| ForEach             | 32.8 μs | 49.3 KB | 597         | 30,520/sec |

### Key Comparison: Shared vs New Pipeline

| Metric          | Shared Instance | New Per Render | Difference |
| --------------- | --------------- | -------------- | ---------- |
| **Time**        | 208 μs          | 1.61 ms        | 7.7x       |
| **Memory**      | 377 KB          | 2.05 MB        | 5.4x       |
| **Allocations** | 3,448           | 30,083         | 8.7x       |

## Performance Characteristics

### Cache Impact & Sharing

The template engine uses **2-level LRU caching**:

**Level 1 - Environment Cache** (100 entries max):

- Caches CEL environments by top-level variable names only (not values or types)
- Typically uses **only 2 entries** in production:
  1. Component context: `{parameters, metadata, workload, component, environment}`
  2. Addon context: `{parameters, metadata, addon, component, environment}`

**Level 2 - Program Cache** (2000 entries max):

- Caches compiled CEL programs by `(environment_key, expression)`
- Expected usage: ~875 programs for typical deployment (5 CTDs × 25 expressions + 50 addons × 15 expressions)
- Provides 2.3x headroom for growth

**Key Insight**: A single `Pipeline` instance can be shared across:

- All component types (web-service, worker, cron-job, etc.)
- All components of the same type
- All environments (dev, staging, prod)
- All addons of any type

**Cache Effectiveness**:

- Environment cache: Near 100% hit rate after warmup (2 entries shared globally)
- Program cache: Very high hit rate for repeated component types
  - Benchmark shows 100% hit rate (single component type + addon rendered repeatedly)
  - Production hit rate varies with component type diversity
  - Expected: ~875 unique programs cached for typical deployment (5 CTDs + 50 addons)
- Warm cache performance: 208μs vs cold cache: 1.61ms

### Memory Allocations

| Benchmark            | Allocations | Memory  | Note                                               |
| -------------------- | ----------- | ------- | -------------------------------------------------- |
| RenderSimple         | 394         | 33.4 KB | Baseline (2 resources)                             |
| RenderWithForEach    | 597         | 49.3 KB | 5x iterations = ~1.5x allocations                  |
| RealSample           | 3,448       | 377 KB  | Full addon processing (2 base + 1 addon + patches) |
| NewPipelinePerRender | 30,083      | 2.05 MB | Cold cache penalty (8.7x more allocations)         |

## Regression Detection

Use these benchmarks to detect performance regressions:

```bash
# Install benchstat (if not already installed)
go install golang.org/x/perf/cmd/benchstat@latest

# Run benchmarks and save baseline
go test -bench=. -benchmem ./internal/crd-renderer/component-pipeline/ > baseline.txt

# After making changes, compare
go test -bench=. -benchmem ./internal/crd-renderer/component-pipeline/ > new.txt
benchstat baseline.txt new.txt
```

**Acceptable ranges** (for regression monitoring):

- RenderSimple: 18-25μs (baseline, 2 resources)
- RenderWithForEach: 28-40μs (5 iterations)
- RealSample: 180-250μs (full featured with addons)
- NewPipelinePerRender: 1.4-2.0ms (cold cache scenario)
