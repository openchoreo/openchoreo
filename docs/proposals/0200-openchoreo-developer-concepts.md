# OpenChoreo Developer Concepts

**Authors**:  
@sameerajayasoma
@Mirage20  

**Reviewers**:  

**Created Date**:  
2025-04-19

**Status**:  
Approved

**Related Issues/PRs**:  
[Issue #200 – openchoreo/openchoreo](https://github.com/openchoreo/openchoreo/issues/200)

---

## Summary
This proposal defines a set of developer-facing abstractions for OpenChoreo (Project, Component, API, and WorkloadSpec) to declaratively model application architecture and runtime behavior. It suggests a GitOps-compatible workflow for managing these resources in version control, while also introducing a control plane API to support interactive operations via CLI and UI. The design supports both monorepo and multi-repo setups, enabling the reuse of the same source code across multiple components with distinct runtime contracts.

## Goals
- Enable developers to define Projects, Components, and APIs using declarative YAML files.
- Provide a clear way to declare a component’s runtime contract (e.g., endpoints, connections, etc) separate from source code.
- Ensure all declarations are easily managed in Git repositories and are compatible with GitOps workflows (e.g., Argo CD, Flux) for automated reconciliation and promotion.
- Provide recommended patterns for organizing declarations across mono- and multi-repository setups.
- Allow a single codebase to back multiple components, each with its own workload configuration.
- Expose an API server in the OpenChoreo control plane to support CLI, UI, and CI systems, complementing GitOps with interactive and event-driven workflows.

## Developer-facing abstractions
The following is a set of high-level, declarative constructs that enable developers to model project or application structure, runtime contracts, and connectivity without needing to manage low-level infrastructure details.

OpenChoreo introduces a set of high-level, Kubernetes-native abstractions that help developers and platform engineers structure, deploy, and expose cloud-native applications with clarity and control. These abstractions are explicitly separated to address concerns like identity, runtime behavior, API exposure, version tracking, and environment-specific deployment.

### Project
A Project is a logical grouping of related components that collectively define an application or bounded context. It serves as the main organizational boundary for source code, deployments, and network policies.
Projects:
- Define team or application-level boundaries.
- Govern internal access across components.

A project defines deployment isolation, visibility scope, and governs how components communicate internally and with external systems via ingress and egress gateways.

### Component 
A component represents a deployable piece of software, such as a backend service, frontend webapp, background task, or API proxy.

Each Component:
- Defines its identity within a Project.
- Declares its type (e.g., service, webapp, task, api).
  - Specifies the component type-specific settings (cron expression, replica sets, API mgt stuff) 
- Refers to a source code location (e.g., Git repo, image registry).
  - I.e, Workload of the component.

### Component Types
Each Component in OpenChoreo is assigned a type that determines its behavior, settings, and how it is deployed and exposed. A Component Type captures the high-level intent of the component. Whether it runs continuously, executes on a schedule, proxies an API, or represents a managed resource, such as a database.

OpenChoreo includes several built-in Component Types, each with its own set of expected behaviors and type-specific settings:

#### WebApp
A web-facing application that serves frontend assets or handles HTTP traffic. It may expose ports and routing paths via a gateway.
#### Service
A backend service, often HTTP or gRPC-based, that serves as an internal or external API provider.
#### ScheduledTask
A task that runs on a cron schedule, ideal for background jobs or periodic automation.
#### ManualTask
A placeholder for components that involve manual execution or human workflow, often used for approval gates or ops procedures.
#### ProxyAPI
An API proxy that exposes an upstream backend through OpenChoreo’s built-in API gateway with support for versioning, throttling, and authentication.


Component Types are pluggable. Platform teams can define their own types to represent custom workloads, third-party services, or golden-path abstractions, each with their own schema and runtime behavior.

### Workload 
A Workload captures the runtime contract of a Component within a DeploymentTrack. It defines how the component runs: its container image, ports, environment variables, and runtime dependencies.
Workloads:
- They are versioned and linked to a specific track.
- Can change frequently (e.g., after every CI build).

The WorkloadDescriptor defines the runtime contract of the component. What OpenChoreo needs to know in order to wire, expose, and run it. It describes:
- What ports and protocols does the component expose?
- What are the connection dependencies (e.g., databases, queues, other component endpoints)?
- What container image should be deployed?

It is typically maintained along with the source code of a component. 

If we decouple the OpenChoreo component from its workload descriptor, then we can support the goal of linking a single source code to multiple OpenChoreo components. This is what we have in WSO2 Choreo SaaS.

By combining a component’s type, its settings, and an optional workload, OpenChoreo provides a consistent and extensible model for defining, deploying, and operating software components across various environments.

### API
An API describes how a component’s endpoint is exposed beyond its Project. This includes HTTP routing, versioning, authentication, rate limits, and scopes.

APIs:
- Are optional — many components may remain internal.
- Can be independently versioned.
- Enable controlled exposure (organization-wide or public) and integration into API gateways or developer portals.

Both Service and ProxyAPI Component types can expose APIs. 

### DeploymentTrack
A DeploymentTrack represents a line of development or a delivery stream for a component. It can be aligned with a Git branch (main, release-1.0, feature-xyz), an API version (v1, v2), or any other meaningful label that denotes evolution over time.

This is optional when getting started or for simpler use cases. If omitted, it is assumed to belong to the default track (default).

### Environments 
An Environment represents a target runtime space — such as dev, staging, or prod. It encapsulates infrastructure-specific configuration like cluster bindings, gateway setups, secrets, and policies.
Environments:
- Are where actual deployments occur.
- May have constraints or validations (e.g., only approved workloads may run in prod).

### Deployments
A Deployment binds a specific Workload (and optionally an API) to an Environment within a given track. It represents the act of deploying a concrete version of a component to a specific environment.

Deployments:
- Are the unit of rollout and promotion.
- Track the exact workload/API deployed in each environment.
- Enable progressive delivery by reusing the same workload across environments (e.g., dev → staging → prod).

The Deployment is always derived, never written by hand
- Developers author Component, Workload (?), and optionally API
- GitOps or CI/CD workflows generate a Deployment by linking the current Workload + API + Track + Environment
- Promotion means copying or regenerating the Deployment with the same Workload in a different environment

### Why are Component, Workload and Type-specific Settings separated?
Separating these concerns improves clarity, reusability, and GitOps alignment:
- Component defines identity, ownership, and deployment intent—what the unit of software is.
- Workload captures how the component runs—image, ports, environment—often generated or updated by CI.
- Type-specific Settings configure how the component behaves based on its type—e.g., a cron schedule for a task, or routing rules for an API.

## YAML file format
Projects, components and APIs are Kubernetes CRDs. We can make them less complicated for developers and PEs to maintain. 

This section describes the shape and semantics of these kinds. 

### Namespaces and Organizations
There is a 1:1 mapping between OpenChoreo organizations and Kubernetes namespaces in the OpenChoreo control plane. 

We create a K8s namespace for each OpenChoreo organization. This is a cluster-scoped resource. All other resources within this organization are created as namespace-scoped resources.

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Organization
metadata:
  name: acme

---
apiVersion: openchoreo.dev/v1alpha1
kind: Organization
metadata:
  name: default
```

The following project is part of the Acme organization. 
```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: hello-project
  namespace: acme
spec:
  owner:
    organization: acme
```

The `metadata.namespace` is a mandatory field unless it is in the default organization or namespace. See the semantics of the resource with the name default: [https://github.com/openchoreo/openchoreo/issues/227](https://github.com/openchoreo/openchoreo/issues/227)

The `spec.organization` field is an optional field and OpenChoreo operator can add it if it is not present.

#### The default organization
By default, OpenChoreo creates an organization called “default” which gets mapped to the Kubernetes “default” namespace. This default namespace will always be there in a K8s cluster and it cannot be deleted AFAIK. 

The beauty of this default org and the corresponding default K8s namespace is that you can omit the namespace field in CRs. You will see this in the following samples. 

### Custom resource(CR) naming scope
In OpenChoreo, all first-class developer abstractions — Organization, Project, Component, and API — are modeled as Kubernetes Custom Resources (CRs). 

These resources follow a global naming model within the control plane:
- Organization CR names must be unique within the K8s cluster where the OpenChoreo control plane resides.
- Project CR names must be unique within an Organization (K8s namespace)
- Component CR names must be globally unique within an Organization (K8s namespace)
- API CR names must be globally unique within an Organization (K8s namespace)

### Immutable fields
Certain spec fields in Project, API and Component CRs are immutable after creation. Consider the following example. 

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: reading-list
  namespace: acme
spec:
  owner:
    organizationName: acme
    projectName: hello-world
```

The fields spec.organization and spec.project are immutable after creation. The OpenChoreo admission webhooks enforce this. 

We may make them mutable later. Who knows :) 

