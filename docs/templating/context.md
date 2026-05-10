# Template Context Variables

This guide documents the context variables available in OpenChoreo templates. Different template locations have access
to different context types.

## Overview

OpenChoreo uses two context types depending on where the template is evaluated:

| Context Type         | Used In                                       | Key Variables                                                                                                                                |
|----------------------|-----------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------|
| **ComponentContext** | ComponentType `validations` and `resources`   | `metadata`, `parameters`, `environmentConfigs`, `dataplane`, `gateway`, `environment`, `workload`, `configurations`, `dependencies`          |
| **TraitContext**     | Trait `validations`, `creates`, and `patches` | `metadata`, `parameters`, `environmentConfigs`, `dataplane`, `gateway`, `environment`, `trait`, `workload`, `configurations`, `dependencies` |

## ComponentContext

ComponentContext is used when rendering ComponentType resources. It provides access to component metadata, parameters (
from Component), environment overrides (from ReleaseBinding), workload information, and configurations.

### Available in ComponentType

| Location                  | Context                          | Notes                               |
|---------------------------|----------------------------------|-------------------------------------|
| `validations[].rule`      | ComponentContext                 | Evaluated before resource rendering |
| `resources[].template`    | ComponentContext                 | Full context                        |
| `resources[].includeWhen` | ComponentContext                 | Evaluated before forEach            |
| `resources[].forEach`     | ComponentContext                 | Expression to iterate over          |
| Inside forEach iteration  | ComponentContext + loop variable | Loop variable added to context      |

### metadata

Platform-computed metadata for resource generation.

```yaml
# Access pattern: ${metadata.<field>}

metadata:
  # Component identity
  componentName: "my-service"           # ${metadata.componentName}
  componentUID: "a1b2c3d4-..."          # ${metadata.componentUID}

  # Project identity
  projectName: "my-project"             # ${metadata.projectName}
  projectUID: "b2c3d4e5-..."            # ${metadata.projectUID}

  # Environment identity
  environmentName: "production"         # ${metadata.environmentName}
  environmentUID: "d4e5f6g7-..."        # ${metadata.environmentUID}

  # DataPlane identity
  dataPlaneName: "my-dataplane"         # ${metadata.dataPlaneName}
  dataPlaneUID: "c3d4e5f6-..."          # ${metadata.dataPlaneUID}

  # Generated resource naming (use these in all resource templates)
  name: "my-service-dev-a1b2c3d4"       # ${metadata.name} - use as prefix for all resource names to avoid conflicts between components
  namespace: "dp-acme-corp-dev-x1y2z3"  # ${metadata.namespace} - use for all namespaced resources to ensure components in a project share the same namespace per environment

  # Common labels for all resources
  labels: # ${metadata.labels}
    openchoreo.dev/component: "my-service"
    openchoreo.dev/project: "my-project"
    # ... other platform labels

  # Common annotations for all resources
  annotations: { }                       # ${metadata.annotations}

  # Pod selectors - use for selector.matchLabels, pod template labels, and service selectors
  podSelectors: # ${metadata.podSelectors}
    openchoreo.dev/component-uid: "abc123"
    openchoreo.dev/environment-uid: "dev"
    openchoreo.dev/project-uid: "xyz789"
```

**Example usage:**

```yaml
# In ComponentType resource template
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${metadata.name}
  namespace: ${metadata.namespace}
  labels: ${metadata.labels}
spec:
  selector:
    matchLabels: ${metadata.podSelectors}
  template:
    metadata:
      labels: ${oc_merge(metadata.labels, metadata.podSelectors)}
```

### parameters

Component parameters from `Component.Spec.Parameters`, pruned to the ComponentType's `schema.parameters` section with
defaults applied. Use for static configuration that doesn't change across environments.

```yaml
# Access pattern: ${parameters.<field>}

# Given this schema in ComponentType:
schema:
  parameters:
    replicas: "integer | default=1"
    port: "integer | default=8080"

# And this Component:
spec:
  parameters:
    replicas: 3

# The parameters context would be:
parameters:
  replicas: 3                    # ${parameters.replicas} (from Component)
  port: 8080                     # ${parameters.port} (default from schema)
```

**Example usage:**

```yaml
spec:
  replicas: ${parameters.replicas}
  template:
    spec:
      containers:
        - name: app
          ports:
            - containerPort: ${parameters.port}
```

### environmentConfigs

Environment-specific overrides from `ReleaseBinding.Spec.ComponentTypeEnvironmentConfigs`, pruned to the ComponentType's
`schema.environmentConfigs` section with defaults applied. Use for values that vary per environment (resources,
replicas, etc.).

