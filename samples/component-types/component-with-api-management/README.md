# Component with API Management

This sample demonstrates how to use the **api-configuration trait** with the default service component type to add API management capabilities to HTTP services.

## Overview

This example shows a simple approach to adding API management where:
- The Component uses the default **deployment/service** ComponentType
- The **api-configuration trait** is added to enable API management features
- When the trait is applied, traffic is routed through the API Platform Gateway instead of directly to the service
- The Component must set `exposed: true` to create an HTTPRoute that the trait can patch

## Installation

The API Platform module is disabled by default and must be explicitly enabled on the OpenChoreo Data Plane.

### Prerequisites

- OpenChoreo deployed in a Kubernetes cluster
- `kubectl` configured for cluster access
- Helm 3.12 or later

First install the required CRDs:

```bash
kubectl apply --server-side \
  -f https://raw.githubusercontent.com/wso2/api-platform/gateway-v0.3.0/kubernetes/helm/operator-helm-chart/crds/gateway.api-platform.wso2.com_restapis.yaml \
  -f https://raw.githubusercontent.com/wso2/api-platform/gateway-v0.3.0/kubernetes/helm/operator-helm-chart/crds/gateway.api-platform.wso2.com_gateways.yaml
```

Then upgrade the Data Plane with the API Platform enabled:

```bash
helm upgrade --install openchoreo-data-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-data-plane \
  --version 0.0.0-latest-dev \
  --namespace openchoreo-data-plane \
  --reuse-values \
  --set api-platform.enabled=true
```

### Verify Installation

Confirm the API Platform pods are running:

```bash
kubectl get pods -n openchoreo-data-plane --selector="app.kubernetes.io/instance=api-platform-default-gateway"
```

You should see three running pods: the controller, policy engine, and router.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│ Without API Management (default service type with exposed=true) │
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
│                        └─ RestApi CRD                           │
│                           (policies, operations, etc.)          │
└─────────────────────────────────────────────────────────────────┘
```

## Files

- [`component-with-api-management.yaml`](component-with-api-management.yaml) - Complete example including:
  - API Configuration Trait definition
  - Sample Component, Workload, and ReleaseBinding using the default service type

## How It Works

### 1. Default Service Component Type

The Component uses the standard `deployment/service` ComponentType which:
- Creates a Deployment for running containers
- Creates a Service to expose the deployment as a ClusterIP service
- Creates an HTTPRoute (when `exposed: true`) to route ingress traffic to the service

By default, the HTTPRoute routes traffic **directly** to the component service.

### 2. API Configuration Trait (Added by Developer)

The `api-configuration` Trait can be added to any component with an HTTPRoute to enable API management.

#### What It Creates

1. **Backend** - Gateway kgateway.dev custom resource that references the API Platform Gateway router
2. **RestApi** - WSO2 API Platform resource that defines the API with operations, policies, upstream service, etc.

#### What It Patches

The trait patches the component's HTTPRoute to:
- Route through the API Platform Gateway (instead of directly to service)
- Rewrite the URL path to use the API context path

### 3. Backend Resource for Gateway Routing

Instead of using cross-namespace Service references with ReferenceGrants, we use a **Backend** custom resource:

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: Backend
metadata:
  name: ${metadata.componentName}-api-gw-backend
  namespace: ${metadata.namespace}
spec:
  type: Static
  static:
    hosts:
      - host: api-platform-default-gateway-router.openchoreo-data-plane
        port: 8080
```

This Backend resource is created in the component's namespace and references the API Platform Gateway router service using its FQDN.

## Configuration Parameters

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
  componentType: deployment/service
  parameters:
    exposed: true
