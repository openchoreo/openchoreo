# ComponentWorkflow Samples

This directory contains reusable ComponentWorkflow definitions that define how OpenChoreo builds applications from source code. ComponentWorkflows are specialized templates for component builds that integrate with the Build Plane (Argo Workflows) to automate the containerization of your applications.

---
## Table of Contents

1. [Overview](#overview)
2. [How ComponentWorkflows Work](#how-componentworkflows-work)
    - [Key Concepts](#key-concepts)
    - [Referencing Build Plane Templates](#referencing-build-plane-templates)
3. [Available ComponentWorkflows](#available-componentworkflows)
    - [Docker ComponentWorkflow](#docker-componentworkflow)
    - [Google Cloud Buildpacks ComponentWorkflow](#google-cloud-buildpacks-componentworkflow)
    - [React ComponentWorkflow](#react-componentworkflow)
4. [Using ComponentWorkflows](#using-componentworkflows)
    - [Method 1: Reference in Component](#method-1-reference-in-component)
    - [Method 2: Manual ComponentWorkflowRun](#method-2-manual-componentworkflowrun)
5. [Deploying ComponentWorkflows](#deploying-componentworkflows)
6. [Template Variables](#template-variables)
7. [System Parameters vs Developer Parameters](#system-parameters-vs-developer-parameters)
8. [Reusable Type Definitions](#reusable-type-definitions)
9. [Working with Private Repositories](#working-with-private-repositories)
10. [Using Secrets in Workflows](#using-secrets-in-workflows)
11. [Platform Engineer vs Developer Responsibilities](#platform-engineer-vs-developer-responsibilities)
12. [ComponentType Governance](#componenttype-governance)
13. [See Also](#see-also)
---
## Overview

In OpenChoreo, a **ComponentWorkflow** is a Custom Resource that:

1. **Defines a build strategy** - Specifies how to build and containerize your application (Docker, Buildpacks, etc.)
2. **Provides structured system parameters** - Required repository configuration for build automation features (webhooks, auto-build, UI actions)
3. **Provides flexible developer parameters** - Platform Engineer-defined schema for additional build configuration
4. **Templates Argo Workflows** - Generates the actual Argo Workflow resources that execute in the Build Plane
5. **Enforces governance** - Platform Engineers control hardcoded parameters (registry URLs, timeouts, security settings)

## How ComponentWorkflows Work

### Key Concepts

- **ComponentWorkflow CR**: Platform Engineer-defined template that lives in the control plane
- **ComponentWorkflowRun CR**: Instance that triggers a build execution (created automatically or manually)
- **System Parameters**: Required structured fields for repository information (url, branch, commit, appPath)
- **Developer Parameters**: Flexible PE-defined schema for build configuration (resources, caching, testing, etc.)
- **Template Variables**: Placeholders like `${metadata.componentName}`, `${systemParameters.repository.url}`, and `${parameters.version}`
- **Build Plane**: A Kubernetes cluster running Argo Workflows
- **ClusterWorkflowTemplate**: Pre-defined Argo Workflow templates in the Build Plane that ComponentWorkflows reference via `workflowTemplateRef`

### Referencing Build Plane Templates

ComponentWorkflows generate Argo Workflow resources that reference ClusterWorkflowTemplates deployed in the Build Plane. This enables reusable build logic:

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
            value: ${systemParameters.repository.url}
          - name: branch
            value: ${systemParameters.repository.revision.branch}
```

## Available ComponentWorkflows

### [Docker ComponentWorkflow](./docker.yaml)

Build applications using a Dockerfile.

**Use Case**: Applications with custom Dockerfiles that define their own build process.

**System Parameters** (required structure):
```yaml
systemParameters:
  repository:
    url: string                    # Git repository URL
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
  componentType: deployment/service
  workflow:
    name: docker
    systemParameters:
      repository:
        url: "https://github.com/myorg/myapp"
        revision:
          branch: "main"
        appPath: "/service"
    parameters:
      docker:
        context: "/service"
        filePath: "/service/Dockerfile"
```

### [Google Cloud Buildpacks ComponentWorkflow](./google-cloud-buildpacks.yaml)

Build applications automatically using Google Cloud Buildpacks (no Dockerfile required).

**Use Case**: Applications where Buildpacks can automatically detect the language and build configuration (Go, Java, Node.js, Python, etc.).

**System Parameters** (required structure):
```yaml
systemParameters:
  repository:
    url: string | description="Git repository URL"
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

### [React ComponentWorkflow](./react.yaml)

Specialized build workflow for React web applications.

**Use Case**: React applications that need Node.js-based builds.

**System Parameters** (required structure):
```yaml
systemParameters:
  repository:
    url: string
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

## Using ComponentWorkflows

### Method 1: Reference in Component

Define the component workflow in your Component resource:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-app
spec:
  owner:
    projectName: my-project
  componentType: deployment/service

  # ComponentWorkflow configuration
  workflow:
    name: docker                    # Reference to ComponentWorkflow

    systemParameters:               # Required repository parameters
      repository:
        url: "https://github.com/myorg/myapp"
        revision:
          branch: "main"
          commit: ""                # Empty means latest
        appPath: "/"

    parameters:                     # Developer-configurable parameters
      docker:
        context: "/"
        filePath: "/Dockerfile"
```

### Method 2: Manual ComponentWorkflowRun

Create a ComponentWorkflowRun resource to trigger a workflow manually:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: ComponentWorkflowRun
metadata:
  name: my-app-build-01
spec:
  owner:
    projectName: "my-project"
    componentName: "my-app"

  workflow:
    name: docker

    systemParameters:
      repository:
        url: "https://github.com/myorg/myapp"
        revision:
          branch: "main"
          commit: "a1b2c3d4"        # Specific commit SHA
        appPath: "/"

    parameters:
      docker:
        context: "/"
        filePath: "/Dockerfile"
```

## Deploying ComponentWorkflows

Deploy a component workflow to your control plane:

```bash
# Deploy all component workflows
kubectl apply -f samples/component-workflows/

# Deploy a specific component workflow
kubectl apply -f samples/component-workflows/docker.yaml
```

Verify the component workflow is available:

```bash
# List component workflows
kubectl get componentworkflows -n default

# Describe a component workflow
kubectl describe componentworkflow docker -n default
```

## Template Variables

ComponentWorkflows support template variables for dynamic values in the `runTemplate`:

| Variable | Description | Scope |
|----------|-------------|-------|
| `${metadata.workflowRunName}` | Name of the ComponentWorkflowRun CR | All component workflows |
| `${metadata.componentName}` | Component name | Component-level workflows only |
| `${metadata.projectName}` | Project name | Component-level workflows only |
| `${metadata.namespaceName}` | Namespace name | All component workflows |
| `${systemParameters.*}` | System parameter values (repository.url, etc.) | All component workflows |
| `${parameters.*}` | Developer-provided parameter values | All component workflows |

**Example**:
```yaml
spec:
  runTemplate:
    apiVersion: argoproj.io/v1alpha1
    kind: Workflow
    metadata:
      name: ${metadata.workflowRunName}
      namespace: openchoreo-ci-${metadata.namespaceName}
    spec:
      arguments:
        parameters:
          # Context variables
          - name: component-name
            value: ${metadata.componentName}
          - name: project-name
            value: ${metadata.projectName}
          # System parameters
          - name: git-repo
            value: ${systemParameters.repository.url}
          - name: branch
            value: ${systemParameters.repository.revision.branch}
          # Developer parameters
          - name: version
            value: ${parameters.version}
          # Hardcoded PE-controlled values
          - name: image-name
            value: ${metadata.projectName}-${metadata.componentName}-image
```

## System Parameters vs Developer Parameters

### System Parameters (Required Structure)

All ComponentWorkflows must define these structured fields for build automation:

```yaml
systemParameters:
  repository:
    url: string                # Git repository URL
    revision:
      branch: string           # Git branch to build from
      commit: string           # Specific commit SHA (optional)
    appPath: string            # Path to application code in repository
```

**Key Constraints:**
- Field names and structure are fixed (url, revision.branch, revision.commit, appPath)
- All fields must be type `string` for build automation compatibility
- Platform Engineers can customize: defaults, enums, descriptions, validation rules
- Platform Engineers cannot change: field names, nesting structure, or types

**Why This Structure?**
- Enables webhooks to map Git events to components
- Powers UI actions like "build from latest commit"
- Provides build traceability for compliance and debugging
- Supports monorepo workflows

### Developer Parameters (Complete Freedom)

Platform Engineers define these custom parameters with full flexibility:

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

ComponentWorkflows support a `types` field in the schema to define reusable type definitions for complex structures. This enables you to define custom types that can be referenced in parameters, reducing duplication and ensuring consistency.

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

### Example: Complete Schema with Types

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: ComponentWorkflow
metadata:
  name: example-workflow
spec:
  schema:
    # Define reusable types
    types:
      Endpoint:
        name: "string"
        port: "integer"
        type: "string | enum=REST,HTTP,TCP,UDP"

      ResourceRequirements:
        requests: "ResourceQuantity"
        limits: "ResourceQuantity"

      ResourceQuantity:
        cpu: "string | default=100m"
        memory: "string | default=256Mi"

    systemParameters:
      repository:
        url: string
        revision:
          branch: string | default=main
          commit: string
        appPath: string | default=.

    parameters:
      # Use custom types
      endpoints: '[]Endpoint | default=[] description="Service endpoints"'
      resources: "ResourceRequirements | default={}"

      # Regular parameters
      version: integer | default=1
      buildArgs: '[]string | default=[]'
```

### Accessing Typed Parameters in Templates

Typed parameters can be accessed in the `runTemplate` just like any other parameter:

```yaml
spec:
  runTemplate:
    apiVersion: argoproj.io/v1alpha1
    kind: Workflow
    spec:
      arguments:
        parameters:
          # Access array of custom type
          - name: endpoints
            value: ${parameters.endpoints}

          - name: resources
            value: ${parameters.resources}
```

**Note**: Complex types (arrays and objects) are automatically converted to JSON strings when passed to Argo Workflow parameters, as Argo expects scalar string values.

## Working with Private Repositories

All default ComponentWorkflows support cloning from private Git repositories that require authentication. Private repository support is built-in and works seamlessly with GitHub, GitLab, Bitbucket, and AWS CodeCommit.

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
    systemParameters:
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
    systemParameters:
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
    systemParameters:
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

The ComponentWorkflow automatically creates an ExternalSecret for each build that fetches credentials from the secret store and makes them available to the workflow.

## Using Secrets in Workflows

Build workflows often need access to secrets for authentication, such as:
- **Git credentials** for cloning private repositories (see "Working with Private Repositories" section above)
- **Registry credentials** for pushing images to private container registries
- **API tokens** for external services during the build process

ComponentWorkflows support two approaches for managing secrets in the Build Plane:

### Approach 1: External Secrets with Dynamic Secret Names (Recommended)

Use the `resources` section in ComponentWorkflow to define ExternalSecret resources that point to secrets in your secret backend. This approach:
- Automatically creates and syncs secrets in the Build Plane's execution namespace
- Generates unique secret names per workflow run (e.g., `${metadata.workflowRunName}-git-secret`)
- Passes the secret name as a parameter to the workflow, allowing the workflow to reference it during execution
- Ideal for GitOps workflows where all configuration is version-controlled

**Example with Git Secret (supports multiple data fields):**

```yaml
resources:
  - id: git-secret
    includeWhen: ${has(systemParameters.repository.secretRef)}
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
          name: openbao
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

- ✅ Which ComponentWorkflow types are available (Docker, Buildpacks, etc.)
- ✅ System parameters schema (with customized defaults and validation)
- ✅ Developer parameters schema structure and validation rules
- ✅ Hardcoded parameters in runTemplate (registry URLs, security settings, timeouts)
- ✅ Resource limits and constraints
- ✅ Build plane integration details

### Developers Configure:

- ✅ Which ComponentWorkflow to use for their Component
- ✅ System parameters: repository URL, branch, commit, appPath
- ✅ Developer parameters: build-specific settings
- ✅ Version and build settings (within Platform Engineer constraints)

## ComponentType Governance

Platform Engineers can restrict which ComponentWorkflows are available per component type:

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

- **[Build from Source Samples](../from-source/)** - Complete examples using these component workflows
- **[Component Samples](../component-types/)** - Low-level component examples
- **[ComponentWorkflow Discussion](../../discussions/component-workflows/)** - Design rationale and architecture details
