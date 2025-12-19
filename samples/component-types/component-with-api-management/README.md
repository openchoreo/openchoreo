# Component with API Management

This sample demonstrates how to build a reusable **ComponentType** for HTTP services with optional **API Management** capabilities using the OpenChoreo platform.

## Overview

This example shows a modular approach to API management where:
- The base **ComponentType** creates a standard HTTP service with direct routing
- An optional **API Configuration Trait** can be added to enable API management features
- When the trait is applied, traffic is routed through the API Platform Gateway instead of directly to the service

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│ Without API Management (Default)                                │
│                                                                  │
│  Gateway HTTPRoute ──────────────▶ Component Service            │
│  (gateway-default)                 (ClusterIP)                  │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│ With API Management (api-configuration trait applied)           │
│                                                                  │
│  Gateway HTTPRoute ──▶ API Platform Gateway ──▶ Component       │
│  (gateway-default)     (openchoreo-data-plane)   Service        │
│                        │                                         │
│                        └─ APIConfiguration CRD                  │
│                           (policies, operations, etc.)          │
└─────────────────────────────────────────────────────────────────┘
```

## Files

- [`component-with-api-management.yaml`](component-with-api-management.yaml) - Complete example including:
  - ComponentType definition
  - API Configuration Trait
  - Sample Component, Workload, and ReleaseBinding

## How It Works

### 1. Base ComponentType

The `http-service-with-api-management` ComponentType ([lines 1-112](component-with-api-management.yaml#L1-L112)) creates three core resources:

- **Deployment** - Runs your HTTP service containers
- **Service** - Exposes the deployment as a ClusterIP service
- **HTTPRoute** - Routes ingress traffic to your service

By default, the HTTPRoute routes traffic **directly** to your component service:

```yaml
backendRefs:
- name: ${metadata.name}
  port: 80
```

### 2. API Configuration Trait (Optional Addon)

The `api-configuration` Trait ([lines 115-219](component-with-api-management.yaml#L115-L219)) is a reusable addon that can be applied to any component to enable API management.

#### What It Creates

1. **ReferenceGrant** ([lines 160-175](component-with-api-management.yaml#L160-L175))
   - Allows the HTTPRoute to reference services in the `openchoreo-data-plane` namespace
   - Required for cross-namespace service references in Gateway API
   - Named uniquely per component to avoid conflicts

2. **APIConfiguration CRD** ([lines 177-197](component-with-api-management.yaml#L177-L197))
   - WSO2 API Platform resource that defines the API
   - Configures operations, policies, upstream service, etc.
   - Enables features like JWT authentication, rate limiting, etc.

#### What It Patches

The trait patches the component's HTTPRoute to route through the API Platform Gateway ([lines 207-212](component-with-api-management.yaml#L207-L212)):

```yaml
backendRefs:
- name: api-platform-default-gateway-router
  namespace: openchoreo-data-plane
  port: 8080
```

It also updates the URL rewrite to use the API context path ([lines 217-219](component-with-api-management.yaml#L217-L219)):

```yaml
replacePrefixMatch: ${parameters.context}
```

### 3. Cross-Namespace Reference Handling

Since the HTTPRoute needs to reference a Service in a different namespace (`openchoreo-data-plane`), we use a **ReferenceGrant**:

```yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: allow-httproute-to-api-gateway-${metadata.name}
  namespace: openchoreo-data-plane  # Created in the target namespace
spec:
  from:
  - group: gateway.networking.k8s.io
    kind: HTTPRoute
    namespace: ${metadata.namespace}  # Allow from component namespace
  to:
  - group: ""
    kind: Service
    name: api-platform-default-gateway-router