### Kubernetes labels
Typically, labels are used for querying, filtering, indexing and automation. In this design, developers are not required to add labels, but they are useful to have them. Therefore, OpenChoreo admission webhooks can mutate the resources to add labels during the resource creation process. 

This is what the developer writes:
```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: reading-list
  namespace: acme
spec:
  owner:
    projectName: hello-project
  type: service
```

This is what gets stored in etcd:
```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: reading-list
  namespace: acme
  lables:
    openchoreo.dev/org: acme
    openchoreo.dev/project: hello-project
    openchoreo.dev/component: reading-list\
    openchoreo.dev/type: service

 ... # Other generated fields

spec:
    owner:
      organizationName: acme
      projectName: hello-world
  type: service
```

### Project
A logical boundary that groups related components and APIs. It defines runtime isolation and controls ingress/egress via gateways. 

The following YAML represents a project. 
```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: hello-project
```

Currently, all fields in the spec section are optional. A project sample with the spec section:
```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: hello-project
spec:
  deploymentPipelineRef: default
```

The organization field is also optional. The default organization is called “default” which maps to the “default” namespace of the Kubernetes cluster. AFAIK, this default namespace will always be there in a Kubernetes cluster and it cannot be deleted. 

Here is a sample with the organization name. If you want to use a different organization, then you would have to add two fields:
- `metadata.namepsace` 
- `spec.organiation`

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Project
metadata:
  name: hello-project
  namespace: abc
