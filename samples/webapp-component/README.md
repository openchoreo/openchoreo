# Web Application Component Sample

This sample demonstrates how to deploy a web application component in OpenChoreo using the component type definitions.

## Overview

This sample includes the following OpenChoreo Custom Resources:

### ComponentTypeDefinition (`web-service`)

Defines a reusable component type template for web services. It:

- Specifies the workload type as `deployment`
- Defines a schema with configurable parameters (replicas, port, resources)
- Declares environment-specific overrides for resource limits
- Templates the underlying Kubernetes resources (Deployment, Service, HTTPRoute)
- Uses CEL expressions to dynamically populate values from component metadata and parameters

This allows you to create multiple web service components using the same template with different configurations.

### Component (`demo-app`)

Defines the actual web application component using the `web-service` type. It:

- References the `deployment/web-service` component type
- Specifies configuration parameters:
  - 2 replicas
  - Port 80
  - CPU and memory resource requests/limits
- Belongs to the `default` project

### Workload (`demo-app-workload`)

Specifies the container image and configuration for the component:

- Links to the `demo-app` component
- Defines the container image (`choreoanonymouspullable.azurecr.io/react-spa:v0.9`)
- Can specify multiple containers if needed

### ComponentDeployment (`demo-app-development`)

Represents a deployment instance of the component to a specific environment:

- Links to the `demo-app` component
- Targets the `development` environment
- Provides environment-specific overrides (reduced resource limits for dev)

## How It Works

1. **ComponentTypeDefinition** acts as a template/blueprint
2. **Component** uses that template and provides base configuration
3. **Workload** specifies what container(s) to run
4. **ComponentDeployment** creates an actual deployment to an environment with optional overrides

The controller reads these resources and generates the Kubernetes resources (Deployment, Service, HTTPRoute) based on the templates and parameters.

## Apply resources

Apply the sample:

```bash
kubectl apply -f webapp-component.yaml
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

# Test the Web App by opening it via a web browser

Open your web browser and go to `http://demo-app-development-e040c964-development.openchoreoapis.localhost:9080/`.
