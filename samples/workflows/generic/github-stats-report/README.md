# GitHub Stats Report — Generic Workflow Sample

This sample demonstrates a Generic Workflow that fetches repository statistics from the GitHub public API, transforms the raw response into a concise summary, and prints a formatted report. It has no dependency on any Component and serves as a clear example of how Generic Workflows differ from CI Workflows.

---

## Pipeline Overview

```
WorkflowRun
    │
    ▼
[fetch-step]   — curl GitHub API → raw.json (shared volume)
    │
    ▼
[transform-step] — jq extracts fields → stats.json (shared volume)
    │
    ▼
[report-step]  — formats and prints the report (table or JSON)
```

## Files

| File | Kind | Description |
|------|------|-------------|
| `cluster-workflow-template-github-stats-report.yaml` | `ClusterWorkflowTemplate` | Argo Workflows template with the three execution steps |
| `workflow-github-stats-report.yaml` | `Workflow` | OpenChoreo CR — defines the parameter schema and references the template |
| `workflow-run-github-stats-report.yaml` | `WorkflowRun` | Triggers a run with specific parameter values |

## Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `source.org` | `openchoreo` | GitHub organization name |
| `source.repo` | `openchoreo` | GitHub repository name |
| `output.format` | `table` | Output format: `table` (human-readable) or `json` (machine-readable) |

## How to Run

Deploy the resources in order:

```bash
# 1. Deploy the ClusterWorkflowTemplate to the Workflow Plane
kubectl apply -f cluster-workflow-template-github-stats-report.yaml

# 2. Deploy the Workflow CR to the Control Plane
kubectl apply -f workflow-github-stats-report.yaml

# 3. Trigger an execution by creating a WorkflowRun
kubectl apply -f workflow-run-github-stats-report.yaml
```

To report on a different repository, override the parameters in your `WorkflowRun`:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: WorkflowRun
metadata:
  name: my-stats-run
spec:
  workflow:
    name: github-stats-report
    parameters:
      source:
        org: "kubernetes"
        repo: "kubernetes"
      output:
        format: "json"
```

## Example Output (table format)

```
=============================
  GitHub Repository Report
=============================
Name:         openchoreo/openchoreo
Description:  The OpenChoreo internal developer platform
Language:     Go
Stars:        312
Forks:        48
Open Issues:  27
License:      Apache License 2.0
Topics:       platform-engineering, kubernetes, developer-platform
Created:      2023-11-01T08:00:00Z
Updated:      2026-02-28T14:32:11Z
=============================
```