```yaml
# Access pattern: ${environmentConfigs.<field>}

# Given this schema in ComponentType:
schema:
  environmentConfigs:
    resources:
      $default: { }
      requests:
        $default: { }
        cpu: "string | default=100m"
        memory: "string | default=128Mi"
      limits:
        $default: { }
        cpu: "string | default=500m"
        memory: "string | default=512Mi"

# And this ReleaseBinding:
spec:
  componentTypeEnvironmentConfigs:
    resources:
      requests:
        cpu: "200m"
        memory: "256Mi"

# The environmentConfigs context would be:
environmentConfigs:
  resources:
    requests:
      cpu: "200m"                # ${environmentConfigs.resources.requests.cpu} (from ReleaseBinding)
      memory: "256Mi"            # ${environmentConfigs.resources.requests.memory} (from ReleaseBinding)
    limits:
      cpu: "500m"                # ${environmentConfigs.resources.limits.cpu} (default)
      memory: "512Mi"            # ${environmentConfigs.resources.limits.memory} (default)
```

**Example usage:**

```yaml
spec:
  template:
    spec:
      containers:
        - name: app
          resources:
            requests:
              cpu: ${environmentConfigs.resources.requests.cpu}
              memory: ${environmentConfigs.resources.requests.memory}
            limits:
              cpu: ${environmentConfigs.resources.limits.cpu}
              memory: ${environmentConfigs.resources.limits.memory}
```

**Key difference from parameters:**

- `parameters`: Static values from Component - same across all environments
- `environmentConfigs`: Environment-specific values from ReleaseBinding - different per environment

### dataplane

DataPlane configuration for the target environment.

```yaml
# Access pattern: ${dataplane.<field>}

dataplane:
  secretStore: "my-secret-store"              # ${dataplane.secretStore}
  gateway: # ${dataplane.gateway}
    ingress: # ${dataplane.gateway.ingress}
      external: # ${dataplane.gateway.ingress.external}
        name: "gateway-default"               # ${dataplane.gateway.ingress.external.name}
        namespace: "openchoreo-data-plane"     # ${dataplane.gateway.ingress.external.namespace}
        http: # ${dataplane.gateway.ingress.external.http}
          listenerName: "http"                # ${dataplane.gateway.ingress.external.http.listenerName}
          port: 8080                          # ${dataplane.gateway.ingress.external.http.port}
          host: "app.example.com"             # ${dataplane.gateway.ingress.external.http.host}
        https: # ${dataplane.gateway.ingress.external.https}
          listenerName: "https"
          port: 8443
          host: "app.example.com"
  observabilityPlaneRef: # ${dataplane.observabilityPlaneRef}
    kind: "ObservabilityPlane"                # ${dataplane.observabilityPlaneRef.kind} - "ObservabilityPlane" or "ClusterObservabilityPlane"
    name: "my-obs-plane"                      # ${dataplane.observabilityPlaneRef.name}
```

**Optional fields:** `secretStore`, `gateway`, and `observabilityPlaneRef` are optional. If not configured on the
DataPlane, the field will be absent from the context. Use `has()` to guard conditional logic:

```yaml
# Guard with has() for conditional inclusion
includeWhen: ${has(dataplane.secretStore)}

# Or use ternary for conditional values
secretStoreRef:
  ${has(dataplane.secretStore) ? {"name":
    dataplane.secretStore}: oc_omit()}
```

**Example usage:**

```yaml
# Creating an ExternalSecret that references the dataplane's secret store
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: ${metadata.name}
spec:
  secretStoreRef:
    name: ${dataplane.secretStore}
    kind: ClusterSecretStore
```

### gateway

Top-level gateway configuration resolved for the component's environment. If the environment has its own gateway
configuration, it takes precedence over the dataplane gateway.

```yaml
# Access pattern: ${gateway.<field>}

gateway:
  ingress:
    external:
      name: "gateway-default"                 # ${gateway.ingress.external.name}
      namespace: "openchoreo-data-plane"       # ${gateway.ingress.external.namespace}
      http:
        listenerName: "http"                   # ${gateway.ingress.external.http.listenerName}
        port: 8080                             # ${gateway.ingress.external.http.port}
        host: "app.example.com"                # ${gateway.ingress.external.http.host}
      https:
        listenerName: "https"                  # ${gateway.ingress.external.https.listenerName}
        port: 8443                             # ${gateway.ingress.external.https.port}
        host: "app.example.com"               # ${gateway.ingress.external.https.host}
    internal:
      name: "gateway-internal"                 # ${gateway.ingress.internal.name}
      namespace: "openchoreo-data-plane"       # ${gateway.ingress.internal.namespace}
```

