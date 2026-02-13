# Component with Embedded Traits Example

This example demonstrates PE-defined embedded traits in ComponentTypes, where Platform Engineers pre-configure trait instances that are automatically applied to all Components of that type.

## Overview

This example shows:

- Embedded traits with CEL expression bindings
- Extending the default service component type structure with embedded traits
- Wired vs locked parameter bindings
- Allowed traits restriction for developers
- Environment-specific trait overrides

## Components

The example includes a `component-with-embedded-traits.yaml` file containing all resources for easy deployment.

### Trait (`horizontal-pod-autoscaler`)

Defines a HorizontalPodAutoscaler trait that:

- **Creates**: A HorizontalPodAutoscaler (when enabled)
- Uses `includeWhen` for conditional creation
- Supports environment-specific replica bound overrides

Key features:

- `enabled` parameter controls HPA creation
- `minReplicas`, `maxReplicas` for controlling autoscaling bounds
- `targetCPUUtilizationPercentage` for CPU-based scaling threshold
- Environment overrides via `envOverrides.minReplicas` and `envOverrides.maxReplicas`

### Trait (`persistent-volume`)

Defines a persistent volume trait that:

- **Creates**: A PersistentVolumeClaim
- **Patches**: Adds volume and volumeMount to the Deployment

### ComponentType (`service-with-autoscaling`)

Defines a component type that extends the default service structure with an embedded HPA trait:

- **Based on**: Default `deployment/service` component type structure
- **Embedded trait**: `horizontal-pod-autoscaler` with instance name `autoscaler`
- **Wired parameter bindings**: CEL expressions like `${parameters.autoscaling.enabled}`
- **Allowed traits**: `api-configuration`, `observability-alert-rule`, and `persistent-volume` can be added by developers
- **Allowed workflows**: `google-cloud-buildpacks`, `ballerina-buildpack`, and `docker`

Key features:

- Uses the same resource structure as the default service component type
- Includes the `exposed` parameter for controlling HTTPRoute creation
- Exposes autoscaling config through ComponentType schema
- Developers configure HPA without knowing about the underlying trait
- **Embedded trait parameters demonstrate three patterns:**
  - **Locked**: `targetCPUUtilizationPercentage: 75` - PE enforces a fixed value (platform policy)
  - **Developer-configurable**: `minReplicas`, `maxReplicas` - Developers set scaling bounds via parameters
  - **Environment-tunable**: Platform teams can override replica bounds per environment via componentTypeEnvOverrides

### Component (`demo-app-with-embedded-traits`)

Defines a component that:

- Uses the `service-with-autoscaling` component type
- Enables and configures autoscaling via ComponentType parameters (2-10 replicas)
- Note: CPU threshold is locked by PE at 75%, not configurable by developers
- Sets `exposed: true` to create an HTTPRoute
- Adds a `persistent-volume` trait (from allowed list)

### Workload

Represents the build output with:

- Container image: `ghcr.io/openchoreo/samples/greeter-service:latest`
- Configured to run on port 9090

### ReleaseBinding

Defines deployment settings for the `development` environment:

- Sets replicas to 3 for adequate redundancy
- Reduces resource requests/limits for cost savings via `componentTypeEnvOverrides.resources`
- Overrides autoscaling replica bounds to 1-5 via `componentTypeEnvOverrides.autoscaling` (environment-tunable)
- Note: `targetCPUUtilizationPercentage` is locked by PE at 75% and cannot be changed per environment
- Overrides PVC size to 10Mi via `traitOverrides.data-storage`

## How It Works

1. **PE extends default service type** by copying its structure and adding embedded HPA trait
2. **PE locks CPU threshold** at 75% while exposing replica bounds to developers
3. **Developer creates Component** and configures autoscaling replica bounds (2-10) via ComponentType parameters
4. **Binding resolution**: CEL expressions like `${parameters.autoscaling.enabled}` are resolved against the component context
5. **Trait renders**: The HPA trait receives resolved values (75% CPU threshold locked, replica bounds from component)
6. **Environment overrides**: ReleaseBinding can override replica bounds per environment (e.g., 1-5 for dev)

