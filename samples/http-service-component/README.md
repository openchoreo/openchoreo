# HTTP Service Component Sample

This sample demonstrates how to deploy an HTTP service component in OpenChoreo with path-based routing.

## Overview

This sample includes the following OpenChoreo Custom Resources:

### ComponentType (`http-service`)

Defines a reusable component type template for HTTP services. It:

- Specifies the workload type as `deployment`
- Defines a schema with configurable parameters (replicas, port, resources)
- Declares environment-specific overrides for resource limits
- Templates the underlying Kubernetes resources (Deployment, Service, HTTPRoute)
- Configures path-based routing with specific HTTP methods:
  - `GET /{component-name}/{resource-paths}`
- Uses CEL expressions to dynamically populate values from component metadata and parameters

This allows you to create multiple HTTP service components using the same template with different configurations.

### Component (`demo-app`)

Defines the actual HTTP service component using the `http-service` type. It:

- References the `deployment/http-service` component type
- Specifies configuration parameters:
  - 2 replicas
  - Port 8080
  - CPU and memory resource requests/limits
- Belongs to the `default` project

### Workload (`demo-app-workload`)

Specifies the container image and configuration for the component:

- Links to the `demo-app` component
- Defines the container image (`ghcr.io/openchoreo/samples/greeter-service:latest`)
- Can specify multiple containers if needed

### ComponentDeployment (`demo-app-development`)

Represents a deployment instance of the component to a specific environment:

- Links to the `demo-app` component
- Targets the `development` environment
- Provides environment-specific overrides (reduced resource limits for dev)

## How It Works

1. **ComponentType** acts as a template/blueprint with path-based routing rules
2. **Component** uses that template and provides base configuration
3. **Workload** specifies what container(s) to run (echo server for testing)
4. **ComponentDeployment** creates an actual deployment to an environment with optional overrides

The OpenChoreo controller manager uses these resources and generates the Kubernetes resources (Deployment, Service, HTTPRoute) based on the templates and parameters.

## Deploy the sample

Apply the sample:

```bash
kubectl apply -f http-service-component.yaml
```

## Verify the Release is created

```bash
# Check Release was created
kubectl get release -n default

# View Release details
kubectl get release demo-app-development -n default -o yaml

# Check the rendered resources
kubectl get release demo-app-development -n default -o jsonpath='{.spec.resources[*].id}'
```

## Test the Service by invoking

```bash
curl http://demo-app-development-e040c964-development.openchoreoapis.localhost:9080/demo-app-development-e040c964/greeter/greet
Hello, Stranger!
```
