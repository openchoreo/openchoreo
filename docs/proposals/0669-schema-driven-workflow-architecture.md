# Schema-Driven Workflow Architecture for OpenChoreo CI and Generic Workflows

**Authors**:  
@chalindukodikara

**Reviewers**:  
@mirage20 @sameerajayasoma @binura-g

**Created Date**:  
2025-10-30

**Status**:  
Submitted

**Related Issues/PRs**:  
[Issue #669](https://github.com/openchoreo/openchoreo/issues/669)  
[Proposal Discussion #568](https://github.com/openchoreo/openchoreo/discussions/568)

---

## Summary

This proposal introduces a schema-driven workflow architecture that replaces OpenChoreo's rigid Build CR with a flexible, template-based system which can be used for both OpenChoroe CI and Generic Workflows. The new design introduces **WorkflowDefinition** CRDs that enable platform engineers to define type-safe, validated schemas for both component-specific builds and organization-level generic workflows. This approach separates platform engineer governance (security policies, compliance requirements) from developer configuration (application-specific parameters) while maintaining a unified developer experience through the Component CR.

---

## Motivation

OpenChoreo's current Build CR uses a fixed schema with flat key-value parameter lists, creating several critical limitations for both platform engineers and developers:

### Current Limitations

**1. Rigid Schema Without Extensibility**

The Build CR uses a fixed schema that cannot be extended by platform engineers. The parameter section only supports flat key-value pairs without structure, type validation, or nested objects. While Argo Workflows supports descriptions, defaults, and enums, these are defined in the workflow template itself and not visible in the OpenChoreo developer experience.

```yaml
# Current Build CR - flat parameters and a Schema provided by OpenChoreo
apiVersion: openchoreo.dev/v1alpha1
kind: Build
metadata:
  name: private-app-build
  namespace: myorg
spec:
  owner:
    projectName: "backend-services"
    componentName: "user-service"
  # Schema provided by OpenChoreo
  repository: 
    url: "https://github.com/myorg/private-user-service.git"
    revision:
      branch: "main"
    appPath: "."
    credentialsRef: "github-pat"  
  templateRef:
    engine: "argo"
    name: "buildpack-nodejs"
    # Developer params provided by PE
    parameters:
      - name: "buildpack"
        value: "nodejs"
      - name: "dev-registry-url" 
        value: "docker.io/myorg-dev"
      - name: "dev-registry-credentials" 
        value: "dockerhub-push-dev"
      - name: "prod-registry-url" 
        value: "docker.io/myorg-prod"
      - name: "prod-registry-credentials"
        value: "dockerhub-push-prod"
```

**Problems:**
- No nested structures for related configuration (e.g., `registry[0].url` and `registry[0].credentials`)
- No type validation (nothing prevents passing a string where an integer is expected)
- No enum validation for restricted values
- Relationships between related parameters are lost

**2. No Platform Engineer Control Over Security Policies**

Platform Engineers cannot enforce security, compliance, or resource policies through parameters that vary by component type without creating duplicate workflow templates:

Options:
- **Hardcoding in templates**: Cannot vary by component type (e.g., enable SCA for services, disable for scheduled tasks)
- **Letting developers control**: Developers can bypass security policies
- **Creating duplicate templates**: Results in `buildpacks-service/`, `buildpacks-webapp/`, `buildpacks-scheduled-task/` with nearly identical code

**What Platform Engineers Need**: Parameters that vary by component type but are hidden from developers.

**3. Limited Workflow Template Reusability**

Because PE-controlled parameters must be hardcoded, a single workflow template cannot be reused across component types with different policy requirements. For example, using the same buildpacks template for services (go, SCA enabled), web-apps (nodejs, SCA enabled), and scheduled-tasks (python, SCA disabled) currently requires three separate template definitions.

**4. No Support for Generic Workflows**

The Build CR schema is tightly coupled to container image builds, making it unsuitable for generic workflows like:
- Package publishing (npm, Maven Central, Ballerina Central)
- Infrastructure provisioning (Terraform/OpenTofu)
- Database migrations
- ETL pipelines and data processing
- Compliance scanning and security testing

Using separate CRs for builds and generic workflows creates technical debt through duplicate validation logic, parameter handling, and status tracking.

---

## Goals

- **Schema-Driven Extensibility**: Enable platform engineers to define custom, type-safe schemas for build workflow.
- **Separation of Concerns**: Platform engineers control security policies and compliance requirements while developers control application-specific parameters within PE-defined guardrails.
- **Template Reusability**: Allow a single WorkflowDefinition to be shared across multiple ComponentTypeDefinitions with different policy configurations.
- **Unified Architecture**: Support both component-specific builds and organization-level generic workflows through the same WorkflowDefinition mechanism.
- **Developer Experience**: Maintain a simplified developer experience through the Component CR with auto-generated UI forms based on defined schemas.
- **Type Safety**: Provide type validation, nested structures, enums, defaults, and documentation for all parameters.

---

## Non-Goals

- **Replace Existing Build Functionality**: This proposal extends rather than replaces the current build system architeture.
- **UI Implementation**: While the proposal mentions auto-generated UI forms based on schemas, the actual UI/portal implementation details are out of scope.
- **Workflow Templates**: This proposal defines the OpenChoreo abstraction layer for referencing workflows such as Argo Workflows, Tekton Pipelines, etc. But it does not cover the authoring of the underlying workflow templates themselves.

---

## Impact

### New CRDs
- **WorkflowDefinition** (v1alpha1): Defines schemas, parameter mappings, and fixed PE-controlled parameters
- **Workflow** (v1alpha1): Runtime execution resource replacing the current Build CR

### Modified CRDs
- **Component** (v1alpha1): Updated to support schema-based build configuration
- **ComponentTypeDefinition** (v1alpha1): Extended with `build.allowedTemplates` for workflow template restrictions

### Controllers
- **Workflow Controller**: New controller for managing workflow executions and rendering Argo Workflow resources
- **Build Controller**: Marked as deprecated

### Build Plane
- No changes required to BuildPlane infrastructure or Argo Workflows installation

---

## Design

This proposal introduces a flexible, schema-driven architecture using four key custom resources that separate concerns between platform engineering control and developer flexibility.

### Architecture Overview

**OpenChoreo CI Flow**

![OpenChoreo CI](https://github.com/user-attachments/assets/8b470dbb-f478-4a11-a52c-d360eb384f8f)

**Generic Workflows Flow**

![Generic Workflows](https://github.com/user-attachments/assets/9643356f-a613-4145-bff4-128395195864)

### Design Principles

1. **Extensibility from Component Type Definition**: Follows the same template-driven approach as Component Type Definitions, enabling platform engineers to define flexible schemas

2. **Separation of Concerns**:
   - Platform Engineers control: security policies, compliance requirements, resource templates
   - Developers control: application-specific parameters within PE-defined guardrails

3. **Developer Experience**: Developers interact with a single, familiar Component CR with:
   - Type-safe parameter validation
   - Auto-generated UI forms with descriptions and defaults
   - Clear documentation of available options

4. **Reusability**:
   - WorkflowDefinitions can be shared across multiple ComponentTypeDefinitions
   - Same architecture supports both component-specific builds and organization-level generic workflows
   - PE parameters can be defined once and reused with different values across component types

5. **Generic Workflow Support**: The architecture supports any workflow type including:
   - Infrastructure Provisioning (Terraform/OpenTofu pipelines)
   - Data Processing (ETL pipelines, data transformation)
   - Storage Operations (S3/blob storage uploads)
   - Database Migrations (schema changes)
   - Testing (end-to-end, performance)
   - Compliance Scanning (security scans, license compliance)
   - Scheduled maintenance tasks (using Argo Events)
   - Package Publishing (npm, Maven Central, Ballerina Central)

---

### Custom Resource Definitions

#### 1. WorkflowDefinition CR

The WorkflowDefinition CRD defines the schema, parameter mappings, and PE-controlled fixed parameters for a workflow template.

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: WorkflowDefinition
metadata:
   name: google-cloud-buildpacks
   annotations:
      openchoreo.dev/description: "Google Cloud Buildpacks workflow for containerized builds"
spec:
   # Template Variable Reference (processed by controller):
   # ${ctx.componentName}           - Component name 
   # ${ctx.projectName}             - Project name 
   # ${ctx.orgName}                 - Organization name
   # ${ctx.timestamp}               - Unix timestamp (e.g., 1234567890)
   # ${ctx.uuid}                    - UUID (8 chars)
   # ${schema.*}                    - Developer-provided values from schema
   # ${fixedParameters.*}           - PE-controlled fixed parameters
   # Developer-facing schema with type validation
   schema:
      repository:
         url: string
         revision:
            branch: string | default=main
            commit: string | default=HEAD
         appPath: string | default=.
         credentialsRef: string | enum=["checkout-repo-credentials-dev","payments-repo-credentials-dev"]
      version: integer | default=1
      testMode: string | enum=["unit", "integration", "none"] | default=unit

   # Secret references to inject into build plane
   secrets:
      - ${schema.repository.credentialsRef}

   # Static, PE-controlled parameters (hidden from developer)
   fixedParameters:
      - name: builder-image
        value: gcr.io/buildpacks/builder:v1
      - name: registry-url
        value: gcr.io/openchoreo-dev/images
      - name: security-scan-enabled
        value: "true"
      - name: build-timeout
        value: "30m"

   # Rendered resource
   resource:
      template:
         apiVersion: argoproj.io/v1alpha1
         kind: Workflow
         metadata:
            name: ${ctx.componentName}-${schema.repository.revision.commit}-${ctx.uuid} # PE needs to ensure uniqueness
            namespace: openchoreo-ci-${ctx.orgName}
         spec:
            arguments:
               parameters:
                  - name: component-name
                    value: ${ctx.componentName}
                  - name: project-name
                    value: ${ctx.projectName}
                  # Parameters from schema (developer-facing)
                  - name: repo-url
                    value: ${schema.repository.url}
                  - name: branch
                    value: ${schema.repository.revision.branch}
                  - name: commit
                    value: ${schema.repository.revision.commit}
                  - name: app-path
                    value: ${schema.repository.appPath}
                  - name: version
                    value: ${schema.version}
                  - name: test-mode
                    value: ${schema.testMode}
                  # Parameters from fixedParameters (PE-controlled)
                  - name: builder-image
                    value: ${fixedParameters.builder-image}
                  - name: registry-url
                    value: ${fixedParameters.registry-url}
                  - name: security-scan-enabled
                    value: ${fixedParameters.security-scan-enabled}
                  - name: build-timeout
                    value: ${fixedParameters.build-timeout}
            serviceAccountName: workflow-sa
            workflowTemplateRef:
               clusterScope: true
               name: google-cloud-buildpacks
```

**Key Features:**
- **Schema Definition**: Defines the structure developers interact with, including types, defaults, enums, and nested objects
- **Parameter Mapping**: Maps schema fields to Argo Workflow parameters using CEL expressions
- **Fixed Parameters**: PE-controlled parameters hidden from developers for security and compliance
- **Secret Management**: Declares which secrets need to be synchronized to the build plane

#### 2. ComponentTypeDefinition CR (Extended)

ComponentTypeDefinition is extended to restrict which workflow templates developers can use and override fixed parameters per component type.

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: ComponentTypeDefinition
metadata:
  name: service
spec:
  # Restrict which workflow templates developers can use for this component type
  build:
    allowedTemplates:
      - name: google-cloud-buildpacks
        # PE-controlled parameters that override WorkflowDefinition defaults
        fixedParameters:
          - name: security-scan-enabled
            value: false
          - name: build-timeout
            value: "45m"
      - name: docker
```

**Key Features:**
- **Template Allowlist**: Controls which WorkflowDefinitions developers can use for this component type.
- **Parameter Overrides**: Overrides fixed parameters from WorkflowDefinition for component-type-specific policies.
- **Reusability**: Same WorkflowDefinition can be used across multiple component types with different fixed parameters.

#### 3. Component CR

Developer-created resource containing build configuration conforming to the WorkflowDefinition schema.

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: checkout-service
spec:
  # References the ComponentTypeDefinition
  componentType: deployment/service
  
  # Selects a WorkflowDefinition from allowed templates in ComponentTypeDefinition
  workflowTemplate: google-cloud-buildpacks
  
  # Build configuration matching the schema from WorkflowDefinition
  build:
    # Nested structure from schema
    repository:
      url: "https://github.com/myorg/checkout-service.git"
      revision:
        branch: "release/v2"
        commit: "a1b2c3d"
      appPath: "./src"
      credentialsRef: "checkout-repo-credentials-dev"

    # Simple schema fields
    version: 3
    testMode: "integration"  # From enum: ["unit", "integration", "none"]
```

**Key Features:**
- **Type Safety**: All fields are validated against the WorkflowDefinition schema
- **Nested Structures**: Supports complex parameter hierarchies
- **Enum Validation**: Restricts values to predefined options
- **Defaults**: Inherits default values from WorkflowDefinition schema

#### 4. Workflow CR

Runtime execution resource created when triggering a workflow. Contains the same parameters as Component but with ownership metadata and execution status.

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Workflow
metadata:
  # Should be unique per execution, PEs must ensure uniqueness
  name: checkout-service-build-01-abc123
spec:
  # Ownership tracking for the workflow execution
  owner:
    projectName: "backend-services"
    componentName: "checkout-service"

  # Developer parameters from Component CR
  parameters:
    repository:
      url: "https://github.com/myorg/checkout-service.git"
      revision:
        branch: "release/v2"
        commit: "a1b2c3d"
      appPath: "./src"
      credentialsRef: "checkout-repo-credentials-dev"
    version: 3
    testMode: "integration"

status:
  # Execution status tracking
  phase: Running
```
---

### Generated Argo Workflow

The Workflow controller generates the final Argo Workflow CR by combining all parameter sources:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Workflow
metadata:
  name: checkout-service-build-01-abc123
  namespace: openchoreo-ci-default
  labels:
    openchoreo.dev/project: backend-services
    openchoreo.dev/component: checkout-service
    openchoreo.dev/workflow: checkout-service-build-01
spec:
  arguments:
    parameters:
      # Developer parameters (from schema)
      - name: repo-url
        value: https://github.com/myorg/checkout-service.git
      - name: branch
        value: release/v2
      - name: commit
        value: a1b2c3d
      - name: app-path
        value: ./src
      - name: version
        value: "3"
      - name: test-mode
        value: integration

      # Fixed PE parameters (from WorkflowDefinition/ComponentTypeDefinition)
      - name: language
        value: go
      - name: sca-scan
        value: "true"
      - name: cache-enabled
        value: "true"

      # OpenChoreo context parameters (injected by controller)
      - name: project-name
        value: backend-services
      - name: component-name
        value: checkout-service

  # From WorkflowDefinition
  serviceAccountName: choreo-build-bot

  # Reference to the Argo Workflow template
  workflowTemplateRef:
    clusterScope: true
    name: google-cloud-buildpacks
```

---

### Advanced WorkflowDefinition with Template Rendering

For advanced use cases, WorkflowDefinition supports direct resource template rendering with contextual variables:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: WorkflowDefinition
metadata:
  name: google-cloud-buildpacks
  annotations:
    openchoreo.dev/description: "Google Cloud Buildpacks workflow for containerized builds"
spec:
  # Template Variable Reference (processed by controller):
  # ${ctx.componentName}           - Component name
  # ${ctx.projectName}             - Project name
  # ${ctx.orgName}                 - Organization name
  # ${ctx.timestamp}               - Unix timestamp (e.g., 1234567890)
  # ${ctx.uuid}                    - UUID (8 chars)
  # ${schema.*}                    - Developer-provided values from schema
  # ${fixedParameters.*}           - PE-controlled fixed parameters
  
  schema:
        ...
  fixedParameters:
        ...
  
  # Rendered resource template
  resource:
    template:
      apiVersion: argoproj.io/v1alpha1
      kind: Workflow
      metadata:
        name: ${ctx.componentName}-${schema.repository.revision.commit}-${ctx.uuid}
        namespace: openchoreo-ci-${ctx.orgName}
      spec:
        arguments:
          parameters:
            # Context parameters
            - name: component-name
              value: ${ctx.componentName}
            - name: project-name
              value: ${ctx.projectName}

            # Parameters from schema (developer-facing)
            - name: repo-url
              value: ${schema.repository.url}
            - name: branch
              value: ${schema.repository.revision.branch}
            - name: commit
              value: ${schema.repository.revision.commit}
            - name: app-path
              value: ${schema.repository.appPath}
            - name: version
              value: ${schema.version}
            - name: test-mode
              value: ${schema.testMode}

            # Parameters from fixedParameters (PE-controlled)
            - name: builder-image
              value: ${fixedParameters.builder-image}
            - name: registry-url
              value: ${fixedParameters.registry-url}
            - name: security-scan-enabled
              value: ${fixedParameters.security-scan-enabled}
            - name: build-timeout
              value: ${fixedParameters.build-timeout}

        serviceAccountName: workflow-sa
        workflowTemplateRef:
          clusterScope: true
          name: google-cloud-buildpacks
```

**Template Variables:**
- **Context Variables** (`${ctx.*}`): Injected by the controller.
- **Schema Variables** (`${schema.*}`): Developer-provided values from the Component CR.
- **Fixed Parameters** (`${fixedParameters.*}`): PE-controlled values from WorkflowDefinition/ComponentTypeDefinition.
