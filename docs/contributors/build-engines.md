# OpenChoreo Build Engines

This document describes the pluggable build engine architecture in OpenChoreo and provides a practical, high-level guide for adding new build engines.

## Overview

OpenChoreo supports multiple build engines through a pluggable architecture that allows platform teams to choose the best build technology for their needs. The system currently supports:

- Argo Workflows (default) — Kubernetes-native workflow engine
- Tekton (planned) — Cloud-native CI/CD building blocks
- Custom Engines — Your own build implementation

## Architecture

### High-Level Architecture

```
Control Plane                                  Build Plane
┌───────────────────────────────────────┐      ┌─────────────────────────────┐
│                                       │      │                             │
│  ┌──────────────────┐                 │      │  Engine-Specific Resources: │
│  │ Build Controller │                 │      │  • Namespaces               │
│  │ - Watches Build  │                 │      │  • RBAC (SA, Role, RB)      │
│  │ - Status updates │                 │      │  • Workflows/Pipelines      │
│  │ - Conditions     │                 │      │  • Templates                │
│  └─────────┬────────┘                 │      │                             │
│            │                          │      └─────────────────────────────┘
│            │ delegates                │                     ▲
│            ▼                          │                     │
│  ┌─────────────────────────────────┐  │                     │
│  │ Builder (Mediator)              │  │                     │
│  │ ┌─────────────────────────────┐ │  │                     │
│  │ │ Engine Registry             │ │  │                     │
│  │ │ ┌─────────────────────────┐ │ │  │                     │
│  │ │ │ "argo" → ArgoEngine     │ │ │  │                     │
│  │ │ │         (implemented)   │ │ │  │                     │
│  │ │ │ "tekton" → TektonEngine │ │ │  │                     │
│  │ │ │           (planned)     │ │ │  │                     │
│  │ │ │ "custom" → YourEngine   │ │ │  │                     │
│  │ │ └─────────────────────────┘ │ │  │                     │
│  │ └─────────────────────────────┘ │  │                     │
│  │                                 │  │                     │
│  │ Engine Selection:               │  │                     │
│  │ • spec.templateRef.engine       │  │                     │
│  │ • Default: "argo"               │  │                     │
│  └─────────────────────────────────┘  │                     │
│                                       │                     │
└───────────────────────────────────────┘                     │
                                                              │
                             ┌────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│ All Engines Implement Same Interface:                       │
│                                                             │
│ GetName() string                                            │
│ EnsurePrerequisites(ctx, bpClient, build) error             │
│ CreateBuild(ctx, bpClient, build) (response, error)         │
│ GetBuildStatus(ctx, bpClient, build) (status, error)        │
│ ExtractBuildArtifacts(ctx, bpClient, build) (artifacts, e)  │
└─────────────────────────────────────────────────────────────┘


Shared Utilities (Used by All Engines):
┌─────────────────────────────────────────────────────────────────────────────────┐
│ names package                          engines package                          │
│ ┌─────────────────────────────────────┐ ┌─────────────────────────────────────┐ │
│ │ • MakeImageName(build)              │ │ • EnsureResource(ctx, client, obj)   │ │
│ │ • MakeWorkflowName(build)           │ │ • MakeNamespace(build)               │ │
│ │ • MakeNamespaceName(build)          │ │ • Standard step names                │ │
│ │ • MakeWorkflowLabels(build)         │ │                                      │ │
│ └─────────────────────────────────────┘ └─────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────────┘

Build Lifecycle Flow:
1. Build CR created/updated
2. Controller calls Builder.ProcessBuild()
3. Builder selects engine based on spec.templateRef.engine
4. Builder gets build-plane client (cross-cluster)
5. Engine.EnsurePrerequisites() → Create namespace, RBAC, etc.
6. Engine.CreateBuild() → Create Workflow/PipelineRun/etc.
7. Controller periodically calls Engine.GetBuildStatus()
8. On success, Engine.ExtractBuildArtifacts() → Get image, workload CR
9. Builder updates Build status and creates Workload resource
```

### Component Structure

```
internal/controller/build/
├── builder.go                    # Builder mediator pattern (engine selection, lifecycle)
├── controller.go                 # Main build controller
├── controller_conditions.go      # Status condition management
├── engines/                      # Engine abstraction layer
│   ├── interface.go              # BuildEngine interface and shared types/utilities
│   └── argo/                     # Argo Workflows implementation (reference)
│       ├── engine.go             # Core Argo engine logic
│       └── workflow.go           # Argo workflow creation
├── names/                        # Naming utilities (used by engines)
│   ├── images.go                 # Docker image naming
│   ├── workflows.go              # Workflow/namespace naming
│   └── resources.go              # Kubernetes resource naming
```

