# ComponentWorkflow Samples

This directory contains reusable ComponentWorkflow definitions that define how OpenChoreo builds applications from source code. ComponentWorkflows are specialized templates for component builds that integrate with the Build Plane (Argo Workflows) to automate the containerization of your applications.

## Overview

In OpenChoreo, a **ComponentWorkflow** is a Custom Resource that:

1. **Defines a build strategy** - Specifies how to build and containerize your application (Docker, Buildpacks, etc.)
2. **Provides structured system parameters** - Required repository configuration for build automation features (webhooks, auto-build, UI actions)
3. **Provides flexible developer parameters** - Platform Engineer-defined schema for additional build configuration
4. **Templates Argo Workflows** - Generates the actual Argo Workflow resources that execute in the Build Plane
5. **Enforces governance** - Platform Engineers control hardcoded parameters (registry URLs, timeouts, security settings)

## Why ComponentWorkflow vs Generic Workflow?

Component builds have unique requirements that need predictable structure for platform features:

- **Manual Build Actions** - UI actions like "build from latest commit" or "build from specific commit"
- **Auto-Build / Webhook Integration** - Automated builds triggered by Git push events
- **Build Traceability** - Tracking which Git repository, branch, and commit produced each build
- **Monorepo Support** - Identifying the specific application path within a repository

ComponentWorkflow enforces a required structure for repository information while maintaining schema flexibility for Platform Engineer-defined build parameters.

## How ComponentWorkflows Work

### Key Concepts

- **ComponentWorkflow CR**: Platform Engineer-defined template that lives in the control plane
- **ComponentWorkflowRun CR**: Instance that triggers a build execution (created automatically or manually)
- **System Parameters**: Required structured fields for repository information (url, branch, commit, appPath)
- **Developer Parameters**: Flexible PE-defined schema for build configuration (resources, caching, testing, etc.)
- **Template Variables**: Placeholders like `${ctx.componentName}`, `${systemParameters.repository.url}`, and `${parameters.version}`
- **Build Plane**: A Kubernetes cluster running Argo Workflows

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

## Template Variables

ComponentWorkflows support template variables for dynamic values in the `runTemplate`:

| Variable | Description | Scope |
|----------|-------------|-------|
| `${ctx.componentWorkflowRunName}` | Name of the ComponentWorkflowRun CR | All component workflows |
| `${ctx.componentName}` | Component name | Component-level workflows only |
| `${ctx.projectName}` | Project name | Component-level workflows only |
| `${ctx.orgName}` | Organization (namespace) | All component workflows |
| `${systemParameters.*}` | System parameter values (repository.url, etc.) | All component workflows |
| `${parameters.*}` | Developer-provided parameter values | All component workflows |

**Example**:
```yaml
spec:
  runTemplate:
    apiVersion: argoproj.io/v1alpha1
    kind: Workflow
    metadata:
      name: ${ctx.componentWorkflowRunName}
      namespace: openchoreo-ci-${ctx.orgName}
    spec:
      arguments:
        parameters:
          # Context variables
          - name: component-name
            value: ${ctx.componentName}
          - name: project-name
            value: ${ctx.projectName}
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
            value: ${ctx.projectName}-${ctx.componentName}-image
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
