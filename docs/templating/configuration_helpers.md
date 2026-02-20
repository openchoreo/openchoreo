# Configuration Helpers

This document provides detailed documentation for all configuration helper methods available in the templating context. These helpers simplify working with container configurations, environment variables, and file mounts.

## Overview

Configuration helpers are CEL extension functions that provide convenient methods to work with the `configurations` object in your templates. They help reduce boilerplate code and make templates more readable and maintainable.

## Helper Methods

### Environment Variable Helpers

#### configurations.toContainerEnvFrom()

Helper method that generates an `envFrom` array for the container configuration. This simplifies the creation of `configMapRef` and `secretRef` entries based on what environment variables are available.

**Parameters:** None

**Returns:** List of envFrom entries, each containing either:

| Field | Type | Description |
|-------|------|-------------|
| `configMapRef` | map | Reference to ConfigMap (only present if container has config envs) |
| `secretRef` | map | Reference to Secret (only present if container has secret envs) |

**Example usage:**

```yaml
# Simple envFrom generation using the helper
spec:
  template:
    spec:
      containers:
        - name: main
          image: myapp:latest
          envFrom: ${configurations.toContainerEnvFrom()}

# Before - verbose manual logic:
envFrom: |
  ${(has(configurations.configs.envs) && configurations.configs.envs.size() > 0 ?
    [{
      "configMapRef": {
        "name": oc_generate_name(metadata.name, "env-configs")
      }
    }] : []) +
  (has(configurations.secrets.envs) && configurations.secrets.envs.size() > 0 ?
    [{
      "secretRef": {
        "name": oc_generate_name(metadata.name, "env-secrets")
      }
    }] : [])}

# After - clean helper usage:
envFrom: ${configurations.toContainerEnvFrom()}
```

**Combining with additional entries:**

```yaml
# Can be combined with CEL operations
envFrom: |
  ${configurations.toContainerEnvFrom() +
    [{"configMapRef": {"name": "additional-config"}}]}
```

#### configurations.toConfigEnvsByContainer()

Helper method that generates a list of objects for creating ConfigMaps from environment variables. Each object contains the generated resource name and the list of environment variables for the container. This is useful for `forEach` iteration when creating ConfigMaps for the container's config envs.

**Parameters:** None

**Returns:** List of objects, each containing:

| Field | Type | Description |
|-------|------|-------------|
| `resourceName` | string | Generated ConfigMap name (componentName-environmentName-env-configs-hash) |
| `envs` | array | List of environment variable objects with `name` and `value` |

**Example usage:**

```yaml
# Generate ConfigMaps for the container's config envs
- id: env-config
  forEach: ${configurations.toConfigEnvsByContainer()}
  var: envConfig
  template:
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: ${envConfig.resourceName}
      namespace: ${metadata.namespace}
    data: |
      ${envConfig.envs.transformMapEntry(index, env, {env.name: env.value})}

# Before - verbose manual logic:
forEach: |
  ${has(configurations.configs.envs) && configurations.configs.envs.size() > 0 ?
    [{
      "resourceName": oc_generate_name(metadata.name, "env-configs"),
      "envs": configurations.configs.envs
    }] : []}

# After - clean helper usage:
forEach: ${configurations.toConfigEnvsByContainer()}
```

**Notes:**
- Only returns an entry if the container has config environment variables
- Returns empty list if there are no config envs or only secret envs
- Generated resource names follow the format: `componentName-environmentName-env-configs-hash`

#### configurations.toSecretEnvsByContainer()

Helper method that generates a list of objects for creating ExternalSecrets from secret environment variables. Each object contains the generated resource name and the list of secret environment variables for the container. This is useful for `forEach` iteration when creating ExternalSecrets for the container's secret envs.

The resource names are automatically generated using `metadata.componentName + "-" + metadata.environmentName` as the prefix.

**Parameters:** None

**Returns:** List of objects, each containing:

