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