```

This creates a basic HTTP service accessible at the base path `/{component-name}`:
```
https://{environment}-{componentNamespace}.{publicVirtualHost}/{component-name}
```

### HTTP Service with API Management

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-http-service
  namespace: default
spec:
  componentType: deployment/service
  parameters:
    exposed: true  # Required for HTTPRoute creation

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

This creates an HTTP service with API management enabled, accessible at the base path `/{component-name}`:
```
https://{environment}-{componentNamespace}.{publicVirtualHost}/{component-name}
```

Traffic flows through the API Platform Gateway which:
- Rewrites the path from `/{component-name}` to `/my-api/v1.0`
- Applies JWT authentication
- Enforces any other configured policies
- Routes to your service

## Complete Example

See [component-with-api-management.yaml](component-with-api-management.yaml) for a complete working example including:
- API Configuration Trait definition
- Component instance using default service type
- Workload definition
- ReleaseBinding with environment overrides

## Key Features

1. **Uses Default Service Type** - No custom ComponentType needed
2. **Modular Design** - API management is optional, added via a trait
3. **Reusable** - The trait can be applied to any service component
4. **Environment-Specific** - Override gateway references per environment
5. **Standards-Based** - Uses Gateway kgateway.dev Backend resource for flexible routing
6. **Policy-Driven** - Support for authentication, rate limiting, and other API policies
7. **Developer-Friendly** - Simple parameters with sensible defaults

## API Platform Integration

The `RestApi` resource is processed by the WSO2 API Platform Gateway which:
- Creates routing rules for your API operations
- Applies configured policies (authentication, rate limiting, etc.)
- Provides API analytics and monitoring
- Handles request/response transformations
- Manages API lifecycle (versioning, deprecation, etc.)

## Testing

Deploy the example:

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/release-v0.15/samples/component-types/component-with-api-management/component-with-api-management.yaml
```

Verify resources created:

```bash
# Check the component
kubectl get component demo-app-http-service

# Check the HTTPRoute
kubectl get httproute demo-app-http-service

# Check the RestApi
kubectl get restapi demo-app-http-service

# Check the Backend resource
kubectl get backend demo-app-http-service-api-gw-backend
```

### Testing the Secured API

Since the API is secured with JWT authentication, you need to obtain an access token first. For testing purposes, we use the OpenChoreo Control Plane Thunder as the IDP.

1. **Get an access token** using the OAuth2 client credentials flow:

```bash
curl -k -X POST http://thunder.openchoreo.localhost:8080/oauth2/token \
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

The service is exposed at the base path `/{component-name}`. For this sample, the component name is `demo-app-http-service`.

```bash
# Export the token
export TOKEN="<access_token_from_previous_response>"

# Call the API (base path is /{component-name})
curl http://development-default.openchoreoapis.localhost:19080/demo-app-http-service/greeter/greet -kv \
  -H "Authorization: Bearer ${TOKEN}"
```

The API Platform Gateway will:
- Validate the JWT token
- Rewrite the path from `/{component-name}` to `/greeter-api/v1.0`
- Route the request to the backend service
- Return the response from the greeter service

## Cleanup

### 1. Disable API Management for a Component

To disable API management for a component, remove the trait:

```bash
kubectl patch component demo-app-http-service --type=json \
  -p='[{"op": "remove", "path": "/spec/traits"}]'
```

### 2. Delete the API Platform Gateway Instance

```bash
kubectl delete gateway/api-platform-default -n openchoreo-data-plane
```

### 3. Uninstall the API Platform Module

```bash
helm upgrade openchoreo-data-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-data-plane \
  --version 0.0.0-latest-dev \
  --namespace openchoreo-data-plane \
  --reuse-values \
  --set api-platform.enabled=false
```

## Notes

- The Backend resource is created in the **component's namespace**, avoiding cross-namespace permission issues
- The Backend references the API Platform Gateway router using its FQDN (Fully Qualified Domain Name)
- The upstream URL in RestApi includes the full service FQDN with namespace
- JWT authentication requires proper token configuration in the ThunderKeyManager
- The `operations` parameter has sensible defaults (all HTTP methods on /*) for quick prototyping
- The Component **must** set `exposed: true` to create the HTTPRoute that the trait patches