## Engine Contract (What you must implement)

Every build engine must implement the following contract. The Builder calls these in order during the Build lifecycle. You do not need to change the controller or Builder to integrate a new engine, but you must register it (see "Register your engine").

- `GetName()`: Return a short unique identifier for the engine (e.g., "argo", "tekton").
- `EnsurePrerequisites(ctx, k8sClient, build)`: Create or verify any resources required to run builds. Typical items: namespace, service account, role, role binding, CRDs of the target engine, and any config maps/secrets. Must be idempotent.
- `CreateBuild(ctx, k8sClient, build)`: Start the build in the target engine (e.g., Workflow, PipelineRun). Must be idempotent and return an identifier and whether a new run was created.
- `GetBuildStatus(ctx, k8sClient, build)`: Return a normalized status with phase and message. Map engine-native phases to: Running, Succeeded, Failed, or Unknown.
- `ExtractBuildArtifacts(ctx, k8sClient, build)`: When the build is complete, extract artifacts such as the built image reference and the generated Workload custom resource (YAML string) from the engine’s outputs.

Shared types used across engines:
- `BuildCreationResponse`: `{ id, created }`
- `BuildStatus`: `{ phase, message }`
- `BuildArtifacts`: `{ image, workloadCR }`

## Engine Lifecycle in OpenChoreo

1. **Engine selection**
   - Builder determines the engine using `Build.spec.templateRef.engine`. If omitted, the default is `argo`.
2. **Prerequisites**
   - Builder calls `EnsurePrerequisites` to prepare namespace, RBAC, and engine-specific resources.
3. **Create or get build**
   - Builder calls `CreateBuild`. If the engine reports “already exists,” Builder treats it as a previously created run.
4. **Status tracking**
   - Builder periodically calls `GetBuildStatus` and updates `Build.status` conditions (`InProgress`, `Completed`, `Failed`).
5. **Artifact extraction**
   - When `Succeeded`, Builder calls `ExtractBuildArtifacts` and persists the image in `Build.status.imageStatus.image`.
6. **Workload creation** (optional, handled by Builder)
   - If a workload CR is present in artifacts, Builder creates the Workload resource in the Build’s namespace.

## Control Plane vs Build Plane

- The Builder runs in the control plane and talks to the build plane cluster selected by the BuildPlane resource.
- All engine methods (`EnsurePrerequisites`, `CreateBuild`, `GetBuildStatus`, `ExtractBuildArtifacts`) receive a Kubernetes client that targets the build plane. Do not assume access to control-plane resources inside the engine.
- Ensure that any CRDs and templates your engine needs are installed in the build plane cluster referenced by the BuildPlane.

## Artifact Contract (Outputs your engine should provide)

Engines should expose at least these outputs when the build succeeds:
- **Container image**: A fully qualified image reference (e.g., `registry.example.com/org/project:tag`).
- **Workload CR**: A serialized YAML string of a valid openchoreo Workload resource (optional, only if your template generates it).

Standard output names and steps:
- Prefer emitting an output parameter named `image` for the built image reference.
- Prefer emitting an output parameter named `workload-cr` containing the YAML string for the Workload resource.
- When modeling steps, use stable step/template names such as `push-step` and `workload-create-step` for clarity and consistency across engines.

Argo reference implementation exposes these as named outputs that the Builder knows how to read. If you implement a new engine, ensure you can surface equivalent outputs and map them to the generic `BuildArtifacts` fields above. The Builder will then set `status.imageStatus.image` and create the Workload if `workload-cr` is present.

## Engine Selection and Configuration

- **Per-Build selection**: Users choose the engine in the Build resource under `spec.templateRef.engine` (e.g., "argo", "tekton").
- **Default engine**: If not set, the Builder defaults to `argo`. Platform teams can change this behavior in the Builder’s selection logic if needed.

Example Build snippet:

```yaml
spec:
  templateRef:
    name: docker-build
    engine: argo   # or tekton, or your engine name
    parameters:
      - name: dockerfile-path
        value: ./Dockerfile
```

## Adding a New Build Engine

Follow this checklist to add a new engine implementation:

1. **Create a new engine package**
   - Location: `internal/controller/build/engines/<your-engine>/`
   - Use the Argo engine as a structural reference only; do not copy logic blindly.

2. **Implement the engine contract**
   - Provide `GetName`, `EnsurePrerequisites`, `CreateBuild`, `GetBuildStatus`, and `ExtractBuildArtifacts`.
   - Ensure `EnsurePrerequisites` and `CreateBuild` are idempotent (safe to call multiple times).
   - Map your engine’s phases to the generic `BuildPhase` values.
   - Provide artifacts (image, workloadCR) in `ExtractBuildArtifacts` after success.

