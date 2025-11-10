# Workflow Samples

This directory contains sample configurations demonstrating OpenChoreo's schema-driven workflow architecture for building and deploying applications on the platform.

## Overview

OpenChoreo's Workflow system enables **Platform Engineers** to define reusable build templates (Workflows) that generate Argo Workflows, while **Developers** provide application-specific configuration through Component and WorkflowRun resources. The workflowrun controller automatically:

1. Renders Argo Workflows from templates using CEL expressions
2. Merges developer parameters with schema defaults
3. Deploys workflows to the build plane for execution
4. Extracts and creates Workload CRs from successful workflow outputs

## Architecture

```
Component (Developer Intent)
    ↓
ComponentType (PE Governance) + Workflow (PE Template)
    ↓
WorkflowRun CR (Execution Instance)
    ↓
[WorkflowRun Controller Renders]
    ↓
Argo Workflow (Build Plane)
    ↓
Workload CR (Extracted from outputs)
```

## Samples

### 1. Docker Build Workflow (`docker-greeter-service.yaml`)

Demonstrates a Docker-based build workflow using a Dockerfile.

**What it shows:**
- Docker build workflow definition with minimal schema
- Component configuration for Docker builds
- WorkflowRun execution instance

**Key Resources:**
- **Workflow**: `docker` - Defines Docker build template with repository and Docker-specific parameters
- **ComponentType**: `service` - Allows `docker` workflow
- **Component**: `greeting-service` - Uses Docker workflow with Dockerfile at `/service-go-greeter/Dockerfile`
- **WorkflowRun**: `greeting-service-build-04` - Execution instance for the greeting service

**Developer Parameters:**
- `repository.url` - Git repository URL
- `repository.revision.branch` - Git branch (default: `main`)
- `repository.revision.commit` - Git commit SHA (default: `""`)
- `repository.appPath` - Application path in repository (default: `.`)
- `repository.secretRef` - Secret for git credentials
- `docker.context` - Docker build context (default: `.`)
- `docker.filePath` - Path to Dockerfile (default: `./Dockerfile`)

### 2. Google Cloud Buildpacks Workflow (`go-buildpack-reading-list-service.yaml`)

Demonstrates a comprehensive buildpack-based workflow with extensive configuration options.

**What it shows:**
- Complex schema with nested objects, arrays, enums, and type validation
- Extensive build configuration options
- Secret injection from control plane to build plane

**Key Resources:**
- **Workflow**: `google-cloud-buildpacks` - Comprehensive buildpack template
- **ComponentType**: `service` - Allows both `google-cloud-buildpacks` and `docker` workflows
- **Component**: `reading-list-service` - Uses buildpacks with full configuration
- **WorkflowRun**: `reading-list-service-build-01` - Execution instance for reading list service

**Developer Parameters:**
- `repository.*` - Git repository configuration (URL, branch, commit, path, secretRef)
- `version` - Build version number (integer)
- `testMode` - Test mode enum: `unit`, `integration`, or `none` (default: `unit`)
- `command` - Build command array (default: `[]`)
- `args` - Build arguments array (default: `[]`)
- `resources.cpuCores` - CPU cores for build (1-8, default: 1)
- `resources.memoryGb` - Memory in GB (1-32, default: 2)
- `timeout` - Build timeout string (default: `30m`)
- `cache.enabled` - Enable caching (default: `true`)
- `cache.paths` - Cache paths array (default: `["/root/.cache"]`)
- `limits.maxRetries` - Max retry attempts (0-10, default: 3)
- `limits.maxDurationMinutes` - Max duration in minutes (5-240, default: 60)

### 3. React Web Application Workflow (`react-web-app.yaml`)

Demonstrates a React-specific build workflow with web application deployment configuration.

**What it shows:**
- React build workflow definition with Node.js version configuration
- Web application component type definition with Deployment and Service resources
- Component configuration for React web apps
- WorkflowRun execution instance for React builds

**Key Resources:**
- **Workflow**: `react` - Defines React build template with repository and Node.js parameters
- **ComponentType**: `web-application` - Defines deployment resources (Deployment + Service) with resource limits
- **Component**: `react-web-app` - Uses React workflow with Node v20 and custom resource configurations
- **WorkflowRun**: `react-web-app-build-01` - Execution instance for the React web application

**Developer Parameters:**
- `repository.url` - Git repository URL
- `repository.revision.branch` - Git branch (default: `main`)
- `repository.revision.commit` - Git commit SHA (default: `""`)
- `repository.appPath` - Application path in repository (default: `.`)
- `repository.secretRef` - Secret for git credentials
- `nodeVersion` - Node.js version (default: `"18"`)

