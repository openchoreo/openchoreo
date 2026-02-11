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

The `api-configuration` Trait ([lines 115-217](component-with-api-management.yaml#L115-L217)) is a reusable addon that can be applied to any component to enable API management.

#### What It Creates

1. **Backend** ([lines 160-173](component-with-api-management.yaml#L160-L173))
   - Gateway kgateway.dev custom resource that references the API Platform Gateway router
   - Provides static host configuration for the gateway service
   - Enables routing to the API Platform Gateway in the `openchoreo-data-plane` namespace

2. **APIConfiguration CRD** ([lines 175-194](component-with-api-management.yaml#L175-L194))
   - WSO2 API Platform resource that defines the API
   - Configures operations, policies, upstream service, etc.
   - Enables features like JWT authentication, rate limiting, etc.

#### What It Patches

The trait patches the component's HTTPRoute to route through the API Platform Gateway ([lines 203-209](component-with-api-management.yaml#L203-L209)):

```yaml
backendRefs:
- group: gateway.kgateway.dev
  kind: Backend
  name: api-platform-default-gateway-router
```

It also updates the URL rewrite to use the API context path ([lines 212-216](component-with-api-management.yaml#L212-L216)):

```yaml
replacePrefixMatch: ${parameters.context}
```

### 3. Backend Resource for Gateway Routing

Instead of using cross-namespace Service references with ReferenceGrants, we use a **Backend** custom resource:

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: Backend
metadata:
  name: api-platform-default-gateway-router
  namespace: ${metadata.namespace}  # Created in the component namespace
spec:
  type: Static
  static:
    hosts:
      - host: api-platform-default-gateway-router.openchoreo-data-plane
        port: 8080
```

This Backend resource is created in the component's namespace and references the API Platform Gateway router service using its fully qualified domain name (FQDN). The HTTPRoute then references this Backend resource, avoiding the need for cross-namespace Service references.

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

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `apiName` | string | Yes | - | Name of the API |
| `apiVersion` | string | No | v1.0 | API version |
| `context` | string | Yes | - | API context path (e.g., /greeter-api/v1.0) |
| `upstreamPort` | integer | No | 80 | Service port |
| `operations` | array | No | All HTTP methods on /* | List of API operations (method, path) |
| `policies` | array | No | [] | List of policies (authentication, rate limiting, etc.) |

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
https://{componentNamespace}.{environment}.{publicVirtualHost}/my-http-service
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
https://{componentNamespace}.{environment}.{publicVirtualHost}/my-http-service
```

Traffic flows through the API Platform Gateway which:
- Rewrites the path to `/my-api/v1.0`
- Applies JWT authentication
- Enforces any other configured policies
- Routes to your service

## Complete Example

See [component-with-api-management.yaml](component-with-api-management.yaml) for a complete working example including:
- ComponentType definition ([lines 1-112](component-with-api-management.yaml#L1-L112))
- API Configuration Trait ([lines 115-217](component-with-api-management.yaml#L115-L217))
- Component instance ([lines 220-248](component-with-api-management.yaml#L220-L248))
- Workload definition ([lines 250-266](component-with-api-management.yaml#L250-L266))
- ReleaseBinding with environment overrides ([lines 268-296](component-with-api-management.yaml#L268-L296))

## Key Features

1. **Modular Design** - API management is optional, added via a trait
2. **Reusable** - The trait can be applied to any ComponentType
3. **Environment-Specific** - Override gateway references per environment
4. **Standards-Based** - Uses Gateway kgateway.dev Backend resource for flexible routing
5. **Policy-Driven** - Support for authentication, rate limiting, and other API policies
6. **Developer-Friendly** - Simple parameters with sensible defaults hide infrastructure complexity

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

# Check the Backend resource
kubectl get backend api-platform-default-gateway-router
```

### Testing the Secured API

Since the API is secured with JWT authentication, you need to obtain an access token first. For testing purposes, we use the OpenChoreo Control Plane Thunder as the IDP.

1. **Get an access token** using the OAuth2 client credentials flow:

```bash
curl -k -X POST https://thunder.openchoreo.localhost:8443/oauth2/token \
  -d 'grant_type=client_credentials' \
  -d 'client_id=customer-portal-client' \
  -d 'client_secret=supersecret'
```

Response:
```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIsImtpZCI6Ik11WGVKSlg5MTdzb1pmOTBCeWF6VXBJdnpLZGFKMWtWUlFIdGs0NkYyY2M9IiwidHlwIjoiSldUIn0...",
  "token_type": "Bearer",
  "expires_in": 3600
}
```

2. **Access the API** with the Bearer token:

```bash
# Export the token
export TOKEN="<access_token_from_previous_response>"

# Call the API
curl https://development-default.openchoreoapis.localhost:19443/demo-app-http-service/greeter/greet -kv \
  -H "Authorization: Bearer ${TOKEN}"
```

The API Platform Gateway will:
- Validate the JWT token
- Rewrite the path from `/demo-app-http-service` to `/greeter-api/v1.0`
- Route the request to the backend service
- Return the response from the greeter service

## Notes

- The Backend resource is created in the **component's namespace**, avoiding cross-namespace permission issues
- The Backend references the API Platform Gateway router using its FQDN (Fully Qualified Domain Name)
- The upstream URL in APIConfiguration includes the full service FQDN with namespace
- JWT authentication requires proper token configuration in the ThunderKeyManager
- The `operations` parameter has sensible defaults (all HTTP methods on /*) for quick prototyping
