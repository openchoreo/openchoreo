# GitOps Build and Release Workflow (Google Cloud Buildpacks)

This directory contains a Workflow for automating the complete CI/CD pipeline using Google Cloud Buildpacks for building container images and creating pull requests in your GitOps repository.

## Overview

The `google-cloud-buildpacks-gitops-release` Workflow automates:
1. Building a container image using Google Cloud Buildpacks (no Dockerfile required)
2. Pushing to a container registry
3. Generating deployment manifests (Workload, ComponentRelease, ReleaseBinding)
4. Creating a pull request in your GitOps repository

## Architecture

```mermaid
flowchart TB
    subgraph workflow["google-cloud-buildpacks-gitops-release Workflow"]
        subgraph build["BUILD PHASE"]
            B1["1. clone-source"]
            B2["2. build-image (Buildpacks)"]
            B3["3. push-image"]
            B4["4. extract-descriptor"]
            B1 --> B2 --> B3 --> B4
        end

        subgraph release["RELEASE PHASE"]
            R1["5. clone-gitops"]
            R2["6. create-feature-branch"]
            R3["7. generate-gitops-resources"]
            R4["8. git-commit-push-pr"]
            R1 --> R2 --> R3 --> R4
        end

        B4 --> R1
    end

    R4 --> PR["Pull Request Created in GitOps Repository"]
```

## Prerequisites

