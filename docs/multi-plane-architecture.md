# OpenChoreo Architecture: Control, Data, and Build Planes

OpenChoreo follows a **multi-plane architecture** to clearly separate concerns in your internal developer platform. Each plane represents a distinct role and responsibility within the platform engineering setup and is modeled using custom resources (CRs) in the Control Plane.

These planes are usually deployed in separate Kubernetes clusters, although colocated setups are also supported for lightweight environments like local development.

---

## Control Plane

The **Control Plane** is the heart of the OpenChoreo platform. It manages all developer intent and orchestrates platform-wide activities such as deployment, configuration, and lifecycle management.

### Responsibilities

- Hosts key components:
    - OpenChoreo Controller Manager
    - API Server
    - GitOps tools (e.g., Argo CD)
- Manages high-level abstractions like:
    - `DataPlane`, `BuildPlane`, `Organization`, `Environment`, `Deployment Pipeline`. `Project`, `Deployment Track`, `Component`, `Build`, and `Deployment` CRs
- Translates developer intent into actionable configurations
- Maintains the global state of the platform
- Coordinates operations across data and build planes

### Deployment

- Typically deployed in a dedicated Kubernetes cluster
- Can be colocated with a Data Plane or Build Plane for smaller or local setups

---

## Data Plane

The **Data Plane** is where applications are actually deployed and run. It handles the execution of developer-defined workloads such as: Microservices, APIs, and Scheduled Jobs.

Each data plane is modeled as a `DataPlane` custom resource in the Control Plane.

### Multi-Region Support

You can register multiple Data Planes to represent different environments or regions, such as:

- `staging` (e.g., us-west-2)
- `production` (e.g., eu-central-1)

This enables teams to run and manage workloads independently across geographic boundaries.

---

## Build Plane

The **Build Plane** is dedicated to executing continuous integration (CI) workflows. It focuses on tasks such as:

- Building container images
- Running automated tests
- Publishing deployable artifacts

Powered by **Argo Workflows**, the Build Plane runs in its own Kubernetes cluster, isolated from runtime environments.

### Key Benefits

- Better **resource isolation**: build jobs don’t affect application performance
- Easier **security hardening**: build clusters can be locked down separately
- Greater **scalability**: build capacity can be scaled independently

Each Build Plane is (or will be) registered using a `BuildPlane` custom resource, which provides the Control Plane with necessary credentials and connection info for dispatching build jobs.

> [!TIP]
> While the Build Plane is usually deployed independently, it can also be colocated with a Data Plane when resource sharing is acceptable (e.g., during development or in small-scale environments).

> [!NOTE]
> `BuildPlane` CRD support is on the roadmap and will be available in a future release. Currently, the Build Plane runs within a **Data Plane** cluster.

---

## Summary

| Plane        | Purpose                                         | Backed By                              |
|--------------|--------------------------------------------------|----------------------------------------|
| Control Plane | Manages intent, state, and orchestration         | Kubernetes + CRDs  |
| Data Plane   | Runs application workloads                        | Kubernetes + Cilium + More             |
| Build Plane  | Executes CI pipelines and builds artifacts        | Kubernetes + Argo Workflows            |

OpenChoreo’s plane-based architecture allows you to modularize and scale your internal developer platform with clarity and control.