| Field | Type | Description |
|-------|------|-------------|
| `resourceName` | string | Generated ExternalSecret name (componentName-environmentName-env-secrets-hash) |
| `envs` | array | List of secret environment variable objects with `name` and `remoteRef` |

**Example usage:**

```yaml
# Generate ExternalSecrets for the container's secret envs
- id: secret-env-external
  forEach: ${configurations.toSecretEnvsByContainer()}
  var: secretEnv
  template:
    apiVersion: external-secrets.io/v1
    kind: ExternalSecret
    metadata:
      name: ${secretEnv.resourceName}
      namespace: ${metadata.namespace}
    spec:
      refreshInterval: 15s
      secretStoreRef:
        name: ${dataplane.secretStore}
        kind: ClusterSecretStore
      target:
        name: ${secretEnv.resourceName}
        creationPolicy: Owner
      data: |
        ${secretEnv.envs.map(secret, {
          "secretKey": secret.name,
          "remoteRef": {
            "key": secret.remoteRef.key,
            "property": has(secret.remoteRef.property) ? secret.remoteRef.property : oc_omit()
          }
        })}

# Before - verbose manual logic:
forEach: |
  ${has(configurations.secrets.envs) && configurations.secrets.envs.size() > 0 ?
    [{
      "resourceName": oc_generate_name(metadata.name, "env-secrets"),
      "envs": configurations.secrets.envs
    }] : []}

# After - clean helper usage:
forEach: ${configurations.toSecretEnvsByContainer()}
```

**Notes:**
- Only returns an entry if the container has secret environment variables
- Returns empty list if there are no secret envs or only config envs
- Generated resource names follow the format: `componentName-environmentName-env-secrets-hash`

### File Configuration Helpers

#### configurations.toConfigFileList()

Helper method that extracts `configs.files` from the `configurations` object into a list, useful for `forEach` iteration. This provides config files from the workload container.

**Parameters:** None

**Returns:** List of maps, each containing:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | File name |
| `mountPath` | string | Mount path |
| `value` | string | File content (empty string if using remoteRef) |
| `resourceName` | string | Generated Kubernetes-compliant resource name (componentName-environmentName-config-fileName) |
| `remoteRef` | map | Remote reference (only present if the file uses a secret reference) |

**Example usage:**

```yaml
# Generate a ConfigMap for each config file
resources:
  - id: file-configs
    forEach: ${configurations.toConfigFileList()}
    var: config
    template:
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: ${config.resourceName}
        namespace: ${metadata.namespace}
      data:
        ${config.name}: |
          ${config.value}
```

**Equivalent CEL expression:**

If you need additional fields (e.g., `container` name) or different behavior, use the underlying data directly:

```yaml
forEach: |
  ${configurations.transformList(containerName, cfg,
    cfg.configs.files.map(f, oc_merge(f, {
      "container": containerName,
      "resourceName": oc_generate_name(metadata.name, containerName, "config", f.name.replace(".", "-"))
    }))
  ).flatten()}
```

#### configurations.toSecretFileList()

Helper method that extracts `secrets.files` from the `configurations` object into a list, useful for `forEach` iteration. This provides secret files from the workload container.

The resource names are automatically generated using `metadata.componentName + "-" + metadata.environmentName` as the prefix.

**Parameters:** None

**Returns:** List of maps, each containing:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | File name |
| `mountPath` | string | Mount path |
| `value` | string | File content (empty string if using remoteRef) |
| `resourceName` | string | Generated Kubernetes-compliant resource name (componentName-environmentName-secret-fileName) |
| `remoteRef` | map | Remote reference (only present if the file uses a secret reference) |

**Example usage:**