## Deploy the sample

Apply the sample:

```bash
kubectl apply --server-side -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/main/samples/component-types/component-with-embedded-traits/component-with-embedded-traits.yaml
```

## Check the ReleaseBinding status

```bash
kubectl get releasebinding demo-app-with-embedded-traits-development -o yaml | grep -A 50 "^status:"
```

## Test the Service

Once deployed, test the greeter service. The service is exposed at the base path `/{component-name}`. For this sample, the component name is `demo-app-with-embedded-traits`.

```bash
curl http://development-default.openchoreoapis.localhost:19080/demo-app-with-embedded-traits/greeter/greet
```

Output:
```text
Hello, Stranger!
```

With a name parameter:
```bash
curl "http://development-default.openchoreoapis.localhost:19080/demo-app-with-embedded-traits/greeter/greet?name=Alice"
```

Output:
```text
Hello, Alice!
```

## Key Features Demonstrated

### 1. Extending Default Component Types

- ComponentType uses the same structure as the default `deployment/service` type
- Adds embedded HPA trait for automatic scaling
- Maintains compatibility with standard service features (endpoints, configs, secrets)

### 2. Embedded Traits

- PE pre-configures traits in ComponentType
- Traits are automatically applied to all Components
- Developers configure through simple ComponentType parameters

### 3. CEL Bindings and Parameter Patterns

- **Locked**: `targetCPUUtilizationPercentage: 75` - Concrete values controlled by PE (platform policy)
- **Wired**: `${parameters.autoscaling.minReplicas}` - References ComponentType schema (developer-configurable)
- **Environment-tunable**: `${envOverrides.autoscaling.maxReplicas}` - Per-environment overrides

### 4. Allowed Traits

- `allowedTraits` restricts which traits developers can add
- Embedded traits are always applied (not in allowedTraits list)

### 5. Environment Overrides

- **Environment-tunable overrides**: Use `componentTypeEnvOverrides` to override replica bounds per environment (e.g., `autoscaling.minReplicas`, `autoscaling.maxReplicas`)
- **Locked parameters**: Set by PE in embedded trait, cannot be changed per environment (e.g., `targetCPUUtilizationPercentage: 75`)
- **Developer trait overrides**: Use `traitOverrides[instanceName]` to override developer-added traits (e.g., `data-storage.size`)

## Cleanup

Remove all resources:

```bash
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/main/samples/component-types/component-with-embedded-traits/component-with-embedded-traits.yaml
```

## Troubleshooting

If the application is not accessible or resources are not created:

1. **Check ReleaseBinding status:**
   ```bash
   kubectl get releasebinding demo-app-with-embedded-traits-development -o yaml
   ```

2. **Check ReleaseBinding conditions:**
   ```bash
   kubectl get releasebinding demo-app-with-embedded-traits-development -o jsonpath='{.status.conditions}' | jq .
   ```

3. **Verify HTTPRoute is configured:**
   ```bash
   kubectl get httproute -A -l openchoreo.dev/component=demo-app-with-embedded-traits -o yaml
   ```

4. **Check deployment status:**
   ```bash
   kubectl get deployment -A -l openchoreo.dev/component=demo-app-with-embedded-traits
   ```

5. **Check HorizontalPodAutoscaler status (embedded trait):**
   ```bash
   kubectl get hpa -A -l openchoreo.dev/component=demo-app-with-embedded-traits
   ```

6. **Check pod logs:**
   ```bash
   kubectl logs -n $(kubectl get pods -A -l openchoreo.dev/component=demo-app-with-embedded-traits -o jsonpath='{.items[0].metadata.namespace}') -l openchoreo.dev/component=demo-app-with-embedded-traits --tail=50
   ```

7. **Verify service endpoints:**
   ```bash
   kubectl get service -A -l openchoreo.dev/component=demo-app-with-embedded-traits
   ```

8. **Verify PersistentVolumeClaim (developer-added trait):**
   ```bash
   kubectl get pvc -A -l openchoreo.dev/component=demo-app-with-embedded-traits
   ```