3. **Register your engine with the Builder**
   - Update `internal/controller/build/builder.go` in the `registerBuildEngines()` function to instantiate and store your engine by `GetName()`.
   - Ensure the name returned by `GetName()` matches the user-facing value in `Build.spec.templateRef.engine`.

4. **Use shared naming and labels**
   - Use the names package to construct workflow/pipeline names, namespaces, labels, and image names/tags.
   - Keep naming deterministic and collision-free.

5. **Template and resource expectations**
   - If your engine uses cluster templates (e.g., pipeline templates), ensure they are installed in the build plane cluster.
   - Ensure any engine CRDs are present.

6. **Security and RBAC**
   - Create a least-privileged ServiceAccount, Role, and RoleBinding in the build namespace.
   - Scope permissions to the engine’s required resources only.

7. **Status and observability**
   - Provide meaningful status messages for easier troubleshooting.
   - Expose engine-native IDs in logs to correlate with external systems.

8. **Testing**
   - Unit test your engine logic.
   - Use integration tests against a real API server (envtest or a local kind cluster).
   - Validate artifact extraction and workload creation end-to-end.

## Conventions and Shared Utilities

To ensure consistency across engines:

- **Naming utilities (names package)**
  - Image naming and tagging (e.g., `project-component:default`) based on Build metadata.
  - Workflow/pipeline and namespace naming helpers.
  - Standard labels for workflows/pipelines and related resources.

- **Prerequisite helpers (engines package)**
  - Use shared helpers to create namespaces and to apply resources idempotently.

- **Step names and outputs**
  - If your engine models steps, prefer consistent names, such as `push-step` for the image push and `workload-create-step` for creating the Workload CR.
  - Expose outputs with stable names so `ExtractBuildArtifacts` can retrieve the image and workload CR reliably (`image`, `workload-cr`).

- **Ownership and cleanup**
  - Set owner references where appropriate so that resources are garbage-collected when a Build is deleted.

## Best Practices

- **What the Builder does (do not duplicate in engines)**
  - Chooses the engine and invokes its lifecycle methods.
  - Updates `status.conditions` using normalized phases.
  - Persists the built image to `status.imageStatus.image`.
  - Creates a Workload resource from `workload-cr` if provided. Engines should not create Workload resources directly.

- **Naming consistency**: Reuse naming utilities and follow Kubernetes naming rules.
- **Error handling**: Wrap errors with context and use structured logs.
- **Resource management**: Make creation idempotent; clean up on failures when possible.
- **Status reporting**: Map engine-native statuses clearly; keep messages actionable.
- **Security**: Follow least-privilege RBAC and avoid cluster-wide permissions where not required.

## Testing Your Engine

- **Unit tests**: Validate lifecycle behavior and status mapping.
- **Integration tests**: Use envtest or a local cluster to validate resource creation and status retrieval.
- **End-to-end checks**: Ensure artifacts (image, Workload CR) are produced as expected.

Useful commands:
- `go test ./internal/controller/build/...`
- `kubectl get builds -n <ns> -w`
- `kubectl get workflows -n <ns>`     # Argo
- `kubectl get pipelineruns -n <ns>`  # Tekton

## Manual Testing

1. Deploy OpenChoreo in a development cluster.
2. Apply a Build CR that selects your engine via `spec.templateRef.engine`.
3. Watch the Build resource for status changes and conditions.
4. Inspect engine-specific resources (e.g., workflows or pipelineruns) in the build namespace.
5. After success, verify the image reference in `Build.status.imageStatus.image` and that a Workload CR is created if applicable.

## Troubleshooting

Common issues and hints:
- **Import cycles**: Keep engine code isolated; use shared utilities from names and engines packages.
- **Resource conflicts**: Ensure unique names and correct owner references.
- **Status sync**: Make sure `GetBuildStatus` reflects the actual engine state and terminal phases are recognized.
- **Missing CRDs**: Verify your engine’s CRDs are installed in the build plane cluster.

Debugging tips:
- Check controller logs in the control-plane namespace.
- Describe engine-native resources (e.g., Workflow, PipelineRun) for detailed status.
- Confirm RBAC permissions for the ServiceAccount used by the engine.

## Contributing

When adding a new build engine:
- Follow this guide and implement all required contract methods.
- Add comprehensive tests (unit, integration, and, if applicable, E2E).
- Update documentation including this guide and any engine-specific docs.
- Submit a PR describing the engine, requirements, and limitations.
- Consider backward compatibility and migration paths when changing defaults.

—

This document is maintained by the OpenChoreo core team. Last updated: 2025-08-20