```yaml
# Generate an ExternalSecret for each secret file
resources:
  - id: file-secrets
    forEach: ${configurations.toSecretFileList()}
    var: secret
    includeWhen: ${has(secret.remoteRef)}
    template:
      apiVersion: external-secrets.io/v1beta1
      kind: ExternalSecret
      metadata:
        name: ${secret.resourceName}
        namespace: ${metadata.namespace}
      spec:
        secretStoreRef:
          name: ${dataplane.secretStore}
          kind: ClusterSecretStore
        target:
          name: ${secret.resourceName}
          creationPolicy: Owner
        data:
          - secretKey: ${secret.name}
            remoteRef:
              key: ${secret.remoteRef.key}
              property: ${secret.remoteRef.property}

  # Generate a Secret for files with inline values
  - id: inline-file-secrets
    forEach: ${configurations.toSecretFileList()}
    var: secret
    includeWhen: ${!has(secret.remoteRef) && secret.value != ""}
    template:
      apiVersion: v1
      kind: Secret
      metadata:
        name: ${secret.resourceName}
        namespace: ${metadata.namespace}
      data:
        ${secret.name}: ${base64.encode(secret.value)}
```

**Equivalent CEL expression:**

If you need additional fields (e.g., `container` name) or different behavior, use the underlying data directly:

```yaml
forEach: |
  ${configurations.transformList(containerName, cfg,
    cfg.secrets.files.map(f, oc_merge(f, {
      "container": containerName,
      "resourceName": oc_generate_name(metadata.name, containerName, "secret", f.name.replace(".", "-"))
    }))
  ).flatten()}
```

### Volume Mount Helpers

#### configurations.toContainerVolumeMounts()

Helper method that generates a `volumeMounts` array for the default container configuration. This simplifies the creation of volume mount entries based on the container's config and secret files. The helper automatically selects the `"default"` container from configurations.

**Parameters:** None

**Returns:** List of volumeMount entries, each containing:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Volume name (file-mount-hash format) |
| `mountPath` | string | Full mount path (mountPath + "/" + filename) |
| `subPath` | string | Filename to mount as subPath |

**Example usage:**

```yaml
# Simple volumeMounts generation using the helper
spec:
  template:
    spec:
      containers:
        - name: main
          image: myapp:latest
          volumeMounts: ${configurations.toContainerVolumeMounts()}

# Before - verbose manual logic:
volumeMounts: |
  ${has(configurations.configs.files) && configurations.configs.files.size() > 0 || has(configurations.secrets.files) && configurations.secrets.files.size() > 0 ?
    (has(configurations.configs.files) && configurations.configs.files.size() > 0 ?
      configurations.configs.files.map(f, {
        "name": "file-mount-"+oc_hash(f.mountPath+"/"+f.name),
        "mountPath": f.mountPath+"/"+f.name ,
        "subPath": f.name
      }) : []) +
    (has(configurations.secrets.files) && configurations.secrets.files.size() > 0 ?
      configurations.secrets.files.map(f, {
        "name": "file-mount-"+oc_hash(f.mountPath+"/"+f.name),
        "mountPath": f.mountPath+"/"+f.name,
        "subPath": f.name
      }) : [])
  : oc_omit()}

# After - clean helper usage:
volumeMounts: ${configurations.toContainerVolumeMounts()}
```

**Combining with additional mounts:**

```yaml
# Can be combined with CEL operations
volumeMounts: |
  ${configurations.toContainerVolumeMounts() +
    [{"name": "extra-mount", "mountPath": "/extra", "subPath": "extra.txt"}]}
```

#### configurations.toVolumes()

Helper method that generates a `volumes` array for all files in the configurations. This simplifies the creation of volume definitions based on all config and secret files.

The resource names are automatically generated using `metadata.componentName + "-" + metadata.environmentName` as the prefix.

**Parameters:** None

**Returns:** List of volume entries, each containing:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Volume name (generated using hash of mountPath and filename) |
| `configMap` | map | ConfigMap volume source (only present for config files) |
| `secret` | map | Secret volume source (only present for secret files) |

**Example usage:**

