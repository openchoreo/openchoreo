# Configuration Helpers

This document provides detailed documentation for all configuration helper methods available in the templating context. These helpers simplify working with container configurations, environment variables, and file mounts.

## Overview

Configuration helpers are CEL extension functions that provide convenient methods to work with the `configurations` object in your templates. They help reduce boilerplate code and make templates more readable and maintainable.

## Helper Methods

### Environment Variable Helpers

#### configurations[containerName].envFrom(prefix)

Helper method that generates an `envFrom` array for a single container configuration. This simplifies the creation of `configMapRef` and `secretRef` entries based on what environment variables are available.

**Parameters:**
- `prefix` - String used to generate unique resource names (typically `metadata.name`)

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
          envFrom: ${configurations["main"].envFrom(metadata.name)}

# Before - verbose manual logic:
envFrom: |
  ${(has(configurations["main"].configs.envs) && configurations["main"].configs.envs.size() > 0 ?
    [{
      "configMapRef": {
        "name": oc_generate_name(metadata.name, "env-configs")
      }
    }] : []) +
  (has(configurations["main"].secrets.envs) && configurations["main"].secrets.envs.size() > 0 ?
    [{
      "secretRef": {
        "name": oc_generate_name(metadata.name, "env-secrets")
      }
    }] : [])}

# After - clean helper usage:
envFrom: ${configurations["main"].envFrom(metadata.name)}
```

**Dynamic container names:**

```yaml
# Works with dynamic container names from parameters
envFrom: ${configurations[parameters.containerName].envFrom(metadata.name)}

# Can be combined with CEL operations
envFrom: |
  ${configurations["main"].envFrom(metadata.name) + 
    [{"configMapRef": {"name": "additional-config"}}]}
```

#### configurations.toConfigEnvList(resourceNamePrefix)

Helper method that generates a list of objects for creating ConfigMaps from environment variables. Each object contains the container name, generated resource name, and the list of environment variables for that container. This is useful for `forEach` iteration when creating ConfigMaps for each container's config envs.

**Parameters:**
- `resourceNamePrefix` - String used to generate unique resource names (typically `metadata.name`)

**Returns:** List of objects, each containing:

| Field | Type | Description |
|-------|------|-------------|
| `container` | string | Name of the container |
| `resourceName` | string | Generated ConfigMap name (prefix-containerName-env-configs-hash) |
| `envs` | array | List of environment variable objects with `name` and `value` |

**Example usage:**

```yaml
# Generate ConfigMaps for each container's config envs
- id: env-config
  forEach: ${configurations.toConfigEnvList(metadata.name)}
  var: envConfig
  template:
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: ${envConfig.resourceName}
      namespace: ${metadata.namespace}
    data: |
      ${envConfig.envs.transformMapEntry(index, env, {env.name: env.value})}

# Before - verbose manual logic with transformList:
forEach: |
  ${configurations.transformList(containerName, cfg, 
    {
      "container": containerName,
      "resourceName": oc_generate_name(metadata.name, containerName, "env-configs"),
      "envs": cfg.configs.envs
    }
  )}

# After - clean helper usage:
forEach: ${configurations.toConfigEnvList(metadata.name)}
```

**Notes:**
- Only returns entries for containers that have config environment variables
- Skips containers with no config envs or only secret envs
- Generated resource names include container name and a hash for uniqueness
- Works seamlessly with `transformMapEntry` helper for creating the ConfigMap data

#### configurations.toSecretEnvList(resourceNamePrefix)

Helper method that generates a list of objects for creating ExternalSecrets from secret environment variables. Each object contains the container name, generated resource name, and the list of secret environment variables for that container. This is useful for `forEach` iteration when creating ExternalSecrets for each container's secret envs.

**Parameters:**
- `resourceNamePrefix` - String used to generate unique resource names (typically `metadata.name`)

**Returns:** List of objects, each containing:

| Field | Type | Description |
|-------|------|-------------|
| `container` | string | Name of the container |
| `resourceName` | string | Generated ExternalSecret name (prefix-containerName-env-secrets-hash) |
| `envs` | array | List of secret environment variable objects with `name` and `remoteRef` |

**Example usage:**

```yaml
# Generate ExternalSecrets for each container's secret envs
- id: secret-env-external
  forEach: ${configurations.toSecretEnvList(metadata.name)}
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

# Before - verbose manual logic with transformList:
forEach: |
  ${configurations.transformList(containerName, cfg, 
    {
      "container": containerName,
      "resourceName": oc_generate_name(metadata.name, containerName, "env-secrets"),
      "envs": cfg.secrets.envs
    }
  )}

# After - clean helper usage:
forEach: ${configurations.toSecretEnvList(metadata.name)}
```

**Notes:**
- Only returns entries for containers that have secret environment variables
- Skips containers with no secret envs or only config envs
- Generated resource names include container name and a hash for uniqueness
- Works with ExternalSecret resources that reference remote secret stores

### File Configuration Helpers

#### configurations.toConfigFileList(prefix)

Helper method that flattens `configs.files` from all containers in the `configurations` object into a single list, useful for `forEach` iteration. This aggregates config files across all workload containers.

**Parameters:**
- `prefix` - String used to generate unique resource names (typically `metadata.name`)

**Returns:** List of maps, each containing:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | File name |
| `mountPath` | string | Mount path |
| `value` | string | File content (empty string if using remoteRef) |
| `resourceName` | string | Generated Kubernetes-compliant resource name |
| `remoteRef` | map | Remote reference (only present if the file uses a secret reference) |

**Example usage:**

```yaml
# Generate a ConfigMap for each config file across all containers
resources:
  - id: file-configs
    forEach: ${configurations.toConfigFileList(metadata.name)}
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

