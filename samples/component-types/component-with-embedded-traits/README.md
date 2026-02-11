# Component with Embedded Traits Example

This example demonstrates PE-defined embedded traits in ComponentTypes, where Platform Engineers pre-configure trait instances that are automatically applied to all Components of that type.

## Overview

This example shows:

- Embedded traits with CEL expression bindings
- Wired vs locked parameter bindings
- Allowed traits restriction for developers
- Environment-specific trait overrides

## Components

The example includes a `component-with-embedded-traits.yaml` file containing all resources for easy deployment.

### Trait (`horizontal-pod-autoscaler`)

Defines an HPA trait that:

- **Creates**: A HorizontalPodAutoscaler (when enabled)
- Uses `includeWhen` for conditional creation
- Supports environment-specific min/max replica overrides

Key features:

- `enabled` parameter controls HPA creation
- `minReplicas`, `maxReplicas`, `targetCPUPercent` for scaling config
- Environment overrides via `envOverrides.minReplicas` and `envOverrides.maxReplicas` (no defaults - required when trait is instantiated)

### Trait (`persistent-volume`)

Defines a persistent volume trait that:

- **Creates**: A PersistentVolumeClaim
- **Patches**: Adds volume and volumeMount to the Deployment

### ComponentType (`service-with-autoscaling`)

Defines a component type with an embedded HPA trait:

- **Embedded trait**: `horizontal-pod-autoscaler` with instance name `autoscaler`
- **Wired parameter bindings**: CEL expressions like `${parameters.autoscaling.enabled}`
- **Allowed traits**: Only `persistent-volume` can be added by developers

Key features:

- Uses `${metadata.name}` and `${metadata.namespace}` for computed names
- Exposes autoscaling config through ComponentType schema
- Developers configure autoscaling without knowing about HPA trait
- **Embedded trait envOverrides demonstrate two patterns:**
  - **Locked**: `minReplicas: 2` - PE sets a fixed value (platform policy)
  - **Wired**: `maxReplicas: ${envOverrides.autoscaling.maxReplicas}` - PE exposes for per-environment tuning

### Component (`demo-app-with-embedded-traits`)

Defines a component that:

- Uses the `service-with-autoscaling` component type
- Enables autoscaling via ComponentType parameters
- Adds a `persistent-volume` trait (from allowed list)

### Workload

Represents the build output with:

- Container image: `ghcr.io/openchoreo/samples/greeter-service:latest`
- Configured to run on port 9090

### ReleaseBinding

Defines deployment settings for the `development` environment:

- Reduces resource requests/limits for cost savings via `componentTypeEnvOverrides.resources`
- Overrides `maxReplicas` to 3 via `componentTypeEnvOverrides.autoscaling.maxReplicas` (wired envOverride)
- Note: `minReplicas` is locked by PE at 2 and cannot be changed per environment
- Overrides PVC size to 1Gi via `traitOverrides.data-storage`

## How It Works

1. **PE defines embedded traits** in the ComponentType with CEL bindings
2. **Developer creates Component** and configures autoscaling via ComponentType parameters
3. **Binding resolution**: CEL expressions like `${parameters.autoscaling.enabled}` are resolved against the component context
4. **Trait renders**: The HPA trait receives resolved concrete values
5. **Environment overrides**: ReleaseBinding can override trait settings per environment

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

Once deployed, test the greeter service:

```bash
curl http://development-default.openchoreoapis.localhost:19080/demo-app-with-embedded-traits-development-d47a92df/greeter/greet
```

Output:
```text
Hello, Stranger!
```

With a name parameter:
```bash
curl "http://development-default.openchoreoapis.localhost:19080/demo-app-with-embedded-traits-development-d47a92df/greeter/greet?name=Alice"
```

Output:
```text
Hello, Alice!
```

## Key Features Demonstrated

### 1. Embedded Traits

- PE pre-configures traits in ComponentType
- Traits are automatically applied to all Components

### 2. CEL Bindings

- **Wired**: `${parameters.autoscaling.enabled}` - references ComponentType schema
- **Locked**: Concrete values controlled by PE

### 3. Allowed Traits

- `allowedTraits` restricts which traits developers can add
- Embedded traits are always applied (not in allowedTraits list)

### 4. Environment Overrides

- **Wired envOverrides**: Use `componentTypeEnvOverrides` to override values exposed through ComponentType schema (e.g., `autoscaling.maxReplicas`)
- **Locked envOverrides**: Set by PE in embedded trait definition, cannot be changed per environment (e.g., `minReplicas: 2`)
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

5. **Check HPA status (embedded trait):**
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