**Component Type Parameters:**
- `replicas` - Number of replicas (default: `1`)
- `imagePullPolicy` - Image pull policy (default: `IfNotPresent`)
- `port` - Container port (default: `80`)
- `resources.requests.cpu` - CPU request (default: `100m`)
- `resources.requests.memory` - Memory request (default: `256Mi`)
- `resources.limits.cpu` - CPU limit
- `resources.limits.memory` - Memory limit

## Key Concepts

### CEL Expression Support

Templates support CEL expressions for dynamic value resolution:

**Context Variables** (`${ctx.*}`):
- `${ctx.orgName}` - Organization name (namespace)
- `${ctx.projectName}` - Project name from WorkflowRun.spec.owner
- `${ctx.componentName}` - Component name from WorkflowRun.spec.owner
- `${ctx.workflowRunName}` - WorkflowRun CR name
- `${ctx.timestamp}` - Auto-generated Unix timestamp
- `${ctx.uuid}` - Auto-generated 8-character UUID

**Schema Variables** (`${schema.*}`):
- `${schema.repository.url}` - Access nested developer parameters
- `${schema.version}` - Access simple developer parameters
- `${schema.resources.cpuCores}` - Access nested developer parameters

### Schema Format

Workflow schemas use a shorthand syntax:

```yaml
schema:
  # Simple field with default
  branch: "string | default=main"

  # Integer with constraints
  timeout: "integer | default=300 minimum=60 maximum=3600"

  # Enum with default
  testMode: "string | enum=[\"unit\", \"integration\", \"none\"] default=unit"

  # Array with default
  flags: "[]string | default=[\"--verbose\"]"

  # Nested object
  repository:
    url: string
    revision:
      branch: "string | default=main"
```

### Argo Workflow Parameter Conversion

All parameter values are automatically converted to strings when applied to Argo Workflows:
- Integers: `42` → `"42"`
- Booleans: `true` → `"true"`
- Arrays: `[1,2,3]` → `"[1,2,3]"` (JSON string)
- Objects: `{key: value}` → `"{\"key\":\"value\"}"` (JSON string)

### Secret Injection

Workflows can reference secrets that will be injected from the control plane to the build plane:

```yaml
spec:
  schema:
    repository:
      secretRef: string
  secrets:
    - ${schema.repository.secretRef}  # Secret name from developer schema
```

The workflowrun controller ensures these secrets exist in the build plane namespace before creating the Argo Workflow.

### Workload Creation

After an Argo Workflow completes successfully, the workflowrun controller automatically:

1. Looks for a step named `workload-create-step` in the workflow nodes
2. Extracts the `workload-cr` output parameter from that step
3. Parses the YAML into a Workload CR
4. Applies the Workload CR to the control plane using server-side apply

This enables the build process to define the runtime workload specification (containers, configurations, endpoints) that will be deployed to data planes.

## Usage

### For Platform Engineers

1. **Define Workflow**:
   - Create a template with schema for developer parameters
   - Use CEL expressions in the resource template
   - Specify secrets to inject into build plane

2. **Define ComponentType**:
   - List allowed Workflows in `build.allowedTemplates`
   - Define workload resource templates for deployment

### For Developers

1. **Create Component**:
   - Reference a ComponentType via `componentType`
   - Select a Workflow from allowed templates via `build.workflowTemplate`
   - Provide configuration in `build.schema` matching Workflow schema

2. **Create WorkflowRun**:
   - Reference the Workflow via `workflowRef`
   - Specify owner tracking (project and component names)
   - Provide developer parameters in `schema` field

3. **Monitor WorkflowRun**:
   - Check WorkflowRun status conditions: `WorkflowPending`, `WorkflowRunning`, `WorkflowSucceeded`, `WorkflowFailed`
   - Check Workload creation status: `WorkloadUpdated` condition
   - View rendered Argo Workflow in build plane namespace

## Applying Samples

```bash
# Apply Docker build sample
kubectl apply -f samples/workflow/docker-greeter-service.yaml

# Apply Buildpacks sample
kubectl apply -f samples/workflow/go-buildpack-reading-list-service.yaml

# Apply React web app sample
kubectl apply -f samples/workflow/react-web-app.yaml
```

## Troubleshooting

### WorkflowRun stuck in Pending

- Check if BuildPlane resource exists and is accessible
- Verify build plane cluster credentials are valid
- Check workflowrun controller logs for errors

### Rendered workflow has incorrect values

- Check CEL expressions in Workflow template
- Review schema defaults and developer-provided values

### Workload not created after workflow succeeds

- Verify Argo Workflow has a step named `workload-create-step`
- Check that step has succeeded (`phase: Succeeded`)
- Ensure step outputs a parameter named `workload-cr` with valid YAML
- Check workflowrun controller logs for extraction errors
- Verify `WorkloadUpdated` condition in WorkflowRun status
