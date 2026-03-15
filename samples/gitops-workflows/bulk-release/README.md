# Bulk GitOps Release Workflow

This directory contains a Workflow for automating bulk releases of components when using OpenChoreo with GitOps. This Workflow can release all components across all projects or all components within a specific project in the given GitOps repository.

## Overview

The `bulk-gitops-release` Workflow automates:
1. Cloning the GitOps repository
2. Creating a feature branch for the bulk release
3. Generating/Updating ReleaseBindings targeting a specified environment
4. Creating a pull request in your GitOps repository

## Architecture

```mermaid
flowchart TB
    subgraph workflow["bulk-gitops-release Workflow"]
        subgraph release["RELEASE PHASE"]
            R1["1. clone-gitops"]
            R2["2. create-feature-branch"]
            R3["3. generate-bulk-bindings"]
            R4["4. git-commit-push-pr"]
            R1 --> R2 --> R3 --> R4
        end
    end

    R4 --> PR["Pull Request Created in GitOps Repository"]
```

## Prerequisites

- OpenChoreo installed with workflow plane
- ClusterSecretStore configured (comes with OpenChoreo installation)
- GitOps repository with OpenChoreo manifests
> [!NOTE]
> The GitOps repository should contain manifests for Projects, Components, Deployment Pipelines, and Target Environments. Each component should have an existing Workload manifest. A sample GitOps repository can be found in the [openchoreo/sample-gitops](https://github.com/openchoreo/sample-gitops) repository.
> At the moment, this workflow only supports with **GitHub** as the GitOps repository.
- GitHub Personal Access Token (PAT) with `repo` scope to access the GitOps repository

## Installation

### 1. Install the Workflow

```bash
# Apply the ClusterWorkflowTemplate and the Workflow
kubectl apply -f samples/gitops-workflows/bulk-release/bulk-gitops-release-template.yaml
kubectl apply -f samples/gitops-workflows/bulk-release/bulk-gitops-release.yaml

# Verify installation
kubectl get clusterworkflowtemplate bulk-gitops-release
kubectl get workflows.openchoreo.dev bulk-gitops-release -n default
```

### 2. Configure Secrets in ClusterSecretStore

The workflow uses `ExternalSecrets` to automatically provision credentials. Add your tokens to the ClusterSecretStore:

> [!NOTE]
> The following commands use OpenBao (the default secret backend for local k3d development). For production, use your organization's secret provider.

```bash
# Your GitHub PAT for GitOps repository (required - must have repo scope)
GITOPS_GIT_TOKEN="ghp_your_gitops_repo_token"

# Store secrets in OpenBao
kubectl exec -n openbao openbao-0 -- sh -c "
  export BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN=root
  bao kv put secret/gitops-token git-token='${GITOPS_GIT_TOKEN}'
"

# Verify ClusterSecretStore is healthy
kubectl get clustersecretstore default
```

#### Required Secret Keys

| Key | Description | Used By |
|-----|-------------|---------|
| `gitops-token` | PAT for GitOps repository (clone, push, PR creation) | `clone-gitops`, `git-commit-push-pr` steps |

## Usage

### Release All Components Across All Projects

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: WorkflowRun
metadata:
  name: bulk-release-all-001
  namespace: default
spec:
  workflow:
    name: bulk-gitops-release

    parameters:
      scope:
        all: true
        projectName: "placeholder"
      gitops:
        repositoryUrl: "https://github.com/<your_org>/<repo_name>"
        branch: "main"
        targetEnvironment: "development"
        deploymentPipeline: "default-pipeline"
```

### Release Components for a Specific Project

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: WorkflowRun
metadata:
  name: bulk-release-project-001
  namespace: default
spec:
  workflow:
    name: bulk-gitops-release

    parameters:
      scope:
        all: false
        projectName: "demo-project"
      gitops:
        repositoryUrl: "https://github.com/<your_org>/<repo_name>"
        branch: "main"
        targetEnvironment: "staging"
        deploymentPipeline: "default-pipeline"
```

### Monitor Progress

```bash
# Watch the WorkflowRun status
kubectl get workflowrun bulk-release-all-001 -w

# View Argo Workflow status in the workflow plane
kubectl get workflows.argoproj.io -n workflows-default

# View logs for a specific step
kubectl logs -n workflows-default -l workflows.argoproj.io/workflow=<workflow-name> --all-containers=true
```

## Parameters Reference

### Scope Configuration

| Parameter           | Type    | Required | Default | Description                                     |
|---------------------|---------|----------|---------|-------------------------------------------------|
| `scope.all`         | boolean | No       | `false` | Release all components across all projects      |
| `scope.projectName` | string  | Yes      | -       | Project name to release (ignored if `all=true`) |

### GitOps Configuration

| Parameter                   | Type   | Required | Default       | Description                                         |
|-----------------------------|--------|----------|---------------|-----------------------------------------------------|
| `gitops.repositoryUrl`      | string | Yes      | -             | GitOps repository URL                               |
| `gitops.branch`             | string | No       | `main`        | GitOps repository branch                            |
| `gitops.targetEnvironment`  | string | No       | `development` | Target environment name for deployment              |
| `gitops.deploymentPipeline` | string | Yes      | -             | Deployment pipeline name for the target environment |

## Workflow Steps

| Step                        | Description                                                                      | Output                   |
|-----------------------------|----------------------------------------------------------------------------------|--------------------------|
| 1. `clone-gitops`           | Clones the GitOps repository                                                     | GitOps workspace         |
| 2. `create-feature-branch`  | Creates a release branch (`bulk-release/all-*` or `bulk-release/<project>-*`)    | Branch name              |
| 3. `generate-bulk-bindings` | Generates ReleaseBindings for all components using `occ releasebinding generate` | ReleaseBinding manifests |
| 4. `git-commit-push-pr`     | Commits changes, pushes to remote, and creates PR using GitHub CLI               | PR URL                   |

## Files in This Directory

```
bulk-release/
├── README.md                           # This file
├── bulk-gitops-release.yaml            # Workflow CR
└── bulk-gitops-release-template.yaml   # ClusterWorkflowTemplate (4 steps)
```

## Support

For issues or questions:
- GitHub Issues: https://github.com/openchoreo/openchoreo/issues
- Documentation: https://openchoreo.dev/docs
