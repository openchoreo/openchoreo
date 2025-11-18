# Platform Abstractions

> **Relevant source files**
> * [PROJECT](https://github.com/openchoreo/openchoreo/blob/a577e969/PROJECT)
> * [README.md](https://github.com/openchoreo/openchoreo/blob/a577e969/README.md)
> * [cmd/main.go](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go)
> * [config/crd/kustomization.yaml](https://github.com/openchoreo/openchoreo/blob/a577e969/config/crd/kustomization.yaml)
> * [config/rbac/kustomization.yaml](https://github.com/openchoreo/openchoreo/blob/a577e969/config/rbac/kustomization.yaml)
> * [config/rbac/role.yaml](https://github.com/openchoreo/openchoreo/blob/a577e969/config/rbac/role.yaml)
> * [config/samples/kustomization.yaml](https://github.com/openchoreo/openchoreo/blob/a577e969/config/samples/kustomization.yaml)
> * [docs/images/openchoreo-cell-runtime-view.png](https://github.com/openchoreo/openchoreo/blob/a577e969/docs/images/openchoreo-cell-runtime-view.png)
> * [docs/images/openchoreo-ddd-to-cell-mapping.png](https://github.com/openchoreo/openchoreo/blob/a577e969/docs/images/openchoreo-ddd-to-cell-mapping.png)
> * [docs/images/openchoreo-development-abstractions.png](https://github.com/openchoreo/openchoreo/blob/a577e969/docs/images/openchoreo-development-abstractions.png)
> * [docs/images/openchoreo-overall-architecture.png](https://github.com/openchoreo/openchoreo/blob/a577e969/docs/images/openchoreo-overall-architecture.png)
> * [docs/images/openchoreo-platform-abstractions.png](https://github.com/openchoreo/openchoreo/blob/a577e969/docs/images/openchoreo-platform-abstractions.png)

## Purpose and Scope

This page explains OpenChoreo's three-layer abstraction model that transforms Kubernetes primitives into a developer-friendly Internal Developer Platform. These abstractions separate concerns between platform engineers (who define infrastructure), application developers (who build services), and runtime operations (how components execute).

For details on the Cell runtime model and traffic patterns, see [Cell Runtime Model](/openchoreo/openchoreo/2.2-cell-runtime-model). For information on how controllers manage these abstractions, see [Controller Manager](/openchoreo/openchoreo/2.4-controller-manager).

## Overview of the Three Layers

OpenChoreo organizes abstractions into three distinct layers:

| Layer | Managed By | Purpose | Key Resources |
| --- | --- | --- | --- |
| **Platform** | Platform Engineers | Define infrastructure topology, deployment environments, and promotion rules | Organization, DataPlane, BuildPlane, Environment, DeploymentPipeline |
| **Development** | Application Developers | Define application architecture, components, and their interfaces | Project, Component, Endpoint, Connection |
| **Runtime** | System (automated) | Execution model that enforces boundaries and observability | Cell (Project instance) |

```mermaid
flowchart TD

Org["Organization"]
DP["DataPlane"]
BP["BuildPlane"]
Env["Environment"]
DPipe["DeploymentPipeline"]
Proj["Project"]
Comp["Component"]
EP["Endpoint"]
Conn["Connection"]
Cell["Cell (Project Instance)"]
NorthIngress["Northbound Ingress<br>(Public)"]
WestIngress["Westbound Ingress<br>(Organization)"]
SouthEgress["Southbound Egress<br>(Internet)"]
EastEgress["Eastbound Egress<br>(Internal)"]

Proj --> Env
Proj --> Cell
Comp --> Cell
EP --> NorthIngress
EP --> WestIngress
Conn --> SouthEgress
Conn --> EastEgress

subgraph subGraph2 ["Runtime Layer - Execution Model"]
    Cell
    NorthIngress
    WestIngress
    SouthEgress
    EastEgress
end

subgraph subGraph1 ["Development Layer - Application Model"]
    Proj
    Comp
    EP
    Conn
end

subgraph subGraph0 ["Platform Layer - Infrastructure Topology"]
    Org
    DP
    BP
    Env
    DPipe
    Org --> DP
    Org --> BP
    Org --> Env
    Org --> DPipe
end
```

**Sources:** [README.md L21-L88](https://github.com/openchoreo/openchoreo/blob/a577e969/README.md#L21-L88)

## Platform Abstractions

Platform abstractions enable platform engineers to define the infrastructure topology and operational policies. These are **cluster-scoped or organization-scoped** resources that establish the foundation for application deployment.

### Organization

The `Organization` is a cluster-scoped Custom Resource representing a logical grouping of users and resources, typically aligned to a company, business unit, or team.

**CRD Definition:** `openchoreo.dev_organizations.yaml`

**Controller:** `organization.Reconciler`

**Key Characteristics:**

* Cluster-scoped (not namespaced)
* Acts as the root of the resource hierarchy
* Contains Projects, Environments, DataPlanes, and BuildPlanes
* Defines organizational boundaries for multi-tenancy

```mermaid
flowchart TD

Org["Organization<br>(Cluster-scoped)"]
Proj1["Project A<br>(Namespace: org-name)"]
Proj2["Project B<br>(Namespace: org-name)"]
EnvDev["Environment: dev<br>(Namespace: org-name)"]
EnvProd["Environment: prod<br>(Namespace: org-name)"]
DP["DataPlane<br>(Namespace: org-name)"]
BP["BuildPlane<br>(Namespace: org-name)"]

Org --> Proj1
Org --> Proj2
Org --> EnvDev
Org --> EnvProd
Org --> DP
Org --> BP
```

**Sources:** [cmd/main.go L179-L185](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L179-L185)

 [config/crd/kustomization.yaml L5](https://github.com/openchoreo/openchoreo/blob/a577e969/config/crd/kustomization.yaml#L5-L5)

 [README.md L32-L33](https://github.com/openchoreo/openchoreo/blob/a577e969/README.md#L32-L33)

### DataPlane

The `DataPlane` resource represents a Kubernetes cluster that hosts application workloads. It defines connection details for the controller to provision resources in the target cluster.

**CRD Definition:** `openchoreo.dev_dataplanes.yaml`

**Controller:** `dataplane.Reconciler`

**Namespace:** Organization name (e.g., `org-name`)

**Purpose:**

* References a Kubernetes cluster where applications run
* Stores connection credentials (kubeconfig or service account)
* Enables multi-cluster deployments

**Status Fields:**

* Connectivity status
* Available resources
* Health checks

```mermaid
flowchart TD

ControlPlane["Control Plane Cluster<br>(OpenChoreo Controllers)"]
DataPlane1["DataPlane: us-west<br>spec.kubeconfig"]
DataPlane2["DataPlane: us-east<br>spec.kubeconfig"]
K8sCluster1["Kubernetes Cluster<br>(us-west)"]
K8sCluster2["Kubernetes Cluster<br>(us-east)"]
Apps1["Application Workloads<br>Deployments, Services"]
Apps2["Application Workloads<br>Deployments, Services"]

ControlPlane --> DataPlane1
ControlPlane --> DataPlane2
DataPlane1 --> K8sCluster1
DataPlane2 --> K8sCluster2
K8sCluster1 --> Apps1
K8sCluster2 --> Apps2
```

**Sources:** [cmd/main.go L200-L206](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L200-L206)

 [config/crd/kustomization.yaml L8](https://github.com/openchoreo/openchoreo/blob/a577e969/config/crd/kustomization.yaml#L8-L8)

 [README.md L34-L35](https://github.com/openchoreo/openchoreo/blob/a577e969/README.md#L34-L35)

### BuildPlane

The `BuildPlane` resource represents a Kubernetes cluster configured with Argo Workflows for executing builds. This is where source code is compiled into container images.

**CRD Definition:** `openchoreo.dev_buildplanes.yaml`

**Controller:** `buildplane.BuildPlaneReconciler`

**Namespace:** Organization name

**Purpose:**

* References a Kubernetes cluster running Argo Workflows
* Defines build execution environment
* Separates build workloads from application workloads

```mermaid
flowchart TD

BP["BuildPlane CR<br>spec.kubeconfig"]
ArgoCluster["Kubernetes Cluster<br>(Argo Workflows)"]
WFTemplates["ClusterWorkflowTemplates<br>ballerina-buildpack<br>react<br>docker"]
Workflows["Workflow Instances<br>(Build Jobs)"]

BP --> ArgoCluster
ArgoCluster --> WFTemplates
ArgoCluster --> Workflows
```

**Sources:** [cmd/main.go L374-L380](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L374-L380)

 [config/crd/kustomization.yaml L32](https://github.com/openchoreo/openchoreo/blob/a577e969/config/crd/kustomization.yaml#L32-L32)

 [README.md L34-L35](https://github.com/openchoreo/openchoreo/blob/a577e969/README.md#L34-L35)

### Environment

The `Environment` resource represents a runtime context (e.g., dev, test, staging, prod) where workloads are deployed and executed.

**CRD Definition:** `openchoreo.dev_environments.yaml`

**Controller:** `environment.Reconciler`

**Namespace:** Organization name

**Key Attributes:**

* References a `DataPlane` for workload placement
* Defines environment-specific configuration
* Used for deployment targeting and promotion

**Namespace Mapping:**
Projects deployed to an Environment create a namespace in the DataPlane cluster following the pattern: `{project-name}-{environment-name}`

```mermaid
flowchart TD

Env1["Environment: dev<br>spec.dataPlaneRef"]
Env2["Environment: prod<br>spec.dataPlaneRef"]
DP["DataPlane: us-west"]
NS1["Namespace: project-a-dev"]
NS2["Namespace: project-a-prod"]
Workloads1["Deployments<br>Services<br>ConfigMaps"]
Workloads2["Deployments<br>Services<br>ConfigMaps"]

Env1 --> DP
Env2 --> DP
DP --> NS1
DP --> NS2
NS1 --> Workloads1
NS2 --> Workloads2
```

**Sources:** [cmd/main.go L193-L199](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L193-L199)

 [config/crd/kustomization.yaml L7](https://github.com/openchoreo/openchoreo/blob/a577e969/config/crd/kustomization.yaml#L7-L7)

 [README.md L36-L37](https://github.com/openchoreo/openchoreo/blob/a577e969/README.md#L36-L37)

### DeploymentPipeline

The `DeploymentPipeline` resource defines the process governing how workloads are promoted across environments.

**CRD Definition:** `openchoreo.dev_deploymentpipelines.yaml`

**Controller:** `deploymentpipeline.Reconciler`

**Namespace:** Organization name

**Purpose:**

* Defines ordered sequence of Environments
* Specifies promotion rules and approval gates
* Enforces deployment topology (e.g., dev → test → staging → prod)

**Key Concepts:**

* **DeploymentTracks:** Ordered list of environment stages
* **Promotion Rules:** Conditions for moving between stages
* **Validation:** Enforced during component promotion API calls

```mermaid
flowchart TD

DPipe["DeploymentPipeline<br>my-pipeline"]
Track1["DeploymentTrack: Stage 1<br>environmentRef: dev"]
Track2["DeploymentTrack: Stage 2<br>environmentRef: staging"]
Track3["DeploymentTrack: Stage 3<br>environmentRef: prod"]

DPipe --> Track1
DPipe --> Track2
DPipe --> Track3
Track1 --> Track2
Track2 --> Track3
```

**Sources:** [cmd/main.go L207-L213](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L207-L213)

 [config/crd/kustomization.yaml L9](https://github.com/openchoreo/openchoreo/blob/a577e969/config/crd/kustomization.yaml#L9-L9)

 [README.md L38-L39](https://github.com/openchoreo/openchoreo/blob/a577e969/README.md#L38-L39)

## Development Abstractions

Development abstractions enable application developers to define their services and APIs without dealing with Kubernetes primitives. These resources describe **application structure** and **intent**.

### Project

The `Project` represents a cloud-native application composed of multiple components. It serves as the unit of isolation and maps to a bounded context in Domain-Driven Design.

**CRD Definition:** `openchoreo.dev_projects.yaml`

**Controller:** `project.Reconciler`

**Namespace:** Organization name

**Key Characteristics:**

* Groups related Components
* Maps to a set of Namespaces (one per Environment) in DataPlanes
* Instantiated as a **Cell** at runtime
* References a `DeploymentPipeline` for promotion rules

**Kubernetes Mapping:**

```yaml
Project: "my-app"
Environment: "dev"
→ Namespace: "my-app-dev" (in DataPlane cluster)

Project: "my-app"
Environment: "prod"
→ Namespace: "my-app-prod" (in DataPlane cluster)
```

```mermaid
flowchart TD

Proj["Project: online-store<br>namespace: acme-org"]
Comp1["Component: frontend<br>type: WebApplication"]
Comp2["Component: api<br>type: API"]
Comp3["Component: worker<br>type: Service"]
Comp4["Component: cron-job<br>type: ScheduledTask"]
EnvDev["Environment: dev<br>→ Namespace: online-store-dev"]
EnvProd["Environment: prod<br>→ Namespace: online-store-prod"]

Proj --> Comp1
Proj --> Comp2
Proj --> Comp3
Proj --> Comp4
Proj --> EnvDev
Proj --> EnvProd
```

**Sources:** [cmd/main.go L186-L192](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L186-L192)

 [config/crd/kustomization.yaml L6](https://github.com/openchoreo/openchoreo/blob/a577e969/config/crd/kustomization.yaml#L6-L6)

 [README.md L51-L53](https://github.com/openchoreo/openchoreo/blob/a577e969/README.md#L51-L53)

### Component

The `Component` is a deployable unit within a Project. OpenChoreo supports specialized component types through a **Class-Instance-Binding** pattern.

**Base CRD:** `openchoreo.dev_components.yaml`

**Controller:** `component.Reconciler`

**Namespace:** Project namespace (e.g., `project-name` within the organization namespace)

#### Component Types

OpenChoreo provides four specialized component types:

| Type | Purpose | Kubernetes Mapping | Controller |
| --- | --- | --- | --- |
| **API** | RESTful API service | Deployment + Service | `api.Reconciler` |
| **Service** | Backend service | Deployment + Service | `service.Reconciler` |
| **WebApplication** | Frontend web app | Deployment + Service | `webapplication.Reconciler` |
| **ScheduledTask** | Cron job | CronJob | `scheduledtask.Reconciler` |

Each type has three associated resources following the Class-Instance-Binding pattern:

```mermaid
flowchart TD

Class["APIClass / ServiceClass /<br>WebApplicationClass /<br>ScheduledTaskClass"]
Instance["API / Service /<br>WebApplication /<br>ScheduledTask"]
Binding["APIBinding / ServiceBinding /<br>WebApplicationBinding /<br>ScheduledTaskBinding"]

Class --> Instance
Instance --> Binding
```

**Example - API Component:**

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: API
metadata:
  name: orders-api
  namespace: online-store
spec:
  # Source or image reference
  # Build configuration
  # Resource requirements
---
apiVersion: openchoreo.dev/v1alpha1
kind: APIBinding
metadata:
  name: orders-api-dev
  namespace: online-store
spec:
  apiRef: orders-api
  environmentRef: dev
  state: active  # active, suspend, undeploy
```

**Sources:** [cmd/main.go L251-L357](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L251-L357)

 [config/crd/kustomization.yaml L10-L29](https://github.com/openchoreo/openchoreo/blob/a577e969/config/crd/kustomization.yaml#L10-L29)

 [README.md L54-L56](https://github.com/openchoreo/openchoreo/blob/a577e969/README.md#L54-L56)

### Endpoint

The `Endpoint` resource represents a network-accessible interface exposed by a Component. It defines routing rules, protocols, and **visibility scopes**.

**CRD Definition:** `openchoreo.dev_endpoints.yaml`

**Controller:** `endpoint.Reconciler`

**Namespace:** Project namespace

**Visibility Scopes:**

| Visibility | Ingress Path | Access Scope | Gateway |
| --- | --- | --- | --- |
| **public** | Northbound | Internet (external users) | `gateway-external` |
| **organization** | Westbound | Organization-internal only | `gateway-internal` |
| **project** | Intra-Cell | Same project components | Service mesh |

**Kubernetes Mapping:**

* `HTTPRoute` (Gateway API) for routing
* `SecurityPolicy` (Envoy Gateway) for authentication/authorization
* `HTTPRouteFilter` (Envoy Gateway) for transformations
* `CiliumNetworkPolicy` for zero-trust enforcement

```mermaid
flowchart TD

EP["Endpoint: orders-api-public<br>spec.visibility: public<br>spec.path: /api/orders"]
HTTPRoute["HTTPRoute<br>gateway: gateway-external<br>route: /api/orders → orders-api:8080"]
SecurityPolicy["SecurityPolicy<br>OAuth2 enforcement"]
NetworkPolicy["CiliumNetworkPolicy<br>allow: external → orders-api"]
Gateway["Gateway: gateway-external<br>(Northbound Ingress)"]
Service["Service: orders-api"]

EP --> HTTPRoute
EP --> SecurityPolicy
EP --> NetworkPolicy
Gateway --> HTTPRoute
HTTPRoute --> Service
```

**Sources:** [cmd/main.go L235-L241](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L235-L241)

 [config/crd/kustomization.yaml L14](https://github.com/openchoreo/openchoreo/blob/a577e969/config/crd/kustomization.yaml#L14-L14)

 [README.md L57-L59](https://github.com/openchoreo/openchoreo/blob/a577e969/README.md#L57-L59)

### Connection

The `Connection` represents an outbound service dependency defined by a Component, targeting either other components or external systems.

**Purpose:**

* Declares explicit dependencies
* Enables network policy generation
* Routes through appropriate egress gateways

**Kubernetes Mapping:**

* `CiliumNetworkPolicy` (egress rules)
* Envoy egress gateway routing

**Connection Types:**

| Target | Egress Path | Purpose |
| --- | --- | --- |
| **External Service** | Southbound | Internet-bound traffic (APIs, databases) |
| **Other Project** | Eastbound | Cross-project communication |
| **Same Project** | Intra-Cell | Component-to-component within project |

```mermaid
flowchart TD

Comp["Component: orders-api"]
Conn1["Connection: payment-service<br>target: external<br>egress: southbound"]
Conn2["Connection: inventory-api<br>target: project<br>egress: eastbound"]
SouthGW["Southbound Egress Gateway<br>(Internet)"]
EastGW["Eastbound Egress Gateway<br>(Internal)"]
External["External Payment API<br>(stripe.com)"]
OtherProject["inventory-api<br>(another Cell)"]

Comp --> Conn1
Comp --> Conn2
Conn1 --> SouthGW
Conn2 --> EastGW
SouthGW --> External
EastGW --> OtherProject
```

**Sources:** [README.md L60-L62](https://github.com/openchoreo/openchoreo/blob/a577e969/README.md#L60-L62)

## Runtime Abstraction: Cell

At runtime, OpenChoreo instantiates each Project as a **Cell** – a secure, isolated, and observable unit that enforces domain boundaries through infrastructure.

### Cell Characteristics

**Key Properties:**

* One Cell per Project per Environment
* Components within a Cell can communicate without interception
* All ingress/egress traffic passes through gateways
* Zero-trust enforcement via Cilium and eBPF
* mTLS encryption for all traffic
* Built-in observability (logs, metrics, traces)

**Cell Boundaries:**
Each Cell has four directional traffic paths:

| Path | Direction | Purpose | Gateway | Network Policy |
| --- | --- | --- | --- | --- |
| **Northbound** | Ingress | Public internet → Cell | `gateway-external` | Cilium (public endpoints) |
| **Westbound** | Ingress | Organization → Cell | `gateway-internal` | Cilium (org endpoints) |
| **Southbound** | Egress | Cell → Internet | Egress gateway | Cilium (external connections) |
| **Eastbound** | Egress | Cell → Other Cells | Egress gateway | Cilium (internal connections) |

```mermaid
flowchart TD

Frontend["Component: frontend<br>(WebApplication)"]
API["Component: orders-api<br>(API)"]
Worker["Component: worker<br>(Service)"]
North["Northbound Ingress<br>Endpoint: frontend<br>visibility: public"]
West["Westbound Ingress<br>Endpoint: orders-api<br>visibility: organization"]
South["Southbound Egress<br>Connection: stripe-api"]
East["Eastbound Egress<br>Connection: inventory-api"]
Internet["Public Internet<br>(End Users)"]
OrgNetwork["Organization Network<br>(Internal Services)"]
External["External APIs<br>(Stripe, AWS)"]
OtherCell["Other Cell<br>(Inventory Service)"]
Cilium["Cilium + eBPF<br>Network Policies"]
Envoy["Envoy Gateways<br>API Management"]
mTLS["mTLS Encryption"]
Obs["Observability<br>(Fluentbit, OpenSearch)"]

Internet --> North
North --> Frontend
OrgNetwork --> West
West --> API
Worker --> South
South --> External
API --> East
East --> OtherCell
Cilium --> North
Cilium --> West
Cilium --> South
Cilium --> East
Envoy --> North
Envoy --> West
Envoy --> South
Envoy --> East
mTLS --> Frontend
mTLS --> API
mTLS --> Worker
Obs --> North
Obs --> West
Obs --> South
Obs --> East

subgraph Cell ["Cell (Project: online-store, Environment: prod)"]
    Frontend
    API
    Worker
    Frontend --> API
    API --> Worker
end
```

**Sources:** [README.md L72-L88](https://github.com/openchoreo/openchoreo/blob/a577e969/README.md#L72-L88)

## Abstraction Hierarchy and Relationships

The following diagram illustrates the complete hierarchy from Platform layer through Development layer to Runtime execution:

```mermaid
flowchart TD

Org["Organization<br>apiVersion: openchoreo.dev/v1alpha1<br>kind: Organization"]
DP["DataPlane<br>namespace: {org}<br>kind: DataPlane"]
BP["BuildPlane<br>namespace: {org}<br>kind: BuildPlane"]
Env1["Environment: dev<br>namespace: {org}<br>kind: Environment"]
Env2["Environment: prod<br>namespace: {org}<br>kind: Environment"]
DPipe["DeploymentPipeline<br>namespace: {org}<br>kind: DeploymentPipeline"]
Proj["Project<br>namespace: {org}<br>kind: Project"]
Comp1["Component: API<br>namespace: {project}<br>kind: API"]
Comp2["Component: Service<br>namespace: {project}<br>kind: Service"]
Comp3["Component: WebApp<br>namespace: {project}<br>kind: WebApplication"]
EP1["Endpoint<br>namespace: {project}<br>kind: Endpoint"]
Conn1["Connection<br>(declared in Component spec)"]
Binding1["APIBinding<br>namespace: {project}<br>kind: APIBinding"]
Cell["Cell = Project Instance<br>Namespace: {project}-{env}"]
Deploy1["Deployment<br>apiVersion: apps/v1"]
Svc1["Service<br>apiVersion: v1"]
HTTPRoute1["HTTPRoute<br>apiVersion: gateway.networking.k8s.io/v1"]
NetworkPolicy1["CiliumNetworkPolicy<br>apiVersion: cilium.io/v2"]

Proj --> DPipe
Proj --> Env1
Proj --> Env2
Binding1 --> Env1
Proj --> Cell
Env1 --> Cell
DP --> Cell
EP1 --> HTTPRoute1
Conn1 --> NetworkPolicy1

subgraph Runtime ["Runtime Execution (DataPlane Cluster)"]
    Cell
    Deploy1
    Svc1
    HTTPRoute1
    NetworkPolicy1
    Cell --> Deploy1
    Cell --> Svc1
end

subgraph Development ["Development Abstractions (Org/Project Scoped)"]
    Proj
    Comp1
    Comp2
    Comp3
    EP1
    Conn1
    Binding1
    Proj --> Comp1
    Proj --> Comp2
    Proj --> Comp3
    Comp1 --> EP1
    Comp1 --> Conn1
    Comp1 --> Binding1
end

subgraph Platform ["Platform Abstractions (Cluster/Org Scoped)"]
    Org
    DP
    BP
    Env1
    Env2
    DPipe
    Org --> DP
    Org --> BP
    Org --> Env1
    Org --> Env2
    Org --> DPipe
end
```

**Sources:** [README.md L21-L119](https://github.com/openchoreo/openchoreo/blob/a577e969/README.md#L21-L119)

 [cmd/main.go L62-L409](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L62-L409)

## Controller Registration and Scheme Setup

All abstraction resources are registered with the controller manager at startup and added to the Kubernetes scheme.

**Scheme Setup:**

```go
// cmd/main.go:66-76
func init() {
    utilruntime.Must(clientgoscheme.AddToScheme(scheme))
    utilruntime.Must(ciliumv2.AddToScheme(scheme))
    utilruntime.Must(openchoreov1alpha1.AddToScheme(scheme))
    utilruntime.Must(gwapiv1.Install(scheme))
    utilruntime.Must(egv1a1.AddToScheme(scheme))
    utilruntime.Must(argo.AddToScheme(scheme))
    utilruntime.Must(csisecretv1.Install(scheme))
}
```

**Controller Registration:**
The main function registers reconcilers for each abstraction:

| Abstraction | Controller Type | Registration Code |
| --- | --- | --- |
| Organization | `organization.Reconciler` | [cmd/main.go L179-L185](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L179-L185) |
| Project | `project.Reconciler` | [cmd/main.go L186-L192](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L186-L192) |
| Environment | `environment.Reconciler` | [cmd/main.go L193-L199](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L193-L199) |
| DataPlane | `dataplane.Reconciler` | [cmd/main.go L200-L206](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L200-L206) |
| BuildPlane | `buildplane.BuildPlaneReconciler` | [cmd/main.go L374-L380](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L374-L380) |
| DeploymentPipeline | `deploymentpipeline.Reconciler` | [cmd/main.go L207-L213](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L207-L213) |
| Component | `component.Reconciler` | [cmd/main.go L251-L257](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L251-L257) |
| API | `api.Reconciler` | [cmd/main.go L268-L274](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L268-L274) |
| Service | `service.Reconciler` | [cmd/main.go L291-L297](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L291-L297) |
| WebApplication | `webapplication.Reconciler` | [cmd/main.go L314-L320](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L314-L320) |
| ScheduledTask | `scheduledtask.Reconciler` | [cmd/main.go L337-L343](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L337-L343) |
| Endpoint | `endpoint.Reconciler` | [cmd/main.go L235-L241](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L235-L241) |

**Sources:** [cmd/main.go L66-L409](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L66-L409)

## RBAC Permissions

The controller manager requires permissions to manage all abstraction resources plus the underlying Kubernetes resources they provision.

**Key Permissions:**

* Full CRUD on all OpenChoreo CRDs (`openchoreo.dev/*`)
* Manage core resources: `namespaces`, `services`, `configmaps`
* Manage workload resources: `deployments`, `cronjobs`
* Manage network resources: `ciliumnetworkpolicies`, `httproutes`, `securitypolicies`

**Complete Permission Matrix:** [config/rbac/role.yaml L1-L227](https://github.com/openchoreo/openchoreo/blob/a577e969/config/rbac/role.yaml#L1-L227)

**Sources:** [config/rbac/role.yaml L1-L227](https://github.com/openchoreo/openchoreo/blob/a577e969/config/rbac/role.yaml#L1-L227)

 [config/rbac/kustomization.yaml L1-L101](https://github.com/openchoreo/openchoreo/blob/a577e969/config/rbac/kustomization.yaml#L1-L101)

## Summary

OpenChoreo's three-layer abstraction model provides:

1. **Platform Abstractions**: Infrastructure topology defined by platform engineers (Organization, DataPlane, Environment)
2. **Development Abstractions**: Application structure defined by developers (Project, Component, Endpoint, Connection)
3. **Runtime Abstractions**: Secure execution model enforced by the system (Cell)

This separation enables:

* Platform teams to define standards and enforce policies
* Development teams to focus on business logic without Kubernetes complexity
* Runtime system to enforce security, observability, and isolation automatically

The abstractions are implemented as Kubernetes Custom Resources with dedicated controllers that translate high-level intent into low-level Kubernetes primitives.

**Sources:** [README.md L1-L175](https://github.com/openchoreo/openchoreo/blob/a577e969/README.md#L1-L175)

 [cmd/main.go L1-L410](https://github.com/openchoreo/openchoreo/blob/a577e969/cmd/main.go#L1-L410)

 [PROJECT L1-L202](https://github.com/openchoreo/openchoreo/blob/a577e969/PROJECT#L1-L202)