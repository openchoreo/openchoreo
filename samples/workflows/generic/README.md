# Generic Workflow Samples

This directory contains Generic Workflow definitions — workflows that run independently of any Component. Unlike CI Workflows (which are tightly coupled to the Component build lifecycle), Generic Workflows can automate any kind of task: data pipelines, report generation, scheduled maintenance, integration testing, and more.

---

## When to Use Generic Workflows

| Use Case | Example |
|----------|---------|
| Data processing / ETL | Fetch API data → transform → store results |
| Report generation | Aggregate metrics → format → publish |
| Scheduled maintenance | Purge old records, rotate credentials |
| Integration / acceptance testing | Run end-to-end test suites against a live environment |
| Package publishing | Publish libraries to npm, PyPI, Maven |
| Infrastructure automation | Terraform plan/apply, cloud resource provisioning |

---

## How Generic Workflows Differ from CI Workflows

| | CI Workflows (`ci/`) | Generic Workflows (`generic/`) |
|--|---|---|
| **Linked to Component?** | Yes | No |
| **Parameter schema** | Requires `repository` (url, branch, appPath) | Domain-specific; no git-clone structure |
| **Annotation** | `openchoreo.dev/workflow-scope: component` | None |
| **Triggered by** | Webhooks, API, Component build, `WorkflowRun` | `WorkflowRun` (manual, scheduled, or event-driven) |
| **Typical outcome** | Container image pushed to registry | Report, transformed data, test results, etc. |

---

## Available Samples

| Directory | Description |
|-----------|-------------|
| [`github-stats-report/`](./github-stats-report/) | Fetch GitHub repository statistics, transform the data, and output a formatted report |
| [`scm-create-repo/`](./scm-create-repo/) | Create a new source code repository on GitHub or AWS CodeCommit |

---

## See Also

- **[CI Workflow Samples](../ci/)** - Workflows for building and containerizing Components
- **[Workflow Samples Root](../README.md)** - Overview of both workflow types
