# Workflow Samples

This directory contains reusable Workflow definitions that define how OpenChoreo builds applications from source code. Workflows are templates that integrate with the Build Plane (Argo Workflows) to automate the containerization of your applications.

## Overview

In OpenChoreo, a **Workflow** is a Custom Resource that:

1. **Defines a build strategy** - Specifies how to build and containerize your application (Docker, Buildpacks, etc.)
2. **Provides a schema** - Declares what parameters developers can configure (repository URL, build settings, etc.)
3. **Templates Argo Workflows** - Generates the actual Argo Workflow resources that execute in the Build Plane
4. **Enforces governance** - Platform Engineers control hardcoded parameters (registry URLs, timeouts, security settings)

## How Workflows Work

### Key Concepts

- **Workflow CR**: Platform Engineer-defined template that lives in the control plane
- **WorkflowRun CR**: Developer-created instance that triggers a build execution
- **Schema**: Developer-facing configuration options with type validation
- **Template Variables**: Placeholders like `${ctx.componentName}` and `${schema.repository.url}`
- **Build Plane**: A Kubernetes cluster running Argo Workflows

## Available Workflows

### [Docker Workflow](./docker.yaml)

Build applications using a Dockerfile.

**Use Case**: Applications with custom Dockerfiles that define their own build process.

**Developer Schema**:
```yaml
schema:
  repository:
    url: string                    # Git repository URL
    revision:
      branch: string | default=main
      commit: string | default=""
    appPath: string | default=.    # Path to application in repo
    secretRef: string              # Git credentials secret
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
  workflow:
    name: docker
    schema:
      repository:
        url: "https://github.com/myorg/myapp"
        revision:
          branch: "main"
        appPath: "/service"
        secretRef: "git-credentials"
      docker:
        context: "/service"
        filePath: "/service/Dockerfile"
```

### [Google Cloud Buildpacks Workflow](./google-cloud-buildpacks.yaml)

Build applications automatically using Google Cloud Buildpacks (no Dockerfile required).

**Use Case**: Applications where Buildpacks can automatically detect the language and build configuration (Go, Java, Node.js, Python, etc.).

**Developer Schema**:
```yaml
schema:
  repository:
    url: string
    revision:
      branch: string | default=main
      commit: string | default=HEAD
    appPath: string | default=.
    secretRef: string | enum=["reading-list-repo-credentials-dev","payments-repo-credentials-dev"]
  version: integer | default=1
  testMode: string | enum=["unit", "integration", "none"] default=unit
  command: '[]string | default=[]'
  args: "[]string | default=[]"
  resources:
    cpuCores: integer | default=1 minimum=1 maximum=8
    memoryGb: integer | default=2 minimum=1 maximum=32
  timeout: string | default="30m"
  cache:
    enabled: boolean | default=true
    paths: '[]string | default=["/root/.cache"]'
  limits:
    maxRetries: integer | default=3 minimum=0 maximum=10
    maxDurationMinutes: integer | default=60 minimum=5 maximum=240
```

**Platform Engineer Controls** (hardcoded):
- Builder image: `gcr.io/buildpacks/builder@sha256:...`
- Registry URL: `gcr.io/openchoreo-dev/images`
- Security scanning: enabled
- Build timeout: 30m

### [React Workflow](./react.yaml)

Specialized build workflow for React web applications.

**Use Case**: React applications that need Node.js-based builds.

**Developer Schema**:
```yaml
schema:
  repository:
    url: string
    revision:
      branch: string | default=main
      commit: string | default=""
    appPath: string | default=.
    secretRef: string
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
  componentType: deployment/service

  # Workflow configuration
  workflow:
    name: docker                    # Reference to Workflow
    schema:                         # Schema values
      repository:
        url: "https://github.com/myorg/myapp"
        revision:
          branch: "main"
        appPath: "/"
        secretRef: "git-credentials"
      docker:
        context: "/"
        filePath: "/Dockerfile"
```

### Method 2: Manual WorkflowRun

Create a WorkflowRun resource to trigger a build manually:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: WorkflowRun
metadata:
  name: my-app-build-01
spec:
  owner:
    projectName: "my-project"
    componentName: "my-app"

  workflow:
    name: docker
    schema:
      repository:
        url: "https://github.com/myorg/myapp"
        revision:
          branch: "main"
        appPath: "/"
        secretRef: "git-credentials"
      docker:
        context: "/"
        filePath: "/Dockerfile"
```

## Template Variables

Workflows support template variables for dynamic values:

| Variable | Description | Scope |
|----------|-------------|-------|
| `${ctx.workflowRunName}` | Name of the WorkflowRun CR | All workflows |
| `${ctx.componentName}` | Component name | Component-level workflows only |
| `${ctx.projectName}` | Project name | Component-level workflows only |
| `${ctx.orgName}` | Organization (namespace) | All workflows |
| `${schema.*}` | Developer-provided schema values | All workflows |

**Example**:
```yaml
spec:
  resource:
    spec:
      arguments:
        parameters:
          - name: image-name
            value: ${ctx.projectName}-${ctx.componentName}-image  # Becomes "my-project-my-app-image"
          - name: git-repo
            value: ${schema.repository.url}                       # Developer-provided value
```

## Platform Engineer vs Developer Responsibilities

### Platform Engineers Define:

- ✅ Which Workflow types are available (Docker, Buildpacks, etc.)
- ✅ Hardcoded parameters (registry URLs, security settings, timeouts)
- ✅ Resource limits and constraints
- ✅ Build plane integration details
- ✅ Schema structure and validation rules

### Developers Configure:

- ✅ Which Workflow to use for their Component
- ✅ Repository URL and branch
- ✅ Application-specific build parameters
- ✅ Version and build settings (within Platform Engineer constraints)

## Deploying Workflows

Deploy a workflow to your control plane:

```bash
# Deploy all workflows
kubectl apply -f samples/workflows/

# Deploy a specific workflow
kubectl apply -f samples/workflows/docker.yaml
```

Verify the workflow is available:

```bash
# List workflows
kubectl get workflows -n default

# Describe a workflow
kubectl describe workflow docker -n default
```

## See Also

- **[Build from Source Samples](../from-source/)** - Complete examples using these workflows
- **[Component Samples](../component-types/)** - Low-level component examples