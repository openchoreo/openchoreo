# GitOps Build and Release Workflow (docker)

This directory contains a ComponentWorkflow for automating the complete CI/CD pipeline from building container images to creating pull requests in your GitOps repository.

## Overview

The `docker-gitops-release` ComponentWorkflow automates:
1. Building a container image from source code
2. Pushing to a container registry
3. Generating deployment manifests (Workload, ComponentRelease, ReleaseBinding)
4. Creating a pull request in your GitOps repository

## Architecture

```mermaid
flowchart TB
    subgraph workflow["docker-gitops-release ComponentWorkflow"]
        subgraph build["BUILD PHASE"]
            B1["1. clone-source"]
            B2["2. build-image"]
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

- OpenChoreo installed with build plane
- ClusterSecretStore configured (comes with OpenChoreo installation)
- GitOps repository with openchoreo manifests
> [!NOTE]  
> In the GitOps repository, it should have the manifests for the specified Project, Component, Deployment Pipeline, and Target Environment. A sample GitOps repository can be found in the [openchoreo/sample-gitops](https://github.com/openchoreo/sample-gitops) repository.
- GitHub Personal Access Token (PAT) with `repo` scope to access the GitOps repository
- Source code repository with a Dockerfile
- GitHub Personal Access Token (PAT) with `repo` scope to access the source repository

## Installation

### 1. Install the Workflow

```bash
# Apply the ClusterWorkflowTemplate and the ComponentWorkflow
kubectl apply -f samples/gitops-workflows/component-workflows/build-and-release/docker/docker-gitops-release-template.yaml
kubectl apply -f samples/gitops-workflows/component-workflows/build-and-release/docker/docker-gitops-release.yaml

# Verify installation
kubectl get clusterworkflowtemplate docker-gitops-release
kubectl get componentworkflow docker-gitops-release -n default
```

### 2. Configure Secrets in ClusterSecretStore

The workflow uses ExternalSecrets to automatically provision credentials. Add your tokens to the ClusterSecretStore:

> [!NOTE] 
> The following commands use the `fake` provider, which is a placeholder for any external secret provider. This is only for development purposes. When deploying to production, use a real secret provider.

```bash
# Your GitHub PAT for source repository (only needed for private repos)
SOURCE_GIT_TOKEN="ghp_your_source_repo_token"

# Your GitHub PAT for GitOps repository (required - must have repo scope)
GITOPS_GIT_TOKEN="ghp_your_gitops_repo_token"

# Patch the ClusterSecretStore
kubectl patch clustersecretstore default --type='json' -p="[
  {
    \"op\": \"add\",
    \"path\": \"/spec/provider/fake/data/-\",
    \"value\": {
      \"key\": \"git-token\",
      \"value\": \"${SOURCE_GIT_TOKEN}\"
    }
  },
  {
    \"op\": \"add\",
    \"path\": \"/spec/provider/fake/data/-\",
    \"value\": {
      \"key\": \"gitops-token\",
      \"value\": \"${GITOPS_GIT_TOKEN}\"
    }
  }
]"

# Verify
kubectl get clustersecretstore default -o jsonpath='{.spec.provider.fake.data[*].key}' | tr ' ' '\n'
```

#### Required Secret Keys

| Key | Description | Used By |
|-----|-------------|---------|
| `git-token` | PAT for source repository (only needed for private repos) | `clone-source` step |
| `gitops-token` | PAT for GitOps repository (clone, push, PR creation) | `clone-gitops`, `git-commit-push-pr` steps |

## Usage

### Basic Build and Release

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: ComponentWorkflowRun
metadata:
  name: greeter-build-release-001
  namespace: default
spec:
  # Ownership tracking (required)
  owner:
    projectName: "demo-project-gitops"
    componentName: "greeter-service-gitops"

  workflow:
    # Reference to the merged docker-gitops-release ComponentWorkflow
    name: docker-gitops-release

    # Source repository configuration (required)
    systemParameters:
      repository:
        url: "https://github.com/openchoreo/sample-workloads"
        revision:
          branch: "main"
        appPath: "/service-go-greeter"

    # Workflow parameters (required)
    parameters:
      docker:
        context: "/service-go-greeter"
        filePath: "/service-go-greeter/Dockerfile"
      gitops:
        # Replace with your GitOps repository URL
        repositoryUrl: "https://github.com/openchoreo/sample-gitops"
        branch: "main"
        targetEnvironment: "development"
        deploymentPipeline: "standard"
      workloadDescriptorPath: "workload.yaml"
```

### Building from a Specific Commit

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: ComponentWorkflowRun
metadata:
  name: greeter-build-release-002
  namespace: default
spec:
  # Ownership tracking (required)
  owner:
    projectName: "demo-project-gitops"
    componentName: "greeter-service-gitops"

  workflow:
    # Reference to the merged docker-gitops-release ComponentWorkflow
    name: docker-gitops-release

    # Source repository configuration (required)
    systemParameters:
      repository:
        url: "https://github.com/openchoreo/sample-workloads"
        revision:
          branch: "branch_name"
          commit: "commit_sha"
        appPath: "/service-go-greeter"

    # Workflow parameters (required)
    parameters:
      docker:
        context: "/service-go-greeter"
        filePath: "/service-go-greeter/Dockerfile"
      gitops:
        repositoryUrl: "https://github.com/openchoreo/sample-gitops"
        branch: "main"
        targetEnvironment: "development"
        deploymentPipeline: "standard"
      workloadDescriptorPath: "workload.yaml"