spec:
  organization: abc
```

### Component 
A component abstraction in OpenChoreo is defined by three parts:
1) Declares the identity of the component, including name, project and organization. Declares whether OpenChoreo should build the image or not (BYOR vs BYOI).
   - This is the OpenChoreo component kind, just like the project kind
2) Describes how the component should be executed at runtime, including what it exposes (Endpoints) and depends on (Connections). 
   - Current Choreo SaaS component.yaml or Score’s workload spec.
3) Describes the component type-specific runtime settings.


The obvious choice is to inline all three parts in a single Component CR. But that would make the CR too complicated to author and maintain. I believe separating these concerns improves clarity, reusability, and GitOps alignment. We can consider inlining them later if needed.

#### Part 1: Component CR
This is the OpenChoreo Component CR. It declares the identity of the component, including name, project and organization. It also declares whether OpenChoreo should build the image or not (BYOR vs BYOI).

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: reading-list
  namespace: acme
spec:
  owner:
    organizationName: acme
    projectName: hello-project
  type: service
  Build:
    Repository:
      App Path:  /service-go-reading-list
      Revision:
        Branch:  main
      URL:       https://github.com/openchoreo/sample-workloads
    Template Ref:
      Name:  google-cloud-buildpacks
```

The `spec.build` section is optional. If it is omitted, then it is assumed to be BYOI (Bring Your Own Image) and OpenChoreo will not attempt to build the image. This is useful if you are using an external CI/CD system to build and push images.

#### Part 2: WorkloadDescriptor CR
This is the workload descriptor CR. It describes how the component should be executed at runtime, including what it exposes (Endpoints) and depends on (Connections). In other words, it describes the runtime contract of the component in a programming language-agnostic way.

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: go-reading-list-service
  namespace: default
spec:
  containers:
    main:
      image: localhost:30003/default-reading-list-service:default-8ecbb654
  endpoints:
    reading-list-api:
      port: 8080
      schema:
        content: "openapi: 3.0.1\ninfo:\n  title: Choreo Reading List\n  description: ... "
  owner:
    componentName: reading-list-service
    projectName: default
```

#### Part 3: Type-specific Settings
This section describes the component type-specific runtime settings. The settings vary based on the component type (e.g., service, webapp, task, api).

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Service
metadata:
  name: reading-list-service
  namespace: default
spec:
  apis:
    reading-list-api:
      className: default
      rest:
        backend:
          basePath: /api/v1/reading-list
          port: 8080
        exposeLevels:
        - Public
      type: REST
  className: default
  owner:
    componentName: reading-list-service
    projectName: default
  workloadName: go-reading-list-service
```

### Workload descriptor
A Workload descriptor captures the runtime contract of a Component within a DeploymentTrack. It defines how the component runs: its container image, ports, environment variables, and runtime dependencies. This is typically maintained along with the source code of a component and the Workload CR is created by a CI/CD system or GitOps operator.

We expect the CI system to create or update the Workload CR after a successful build and push of the container image. The Workload CR is linked to a specific Component and DeploymentTrack.

Here is a sample workload descriptor. The current design expects a `workload.yaml` file in the source code repo. We'll come up with a spec for the workload descriptor later.

```yaml
# OpenChoreo Workload Descriptor
# This file defines how your workload exposes endpoints and connects to other services.
# It sits alongside your source code and gets converted to a Workload Custom Resource (CR).
apiVersion: openchoreo.dev/v1alpha1

# Basic metadata for the workload
metadata:
  # +required Name of the workload
  name: go-reading-list-service

# +optional Incoming connection details for the component
# Endpoints define the network interfaces that this workload exposes to other services
endpoints:
  - # +required Unique name for the endpoint
    # This name will be used when generating the managed API and as the key in the CR map
    name: reading-list-api
    # +required Numeric port value that gets exposed via the endpoint
    port: 8080
    # +required Type of traffic that the endpoint is accepting
    # Allowed values: REST, GraphQL, gRPC, TCP, UDP, HTTP, Websocket
    type: REST
    # +optional The path to the schema definition file
    # This is applicable to REST, GraphQL, and gRPC endpoint types
    # The path should be relative to the workload.yaml file location
    schemaFile: docs/openapi.yaml

connections: # Not finalized yet
  - name: book-service
    resourceRef: ...
  - name: reading-list-db 
    resourceRef: ...
```