#### configurations.toSecretFileList(prefix)

Helper method that flattens `secrets.files` from all containers in the `configurations` object into a single list, useful for `forEach` iteration. This aggregates secret files across all workload containers.

**Parameters:**
- `prefix` - String used to generate unique resource names (typically `metadata.name`)

**Returns:** List of maps, each containing:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | File name |
| `mountPath` | string | Mount path |
| `value` | string | File content (empty string if using remoteRef) |
| `resourceName` | string | Generated Kubernetes-compliant resource name |
| `remoteRef` | map | Remote reference (only present if the file uses a secret reference) |

**Example usage:**

```yaml
# Generate an ExternalSecret for each secret file across all containers
resources:
  - id: file-secrets
    forEach: ${configurations.toSecretFileList(metadata.name)}
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
    forEach: ${configurations.toSecretFileList(metadata.name)}
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

#### configurations.toContainerVolumeMounts(containerName)

Helper method that generates a `volumeMounts` array for a single container configuration. This simplifies the creation of volume mount entries based on the container's config and secret files.

**Parameters:**
- `containerName` - String name of the container to generate volume mounts for

**Returns:** List of volumeMount entries, each containing:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Volume name (containerName-file-mount-hash format) |
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
          volumeMounts: ${configurations.toContainerVolumeMounts("main")}

# Before - verbose manual logic:
volumeMounts: |
  ${has(configurations["main"].configs.files) && configurations["main"].configs.files.size() > 0 || has(configurations["main"].secrets.files) && configurations["main"].secrets.files.size() > 0 ?
    (has(configurations["main"].configs.files) && configurations["main"].configs.files.size() > 0 ?
      configurations["main"].configs.files.map(f, {
        "name": "main-file-mount-"+oc_hash(f.mountPath+"/"+f.name),
        "mountPath": f.mountPath+"/"+f.name ,
        "subPath": f.name
      }) : []) +
    (has(configurations["main"].secrets.files) && configurations["main"].secrets.files.size() > 0 ?
      configurations["main"].secrets.files.map(f, {
        "name": "main-file-mount-"+oc_hash(f.mountPath+"/"+f.name),
        "mountPath": f.mountPath+"/"+f.name,
        "subPath": f.name
      }) : [])
  : oc_omit()}

# After - clean helper usage:
volumeMounts: ${configurations.toContainerVolumeMounts("main")}
```

**Dynamic container names:**

```yaml
# Works with dynamic container names from parameters
volumeMounts: ${configurations.toContainerVolumeMounts(parameters.containerName)}

# Can be combined with CEL operations
volumeMounts: |
  ${configurations.toContainerVolumeMounts("main") + 
    [{"name": "extra-mount", "mountPath": "/extra", "subPath": "extra.txt"}]}
```

#### configurations.toVolumes(resourceNamePrefix)

Helper method that generates a `volumes` array for all containers' files. This simplifies the creation of volume definitions based on all config and secret files across the workload containers.

**Parameters:**
- `resourceNamePrefix` - String used to generate unique resource names (typically `metadata.name`)

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
          volumeMounts: ${configurations.toContainerVolumeMounts("main")}
      volumes: ${configurations.toVolumes(metadata.name)}

# Before - verbose manual logic:
volumes: |
  ${has(configurations["main"].configs.files) && configurations["main"].configs.files.size() > 0 || has(configurations["main"].secrets.files) && configurations["main"].secrets.files.size() > 0 ?
    (has(configurations["main"].configs.files) && configurations["main"].configs.files.size() > 0 ?
      configurations["main"].configs.files.map(f, {
        "name": "file-mount-"+oc_hash(f.mountPath+"/"+f.name),
        "configMap": {
          "name": oc_generate_name(metadata.name, "config", f.name).replace(".", "-")
        }
      }) : []) +
    (has(configurations["main"].secrets.files) && configurations["main"].secrets.files.size() > 0 ?
      configurations["main"].secrets.files.map(f, {
        "name": "file-mount-"+oc_hash(f.mountPath+"/"+f.name),
        "secret": {
          "secretName": oc_generate_name(metadata.name, "secret", f.name).replace(".", "-")
        }
      }) : [])
  : oc_omit()}

# After - clean helper usage:
volumes: ${configurations.toVolumes(metadata.name)}
```

**Multi-container support:**

```yaml
# Works across all containers automatically
# If you have configurations for "main", "sidecar", etc., 
# this will generate volumes for all their files
volumes: ${configurations.toVolumes(metadata.name)}

# Can be combined with inline volumes
volumes: |
  ${configurations.toVolumes(metadata.name) + 
    [{"name": "extra-volume", "emptyDir": {}}]}
```

## Best Practices

1. **Use helpers for cleaner templates**: The helper methods significantly reduce boilerplate and make templates more maintainable.

2. **Consistent resource naming**: All helpers use consistent resource naming patterns with hashes for uniqueness.

3. **Combine with CEL operations**: Helpers return standard CEL lists/maps and can be combined with other CEL operations like `map()`, `filter()`, and list concatenation.

4. **Dynamic container names**: When working with parameters or multiple containers, use bracket notation: `configurations[parameters.containerName]`.

5. **Use forEach patterns**: For resources that need to be created per-container (like ConfigMaps or ExternalSecrets), use the `toConfigEnvList` and `toSecretEnvList` helpers with `forEach`.

## See Also

- [Context Reference](./context.md) - Main context documentation
- [CEL Extensions](./cel-extensions.md) - Custom CEL functions available in templates
