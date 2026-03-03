# CI Workflow Samples

This directory contains reusable Workflow definitions that define how OpenChoreo builds applications from source code. CI Workflows are specialized templates for component builds that integrate with the Build Plane (Argo Workflows) to automate the containerization of your applications.

---
## Table of Contents

1. [Overview](#overview)
2. [How CI Workflows Work](#how-ci-workflows-work)
    - [Key Concepts](#key-concepts)
    - [Referencing Build Plane Templates](#referencing-build-plane-templates)
3. [Available Workflows](#available-workflows)
    - [Docker Workflow](#docker-workflow)
    - [Google Cloud Buildpacks Workflow](#google-cloud-buildpacks-workflow)
    - [React Workflow](#react-workflow)
4. [Using Workflows](#using-workflows)
    - [Method 1: Reference in Component](#method-1-reference-in-component)
    - [Method 2: Manual WorkflowRun](#method-2-manual-workflowrun)
5. [Deploying Workflows](#deploying-workflows)
6. [Template Variables](#template-variables)
7. [Parameters and Labels](#parameters-and-labels)
8. [Reusable Type Definitions](#reusable-type-definitions)
9. [Working with Private Repositories](#working-with-private-repositories)
10. [Using Secrets in Workflows](#using-secrets-in-workflows)
11. [Platform Engineer vs Developer Responsibilities](#platform-engineer-vs-developer-responsibilities)
12. [ComponentType Governance](#componenttype-governance)
13. [See Also](#see-also)
---
## Overview

In OpenChoreo, a CI **Workflow** is a Custom Resource that:

1. **Defines a build strategy** - Specifies how to build and containerize your application (Docker, Buildpacks, etc.)
2. **Provides structured system parameters** - Required repository configuration for build automation features (webhooks, auto-build, UI actions)
3. **Provides flexible developer parameters** - Platform Engineer-defined schema for additional build configuration
4. **Templates Argo Workflows** - Generates the actual Argo Workflow resources that execute in the Build Plane
5. **Enforces governance** - Platform Engineers control hardcoded parameters (registry URLs, timeouts, security settings)

CI Workflows carry the annotation `openchoreo.dev/workflow-scope: component` and the `openchoreo.dev/component-workflow-parameters` annotation that maps repository fields for webhook and auto-build integration.

## How CI Workflows Work

### Key Concepts

- **Workflow CR**: Platform Engineer-defined template that lives in the control plane
- **WorkflowRun CR**: Instance that triggers a build execution (created automatically or manually)
- **System Parameters**: Required structured fields for repository information (url, branch, commit, appPath)
- **Developer Parameters**: Flexible PE-defined schema for build configuration (resources, caching, testing, etc.)
- **Template Variables**: Placeholders like `${metadata.labels['openchoreo.dev/component']}`, `${parameters.repository.url}`, and `${parameters.version}`
- **Build Plane**: A Kubernetes cluster running Argo Workflows
- **ClusterWorkflowTemplate**: Pre-defined Argo Workflow templates in the Build Plane that Workflows reference via `workflowTemplateRef`

### Referencing Build Plane Templates

Workflows generate Argo Workflow resources that reference ClusterWorkflowTemplates deployed in the Build Plane. This enables reusable build logic:

```yaml
spec:
  runTemplate:
    apiVersion: argoproj.io/v1alpha1
    kind: Workflow
    spec:
      # Reference a ClusterWorkflowTemplate in the Build Plane
      workflowTemplateRef:
        clusterScope: true
        name: google-cloud-buildpacks  # Pre-defined template in Build Plane
      arguments:
        parameters:
          - name: git-repo
            value: ${parameters.repository.url}
          - name: branch
            value: ${parameters.repository.revision.branch}
```

## Available Workflows

### [Docker Workflow](./docker.yaml)

Build applications using a Dockerfile.

**Use Case**: Applications with custom Dockerfiles that define their own build process.

**Repository Parameters** (required structure):
```yaml
parameters:
  repository:
    url: string                    # Git repository URL
    secretRef: string              # Secret reference for private repos
    revision:
      branch: string | default=main
      commit: string | default=""
    appPath: string | default=.    # Path to application in repo
```

**Developer Parameters** (PE-defined):
```yaml
parameters:
  docker:
    context: string | default=.    # Docker build context
    filePath: string | default=./Dockerfile
```

**Example Usage**:
```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-service
spec:
  componentType:
    kind: ComponentType
    name: deployment/service
  workflow:
    name: docker
    parameters:
      repository:
        url: "https://github.com/myorg/myapp"
        revision:
          branch: "main"
        appPath: "/service"
      docker:
        context: "/service"
        filePath: "/service/Dockerfile"
```

### [Google Cloud Buildpacks Workflow](./google-cloud-buildpacks.yaml)

Build applications automatically using Google Cloud Buildpacks (no Dockerfile required).

**Use Case**: Applications where Buildpacks can automatically detect the language and build configuration (Go, Java, Node.js, Python, etc.).

**Repository Parameters** (required structure):
```yaml
parameters:
  repository:
    url: string | description="Git repository URL"
    secretRef: string | description="Secret reference for private repos"
    revision:
      branch: string | default=main description="Git branch to checkout"
      commit: string | description="Specific commit SHA (optional)"
    appPath: string | default=. description="Path to the application directory"
```

**Developer Parameters** (PE-defined):
```yaml
parameters:
  version: integer | default=1 description="Build version number"
  testMode: string | enum=unit,integration,none default=unit description="Test mode to execute"
  command: '[]string | default=[] description="Custom command to override the default entrypoint"'
  args: '[]string | default=[] description="Custom arguments to pass to the command"'
  resources:
    cpuCores: integer | default=1 minimum=1 maximum=8 description="Number of CPU cores allocated for the build"
    memoryGb: integer | default=2 minimum=1 maximum=32 description="Amount of memory in GB allocated for the build"
  timeout: string | default="30m" description="Build timeout duration (e.g., 30m, 1h)"
  cache:
    enabled: boolean | default=true description="Enable build cache to speed up subsequent builds"
    paths: '[]string | default=["/root/.cache"] description="Paths to cache between builds"'
  limits:
    maxRetries: integer | default=3 minimum=0 maximum=10 description="Maximum number of retry attempts on build failure"
    maxDurationMinutes: integer | default=60 minimum=5 maximum=240 description="Maximum build duration in minutes"
```

**Platform Engineer Controls** (hardcoded in runTemplate):
- Builder image: `gcr.io/buildpacks/builder@sha256:...`
- Registry URL: `gcr.io/openchoreo-dev/images`
- Security scanning: enabled
- Build timeout: 30m

### [React Workflow](./react.yaml)

Specialized build workflow for React web applications.

**Use Case**: React applications that need Node.js-based builds.

**Repository Parameters** (required structure):
```yaml
parameters:
  repository:
    url: string
    secretRef: string
    revision:
      branch: string | default=main
      commit: string | default=""
    appPath: string | default=.
```

**Developer Parameters** (PE-defined):
```yaml
parameters:
  nodeVersion: string | default="18"
```

## Using Workflows

### Method 1: Reference in Component

Define the workflow in your Component resource:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-app
spec:
  owner:
    projectName: my-project
  componentType:
    kind: ComponentType
    name: deployment/service

  # Workflow configuration
  workflow:
    name: docker                    # Reference to Workflow

    parameters:
      repository:                   # Required repository parameters
        url: "https://github.com/myorg/myapp"
        revision:
          branch: "main"
          commit: ""                # Empty means latest
        appPath: "/"
      docker:                       # Developer-configurable parameters
        context: "/"
        filePath: "/Dockerfile"
```

### Method 2: Manual WorkflowRun

Create a WorkflowRun resource to trigger a workflow manually:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: WorkflowRun
metadata:
  name: my-app-build-01
  labels:
    openchoreo.dev/project: "my-project"
    openchoreo.dev/component: "my-app"
spec:
  workflow:
    name: docker

    parameters:
      repository:
        url: "https://github.com/myorg/myapp"
        revision:
          branch: "main"
          commit: "a1b2c3d4"        # Specific commit SHA
        appPath: "/"
      docker:
        context: "/"
        filePath: "/Dockerfile"
```

## Deploying Workflows

Deploy a workflow to your control plane:

```bash
# Deploy all CI workflows
kubectl apply -f samples/workflows/ci/

# Deploy a specific workflow
kubectl apply -f samples/workflows/ci/docker.yaml
```

Verify the workflow is available:

```bash
# List workflows
kubectl get workflows -n default

# Describe a workflow
kubectl describe workflow docker -n default
```

## Template Variables

Workflows support template variables for dynamic values in the `runTemplate`:

| Variable | Description | Scope |
|----------|-------------|-------|
| `${metadata.workflowRunName}` | Name of the WorkflowRun CR | All workflows |
| `${metadata.namespaceName}` | Namespace name | All workflows |
| `${metadata.namespace}` | CI namespace (e.g., `openchoreo-ci-default`) | All workflows |
| `${metadata.labels['key']}` | WorkflowRun labels (any label set on the WorkflowRun) | All workflows |
| `${parameters.*}` | Parameter values (repository, developer params) | All workflows |
| `${secretRef.*}` | Resolved secret reference data | When secretRef is configured |

**Example**:
```yaml
spec:
  runTemplate:
    apiVersion: argoproj.io/v1alpha1
    kind: Workflow
    metadata:
      name: ${metadata.workflowRunName}
      namespace: ${metadata.namespace}
    spec:
      arguments:
        parameters:
          # Context from WorkflowRun labels
          - name: component-name
            value: ${metadata.labels['openchoreo.dev/component']}
          - name: project-name
            value: ${metadata.labels['openchoreo.dev/project']}
          # Repository parameters
          - name: git-repo
            value: ${parameters.repository.url}
          - name: branch
            value: ${parameters.repository.revision.branch}
          # Developer parameters
          - name: version
            value: ${parameters.version}
          # Hardcoded PE-controlled values
          - name: image-name
            value: ${metadata.labels['openchoreo.dev/project']}-${metadata.labels['openchoreo.dev/component']}-image
```

## Parameters and Labels

### WorkflowRun Labels

Context information like project and component names are passed via WorkflowRun labels rather than schema parameters. This makes them available to any workflow template through `${metadata.labels['key']}`:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: WorkflowRun
metadata:
  name: my-build-01
  labels:
    openchoreo.dev/project: "my-project"
    openchoreo.dev/component: "my-component"
```

When triggered via the API or webhooks, these labels are set automatically.

### Repository Parameters (Required Structure)

CI Workflows define repository parameters for build automation:

```yaml
parameters:
  repository:
    url: string                # Git repository URL
    secretRef: string          # Secret reference for private repos
    revision:
      branch: string           # Git branch to build from
      commit: string           # Specific commit SHA (optional)
    appPath: string            # Path to application code in repository
```

The `component-workflow-parameters` annotation maps these fields for webhook and auto-build integration:
```yaml
annotations:
  openchoreo.dev/component-workflow-parameters: |
    repoUrl: parameters.repository.url
    branch: parameters.repository.revision.branch
    commit: parameters.repository.revision.commit
    appPath: parameters.repository.appPath
    secretRef: parameters.repository.secretRef
```

### Developer Parameters (Complete Freedom)

Platform Engineers define custom parameters with full flexibility:

```yaml
parameters:
  # Any structure you design
  version: integer | default=1
  resources:
    cpuCores: integer | minimum=1 maximum=8
    memoryGb: integer | minimum=1 maximum=32
  cache:
    enabled: boolean | default=true
    paths: '[]string | default=[]'
```

## Reusable Type Definitions

Workflows support a `types` field in the schema to define reusable type definitions for complex structures. This enables you to define custom types that can be referenced in parameters, reducing duplication and ensuring consistency.

### Why Use Types?

- **Reusability**: Define complex structures once and reference them multiple times
- **Consistency**: Ensure the same structure is used across different parameters
- **Maintainability**: Update type definitions in one place
- **Clarity**: Make schemas more readable by extracting complex nested structures

### Defining Types

Types are defined at the schema level and can represent objects, arrays, or nested structures:

```yaml
schema:
  types:
    # Object type for endpoint configuration
    Endpoint:
      name: "string"
      port: "integer"
      type: "string | enum=REST,HTTP,TCP,UDP"
      schemaFile: "string | description='Path to the schema file'"

    # Nested object type for resource configuration
    ResourceRequirements:
      requests: "ResourceQuantity | default={}"
      limits: "ResourceQuantity | default={}"

    ResourceQuantity:
      cpu: "string | default=100m"
      memory: "string | default=256Mi"
```

### Using Types in Parameters

Once defined, types can be referenced in parameters using the type name:

```yaml
schema:
  types:
    Endpoint:
      name: "string"
      port: "integer"
      type: "string | enum=REST,HTTP,TCP,UDP"

  parameters:
    # Reference type as an array
    endpoints: '[]Endpoint | default=[] description="List of service endpoints"'

    # Reference type as a single object
    resources: "ResourceRequirements | default={}"

    # Use built-in array types
    tags: '[]string | default=["latest"] description="Image tags"'
```

### Type Reference Syntax

- **Single object**: `ResourceRequirements`
- **Array of objects**: `[]Endpoint`
- **Built-in arrays**: `[]string`, `[]integer`, `[]boolean`
- **Nested types**: Types can reference other types (e.g., `ResourceRequirements` references `ResourceQuantity`)

## Working with Private Repositories

All default CI Workflows support cloning from private Git repositories that require authentication. Private repository support is built-in and works seamlessly with GitHub, GitLab, Bitbucket, and AWS CodeCommit.

### How It Works

When you configure a private repository with authentication, the workflow automatically:
1. Reads credentials from the SecretReference resource (created via OpenChoreo API)
2. Detects the authentication type (basic auth or SSH key)
3. Configures git authentication appropriately
4. Clones the repository securely without exposing credentials in logs

**Supported Authentication Methods:**
- **Basic Authentication (Token/Password)**: Works with GitHub, GitLab, Bitbucket, and AWS CodeCommit
  - Supports optional username (required for AWS CodeCommit, optional for others)
  - Uses HTTPS URLs with embedded credentials
- **SSH Key Authentication**: Works with all major Git providers
  - Uses SSH URLs (git@github.com:...)
  - Automatically configures SSH keys and host verification

**Supported Git Providers:**
- **GitHub**: Token-based or SSH key authentication
- **GitLab**: Token-based or SSH key authentication
- **Bitbucket**: Token-based or SSH key authentication
- **AWS CodeCommit**: Username + password or SSH key authentication (see AWS CodeCommit section below)

### Creating Git Secrets

Use the OpenChoreo API to create git secrets that will be automatically synced to your secret store:

#### Basic Authentication (Token/Password)

For most git providers (GitHub, GitLab, Bitbucket):

```bash
# Create secret with token only
curl -X POST http://openchoreo-api/api/v1/namespaces/default/git-secrets \
  -H "Content-Type: application/json" \
  -d '{
    "secretName": "github-token",
    "secretType": "basic-auth",
    "token": "ghp_xxxxxxxxxxxxxxxxxxxx"
  }'
```

For AWS CodeCommit (requires username):

```bash
# Create secret with username and password
curl -X POST http://openchoreo-api/api/v1/namespaces/default/git-secrets \
  -H "Content-Type: application/json" \
  -d '{
    "secretName": "aws-codecommit-creds",
    "secretType": "basic-auth",
    "username": "my-iam-username-at-123456789012",
    "token": "my-codecommit-password"
  }'
```

#### SSH Key Authentication

```bash
# Create secret with SSH private key
curl -X POST http://openchoreo-api/api/v1/namespaces/default/git-secrets \
  -H "Content-Type: application/json" \
  -d '{
    "secretName": "github-ssh-key",
    "secretType": "ssh-auth",
    "sshKey": "-----BEGIN OPENSSH PRIVATE KEY-----\n...\n-----END OPENSSH PRIVATE KEY-----"
  }'
```

### Using Private Repositories in Components

Once the git secret is created, reference it in your Component's workflow configuration:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-private-app
spec:
  workflow:
    name: google-cloud-buildpacks
    parameters:
      repository:
        url: "https://github.com/myorg/private-repo"  # Private repo URL
        secretRef: "github-token"  # Reference to the git secret
        revision:
          branch: "main"
        appPath: "/"
```

For SSH authentication, use SSH URLs:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-private-app-ssh
spec:
  workflow:
    name: google-cloud-buildpacks
    parameters:
      repository:
        url: "git@github.com:myorg/private-repo.git"  # SSH URL
        secretRef: "github-ssh-key"  # Reference to SSH key secret
        revision:
          branch: "main"
        appPath: "/"
```

### AWS CodeCommit Support

AWS CodeCommit requires special configuration:

#### Authentication Methods

1. **HTTPS with Git Credentials**:
   - Generate Git credentials in IAM Console
   - Username format: `{username}-at-{AWS-account-ID}`
   - Create secret with both username and password

   ```bash
   curl -X POST http://openchoreo-api/api/v1/namespaces/default/git-secrets \
     -H "Content-Type: application/json" \
     -d '{
       "secretName": "codecommit-creds",
       "secretType": "basic-auth",
       "username": "myuser-at-123456789012",
       "token": "my-generated-password"
     }'
   ```

2. **SSH Keys**:
   - Upload SSH public key to IAM Console
   - Get SSH Key ID from IAM
   - Use SSH Key ID as username in SSH URL

Component configuration for AWS CodeCommit:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: aws-codecommit-app
spec:
  workflow:
    name: google-cloud-buildpacks
    parameters:
      repository:
        # HTTPS URL format
        url: "https://git-codecommit.us-east-1.amazonaws.com/v1/repos/my-repo"
        # OR SSH URL format:
        # url: "ssh://git-codecommit.us-east-1.amazonaws.com/v1/repos/my-repo"
        secretRef: "codecommit-creds"
        revision:
          branch: "main"
        appPath: "/"
```

The Workflow automatically creates an ExternalSecret for each build that fetches credentials from the secret store and makes them available to the workflow.

## Using Secrets in Workflows

Build workflows often need access to secrets for authentication, such as:
- **Git credentials** for cloning private repositories (see "Working with Private Repositories" section above)
- **Registry credentials** for pushing images to private container registries
- **API tokens** for external services during the build process

Workflows support two approaches for managing secrets in the Build Plane:

### Approach 1: External Secrets with Dynamic Secret Names (Recommended)

Use the `resources` section in Workflow to define ExternalSecret resources that point to secrets in your secret backend. This approach:
- Automatically creates and syncs secrets in the Build Plane's execution namespace
- Generates unique secret names per workflow run (e.g., `${metadata.workflowRunName}-git-secret`)
- Passes the secret name as a parameter to the workflow, allowing the workflow to reference it during execution
- Ideal for GitOps workflows where all configuration is version-controlled

**Example with Git Secret (supports multiple data fields):**

```yaml
resources:
  - id: git-secret
    includeWhen: ${has(parameters.repository.secretRef) && parameters.repository.secretRef != ""}
    template:
      apiVersion: external-secrets.io/v1
      kind: ExternalSecret
      metadata:
        name: ${metadata.workflowRunName}-git-secret
        namespace: ${metadata.namespace}
      spec:
        refreshInterval: 15s
        secretStoreRef:
          kind: ClusterSecretStore
          name: default
        target:
          name: ${metadata.workflowRunName}-git-secret
          creationPolicy: Owner
          template:
            type: ${secretRef.type}
        # Use secretRef.data array to support multiple fields (e.g., username + password)
        data: |
          ${secretRef.data.map(secret, {
            "secretKey": secret.secretKey,
            "remoteRef": {
              "key": secret.remoteRef.key,
              "property": has(secret.remoteRef.property) && secret.remoteRef.property != "" ? secret.remoteRef.property : oc_omit()
            }
          })}
```

**How secretRef.data Works:**

The `secretRef.data` variable provides access to all credential fields from the SecretReference:
- For **basic-auth**: Contains `password` (always) and `username` (if provided)
- For **ssh-auth**: Contains `ssh-privatekey`

The CEL `map` expression iterates over all data fields and generates ExternalSecret data entries dynamically.

**Benefits:**
- Supports multiple credential fields (username + password)
- Works with both basic-auth and SSH key authentication
- Secrets are automatically created and cleaned up per workflow run
- No manual secret management required
- Secret rotation is handled automatically by ESO
- Each workflow run gets its own isolated secret

### Approach 2: Manual Secrets with Hardcoded Names

Manually create Kubernetes secrets in the Build Plane's execution namespace and hardcode the secret name in the workflow template. This approach:
- Requires pre-creating secrets before running workflows
- Uses fixed secret names that are referenced directly in the workflow
- Useful for development, testing, or when ESO is not available

**Benefits:**
- Simple setup for development and testing
- No additional operators or infrastructure required
- Direct control over secret lifecycle
- Works without External Secrets Operator

## Platform Engineer vs Developer Responsibilities

### Platform Engineers Define:

- ✅ Which Workflow types are available (Docker, Buildpacks, etc.)
- ✅ System parameters schema (with customized defaults and validation)
- ✅ Developer parameters schema structure and validation rules
- ✅ Hardcoded parameters in runTemplate (registry URLs, security settings, timeouts)
- ✅ Resource limits and constraints
- ✅ Build plane integration details

### Developers Configure:

- ✅ Which Workflow to use for their Component
- ✅ Repository parameters: URL, branch, commit, appPath
- ✅ Developer parameters: build-specific settings
- ✅ Version and build settings (within Platform Engineer constraints)

## ComponentType Governance

Platform Engineers can restrict which Workflows are available per component type:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: service
spec:
  allowedWorkflows:
    - google-cloud-buildpacks
    - docker
  workloadType: deployment
```

This ensures developers can only use approved build workflows for each component type.

## See Also

- **[Build from Source Samples](../../from-source/)** - Complete examples using these workflows
- **[Component Samples](../../component-types/)** - Low-level component examples
- **[Generic Workflow Samples](../generic/)** - Standalone automation workflows
