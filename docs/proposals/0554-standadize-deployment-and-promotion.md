# Standardize Deployment and Promotion

**Authors**:
@yashodgayashan
@vajiraprabuddhaka

**Reviewers**:

**Created Date**:
2025-10-27

**Status**:
Pending

**Related Issues/PRs**:
[Discussion #554 – openchoreo/openchoreo](https://github.com/openchoreo/openchoreo/discussions/554)


## Summary

This proposal introduces a standardized deployment and promotion mechanism for OpenChoreo that works seamlessly with both UI/CLI and GitOps workflows. The design uses **Release** and **ReleasePin** CRs to provide an immutable, auditable way to promote component definitions across environments while maintaining environment-specific configuration separation.

The Release CR supports **two flexible approaches** for storing ComponentTypeDefinitions, Addons, and Components:
- **Git references** (lightweight Release, slower reconciliation): Release ~2-5 KB + ComponentEnvSnapshot ~50-200 KB = **~52-205 KB total per environment**. Recommended for GitOps workflows.
- **Embedded content** (larger Release, faster reconciliation): Release ~50-200 KB + ComponentEnvSnapshot ~50-200 KB = **~100-400 KB total per environment**. Recommended for UI/CLI workflows.

**Note**: Both approaches generate ComponentEnvSnapshot during reconciliation with the same size (~50-200 KB). The difference is in Release size (2-5 KB vs 50-200 KB) and reconciliation performance (git fetches vs direct access).

Users can choose the approach that best fits their workflow, or even mix both within the same Release, providing maximum flexibility while addressing etcd storage and performance concerns through garbage collection, git references, and planned compression support.

## Motivation

With the introduction of ComponentTypeDefinitions, Addons, and Workloads ([Proposal #537](0537-introduce-component-type-definitions.md)), OpenChoreo needs a robust deployment and promotion workflow that:

1. **Supports both UI/CLI and GitOps**: Users should be able to deploy and promote components through either the OpenChoreo UI/CLI or through GitOps workflows (ArgoCD, Flux) using the same underlying mechanism.

2. **Maintains environment isolation**: Promotable content (ComponentTypeDefinitions, Addons, Components, Workloads) should be separated from environment-specific configuration (EnvSettings).

3. **Provides auditability**: Every promotion should be traceable with clear history of what was deployed when and where.

4. **Avoids etcd bloat**: Large embedded objects created during frequent deployments and promotions can cause etcd storage issues.

5. **Enables GitOps-friendly workflows**: Promotions should result in clean Git diffs without circular dependencies or controller-generated commits that conflict with user changes.

### Problems with Initial Approach (Auto-Generated ComponentEnvSnapshot)

An initial approach where ComponentEnvSnapshot was **auto-generated on source changes** (ComponentTypeDefinition, Component, Addon, or Workload updates) revealed three critical issues:

1. **GitOps Circular Dependencies**: Auto-generating snapshots on source changes creates circular update loops. When addons or ComponentTypeDefinitions update, controllers auto-generate new snapshots and commit them to git, which triggers reconciliation, which may rollback snapshots before dependent resources sync, causing repeated commits and sync conflicts. **Resolution**: In this proposal, ComponentEnvSnapshot is generated on-demand by the ReleasePin controller during reconciliation, not automatically on source changes.

2. **etcd Storage Bloat**: Large snapshot objects embedding full copies of CTD, Addon, Workload, and EnvSettings accumulate during frequent promotions across multiple environments, causing storage pressure. **Note**: This issue can still occur when using embedded content in the Release CR, but is mitigated through:
   - **Garbage collection** of unused/old Releases (configurable retention policies)
   - **Git references as an alternative** for users who prefer lightweight objects (GitOps workflows)

3. **Poor Developer Experience**: Exposing ComponentEnvSnapshot as the primary user-facing resource for promotions reveals internal implementation details rather than expressing the business intent of "deploy this release to this environment." **Resolution**: Release and ReleasePin become the user-facing abstractions with intuitive commands:
   - **UI**: "Deploy" button or "Promote" button with environment selection
   - **CLI**: `choreo deploy --environment <environment>` or `choreo promote --from <environment> --to <environment>`
   - ComponentEnvSnapshot remains an internal implementation detail


## Goals

- Provide a unified deployment and promotion model that works identically for UI/CLI and GitOps workflows
- Maintain separation between promotable content and environment-specific configuration
- Support immutable, auditable promotion history
- **Manage etcd storage overhead** through:
  - Supporting lightweight git references for GitOps workflows
  - Implementing garbage collection of unused releases
- Produce clean, human-readable Git diffs for GitOps workflows
- Allow garbage collection of old releases to prevent unbounded growth
- Provide flexibility to choose between git references and embedded content based on workflow needs


## Non-Goals

- Automated promotion based on health checks or test results (can be built on top of this mechanism later)
- Blue-green or canary deployment strategies (these are orthogonal concerns)
- Rollback mechanisms (covered separately; this provides the foundation)


## Design

### Overview

The design introduces two primary user-facing CRDs and uses an existing internal resource:

1. **Release** (new, user-facing): An immutable, lightweight promotable unit that pins versions of ComponentTypeDefinitions, Addons, Components, and Workloads (similar to a lockfile)
2. **ReleasePin** (new, user-facing): A per-environment resource that references which Release is deployed to that environment
3. **ComponentEnvSnapshot** (existing, internal): Generated by the ReleasePin controller during reconciliation to cache resolved definitions for a specific environment. This is NOT auto-generated on source changes, avoiding circular dependency issues. 

This separation allows the same Release to be deployed to multiple environments with different EnvSettings, producing environment-specific manifests (via ComponentEnvSnapshot) at reconciliation time.


### 1. Release CR

A **Release** is an immutable snapshot of promotable content. It supports **two approaches** for storing ComponentTypeDefinitions, Addons, and Components: **git references** (lightweight) or **embedded content** (self-contained). The Workload is always embedded as it contains build output.

**Key characteristics:**
- Immutable once created
- Supports both git references and embedded content
- Workload is always embedded inline (includes container image tag from build)
- EnvSettings are **NOT** included (applied per-environment at reconcile time)
- Each component reference must have **exactly one** of `gitRef` or `content`

#### Approach 1: Git References

Stores references to content using git commit SHAs. Keeps Release objects small (~2-5 KB).

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Release
metadata:
  name: checkout-service-v1.2.3
  labels:
    openchoreo.dev/component: checkout-service
    openchoreo.dev/version: v1.2.3
spec:
  component:
    name: checkout-service
    gitRef:
      repository: https://github.com/myorg/openchoreo-infra
      revision: a1b2c3d4  # Git commit SHA
      path: components/checkout-service.yaml

  componentTypeDefinition:
    name: web-app
    gitRef:
      repository: https://github.com/myorg/openchoreo-infra
      revision: e5f6g7h8  # Git commit SHA
      path: component-types/web-app.yaml

  addons:
    - name: persistent-volume-claim
      instanceId: app-data
      gitRef:
        repository: https://github.com/myorg/openchoreo-infra
        revision: i9j0k1l2  # Git commit SHA
        path: addons/persistent-volume-claim.yaml

    - name: add-file-logging-sidecar
      instanceId: app-logs
      gitRef:
        repository: https://github.com/myorg/openchoreo-infra
        revision: i9j0k1l2  # Git commit SHA
        path: addons/file-logging-sidecar.yaml

  workload:
    # Workload is always embedded (contains build output)
    apiVersion: openchoreo.dev/v1alpha1
    kind: Workload
    metadata:
      name: checkout-service
    spec:
      image: gcr.io/project/checkout-service:v1.2.3  # Built image
      endpoints:
        - name: api
          type: http
          port: 8080
          schemaPath: ./openapi/api.yaml
      connections:
        - name: productcatalog
          type: api
          params:
            projectName: gcp-microservice-demo
            componentName: productcatalog
            endpoint: grpc-endpoint
          inject:
            env:
              - name: PRODUCT_CATALOG_SERVICE_ADDR
                value: "{{ .host }}:{{ .port }}"
```

#### Approach 2: Embedded Content

Embeds full definitions directly in the Release. Self-contained but larger (~50-200 KB). For CLI and UI Releases will be generated by the Open Choreo API server on deployment or promotion.

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Release
metadata:
  name: checkout-service-v1.2.3
  labels:
    openchoreo.dev/component: checkout-service
    openchoreo.dev/version: v1.2.3
spec:
  component:
    name: checkout-service
    content:
      # Full embedded Component definition
      apiVersion: openchoreo.dev/v1alpha1
      kind: Component
      metadata:
        name: checkout-service
      spec:
        componentType: deployment/web-app
        parameters:
          lifecycle:
            terminationGracePeriodSeconds: 60
          resources:
            requests:
              cpu: 200m
              memory: 512Mi
        addons:
          - name: persistent-volume-claim
            instanceId: app-data
            config:
              volumeName: app-data-vol
              mountPath: /app/data

  componentTypeDefinition:
    name: web-app
    content:
      # Full embedded ComponentTypeDefinition
      apiVersion: openchoreo.dev/v1alpha1
      kind: ComponentTypeDefinition
      metadata:
        name: web-app
      spec:
        workloadType: deployment
        schema:
          parameters: { ... }
        resources: [ ... ]

  addons:
    - name: persistent-volume-claim
      instanceId: app-data
      content:
        # Full embedded Addon definition
        apiVersion: openchoreo.dev/v1alpha1
        kind: Addon
        metadata:
          name: persistent-volume-claim
        spec:
          creates: [ ... ]
          patches: [ ... ]

  workload:
    # Workload is always embedded
    apiVersion: openchoreo.dev/v1alpha1
    kind: Workload
    metadata:
      name: checkout-service
    spec:
      image: gcr.io/project/checkout-service:v1.2.3
      endpoints: [ ... ]
      connections: [ ... ]
```

#### Approach 3: Mixed (Git References + Embedded)

You can combine both approaches within a single Release:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Release
metadata:
  name: checkout-service-v1.2.3
spec:
  # Component from git
  component:
    name: checkout-service
    gitRef:
      repository: https://github.com/myorg/openchoreo-infra
      revision: a1b2c3d4
      path: components/checkout-service.yaml

  # ComponentTypeDefinition embedded
  componentTypeDefinition:
    name: web-app
    content:
      apiVersion: openchoreo.dev/v1alpha1
      kind: ComponentTypeDefinition
      # ... full definition ...

  # Mix of git and embedded addons
  addons:
    - name: persistent-volume-claim
      instanceId: app-data
      gitRef:
        repository: https://github.com/myorg/openchoreo-infra
        revision: i9j0k1l2
        path: addons/pvc.yaml
    - name: add-file-logging-sidecar
      instanceId: app-logs
      content:
        apiVersion: openchoreo.dev/v1alpha1
        kind: Addon
        # ... full definition ...

  workload: { ... }
```

#### When to Use Each Approach

**Use Git References when:**
- Working with GitOps workflows (ArgoCD, Flux)
- You want clean, readable git diffs for promotion reviews (Release is only ~2-5 KB)
- You want traceability to exact git commits
- ComponentTypeDefinitions/Addons are reused across multiple components
- Reconciliation speed is less critical (async git-triggered deployments)
- Total storage: ~52-205 KB per environment (Release + ComponentEnvSnapshot)
- **Note**: ComponentEnvSnapshot (~50-200 KB) is still generated; the benefit is smaller Release objects

**Use Embedded Content when:**
- Working with UI/CLI workflows (no git integration)
- You need **faster reconciliation** (no git fetches required before generating ComponentEnvSnapshot)
- You want self-contained Releases (no external git dependencies)
- Definitions are dynamically generated by OpenChoreo API server
- Working in air-gapped environments without git access
- Total storage: ~100-400 KB per environment (Release + ComponentEnvSnapshot)
- **Note**: ComponentEnvSnapshot (~50-200 KB) is still generated; the cost is larger Release objects

**Schema Validation Rules:**

Each reference must have exactly one of `gitRef` or `content`:

```yaml
# Valid: Has gitRef
component:
  name: checkout-service
  gitRef: { ... }

# Valid: Has content
component:
  name: checkout-service
  content: { ... }

# Invalid: Has both
component:
  name: checkout-service
  gitRef: { ... }
  content: { ... }

# Invalid: Has neither
component:
  name: checkout-service
```


### 2. ReleasePin CR

A **ReleasePin** represents which Release is currently deployed to a specific environment. It's a per-environment resource that creates the binding between a Release and an Environment.

**Key characteristics:**
- One ReleasePin per component per environment
- Mutable (updated during promotions)
- References a Release by name
- Lightweight (just a reference + environment identifier)

**Structure:**

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: ReleasePin
metadata:
  name: checkout-service-production
  labels:
    openchoreo.dev/component: checkout-service
    openchoreo.dev/environment: production
spec:
  component: checkout-service
  environment: production
  releaseRef:
    name: checkout-service-v1.2.3  # Points to a Release

status:
  # Status fields managed by controller
  appliedRelease: checkout-service-v1.2.3
  observedGeneration: 1
  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2025-10-27T10:00:00Z"
      reason: ReconcileSuccess
      message: "Release successfully applied"
```


### 3. Deployment Flow

**Initial Deployment (First Environment)**

When a Component is created or updated, the deployment flow works as follows:

1. Developer create component and trigger the deployment to the first environment.
2. OpenChoreo API Server will create the Release and ReleasePin for the first environment referencing the Component, ComponentTypeDefinition, Addons, and Workload.

### 3a. Relationship Between Release, ReleasePin, and ComponentEnvSnapshot

To clarify the architecture, here's how these three resources work together:

| Resource | Type | Purpose | Generated When | Content |
|----------|------|---------|----------------|---------|
| **Release** | User-facing, immutable | Promotable unit (like a lockfile) | On build completion or manual creation | References or embeds ComponentTypeDefinition, Component, Addons, and Workload |
| **ReleasePin** | User-facing, mutable | Environment binding | On initial deployment or promotion | References a Release + specifies environment |
| **ComponentEnvSnapshot** | Internal, generated | Resolved definitions cache | During ReleasePin reconciliation | Full embedded copies of resolved ComponentTypeDefinition, Component, Addons, Workload (NOT EnvSettings) |

**Key Differences from Original ComponentEnvSnapshot Approach:**

| Aspect | Original Approach | This Proposal |
|--------|-------------------|---------------|
| **Generation trigger** | Auto-generated on source changes | Generated only during ReleasePin reconciliation |
| **User visibility** | Primary user-facing resource | Internal implementation detail |
| **Promotion mechanism** | Update ComponentEnvSnapshot directly | Update ReleasePin to reference different Release |
| **Circular dependencies** | Yes (auto-generation triggers git commits) | No (only generated on-demand) |
| **GitOps diffs** | Large (full embedded content) | Small (just Release reference change) |

**Example relationship:**

```
Release "checkout-v1.2.3" (user-facing)
  ├─ componentTypeDefinition: gitRef → web-app (lightweight)
  ├─ component: gitRef → checkout-service
  └─ addons: [gitRef → pvc-addon]

     ↓ Referenced by

ReleasePin "checkout-service-production" (user-facing)
  ├─ releaseRef: checkout-v1.2.3
  └─ environment: production

     ↓ Generates during reconciliation

ComponentEnvSnapshot "checkout-service-production-abc123" (internal)
  ├─ componentTypeDefinition: <full embedded copy>
  ├─ component: <full embedded copy>
  ├─ addons: [<full embedded copy>]
  └─ workload: <full embedded copy>
  # EnvSettings applied separately, not stored here
```

**Why this design?**
1. **Separation of concerns**: Release handles promotion, ComponentEnvSnapshot handles resolved state
2. **Clean GitOps**: Promotions are just ReleasePin updates (small diffs)
3. **No circular dependencies**: ComponentEnvSnapshot is never committed to git, only generated
4. **Flexibility**: Release can use git references (lightweight) while ComponentEnvSnapshot provides full resolution
5. **Observability**: ComponentEnvSnapshot shows exactly what's deployed, Release shows what was promoted


### 4. Promotion Flow

Promotion is the process of deploying an existing Release to a subsequent environment. The mechanism differs slightly between UI/CLI and GitOps workflows, but uses the same underlying CRs.

#### UI/CLI Promotion

1. User triggers promotion:
   - **UI**: Clicks "Promote" button, selects source and target environments
   - **CLI**: Runs `choreo promote --from development --to staging`

   ```bash
   # Promote a component from development to staging
   choreo promote --from development --to staging

   # The component name is inferred from current context or can be specified
   choreo promote checkout-service --from development --to staging
   ```

2. OpenChoreo controller:
   - Identifies the ReleasePin in the source environment (e.g., `checkout-service-development`)
   - Reads the `releaseRef` from that ReleasePin
   - Creates or updates the ReleasePin in the target environment (e.g., `checkout-service-staging`)
   - Sets the same `releaseRef` in the target ReleasePin

3. Target environment controller reconciles:
   - Fetches the Release
   - Generates/updates ComponentEnvSnapshot for staging environment
   - Applies target environment's EnvSettings (e.g., staging replica counts, resource limits)
   - Renders and deploys resources to staging

#### GitOps Promotion

1. User creates a Git PR to update the ReleasePin:
   ```diff
   apiVersion: openchoreo.dev/v1alpha1
   kind: ReleasePin
   metadata:
     name: checkout-service-staging
   spec:
     component: checkout-service
     environment: staging
     releaseRef:
   -   name: checkout-service-v1.2.2
   +   name: checkout-service-v1.2.3
   ```

2. PR is reviewed and merged

3. GitOps controller (ArgoCD/Flux) syncs the change

4. OpenChoreo controller reconciles the updated ReleasePin (same as UI/CLI flow above)

**Benefits:**
- Clean, human-readable Git diffs
- No controller-generated commits (avoids circular dependencies)
- Auditability through Git history
- PR-based approval workflows for promotions

### 4a. CLI Command Examples

**Deploy a component to an environment:**

```bash
# Deploy the checkout-service component to the development environment
choreo deploy checkout-service --environment development
```

**Promote a component between environments:**

```bash
# Promote the checkout-service component from staging to production
choreo promote checkout-service --from staging --to production

# Dry-run promotion (show what would change)
choreo promote checkout-service --from staging --to production --dry-run
```


### 5. Environment-Specific Configuration

EnvSettings resources remain per-environment and are applied at reconciliation time, not stored in the Release.

**Example EnvSettings:**

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: EnvSettings
metadata:
  name: checkout-service-production
spec:
  owner:
    componentName: checkout-service
  environment: production

  overrides:
    resources:
      requests:
        cpu: 500m
        memory: 1Gi
      limits:
        cpu: 2000m
        memory: 2Gi
    autoscaling:
      enabled: true
      minReplicas: 5
      maxReplicas: 50

  addonOverrides:
    persistent-volume-claim:
      app-data:
        size: 200Gi
        storageClass: premium
```

**When reconciling a ReleasePin**, the controller:
1. Fetches the Release
2. Resolves git references to get ComponentTypeDefinition, Component, and Addons
3. Fetches EnvSettings for that environment
4. Merges Component parameters with EnvSettings overrides
5. Renders templates with merged values
6. Applies Addon patches (with addon-specific EnvSettings overrides)
7. Creates final Kubernetes resources

This allows the same Release to produce different Kubernetes manifests in different environments (e.g., 1 replica in dev, 10 in production) without duplicating the core component definition.

### 6. Controller Reconciliation Logic

The ReleasePin controller is responsible for rendering and applying resources. The reconciliation algorithm handles both git references and embedded content transparently, and generates a ComponentEnvSnapshot to cache the resolved state:

```go
func (r *ReleasePinReconciler) Reconcile(ctx context.Context, releasePin *ReleasePin) error {
    // 1. Fetch the referenced Release
    release := fetchRelease(releasePin.Spec.ReleaseRef.Name)

    // 2. Resolve definitions (from git or embedded content)
    componentTypeDef := resolveComponentTypeDefinition(release.Spec.ComponentTypeDefinition)
    component := resolveComponent(release.Spec.Component)
    addons := make([]Addon, len(release.Spec.Addons))
    for i, addonRef := range release.Spec.Addons {
        addons[i] = resolveAddon(addonRef)
    }
    workload := release.Spec.Workload  // Always embedded

    // 3. Fetch environment-specific settings
    envSettings := fetchEnvSettings(releasePin.Spec.Component, releasePin.Spec.Environment)

    // 4. Generate or update ComponentEnvSnapshot (internal cache)
    // This is NOT auto-generated on source changes, only during reconciliation
    snapshot := createOrUpdateComponentEnvSnapshot(ctx, ComponentEnvSnapshot{
        Metadata: generateSnapshotMetadata(releasePin),
        Spec: ComponentEnvSnapshotSpec{
            Owner: OwnerRef{
                ComponentName: releasePin.Spec.Component,
            },
            Environment: releasePin.Spec.Environment,
            ComponentTypeDefinition: componentTypeDef,
            Component: component,
            Addons: addons,
            Workload: workload,
            // EnvSettings are applied separately, not stored in snapshot
        },
    })

    // 5. Merge parameters: Component params + EnvSettings overrides
    mergedParams := merge(component.Spec.Parameters, envSettings.Spec.Overrides)

    // 6. Build template context
    templateContext := TemplateContext{
        Metadata: extractMetadata(component),
        Spec: mergedParams,
        Build: component.Spec.Build,
        Workload: workload.Spec,
    }

    // 7. Render ComponentTypeDefinition templates
    renderedResources := renderTemplates(componentTypeDef.Spec.Resources, templateContext)

    // 8. Apply addon patches (with addon-specific overrides)
    for _, addonInstance := range component.Spec.Addons {
        addon := findAddon(addons, addonInstance.Name)
        addonOverrides := envSettings.Spec.AddonOverrides[addonInstance.Name][addonInstance.InstanceId]
        mergedAddonParams := merge(addonInstance.Config, addonOverrides)

        // Create new resources from addon
        newResources := renderTemplates(addon.Spec.Creates, mergedAddonParams)
        renderedResources = append(renderedResources, newResources...)

        // Apply patches to existing resources
        renderedResources = applyPatches(renderedResources, addon.Spec.Patches, mergedAddonParams)
    }

    // 9. Apply rendered resources to cluster
    applyResources(ctx, renderedResources)

    // 10. Update status
    updateStatus(releasePin, release.Name, snapshot.Name)

    return nil
}

// Resolve ComponentTypeDefinition from either gitRef or embedded content
func resolveComponentTypeDefinition(ref ComponentTypeDefinitionRef) *ComponentTypeDefinition {
    if ref.GitRef != nil {
        // Fetch from git repository
        return fetchFromGit(ref.GitRef)
    } else if ref.Content != nil {
        // Use embedded content directly
        return ref.Content
    }
    // Should never happen due to validation
    panic("ComponentTypeDefinitionRef must have either gitRef or content")
}

// Resolve Component from either gitRef or embedded content
func resolveComponent(ref ComponentRef) *Component {
    if ref.GitRef != nil {
        return fetchFromGit(ref.GitRef)
    } else if ref.Content != nil {
        return ref.Content
    }
    panic("ComponentRef must have either gitRef or content")
}

// Resolve Addon from either gitRef or embedded content
func resolveAddon(ref AddonRef) *Addon {
    if ref.GitRef != nil {
        return fetchFromGit(ref.GitRef)
    } else if ref.Content != nil {
        return ref.Content
    }
    panic("AddonRef must have either gitRef or content")
}
```

**Key points:**
- The reconciliation logic is agnostic to whether content comes from git or is embedded
- **ComponentEnvSnapshot is generated during reconciliation** as an internal cache/snapshot of resolved definitions
  - NOT auto-generated on source changes (avoids circular dependencies)
  - Provides observability into what's actually deployed in each environment
  - Can serve as a reconciliation cache (avoiding repeated git fetches)
  - EnvSettings are NOT stored in the snapshot (applied fresh on each reconciliation)
- Git references are fetched fresh on each reconciliation (could be cached with git commit SHA as cache key)
- Embedded content is used directly without external fetches (faster reconciliation)
- EnvSettings are fetched per-environment regardless of content source
- Templates are rendered with merged parameters
- Addon patches are applied after base resources are rendered
- Final resources are applied to the cluster
- Validation webhooks ensure each reference has exactly one of `gitRef` or `content`

**Why ComponentEnvSnapshot is still useful:**
1. **Observability**: Provides a clear view of resolved definitions per environment
2. **Reconciliation cache**: Avoids re-resolving git references on every reconciliation (can check if snapshot needs update)
3. **Audit trail**: Historical record of what was deployed (if retained)
4. **Debugging**: Easy to inspect the exact resolved state that was used for deployment
5. **Consistency**: Ensures the same resolved definitions are used across multiple reconciliation attempts


### 7. Garbage Collection

To prevent unbounded growth of Release and ComponentEnvSnapshot objects, OpenChoreo supports configurable retention policies.

#### UI/CLI Mode

**Release Garbage Collection:**
- **Default policy**: Retain last N releases (where N = number of environments)
- **Example**: For 3 environments (dev, staging, prod) = retain last 3 releases
- **Implementation**: OpenChoreo API Server automatically deletes old Releases when new ones are created, keeping only the most recent N
- **Safeguard**: Never delete a Release that is currently referenced by any ReleasePin

#### GitOps Mode

**Release Garbage Collection:**
- **User-managed**: Users are responsible for cleaning up old Release definitions from git
- **Optional tooling**: OpenChoreo can provide a CLI command to list unused releases:
  ```bash
  choreo releases gc --dry-run  # List releases not referenced by any ReleasePin
  choreo releases gc --confirm  # Delete unused release files from git
  ```
- **Recommendation**: Keep releases for a retention period (e.g., 30 days) even if no longer pinned, for rollback purposes

## etcd Storage Considerations

When choosing between git references and embedded content, consider the impact on both etcd storage and reconciliation performance.

### Important: ComponentEnvSnapshot is Generated for Both Approaches

**Both git references and embedded content approaches generate ComponentEnvSnapshot** during reconciliation. The ComponentEnvSnapshot always contains full embedded copies of resolved definitions, so its size is **identical** for both approaches (~50-200 KB per environment).

The difference is in the **Release object size** and **reconciliation performance**, not in ComponentEnvSnapshot storage.

### Storage & Performance Comparison

**Total etcd Storage per Environment:**
- **Git Reference**: Release (~2-5 KB) + ComponentEnvSnapshot (~50-200 KB) = **~52-205 KB total**
- **Embedded Content**: Release (~50-200 KB) + ComponentEnvSnapshot (~50-200 KB) = **~100-400 KB total**

**Git Reference Approach:**
- **Release object size**: ~2-5 KB (only git commit SHAs and metadata)
- **ComponentEnvSnapshot size**: ~50-200 KB (same as embedded approach)
- **Total per environment**: ~52-205 KB
- **Reconciliation speed**: **Slower** (requires git fetches to resolve references before generating ComponentEnvSnapshot)
- **Trade-off**: Smaller Release objects, cleaner git diffs, but slower reconciliation
- **Best for**: GitOps workflows where clean git diffs are more important than reconciliation speed

**Embedded Content Approach:**
- **Release object size**: ~50-200 KB (full embedded definitions)
  - Future: ~10-40 KB with compression (60-80% reduction)
- **ComponentEnvSnapshot size**: ~50-200 KB (same as git reference approach)
- **Total per environment**: ~100-400 KB
- **Reconciliation speed**: **Faster** (no git fetches needed, can immediately generate ComponentEnvSnapshot)
- **Trade-off**: Larger Release objects (~2x total storage), but faster reconciliation
- **Best for**: UI/CLI workflows where reconciliation speed is important
- **Requires active garbage collection** to prevent unbounded growth of Release objects

### etcd Bloat Mitigation Strategies

While embedded content can cause etcd storage pressure (similar to the ComponentEnvSnapshot issue), this is managed through:

1. **Garbage Collection** (Primary Strategy):
   - Automatically delete old/unused Releases based on retention policies
   - UI/CLI mode: Keep last N releases (e.g., N = number of environments + buffer)
   - Never delete Releases currently referenced by any ReleasePin
   - See [Garbage Collection](#7-garbage-collection) section for details

2. **Git References** (Alternative Strategy):
   - Use git references for GitOps workflows to avoid etcd bloat entirely
   - Particularly recommended for shared ComponentTypeDefinitions and Addons

3. **Hybrid Approach**:
   - Mix git references (for large, shared resources) with embedded content (for small, component-specific config)

4. **Compression** (Planned Future Enhancement):
   - Apply compression (e.g., gzip) to embedded content within Release objects
   - Could reduce embedded Release size by 60-80% (from ~50-200 KB to ~10-40 KB)
   - Transparent to users - compression/decompression handled by the controller
   - Particularly beneficial for UI/CLI workflows where embedded content is preferred
   - Trade-off: Slightly increased CPU usage during reconciliation

### Recommendations

1. **For GitOps workflows**: Use git references for cleaner git diffs and easier promotion reviews
   - Release objects are small (~2-5 KB) making git diffs readable
   - ComponentEnvSnapshot storage is the same either way (~50-200 KB)
   - Slower reconciliation is acceptable for GitOps (triggered by git commits, not user-facing)
   - Total storage: ~52-205 KB per environment

2. **For UI/CLI workflows**: Embedded content provides faster reconciliation
   - No git fetches needed, enabling faster deployments
   - ComponentEnvSnapshot storage is the same either way (~50-200 KB)
   - Total storage: ~100-400 KB per environment (~2x compared to git references)
   - Requires proper garbage collection policies for Release objects

3. **For high-frequency deployments**: Consider git references with git caching
   - Cache resolved git references to reduce fetch overhead
   - Garbage collection is simpler (just delete old Release objects)
   - ComponentEnvSnapshot still needs to be generated either way

4. **For mixed environments**: Use git references for shared ComponentTypeDefinitions/Addons; embed component-specific configuration
   - Reduces duplication of shared definitions in Release objects
   - Component-specific config can be embedded for faster access
   - ComponentEnvSnapshot will be generated regardless

## Implementation Phases

### Phase 1: Core CRDs and Controllers
- Implement Release and ReleasePin CRDs with both gitRef and content field support
- Add validation webhooks to ensure exactly one of gitRef or content is specified
- Implement ReleasePin controller with reconciliation logic for both approaches:
  - Generate ComponentEnvSnapshot during reconciliation (not auto-generated)
  - Support both git references and embedded content resolution
  - Implement garbage collection for old ComponentEnvSnapshots
- Support automatic Release creation on Component updates (first environment only)
- Implement git fetching logic with caching (keyed by commit SHA)

### Phase 2: UI/CLI Deployment and Promotion
- Add deployment command to CLI: `choreo deploy --environment <environment>`
- Add promotion command to CLI: `choreo promote --from <environment> --to <environment>`
- Add "Deploy" button in OpenChoreo console UI with environment selection
- Add "Promote" button in OpenChoreo console UI with source/target environment selection
- Implement deployment and promotion audit logs
- Add Release creation with embedded content for UI/CLI workflows

### Phase 3: GitOps Support
- Document GitOps workflows for promotions
- Add validation webhooks to prevent invalid ReleasePin updates
- Provide example GitOps repository structures
- Document git authentication setup (SSH keys, personal access tokens)

### Phase 4: Garbage Collection
- Implement retention policies for UI/CLI mode
- Add `choreo releases gc` command for GitOps mode
- Add metrics for Release storage usage
- Implement etcd size monitoring and alerting


## Future Enhancements

### 1. Content Compression for Embedded Releases

Add transparent compression to reduce etcd storage footprint for embedded content:

**Implementation approach:**
- Add a `compressed: true` annotation to Release objects
- Controller compresses embedded content using gzip before storing in etcd
- Controller automatically decompresses during reconciliation
- Expected compression ratios: 60-80% reduction (YAML compresses well)

**Benefits:**
- Reduces embedded Release size from ~50-200 KB to ~10-40 KB
- Makes embedded content more competitive with git references in terms of storage
- Particularly beneficial for UI/CLI workflows
- Reduces etcd backup sizes and network transfer during replication

**Example:**
```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Release
metadata:
  name: checkout-service-v1.2.3
  annotations:
    openchoreo.dev/compression: gzip
spec:
  component:
    name: checkout-service
    content:
      # Stored compressed in etcd, automatically decompressed on read
      compressed: "H4sIAAAAAAAA/6tWKkktLlGyUlAqS8wpTtVRKi..."  # Base64-encoded gzip
```

**Considerations:**
- Add CPU overhead during reconciliation (compression/decompression)
- Implement compression only when embedded content exceeds a threshold (e.g., 20 KB)
- Provide metrics on compression ratios achieved


## References

- [Proposal #537: Introduce Component Type Definitions](0537-introduce-component-type-definitions.md)
- [Discussion #554: Deployment and Promotion Flow](https://github.com/openchoreo/openchoreo/discussions/554)
- [Flux Source Controller](https://fluxcd.io/flux/components/source/) - Similar git reference patterns
- [Argo CD](https://argo-cd.readthedocs.io/) - Promotion workflow inspiration
