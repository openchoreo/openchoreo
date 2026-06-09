# Custom Steps for CI Workflows

This directory contains samples for extending OpenChoreo's built-in CI workflows with custom steps.

OpenChoreo CI workflows are built on [Argo Workflows](https://argoproj.github.io/workflows/) and composed from reusable `ClusterWorkflowTemplate` steps. You can insert custom steps into the pipeline by:

1. Creating a `ClusterWorkflowTemplate` that defines your step logic
2. Creating a `ClusterWorkflow` that references your template alongside the standard build steps

## Structure

Each subdirectory covers a category of custom step with its own README and sample YAMLs:

| Directory | Description |
|-----------|-------------|
| [`linter/`](./linter/) | API and code linting steps (e.g. Spectral for OpenAPI specs) |

## How It Works

A standard OpenChoreo build pipeline runs these steps in order:

```
checkout-source → build-image → publish-image → generate-workload-cr
```

Custom steps can be inserted at any point in this chain depending on what the step does.
