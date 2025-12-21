# Private Repository Clone Sample

This sample demonstrates how to configure a ComponentWorkflow to clone source code from a private Git repository that requires authentication.

## Overview

When building applications from private Git repositories (private GitHub repos, GitLab, Bitbucket, etc.), the build workflow needs credentials to clone the repository. This sample shows two approaches:

1. **Manual Secret Creation** - Create the Kubernetes secret directly with a personal access token
2. **External Secrets Operator** - Use ExternalSecret to sync credentials from a secrets manager

## Files

| File | Description |
|------|-------------|
| `component-workflow.yaml` | ComponentWorkflow that references the ClusterWorkflowTemplate |
| `component-workflow-with-es.yaml` | ComponentWorkflow with ExternalSecret resource for automatic secret sync |
| `cluster-workflow-template.yaml` | Argo ClusterWorkflowTemplate with authenticated git clone |
| `git-clone-secret.yaml` | Kubernetes Secret template for Git credentials |

## Deployment Targets

These resources are deployed to different planes:

| Resource | Plane | Namespace |
|----------|-------|-----------|
| `component-workflow.yaml` | Control Plane | Organization namespace (e.g., `default`) |
| `component-workflow-with-es.yaml` | Control Plane | Organization namespace (e.g., `default`) |
| `cluster-workflow-template.yaml` | Build Plane | Cluster-scoped |
| `git-clone-secret.yaml` | Build Plane | Build execution namespace (e.g., `openchoreo-ci-default`) |

## Setup

### Option 1: Manual Secret Creation

1. Generate a personal access token from your Git provider:
   - **GitHub**: Settings > Developer settings > Personal access tokens > Generate new token (with `repo` scope)
   - **GitLab**: User Settings > Access Tokens > Add new token (with `read_repository` scope)
   - **Bitbucket**: Personal settings > App passwords > Create app password

2. Create the secret:
   ```bash
   kubectl create secret generic git-clone-secret \
     --from-literal=clone-secret=<your-personal-access-token> \
     -n openchoreo-ci-default
   ```

   Or update and apply the YAML template:
   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: git-clone-secret
     namespace: openchoreo-ci-default
   type: Opaque
   stringData:
     clone-secret: '<your-personal-access-token>'
   ```

3. Deploy the resources:
   ```bash
   # Apply to Build Plane
   kubectl apply -f cluster-workflow-template.yaml

   # Apply to Control Plane (organization namespace)
   kubectl apply -f component-workflow.yaml
   ```

### Option 2: External Secrets Operator

1. Ensure External Secrets Operator is installed and a ClusterSecretStore is configured

2. Store your Git access token in your secrets manager (e.g., AWS Secrets Manager, HashiCorp Vault)

3. Update `component-workflow-with-es.yaml` with the correct secret reference:
   ```yaml
   remoteRef:
     key: your-secret-key
     property: your-secret-property
   ```

4. Deploy the resources:
   ```bash
   # Apply to Build Plane
   kubectl apply -f cluster-workflow-template.yaml

   # Apply to Control Plane (organization namespace)
   kubectl apply -f component-workflow-with-es.yaml
   ```

The ExternalSecret resource defined in the ComponentWorkflow will be created in the build execution namespace and automatically sync the Git credentials from your secrets manager.

## How It Works

The clone step in `cluster-workflow-template.yaml` handles authentication by:

1. Reading the token from the mounted secret
2. Constructing an authenticated URL based on the repository format:
   - `https://github.com/org/repo` becomes `https://x-access-token:<token>@github.com/org/repo`
   - `git@github.com:org/repo` is converted to HTTPS with token authentication

This approach works with GitHub's personal access tokens and GitHub App installation tokens.

## Usage

Reference the workflow in your Component:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-private-app
spec:
  workflow:
    name: google-cloud-buildpacks-private-repo
    systemParameters:
      repository:
        url: "https://github.com/myorg/my-private-repo"
        revision:
          branch: "main"
        appPath: "/"
```

## Supported Git Providers

The sample is configured for GitHub repositories. For other providers, modify the URL transformation logic in `cluster-workflow-template.yaml`:

- **GitLab**: Use `https://oauth2:<token>@gitlab.com/org/repo`
- **Bitbucket**: Use `https://x-token-auth:<token>@bitbucket.org/org/repo`
- **Azure DevOps**: Use `https://<token>@dev.azure.com/org/project/_git/repo`

## See Also

- [ComponentWorkflow Samples](../README.md) - Overview of all ComponentWorkflow samples
- [Private Registry Sample](../with-private-registry/) - Pushing to private container registries