```

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: ComponentWorkflowRun
metadata:
  name: greeter-build-release-003
  namespace: default
spec:
  owner:
    projectName: "demo-project"
    componentName: "my-service"

  workflow:
    name: docker-gitops-release

    systemParameters:
      repository:
        url: "https://github.com/myorg/my-app"
        revision:
          branch: "main"
          commit: "abc123def456"  # Build specific commit
        appPath: "/services/my-service"

    parameters:
      docker:
        context: "/services/my-service"
        filePath: "/services/my-service/Dockerfile"
      gitops:
        repositoryUrl: "https://github.com/myorg/gitops-config"
        branch: "main"
        targetEnvironment: "development"
        deploymentPipeline: "standard-pipeline"
      workloadDescriptorPath: "workload.yaml"
```

### Monitor Progress

```bash
# Watch the ComponentWorkflowRun status
kubectl get componentworkflowrun greeter-build-release-001 -w

# View Argo Workflow status in the build plane
kubectl get workflow -n openchoreo-ci-default

# View logs for a specific step
kubectl logs -n openchoreo-ci-default -l workflows.argoproj.io/workflow=<workflow-name> --all-containers=true
```

## Parameters Reference

### System Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `repository.url` | string | Yes | - | Git repository URL |
| `repository.revision.branch` | string | No | `main` | Git branch to checkout |
| `repository.revision.commit` | string | No | - | Git commit SHA (optional, defaults to latest) |
| `repository.appPath` | string | No | `.` | Path to the application directory |

### Docker Configuration

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `docker.context` | string | No | `.` | Docker build context path relative to repository root |
| `docker.filePath` | string | No | `./Dockerfile` | Path to the Dockerfile relative to repository root |

### GitOps Configuration

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `gitops.repositoryUrl` | string | Yes | - | GitOps repository URL |
| `gitops.branch` | string | No | `main` | GitOps repository branch |
| `gitops.targetEnvironment` | string | No | `development` | Target environment name |
| `gitops.deploymentPipeline` | string | Yes | - | Deployment pipeline name |

### Workload Descriptor

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `workloadDescriptorPath` | string | No | `workload.yaml` | Path to workload descriptor relative to appPath |

## Workflow Steps

| Step | Description | Output |
|------|-------------|--------|
| 1. `clone-source` | Clones the source repository | Git revision (short SHA) |
| 2. `build-image` | Builds Docker image using Podman | Container image tarball |
| 3. `push-image` | Pushes image to registry | Image reference |
| 4. `extract-descriptor` | Extracts workload descriptor from source | Base64-encoded descriptor |
| 5. `clone-gitops` | Clones the GitOps repository | GitOps workspace |
| 6. `create-feature-branch` | Creates a release branch | Branch name |
| 7. `generate-gitops-resources` | Generates Workload, ComponentRelease, and ReleaseBinding manifests using occ CLI | All GitOps manifests |
| 8. `git-commit-push-pr` | Commits changes, pushes to remote, and creates PR using GitHub CLI | PR URL |

## Workflow Outputs

The workflow produces:

1. **Container Image**: Built and pushed to the configured registry
2. **Workload Manifest**: Defines the component deployment configuration
3. **ComponentRelease**: Immutable artifact for this specific version
4. **ReleaseBinding**: Links the release to the target environment
5. **Pull Request**: Created in the GitOps repository for review

## GitOps Repository Structure

This workflow works with any GitOps repository structure. The workflow generates manifests in your GitOps repository based on how you organized it.

## Troubleshooting

### Clone Source Fails

- Verify the source repository URL is correct and accessible
- For private repos, ensure `git-token` is set in ClusterSecretStore
- Check that the branch/commit exists

### Build Image Fails

- Verify the Dockerfile path is correct
- Check that the build context contains all required files
- Review Podman build logs for specific errors

### Clone GitOps Fails

- Verify GitOps repository URL is correct
- Ensure `gitops-token` is set in ClusterSecretStore with correct permissions
- Check that the branch exists in the GitOps repository

### Git Push Fails

**Error: `Invalid username or token`**
- The GitHub PAT may be expired, revoked, or lacks permissions
- Ensure the token has `repo` scope
- Regenerate the token and update ClusterSecretStore

**Error: `could not read Password`**
- The token URL format may be incorrect
- Ensure the workflow template uses `x-access-token:TOKEN@` format

### Pull Request Not Created

- Verify GitHub token has `repo` scope
- Check that the base branch exists
- Review GitHub API rate limits
- Ensure the GitOps repository allows PR creation

### ExternalSecrets Not Syncing

```bash
# Check ExternalSecrets status
kubectl get externalsecret -n openchoreo-ci-default

# Verify ClusterSecretStore has required keys
kubectl get clustersecretstore default -o jsonpath='{.spec.provider.fake.data[*].key}'
```

## Files in This Directory

```
build-and-release/
├── README.md                           # This file
├── docker-gitops-release.yaml          # ComponentWorkflow CR
└── docker-gitops-release-template.yaml # ClusterWorkflowTemplate (8 steps)
```

## Support

For issues or questions:
- GitHub Issues: https://github.com/openchoreo/openchoreo/issues
- Documentation: https://openchoreo.dev/docs
