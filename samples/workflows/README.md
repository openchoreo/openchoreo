# Workflow Samples

This directory contains Workflow definitions for OpenChoreo. Workflows use the same `Workflow` CRD but serve two distinct purposes, organized into separate subdirectories.

---

## Subdirectories

| Directory | Purpose | Triggered by |
|-----------|---------|--------------|
| [`ci/`](./ci/) | Component CI/build workflows — define how source code is built and containerized | Component builds, webhooks, auto-build |
| [`generic/`](./generic/) | General-purpose workflows — run any automation independently of a Component | Manual `WorkflowRun`, scheduled jobs, event-driven pipelines |

---

## CI Workflows (`ci/`)

CI workflows are tied to the Component lifecycle. They define build strategies (Docker, Buildpacks, etc.) and integrate with the Workflow Plane for automated container image creation.

**Use these when:** you need to build and containerize a Component from source code.

See [`ci/README.md`](./ci/README.md) for details.

---

## Generic Workflows (`generic/`)

Generic workflows run independently of any Component. They can perform any kind of automation: data processing, ETL pipelines, report generation, scheduled maintenance, integration testing, and more.

**Use these when:** you need to automate tasks that are not about building a Component image.

See [`generic/README.md`](./generic/README.md) for details.

---

## Key Differences

| | CI Workflows | Generic Workflows |
|--|---|---|
| **Linked to Component?** | Yes | No |
| **Parameters** | Includes `repository` (url, branch, appPath) | Domain-specific (no git-clone structure) |
| **Triggered by** | Webhooks, API, `WorkflowRun` | `WorkflowRun` (manual or event-driven) |
| **Typical steps** | Clone → Build → Push image | Fetch data → Transform → Report / any pipeline |
| **Annotation** | `openchoreo.dev/workflow-scope: component` | No scope annotation |