**Optional:** The `gateway` field is optional. Use `has()` to guard conditional logic.

### environment

Environment-specific configuration for the target environment.

```yaml
# Access pattern: ${environment.<field>}

environment:
  gateway: # ${environment.gateway} - environment-specific gateway overrides
    ingress:
      external:
        name: "env-gateway"
  defaultNotificationChannel: "my-channel"     # ${environment.defaultNotificationChannel}
```

**Optional:** The `environment` fields are optional. If the environment does not have specific gateway configuration,
the dataplane gateway is used as a fallback via the top-level `gateway` variable.

### workload

Workload specification containing container and endpoint information from the build process.

```yaml
# Access pattern: ${workload.container.<field>}

workload:
  container: # ${workload.container}
    image: "myregistry/myapp:v1.0"        # ${workload.container.image}
    command: [ "./start.sh" ]               # ${workload.container.command}
    args: [ "--port", "8080" ]              # ${workload.container.args}
  endpoints:
    http: # ${workload.endpoints.http}
      type: "HTTP"                      # ${workload.endpoints.http.type}
      port: 8080                        # ${workload.endpoints.http.port}
      basePath: "/api"                  # ${workload.endpoints.http.basePath} (optional, default "/")
      visibility: [ "project", "external" ] # ${workload.endpoints.http.visibility}
      schema: # ${workload.endpoints.http.schema} (optional)
        type: "openapi"
        content: "..."
    grpc:
      type: "gRPC"
      port: 9090
      visibility: [ "project" ]           # project visibility only - no gateway routes created
```

**Endpoint types:** HTTP, gRPC, GraphQL, Websocket, TCP, UDP

**Endpoint visibility:** Endpoints always include `"project"` visibility. Additional scopes — `"external"` and
`"internal"` — determine which gateway HTTPRoutes are created:

| Visibility scope | Effect                                                                                |
|------------------|---------------------------------------------------------------------------------------|
| `project`        | Always present; endpoint is reachable within the project namespace (no gateway route) |
| `external`       | Creates an HTTPRoute on the external ingress gateway                                  |
| `internal`       | Creates an HTTPRoute on the internal ingress gateway                                  |

**Example usage:**

```yaml
spec:
  template:
    spec:
      containers:
        - name: app
          image: ${workload.container.image}
          command: ${workload.container.command}
          args: ${workload.container.args}
```

**Iterating over endpoints by visibility (recommended pattern for HTTPRoute generation):**

```yaml
# Create HTTPRoutes only for endpoints with external visibility
- id: httproute-external
  forEach: '${workload.endpoints.transformMap(name, ep, "external" in ep.visibility && ep.type in ["HTTP", "GraphQL", "Websocket"], ep)}'
  var: endpoint
  template:
    apiVersion: gateway.networking.k8s.io/v1
    kind: HTTPRoute
    metadata:
      name: ${oc_generate_name(metadata.componentName, endpoint.key)}
      namespace: ${metadata.namespace}
      labels: '${oc_merge(metadata.labels, {"openchoreo.dev/endpoint-name": endpoint.key, "openchoreo.dev/endpoint-visibility": "external"})}'
    spec:
      parentRefs:
        - name: ${gateway.ingress.external.name}
          namespace: ${gateway.ingress.external.namespace}
      hostnames: |
        ${[gateway.ingress.external.?http, gateway.ingress.external.?https]
          .filter(g, g.hasValue()).map(g, g.value().host).distinct()
          .map(h, metadata.environmentName + "-" + metadata.componentNamespace + "." + h)}
      rules:
        - matches:
            - path:
                type: PathPrefix
                value: /${metadata.componentName}-${endpoint.key}
          filters:
            - type: URLRewrite
              urlRewrite:
                path:
                  type: ReplacePrefixMatch
                  replacePrefixMatch: '${endpoint.value.?basePath.orValue("/")}'
          backendRefs:
            - name: ${metadata.componentName}
              port: ${endpoint.value.port}

# Create HTTPRoutes only for endpoints with internal visibility
- id: httproute-internal
  forEach: '${workload.endpoints.transformMap(name, ep, "internal" in ep.visibility && ep.type in ["HTTP", "GraphQL", "Websocket"], ep)}'
  var: endpoint
  template:
    apiVersion: gateway.networking.k8s.io/v1
    kind: HTTPRoute
    metadata:
      name: '${oc_generate_name(metadata.componentName, endpoint.key, "internal")}'
      namespace: ${metadata.namespace}
      labels: '${oc_merge(metadata.labels, {"openchoreo.dev/endpoint-name": endpoint.key, "openchoreo.dev/endpoint-visibility": "internal"})}'
    spec:
      parentRefs:
        - name: ${gateway.ingress.internal.name}
          namespace: ${gateway.ingress.internal.namespace}
      hostnames: |
        ${[gateway.ingress.internal.?http, gateway.ingress.internal.?https]
          .filter(g, g.hasValue()).map(g, g.value().host).distinct()
          .map(h, metadata.environmentName + "-" + metadata.componentNamespace + "." + h)}
      rules:
        - matches:
            - path:
                type: PathPrefix
                value: /${metadata.componentName}-${endpoint.key}
          filters:
            - type: URLRewrite
              urlRewrite:
                path:
                  type: ReplacePrefixMatch
                  replacePrefixMatch: '${endpoint.value.?basePath.orValue("/")}'
          backendRefs:
            - name: ${metadata.componentName}
              port: ${endpoint.value.port}
```

