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

**Last Updated**: 2025-01-28

**Environment**: Apple M1, Go 1.23+, darwin/arm64

```
BenchmarkPipeline_RenderWithRealSample-8                        4400     268502 ns/op     207206 B/op     2703 allocs/op
BenchmarkPipeline_RenderWithRealSample_NewPipelinePerRender-8    731    1691351 ns/op    1371706 B/op    21375 allocs/op
BenchmarkPipeline_RenderSimple-8                               42600      28525 ns/op      33375 B/op      397 allocs/op
BenchmarkPipeline_RenderWithForEach-8                          26385      45065 ns/op      49299 B/op      596 allocs/op
```

### Performance Characteristics

| Benchmark           | Time    | Memory  | Allocations | Throughput |
| ------------------- | ------- | ------- | ----------- | ---------- |
| RealSample (shared) | 268 μs  | 207 KB  | 2,703       | 3,725/sec  |
| NewPipeline (cold)  | 1.69 ms | 1.37 MB | 21,375      | 592/sec    |
| Simple              | 28.5 μs | 33.4 KB | 397         | 35,000/sec |
| ForEach             | 45.1 μs | 49.3 KB | 596         | 22,173/sec |

### Key Comparison: Shared vs New Pipeline

| Metric          | Shared Instance | New Per Render | Difference |
| --------------- | --------------- | -------------- | ---------- |
| **Time**        | 268 μs          | 1.69 ms        | 6.3x       |
| **Memory**      | 207 KB          | 1.37 MB        | 6.6x       |
| **Allocations** | 2,703           | 21,375         | 7.9x       |

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
- Warm cache performance: 268μs vs cold cache: 1.69ms

### Memory Allocations

| Benchmark            | Allocations | Memory  | Note                                               |
| -------------------- | ----------- | ------- | -------------------------------------------------- |
| RenderSimple         | 397         | 33.4 KB | Baseline (2 resources)                             |
| RenderWithForEach    | 596         | 49.3 KB | 5x iterations = ~1.5x allocations                  |
| RealSample           | 2,703       | 207 KB  | Full addon processing (2 base + 1 addon + patches) |
| NewPipelinePerRender | 21,375      | 1.37 MB | Cold cache penalty (7.9x more allocations)         |

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

- RenderSimple: 25-35μs (baseline, 2 resources)
- RenderWithForEach: 40-55μs (5 iterations)
- RealSample: 250-300μs (full featured with addons)
- NewPipelinePerRender: 1.5-2.0ms (cold cache scenario)
