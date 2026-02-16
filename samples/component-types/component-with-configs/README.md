# Component with Configurations Sample

This sample demonstrates how to deploy a component with environment variables, config files, and secrets in OpenChoreo.

## Overview

This sample includes the following OpenChoreo Custom Resources:

### ComponentType (`service-with-configs`)

Defines a reusable component type template based on the default `deployment/service` structure that supports configurations. It:

- Specifies the workload type as `deployment`
- Follows the standard service ComponentType structure with:
  - Allowed workflows (google-cloud-buildpacks, ballerina-buildpack, docker)
  - Allowed traits (api-configuration, observability-alert-rule)
  - Validation rules for service endpoints
- Defines a schema with:
  - Parameters: `exposed` (boolean) to control HTTPRoute creation
  - Environment-specific overrides: `replicas`, `resources`, `imagePullPolicy`
- Templates the underlying Kubernetes resources (Deployment, Service, HTTPRoute, ConfigMaps, ExternalSecrets)
- Automatically creates ConfigMaps for environment variables and file mounts
- Automatically creates ExternalSecrets for secret environment variables and secret files
- Uses CEL expressions to dynamically populate values from component metadata, parameters, and configurations

### SecretReference (`database-secret`, `github-pat-secret`)

Defines references to external secrets that can be used by components:

- `database-secret`: References a database password from an external secret store
- `github-pat-secret`: References a GitHub personal access token from an external secret store

### Component (`demo-app`)

Defines the actual component using the `service-with-configs` type. It:

- References the `deployment/service-with-configs` component type
- Enables auto-deployment to environments
- Belongs to the `default` project

### Workload (`demo-app-workload`)

Specifies the container image and configuration for the component:

- Links to the `demo-app` component
- Defines the container image (`nginx:1.25-alpine`)
- Configures environment variables:
  - `LOG_LEVEL`: Plain text value
  - `DATABASE_PASSWORD`: Value from secret reference
- Configures file mounts:
  - `application.toml`: Plain text config file mounted at `/conf`
  - `gitPAT`: Secret file mounted at `/conf` from secret reference

### ReleaseBinding (`demo-app-development`)

Represents a deployment of the component in a specific environment. It:

- Links to the `demo-app` component
- Targets the `development` environment
- Provides ComponentType environment-specific overrides:
  - Replicas: 2
  - Resource requests/limits (CPU and memory)
- Provides workload overrides for environment-specific configurations:
  - Additional environment variables (`FEATURE_FLAG`, `NEW_ENV_VAR`)
  - Environment-specific file content overrides (overriding `application.toml`)

## Key Features

This sample demonstrates the comprehensive configuration capabilities available in OpenChoreo ComponentTypes:

- **Environment Variables**: Both plain text and secret-backed values
- **Config Files**: Plain text configuration files mounted into containers
- **Secret Files**: Secret-backed files (e.g., credentials, tokens) mounted into containers
- **Environment-Specific Overrides**: Different configurations per environment via ReleaseBinding

> **Note**: The default `deployment/service` ComponentType already includes all these configuration capabilities. This sample uses a custom ComponentType (`deployment/service-with-configs`) following the same structure to explicitly demonstrate configuration features.

## How It Works

1. **ComponentType** acts as a template/blueprint that handles configurations and includes:
   - Standard Deployment and Service resources
   - HTTPRoute for external access (when `exposed: true`)
   - Automatic ConfigMap creation for environment variables and files
   - Automatic ExternalSecret creation for secret environment variables and files
2. **SecretReference** defines how to fetch secrets from external secret stores
3. **Component** uses that template and provides base configuration
4. **Workload** specifies what container(s) to run and their configurations (env vars, files, secrets)
5. **ReleaseBinding** specifies the actual deployment with environment-specific overrides

The OpenChoreo controller manager uses these resources and generates the Kubernetes resources (Deployment, Service, HTTPRoute, ConfigMaps, ExternalSecrets) based on the templates and parameters.

## Deploy the sample

Apply the sample:

```bash
kubectl apply --server-side -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/main/samples/component-types/component-with-configs/component-with-configs.yaml
```

## Check the ReleaseBinding status

```bash
kubectl get releasebinding demo-app-development -o yaml | grep -A 50 "^status:"
```

## Verify Generated Resources

### Check ConfigMaps

```bash
kubectl get configmap -A -l openchoreo.dev/component=demo-app
```

### Check ExternalSecrets

```bash
kubectl get externalsecret -A -l openchoreo.dev/component=demo-app
```

### Check Deployment

```bash
kubectl get deployment -A -l openchoreo.dev/component=demo-app
```

## Verify Environment Variables and Config Files in Pod

Once the pods are running, you can verify that environment variables and config files are correctly injected.

### Get a pod name

```bash
POD_NAMESPACE=$(kubectl get deployment -A -l openchoreo.dev/component=demo-app -o jsonpath='{.items[0].metadata.namespace}')
POD_NAME=$(kubectl get pods -n $POD_NAMESPACE -o jsonpath='{.items[0].metadata.name}')
```

### Verify environment variables are injected

```bash
kubectl exec -n $POD_NAMESPACE $POD_NAME -- env | grep -E "LOG_LEVEL|FEATURE_FLAG|NEW_ENV_VAR"
```

Expected output:

```text
FEATURE_FLAG=true
LOG_LEVEL=info
NEW_ENV_VAR=new_value
```

### Verify config file is mounted

```bash
kubectl exec -n $POD_NAMESPACE $POD_NAME -- cat /conf/application.toml
```

Expected output:

```text
schema_generation:
  enable: false
```

## Cleanup

Remove all resources:

```bash
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/main/samples/component-types/component-with-configs/component-with-configs.yaml
```

## Troubleshooting

If the application is not accessible or resources are not created:

1. **Check ReleaseBinding status:**
   ```bash
   kubectl get releasebinding demo-app-development -o yaml
   ```

2. **Check ReleaseBinding conditions:**
   ```bash
   kubectl get releasebinding demo-app-development -o jsonpath='{.status.conditions}' | jq .
   ```

3. **Check deployment status:**
   ```bash
   kubectl get deployment -A -l openchoreo.dev/component=demo-app
   ```

4. **Check pod logs:**
   ```bash
   kubectl logs -n $(kubectl get pods -A -l openchoreo.dev/component=demo-app -o jsonpath='{.items[0].metadata.namespace}') -l openchoreo.dev/component=demo-app --tail=50
   ```

5. **Verify ConfigMaps are created:**
   ```bash
   kubectl get configmap -A -l openchoreo.dev/component=demo-app -o yaml
   ```

6. **Verify ExternalSecrets status:**
   ```bash
   kubectl get externalsecret -A -l openchoreo.dev/component=demo-app -o yaml
   ```

7. **Check if secrets are synced:**
   ```bash
   kubectl get secret -A -l openchoreo.dev/component=demo-app
   ```