**Iterating over all endpoints (generic pattern):**

```yaml
# Using transformList to convert endpoints map to list of ports
ports: |
  ${workload.endpoints.transformList(name, ep, {
    "name": name,
    "port": ep.port,
    "protocol": ep.type == "UDP" ? "UDP" : "TCP"
  })}
```

**Workload Endpoint Helper Method:**

The `workload` object provides a helper method to simplify Service port generation:

| Helper Method               | Description                                                                                     |
|-----------------------------|-------------------------------------------------------------------------------------------------|
| `workload.toServicePorts()` | Converts endpoints map to Service ports list with proper protocol mapping and name sanitization |

For detailed documentation and examples,
see [Configuration Helpers - Workload Endpoint Helpers](./configuration_helpers.md#workload-endpoint-helpers).

**Quick Example:**

```yaml
# Using helper method for cleaner Service generation
- id: service
  includeWhen: ${size(workload.endpoints) > 0}
  template:
    spec:
      ports: ${workload.toServicePorts()}
```

### configurations

Configuration items (environment variables and files) extracted from workload.

```yaml
# Access pattern: ${configurations.<field>}

configurations: # ${configurations}
  configs: # ${configurations.configs}
    envs: # ${configurations.configs.envs}
      - name: "DATABASE_URL"
        value: "postgres://..."
      - name: "LOG_LEVEL"
        value: "info"
    files: # ${configurations.configs.files}
      - name: "config.yaml"
        mountPath: "/etc/app/config.yaml"
        value: "key: value\n..."
  secrets: # ${configurations.secrets}
    envs: # ${configurations.secrets.envs}
      - name: "API_KEY"
        remoteRef:
          key: "my-secret"
          property: "api-key"
    files: # ${configurations.secrets.files}
      - name: "credentials.json"
        mountPath: "/etc/app/credentials.json"
        remoteRef:
          key: "my-secret"
          property: "credentials"
```

**Structure details:**

| Field                          | Type                  | Description                        |
|--------------------------------|-----------------------|------------------------------------|
| `configurations.configs.envs`  | `[]EnvConfiguration`  | Plain config environment variables |
| `configurations.configs.files` | `[]FileConfiguration` | Plain config files                 |
| `configurations.secrets.envs`  | `[]EnvConfiguration`  | Secret environment variables       |
| `configurations.secrets.files` | `[]FileConfiguration` | Secret files                       |

**EnvConfiguration structure:**

```yaml
- name: "VAR_NAME"              # Environment variable name
  value: "plain-value"          # Plain text value (for configs)
  remoteRef: # Remote reference (for secrets)
    key: "secret-name"
    property: "secret-key"
    version: "v1"               # Optional
```

**FileConfiguration structure:**

```yaml
- name: "filename.txt"          # File name
  mountPath: "/path/to/file"    # Where to mount the file
  value: "file-contents"        # Plain text content (for configs)
  remoteRef: # Remote reference (for secrets)
    key: "secret-name"
    property: "secret-key"
```

**Example usage:**

```yaml
# Inject environment variables from configurations
env: |
  ${configurations.configs.envs.map(e, {
    "name": e.name,
    "value": e.value
  })}

# Create ConfigMap from config files
data: |
  ${configurations.configs.files.transformMapEntry(i, f, {f.name: f.value})}

# Check for config envs before creating configMapRef
envFrom: |
  ${has(configurations.configs.envs) &&
   configurations.configs.envs.size() > 0 ?
    [{"configMapRef": {"name": metadata.name + "-config"}}] : []}
```

### dependencies

Dependency information for the component. Covers two kinds of declared dependencies:

- **Endpoint connections** — `Workload.spec.dependencies.endpoints[]`, exposed under `dependencies.items`.
- **Resource dependencies** — `Workload.spec.dependencies.resources[]`, exposed under `dependencies.resources`.

Per-item views are kept separate so templates can iterate either kind. The merged `envVars`,
`volumeMounts`, and `volumes` lists are convenience surfaces for templates that just want a single
combined feed.

```yaml
# Access pattern: ${dependencies.<field>}

dependencies: # ${dependencies}
  items: # ${dependencies.items} - endpoint connections
    - namespace: "ns1"                        # ${dependencies.items[0].namespace} - target component's namespace
      project: "proj1"                        # ${dependencies.items[0].project} - target project name
      component: "svc-a"                      # ${dependencies.items[0].component} - target component name
      endpoint: "http"                        # ${dependencies.items[0].endpoint} - target endpoint name
      visibility: "project"                   # ${dependencies.items[0].visibility} - resolved visibility level
      envVars: # ${dependencies.items[0].envVars} - per-connection env vars
        - name: "SVC_A_URL"
          value: "http://svc-a:8080"
  resources: # ${dependencies.resources} - resource dependencies
    - ref: "orders-db"                        # ${dependencies.resources[0].ref} - Resource name
      envVars: # ${dependencies.resources[0].envVars} - per-resource env vars (literal + valueFrom)
        - name: "DB_HOST"
          value: "10.0.0.5"
        - name: "DB_PASSWORD"
          valueFrom:
            secretKeyRef:
              name: "orders-db-creds"
              key: "password"
      volumeMounts: # ${dependencies.resources[0].volumeMounts} - per-resource mounts
        - name: "r-9f3c4a21"
          mountPath: "/etc/db/ca.crt"
          subPath: "ca.crt"
      volumes: # ${dependencies.resources[0].volumes} - per-resource volumes (deduped per source)
        - name: "r-9f3c4a21"
          secret:
            secretName: "orders-db-creds"
  envVars: # ${dependencies.envVars} - merged: ALL env vars from items[] + resources[]
    - name: "SVC_A_URL"
      value: "http://svc-a:8080"
    - name: "DB_HOST"
      value: "10.0.0.5"
    - name: "DB_PASSWORD"
      valueFrom:
        secretKeyRef:
          name: "orders-db-creds"
          key: "password"
  volumeMounts: # ${dependencies.volumeMounts} - merged from resources[]
    - name: "r-9f3c4a21"
      mountPath: "/etc/db/ca.crt"
      subPath: "ca.crt"
  volumes: # ${dependencies.volumes} - merged from resources[]
    - name: "r-9f3c4a21"
      secret:
        secretName: "orders-db-creds"
```

**Structure details:**

| Field                       | Type                       | Description                                                                             |
|-----------------------------|----------------------------|-----------------------------------------------------------------------------------------|
| `dependencies.items`        | `[]ConnectionItem`         | Resolved endpoint connections with metadata and per-item env vars                       |
| `dependencies.resources`    | `[]ResourceDependencyItem` | Resolved resource dependencies with per-item env vars, volume mounts, and volumes       |
| `dependencies.envVars`      | `[]EnvVar`                 | Merged flat list of env vars from `items` and `resources`                               |
| `dependencies.volumeMounts` | `[]VolumeMount`            | Merged flat list of volume mounts from `resources` (endpoint connections add no mounts) |
| `dependencies.volumes`      | `[]Volume`                 | Merged flat list of volumes from `resources` (endpoint connections add no volumes)      |

**ConnectionItem structure:**

| Field        | Type       | Description                                                                      |
|--------------|------------|----------------------------------------------------------------------------------|
| `namespace`  | `string`   | Target component's control plane namespace                                       |
| `project`    | `string`   | Target project name                                                              |
| `component`  | `string`   | Target component name                                                            |
| `endpoint`   | `string`   | Target endpoint name                                                             |
| `visibility` | `string`   | Resolved visibility level (e.g., `project`, `namespace`, `internal`, `external`) |
| `envVars`    | `[]EnvVar` | Environment variables resolved for this connection                               |

**ResourceDependencyItem structure:**

| Field          | Type            | Description                                                                                            |
|----------------|-----------------|--------------------------------------------------------------------------------------------------------|
| `ref`          | `string`        | Resource name (matches `Workload.spec.dependencies.resources[].ref`)                                   |
| `envVars`      | `[]EnvVar`      | Env vars projected from outputs declared in `envBindings`. Each is literal or `valueFrom`.             |
| `volumeMounts` | `[]VolumeMount` | Volume mounts projected from outputs declared in `fileBindings` (one per binding)                      |
| `volumes`      | `[]Volume`      | Volumes backing those mounts. Deduped per `(ref, sourceKind, sourceName)` so multiple mounts can share |

**EnvVar structure** (JSON-compatible with Kubernetes `corev1.EnvVar`):

| Field       | Type           | Description                                                                                             |
|-------------|----------------|---------------------------------------------------------------------------------------------------------|
| `name`      | `string`       | Environment variable name                                                                               |
| `value`     | `string`       | Literal value. Set when the source output is `value:`. Mutually exclusive with `valueFrom`.             |
| `valueFrom` | `EnvVarSource` | Reference value with `secretKeyRef` or `configMapKeyRef`. Set for ref-kind outputs. Mutually exclusive. |

**VolumeMount structure** (JSON-compatible with `corev1.VolumeMount`):

| Field       | Type     | Description                                                                             |
|-------------|----------|-----------------------------------------------------------------------------------------|
| `name`      | `string` | Volume name. For resource deps, the platform produces an `r-<hash>` deterministic name. |
| `mountPath` | `string` | In-container mount path. Comes from the workload's `fileBindings` value.                |
| `subPath`   | `string` | Key within the Secret/ConfigMap to project at this mount.                               |

**Volume structure** (JSON-compatible with `corev1.Volume`, modeling only the kinds the platform emits):

| Field       | Type           | Description                                   |
|-------------|----------------|-----------------------------------------------|
| `name`      | `string`       | Volume name (matches a `volumeMounts[].name`) |
| `secret`    | `{secretName}` | Set when the backing source is a Secret       |
| `configMap` | `{name}`       | Set when the backing source is a ConfigMap    |

**Example usage:**

```yaml
# Inject all dependency env vars (literal + valueFrom) into a container
env: |
  ${dependencies.envVars.map(e, has(e.value)
    ? {"name": e.name, "value": e.value}
    : {"name": e.name, "valueFrom": e.valueFrom})}
```

Iterate over individual endpoint connections:

```yaml
forEach: ${dependencies.items}
var: dep
template:
# Use dep.component, dep.endpoint, dep.envVars, etc.
```

Iterate over individual resource dependencies:

```yaml
forEach: ${dependencies.resources}
var: rdep
template:
# Use rdep.ref, rdep.envVars, rdep.volumeMounts, rdep.volumes
```

**Note:** If no dependencies are configured, every list under `dependencies` is empty (never null), so
CEL expressions like `dependencies.envVars.size()` and `dependencies.volumes.size()` are always safe.

### Dependency Helper Methods

The `dependencies` object provides helper methods that return the merged flat surfaces in the shapes
templates typically need.

| Helper Method                            | Description                                                                                                   |
|------------------------------------------|---------------------------------------------------------------------------------------------------------------|
| `dependencies.toContainerEnvs()`         | Returns the merged env var list (equivalent to `dependencies.envVars`) for `container.env`                    |
| `dependencies.toContainerVolumeMounts()` | Returns the merged volume mount list (equivalent to `dependencies.volumeMounts`) for `container.volumeMounts` |
| `dependencies.toVolumes()`               | Returns the merged volume list (equivalent to `dependencies.volumes`) for `pod.volumes`                       |

These return types are list-compatible with the matching `configurations.*` helpers, so combining the
two with the `+` operator type-checks in CEL: e.g.
`${configurations.toContainerVolumeMounts() + dependencies.toContainerVolumeMounts()}`.

**Example usage:**

```yaml
# Combined env vars from configurations + dependencies, plus merged volumes / mounts.
# envFrom carries configuration-derived envs (configMapRef + secretRef); env carries
# dependency envs (literal + valueFrom).
spec:
  template:
    spec:
      volumes: ${configurations.toVolumes() + dependencies.toVolumes()}
      containers:
        - name: app
          envFrom: ${configurations.toContainerEnvFrom()}
          env: ${dependencies.toContainerEnvs()}
          volumeMounts: ${configurations.toContainerVolumeMounts() + dependencies.toContainerVolumeMounts()}
```

### Configuration Helper Methods

The `configurations` object provides several helper methods to simplify working with container configurations,
environment variables, and file mounts. These helpers reduce boilerplate and make templates more readable.

**Available Helper Methods:**

| Helper Method                              | Description                                               |
|--------------------------------------------|-----------------------------------------------------------|
| `configurations.toContainerEnvFrom()`      | Generates `envFrom` array with configMapRef and secretRef |
| `configurations.toConfigEnvsByContainer()` | Returns list of config environment variables              |
| `configurations.toSecretEnvsByContainer()` | Returns list of secret environment variables              |
| `configurations.toConfigFileList()`        | Flattens all config files into a single list              |
| `configurations.toSecretFileList()`        | Flattens all secret files into a single list              |
| `configurations.toContainerVolumeMounts()` | Generates volumeMounts array for the container's files    |
| `configurations.toVolumes()`               | Generates volumes array for all files                     |

For detailed documentation, examples, and usage patterns for each helper method,
see [Configuration Helpers](./configuration_helpers.md).

**Quick Example:**

```yaml
# Using helper methods for cleaner templates
spec:
  template:
    spec:
      containers:
        - name: main
          image: myapp:latest
          envFrom: ${configurations.toContainerEnvFrom()}
          volumeMounts: ${configurations.toContainerVolumeMounts()}
      volumes: ${configurations.toVolumes()}
```

## TraitContext

TraitContext is used when rendering Trait creates and patches. It provides access to metadata, trait-specific
information, parameters (from trait instance), and environment overrides (from ReleaseBinding).

### Available in Traits

| Location                       | Context                      | Notes                             |
|--------------------------------|------------------------------|-----------------------------------|
| `validations[].rule`           | TraitContext                 | Evaluated before creates/patches  |
| `creates[].template`           | TraitContext                 | Full trait context                |
| `patches[].operations[].path`  | TraitContext                 | Path can contain expressions      |
| `patches[].operations[].value` | TraitContext                 | Value can contain expressions     |
| `patches[].forEach`            | TraitContext                 | Expression to iterate over        |
| Inside forEach iteration       | TraitContext + loop variable | Loop variable added               |
| `patches[].target.where`       | TraitContext + `resource`    | Special `resource` variable added |

### metadata

Same structure as ComponentContext metadata. See [metadata](#metadata) above.

### trait

Trait-specific metadata identifying the trait and its instance.

```yaml
# Access pattern: ${trait.<field>}

trait:
  name: "storage"                # ${trait.name} - Trait CRD name
  instanceName: "my-storage"     # ${trait.instanceName} - Instance name in Component
```

**Example usage:**

```yaml
# In Trait creates template
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ${oc_generate_name(metadata.name, trait.instanceName)}
  labels:
    trait: ${trait.name}
    instance: ${trait.instanceName}
```

### dataplane

DataPlane configuration for the target environment. Same structure as ComponentContext. See
[dataplane](#dataplane) above for the full shape, including the nested `gateway.ingress` /
`gateway.egress` listeners.

The fields `secretStore`, `gateway`, and `observabilityPlaneRef` are optional; use `has()` to
guard conditional logic.

### parameters

Trait instance parameters from `Component.Spec.Traits[].Parameters`, pruned to the Trait's `schema.parameters` section
with defaults applied. Use for static configuration that doesn't change across environments.

```yaml
# Given this schema in Trait:
schema:
  parameters:
    volumeName: "string"
    mountPath: "string"
    containerName: "string | default=app"

# And this trait instance in Component:
traits:
  - name: persistent-volume
    instanceName: data-storage
    parameters:
      volumeName: "app-data"
      mountPath: "/var/data"

# The parameters context would be:
parameters:
  volumeName: "app-data"         # ${parameters.volumeName} (from trait instance)
  mountPath: "/var/data"         # ${parameters.mountPath} (from trait instance)
  containerName: "app"           # ${parameters.containerName} (default)
```

### environmentConfigs

Environment-specific overrides from `ReleaseBinding.Spec.TraitEnvironmentConfigs[instanceName]`, pruned to the Trait's
`schema.environmentConfigs` section with defaults applied. Use for values that vary per environment.

```yaml
# Given this schema in Trait:
schema:
  environmentConfigs:
    size: "string | default=10Gi"
    storageClass: "string | default=standard"

# And this ReleaseBinding:
spec:
  traitEnvironmentConfigs:
    data-storage: # keyed by instanceName
      size: "50Gi"
      storageClass: "fast-ssd"

# The environmentConfigs context would be:
environmentConfigs:
  size: "50Gi"                   # ${environmentConfigs.size} (from ReleaseBinding)
  storageClass: "fast-ssd"       # ${environmentConfigs.storageClass} (from ReleaseBinding)
```

**Example usage:**

```yaml
# In Trait creates template
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ${metadata.name}-${trait.instanceName}
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: ${environmentConfigs.size}
  storageClassName: ${environmentConfigs.storageClass}
```

### workload

Workload specification containing container and endpoint information. Same structure as ComponentContext workload.
See [workload](#workload) section above for full details.

```yaml
# Access pattern: ${workload.<field>}

workload:
  container:
    image: "myregistry/myapp:v1.0"
    command: [ "./start.sh" ]
    args: [ "--port", "8080" ]
  endpoints:
    http:
      type: "HTTP"
      port: 8080
```

### configurations

Configuration items (environment variables and files) extracted from workload. Same structure as ComponentContext
configurations. See [configurations](#configurations) section above for full details and helper methods.

```yaml
# Access pattern: ${configurations.<field>}

configurations: # ${configurations}
  configs: # ${configurations.configs}
    envs: [ ... ]                           # ${configurations.configs.envs}
    files: [ ... ]                          # ${configurations.configs.files}
  secrets: # ${configurations.secrets}
    envs: [ ... ]                           # ${configurations.secrets.envs}
    files: [ ... ]                          # ${configurations.secrets.files}
```

**Available helper methods** (same as ComponentContext):

- `configurations.toContainerEnvFrom()`
- `configurations.toConfigEnvsByContainer()`
- `configurations.toSecretEnvsByContainer()`
- `configurations.toConfigFileList()`
- `configurations.toSecretFileList()`
- `configurations.toContainerVolumeMounts()`
- `configurations.toVolumes()`

### dependencies

Dependency information covering both endpoint connections and resource dependencies. Same structure
as ComponentContext dependencies. See [dependencies](#dependencies) section above for full details
and helper methods.

```yaml
# Access pattern: ${dependencies.<field>}

dependencies: # ${dependencies}
  items: [ ... ]                               # ${dependencies.items} - endpoint connections (metadata + env vars)
  resources: [ ... ]                           # ${dependencies.resources} - resource deps (env vars + mounts + volumes)
  envVars: [ ... ]                             # ${dependencies.envVars} - merged from items + resources
  volumeMounts: [ ... ]                        # ${dependencies.volumeMounts} - merged from resources
  volumes: [ ... ]                             # ${dependencies.volumes} - merged from resources
```

**Available helper methods** (same as ComponentContext):

- `dependencies.toContainerEnvs()`
- `dependencies.toContainerVolumeMounts()`
- `dependencies.toVolumes()`

## Special Variables

### Loop Variables (forEach)

When using `forEach`, the loop variable is added to the context for each iteration.

```yaml
# ComponentType resource with forEach
resources:
  - forEach: ${parameters.ports}
    var: port                    # Loop variable name (defaults to "item")
    resource:
      apiVersion: v1
      kind: Service
      metadata:
        name: ${oc_generate_name(metadata.name, port.name)}
      spec:
        ports:
          - port: ${port.port}   # Access loop variable
            name: ${port.name}

# Trait patch with forEach
patches:
  - forEach: ${parameters.volumes}
    var: vol
    target:
      kind: Deployment
    operations:
      - op: add
        path: /spec/template/spec/volumes/-
        value:
          name: ${vol.name}
          persistentVolumeClaim:
            claimName: ${vol.claimName}
```

### resource Variable (where clause)

In trait patch `where` clauses, the `resource` variable provides access to the target resource being evaluated.

```yaml
patches:
  - target:
      kind: Deployment
      # Filter to only patch deployments with specific label
      where: ${resource.metadata.labels["app.kubernetes.io/component"] == "backend"}
    operations:
      - op: add
        path: /spec/template/spec/containers/0/env/-
        value:
          name: BACKEND_MODE
          value: "true"

  - target:
      kind: Service
      # Filter based on service type
      where: ${resource.spec.type == "LoadBalancer"}
    operations:
      - op: add
        path: /metadata/annotations/service.beta.kubernetes.io~1aws-load-balancer-type
        value: "nlb"
```

**Available in `resource`:**

The entire rendered Kubernetes resource is available, including:

- `resource.apiVersion`
- `resource.kind`
- `resource.metadata` (name, namespace, labels, annotations, etc.)
- `resource.spec` (resource-specific specification)

## Context Comparison

| Variable               | ComponentContext                                        | TraitContext                                    |
|------------------------|---------------------------------------------------------|-------------------------------------------------|
| `metadata.*`           | ✅                                                       | ✅                                               |
| `parameters.*`         | ✅ (from Component.Spec.Parameters)                      | ✅ (from Trait instance)                         |
| `environmentConfigs.*` | ✅ (from ReleaseBinding.ComponentTypeEnvironmentConfigs) | ✅ (from ReleaseBinding.TraitEnvironmentConfigs) |
| `dataplane.*`          | ✅                                                       | ✅                                               |
| `gateway.*`            | ✅                                                       | ✅                                               |
| `environment.*`        | ✅                                                       | ✅                                               |
| `workload.*`           | ✅                                                       | ✅                                               |
| `configurations.*`     | ✅                                                       | ✅                                               |
| `dependencies.*`       | ✅                                                       | ✅                                               |
| `trait.*`              | ❌                                                       | ✅                                               |
| Loop variable          | ✅ (in forEach)                                          | ✅ (in forEach)                                  |
| `resource`             | ❌                                                       | ✅ (in where only)                               |