- OpenChoreo installed with workflow plane
- ClusterSecretStore configured (comes with OpenChoreo installation)
- GitOps repository with OpenChoreo manifests
> [!NOTE]
> In the GitOps repository, it should have the manifests for the specified Project, Component, Deployment Pipeline, and Target Environment. A sample GitOps repository can be found in the [openchoreo/sample-gitops](https://github.com/openchoreo/sample-gitops) repository.
- GitHub Personal Access Token (PAT) with `repo` scope to access the GitOps repository
- Source code repository with a buildpacks-compatible application
- GitHub Personal Access Token (PAT) with `repo` scope to access the source repository

## Installation

### 1. Install the Workflow

> [!IMPORTANT]
> Before applying, edit `google-cloud-buildpacks-gitops-release.yaml` and set the `gitops-repo-url` parameter (under `spec.runTemplate.spec.arguments.parameters`) to a GitOps repository you have push access to. The workflow pushes a release branch and opens a pull request against this repo, so the default `https://github.com/openchoreo/sample-gitops` will fail unless you fork it first.

```bash
# Apply the ClusterWorkflowTemplate and the Workflow
kubectl apply -f samples/gitops-workflows/build-and-release/google-cloud-buildpacks/google-cloud-buildpacks-gitops-release-template.yaml
kubectl apply -f samples/gitops-workflows/build-and-release/google-cloud-buildpacks/google-cloud-buildpacks-gitops-release.yaml

# Verify installation
kubectl get clusterworkflowtemplate google-cloud-buildpacks-gitops-release
kubectl get workflows.openchoreo.dev google-cloud-buildpacks-gitops-release -n default
```

### 2. Configure Secrets in ClusterSecretStore

The workflow uses ExternalSecrets to automatically provision credentials. Add your tokens to the ClusterSecretStore:

> [!NOTE]
> The following commands use OpenBao (the default secret backend for local k3d development). For production, use your organization's secret provider.

```bash
# Your GitHub PAT for source repository (only needed for private repos)
SOURCE_GIT_TOKEN="ghp_your_source_repo_token"

# Your GitHub PAT for GitOps repository (required - must have repo scope)
GITOPS_GIT_TOKEN="ghp_your_gitops_repo_token"

# Store secrets in OpenBao
kubectl exec -n openbao openbao-0 -- sh -c "
  export BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN=root
  bao kv put secret/git-token git-token='${SOURCE_GIT_TOKEN}'
  bao kv put secret/gitops-token git-token='${GITOPS_GIT_TOKEN}'
"

# Verify ClusterSecretStore is healthy
kubectl get clustersecretstore default
```

#### Required Secret Keys

| Key            | Description                                               | Used By                                    |
|----------------|-----------------------------------------------------------|--------------------------------------------|
| `git-token`    | PAT for source repository (only needed for private repos) | `clone-source` step                        |
| `gitops-token` | PAT for GitOps repository (clone, push, PR creation)      | `clone-gitops`, `git-commit-push-pr` steps |

## Usage

### Basic Build and Release

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: WorkflowRun
metadata:
  name: reading-list-service-build-release-001
  namespace: default
spec:
  workflow:
    name: google-cloud-buildpacks-gitops-release

    parameters:
      componentName: "reading-list-service"
      projectName: "demo-project"
      repository:
        url: "https://github.com/openchoreo/sample-workloads"
        revision:
          branch: "main"
          commit: "abc123"
        appPath: "/service-go-reading-list"
      buildpacks:
        builderImage: "gcr.io/buildpacks/builder:v1"
        env: []
      workloadDescriptorPath: "workload.yaml"
```

### With Build Environment Variables

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: WorkflowRun
metadata:
  name: my-app-build-release-002
  namespace: default
spec:
  workflow:
    name: google-cloud-buildpacks-gitops-release

    parameters:
      componentName: "my-node-service"
      projectName: "demo-project"
      repository:
        url: "https://github.com/myorg/my-app"
        revision:
          branch: "main"
          commit: "abc123def456"
        appPath: "/services/my-node-service"
      buildpacks:
        builderImage: "gcr.io/buildpacks/builder:v1"
        env:
          - "NODE_ENV=production"
          - "NPM_CONFIG_PRODUCTION=true"
      workloadDescriptorPath: "workload.yaml"
```

### Monitor Progress

```bash
# Watch the WorkflowRun status
kubectl get workflowrun reading-list-service-build-release-001 -w

# View Argo Workflow status in the workflow plane
kubectl get workflows.argoproj.io -n workflows-default

# View logs for a specific step
kubectl logs -n workflows-default -l workflows.argoproj.io/workflow=<workflow-name> --all-containers=true
```

## Parameters Reference

| Parameter                    | Type     | Required | Default                        | Description                                            |
|------------------------------|----------|----------|--------------------------------|--------------------------------------------------------|
| `componentName`              | string   | Yes      | -                              | Component name                                         |
| `projectName`                | string   | Yes      | -                              | Project name                                           |
| `repository.url`             | string   | Yes      | -                              | Git repository URL                                     |
| `repository.revision.branch` | string   | No       | `main`                         | Git branch to checkout                                 |
| `repository.revision.commit` | string   | No       | ""                             | Git commit SHA                                         |
| `repository.appPath`         | string   | No       | `.`                            | Path to the application directory                      |
| `buildpacks.builderImage`    | string   | No       | `gcr.io/buildpacks/builder:v1` | Buildpacks builder image to use                        |
| `buildpacks.env`             | []string | No       | `[]`                           | Environment variables for the build (KEY=VALUE format) |
| `workloadDescriptorPath`     | string   | No       | `workload.yaml`                | Path to workload descriptor relative to appPath        |

## Supported Languages

Google Cloud Buildpacks automatically detect and build applications in many languages:

- Go
- Java (Maven, Gradle)
- Node.js (npm, yarn)
- Python (pip)
- .NET Core
- Ruby
- PHP

No Dockerfile required - buildpacks automatically detect the language and framework.

## Workflow Steps

| Step                           | Description                                                                      | Output                    |
|--------------------------------|----------------------------------------------------------------------------------|---------------------------|
| 1. `clone-source`              | Clones the source repository                                                     | Git revision (short SHA)  |
| 2. `build-image`               | Builds image using Google Cloud Buildpacks                                       | Container image tarball   |
| 3. `push-image`                | Pushes image to registry                                                         | Image reference           |
| 4. `extract-descriptor`        | Extracts workload descriptor from source                                         | Base64-encoded descriptor |
| 5. `clone-gitops`              | Clones the GitOps repository                                                     | GitOps workspace          |
| 6. `create-feature-branch`     | Creates a release branch                                                         | Branch name               |
| 7. `generate-gitops-resources` | Generates Workload, ComponentRelease, and ReleaseBinding manifests using occ CLI | All GitOps manifests      |
| 8. `git-commit-push-pr`        | Commits changes, pushes to remote, and creates PR using GitHub CLI               | PR URL                    |

## Files in This Directory

```text
google-cloud-buildpacks/
├── README.md                                            # This file
├── google-cloud-buildpacks-gitops-release.yaml          # Workflow CR
└── google-cloud-buildpacks-gitops-release-template.yaml # ClusterWorkflowTemplate (8 steps)
```

## Support

For issues or questions:
- GitHub Issues: https://github.com/openchoreo/openchoreo/issues
- Documentation: https://openchoreo.dev/docs