```yaml
# Simple volumes generation using the helper
spec:
  template:
    spec:
      containers:
        - name: main
          image: myapp:latest
          volumeMounts: ${configurations.toContainerVolumeMounts()}
      volumes: ${configurations.toVolumes()}

# Before - verbose manual logic:
volumes: |
  ${has(configurations.configs.files) && configurations.configs.files.size() > 0 || has(configurations.secrets.files) && configurations.secrets.files.size() > 0 ?
    (has(configurations.configs.files) && configurations.configs.files.size() > 0 ?
      configurations.configs.files.map(f, {
        "name": "file-mount-"+oc_hash(f.mountPath+"/"+f.name),
        "configMap": {
          "name": oc_generate_name(metadata.name, "config", f.name).replace(".", "-")
        }
      }) : []) +
    (has(configurations.secrets.files) && configurations.secrets.files.size() > 0 ?
      configurations.secrets.files.map(f, {
        "name": "file-mount-"+oc_hash(f.mountPath+"/"+f.name),
        "secret": {
          "secretName": oc_generate_name(metadata.name, "secret", f.name).replace(".", "-")
        }
      }) : [])
  : oc_omit()}

# After - clean helper usage:
volumes: ${configurations.toVolumes()}
```

**Combining with additional volumes:**

```yaml
# Can be combined with inline volumes
volumes: |
  ${configurations.toVolumes() +
    [{"name": "extra-volume", "emptyDir": {}}]}
```

### Workload Endpoint Helpers

#### workload.toServicePorts()

Helper method that converts the `workload.endpoints` map into a list of Service port definitions. This simplifies Service generation by automatically creating ports based on the workload's endpoint configuration.

**Parameters:** None

**Returns:** List of Service port objects, each containing:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Sanitized endpoint name (lowercase, alphanumeric + hyphens) |
| `port` | int | Port number from endpoint configuration |
| `targetPort` | int | Target port (same as port) |
| `protocol` | string | Kubernetes protocol (TCP or UDP) |

**Protocol mapping:**
- HTTP, REST, gRPC, GraphQL, Websocket → TCP
- TCP → TCP
- UDP → UDP

**Example usage:**

```yaml
# Simple Service port generation from workload endpoints
- id: service
  includeWhen: ${size(workload.endpoints) > 0}
  template:
    apiVersion: v1
    kind: Service
    metadata:
      name: ${metadata.name}
      namespace: ${metadata.namespace}
    spec:
      selector: ${metadata.podSelectors}
      ports: ${workload.toServicePorts()}

# Before - verbose manual logic:
ports: |
  ${workload.endpoints.transformList(name, ep, {
    "name": name.toLowerCase().replace("_", "-"),
    "port": ep.port,
    "targetPort": ep.port,
    "protocol": ep.type == "UDP" ? "UDP" : "TCP"
  })}

# After - clean helper usage:
ports: ${workload.toServicePorts()}
```

**With multiple endpoints:**

```yaml
# Given this workload:
workload:
  endpoints:
    http:
      type: HTTP
      port: 8080
    grpc:
      type: gRPC
      port: 9090

# The helper generates:
ports:
  - name: http
    port: 8080
    targetPort: 8080
    protocol: TCP
  - name: grpc
    port: 9090
    targetPort: 9090
    protocol: TCP
```

**Dynamic port references in HTTPRoute:**

```yaml
# Reference a specific endpoint's port
backendRefs:
  - name: ${metadata.componentName}
    port: ${workload.endpoints.http.port}

# Or use the first port from the service ports list
backendRefs:
  - name: ${metadata.componentName}
    port: ${workload.toServicePorts()[0].port}
```

**Notes:**
- Returns an empty list if `workload.endpoints` is empty
- Endpoint names are sanitized for Kubernetes port naming (lowercase alphanumeric + hyphens, max 15 chars)
- Duplicate names after sanitization get unique numeric suffixes (e.g., http, http-2, http-3)
- Both `port` and `targetPort` use the same value from the endpoint configuration
- Endpoints are processed in alphabetical order for deterministic output
- Use with `includeWhen: ${size(workload.endpoints) > 0}` to conditionally create Services only when endpoints exist

## See Also

- [Context Reference](./context.md) - Main context documentation