```

This is the standard Kubernetes Gateway API approach for allowing cross-namespace references.

## Configuration Parameters

### ComponentType Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `replicas` | integer | 1 | Number of pod replicas |
| `imagePullPolicy` | string | IfNotPresent | Image pull policy |
| `port` | integer | 80 | Container port to expose |
| `resources.requests.cpu` | string | 100m | CPU request |
| `resources.requests.memory` | string | 256Mi | Memory request |
| `resources.limits.cpu` | string | 1000m | CPU limit |
| `resources.limits.memory` | string | 1Gi | Memory limit |

### API Configuration Trait Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `apiName` | string | Yes | Name of the API |
| `apiVersion` | string | No | API version (default: v1.0) |
| `context` | string | Yes | API context path (e.g., /greeter-api/v1.0) |
| `upstreamPort` | integer | No | Service port (default: 80) |
| `operations` | array | Yes | List of API operations (method, path) |
| `policies` | array | No | List of policies (authentication, rate limiting, etc.) |

### Environment Overrides

Platform engineers can override per environment:

```yaml
traitOverrides:
  greeter-api:
    gatewayRefs:
      - name: api-platform-default
        namespace: openchoreo-data-plane
```

## Usage Example

### Basic HTTP Service (No API Management)

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-http-service
  namespace: default
spec:
  componentType: deployment/http-service-with-api-management
  parameters:
    replicas: 2
    port: 8080
```

This creates a basic HTTP service accessible at:
```
https://{environment}.{publicVirtualHost}/my-http-service
```

### HTTP Service with API Management

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-http-service
  namespace: default
spec:
  componentType: deployment/http-service-with-api-management
  parameters:
    replicas: 2
    port: 8080

  # Add API management by applying the trait
  traits:
    - name: api-configuration
      instanceName: my-api
      parameters:
        apiName: my-api
        apiVersion: v1.0
        context: /my-api/v1.0
        upstreamPort: 8080
        operations:
          - method: GET
            path: /*
          - method: POST
            path: /orders
        policies:
          - name: JwtAuthentication
            version: v0.1.0
```

This creates an HTTP service with API management enabled, accessible at:
```
https://{environment}.{publicVirtualHost}/my-http-service
```

Traffic flows through the API Platform Gateway which:
- Rewrites the path to `/my-api/v1.0`
- Applies JWT authentication
- Enforces any other configured policies
- Routes to your service

## Complete Example

See [component-with-api-management.yaml](component-with-api-management.yaml) for a complete working example including:
- ComponentType definition ([lines 1-112](component-with-api-management.yaml#L1-L112))
- API Configuration Trait ([lines 115-219](component-with-api-management.yaml#L115-L219))
- Component instance ([lines 222-263](component-with-api-management.yaml#L222-L263))
- Workload definition ([lines 266-281](component-with-api-management.yaml#L266-L281))
- ReleaseBinding with environment overrides ([lines 284-311](component-with-api-management.yaml#L284-L311))

## Key Features

1. **Modular Design** - API management is optional, added via a trait
2. **Reusable** - The trait can be applied to any ComponentType
3. **Environment-Specific** - Override gateway references per environment
4. **Standards-Based** - Uses Kubernetes Gateway API ReferenceGrant for cross-namespace access
5. **Policy-Driven** - Support for authentication, rate limiting, and other API policies
6. **Developer-Friendly** - Simple parameters hide infrastructure complexity

## API Platform Integration

The `APIConfiguration` resource is processed by the WSO2 API Platform Gateway which:
- Creates routing rules for your API operations
- Applies configured policies (authentication, rate limiting, etc.)
- Provides API analytics and monitoring
- Handles request/response transformations
- Manages API lifecycle (versioning, deprecation, etc.)

## Testing

Deploy the example:

```bash
kubectl apply -f component-with-api-management.yaml
```

Verify resources created:

```bash
# Check the component
kubectl get component demo-app-http-service

# Check the HTTPRoute
kubectl get httproute demo-app-http-service

# Check the APIConfiguration
kubectl get apiconfiguration demo-app-http-service

# Check the ReferenceGrant
kubectl get referencegrant -n openchoreo-data-plane
```

Access the API:

```bash
curl https://development.{your-domain}/demo-app-http-service
```

## Notes

- The ReferenceGrant must be created in the **target namespace** (`openchoreo-data-plane`)
- Each component gets a unique ReferenceGrant to avoid conflicts
- The upstream URL in APIConfiguration includes the full service FQDN with namespace
- JWT authentication requires proper token configuration in the ThunderKeyManager
