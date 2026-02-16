# Web Application Component Sample

This sample demonstrates how to deploy a web application component in OpenChoreo using the component types.

## Overview

This sample includes the following OpenChoreo Custom Resources:

### ComponentType (`web-service`)

Defines a reusable component type template for web services. It:

- Specifies the workload type as `deployment`
- Defines a schema with configurable parameters (replicas, port, resources)
- Declares environment-specific overrides for resource limits
- Templates the underlying Kubernetes resources (Deployment, Service, HTTPRoute)
- Uses CEL expressions to dynamically populate values from component metadata and parameters

This allows you to create multiple web service components using the same template with different configurations.

### Component (`demo-app-web-service`)

Defines the actual web application component using the `web-service` type. It:

- References the `deployment/web-service` component type
- Specifies configuration parameters:
  - 2 replicas
  - Port 80
  - CPU and memory resource requests/limits
- Belongs to the `default` project

### Workload (`demo-app-web-service-workload`)

Specifies the container image and configuration for the component:

- Links to the `demo-app-web-service` component
- Defines the container image (`choreoanonymouspullable.azurecr.io/react-spa:v0.9`)
- Can specify multiple containers if needed

### ReleaseBinding (`demo-app-web-service-development`)

Represents a deployment instance of the component to a specific environment:

- Links to the `demo-app-web-service` component
- Targets the `development` environment
- Provides environment-specific overrides (reduced resource limits for dev)

## How It Works

1. **ComponentType** acts as a template/blueprint
2. **Component** uses that template and provides base configuration
3. **Workload** specifies what container(s) to run
4. **ReleaseBinding** specifies the actual deployment of the component in a specific environment

The controller reads these resources and generates the Kubernetes resources (Deployment, Service, HTTPRoute) based on the templates and parameters.

## Apply resources

Apply the sample:

```bash
kubectl apply --server-side -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/main/samples/component-types/component-web-app/webapp-component.yaml
```

## Check the ReleaseBinding status

```bash
kubectl get releasebinding demo-app-web-service-development -o yaml | grep -A 50 "^status:"
```

# Test the Web App by opening it via a web browser

Open your web browser and go to http://demo-app-web-service-development-default.openchoreoapis.localhost:19080/.

## Cleanup

Remove all resources:

```bash
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/main/samples/component-types/component-web-app/webapp-component.yaml
```

## Troubleshooting

If the application is not accessible:

1. **Check ReleaseBinding status:**
   ```bash
   kubectl get releasebinding demo-app-web-service-development -o yaml
   ```

2. **Check ReleaseBinding conditions:**
   ```bash
   kubectl get releasebinding demo-app-web-service-development -o jsonpath='{.status.conditions}' | jq .
   ```

3. **Verify HTTPRoute is configured:**
   ```bash
   kubectl get httproute -A -l openchoreo.dev/component=demo-app-web-service -o yaml
   ```

4. **Check deployment status:**
   ```bash
   kubectl get deployment -A -l openchoreo.dev/component=demo-app-web-service
   ```

5. **Check pod logs:**
   ```bash
   kubectl logs -n $(kubectl get pods -A -l openchoreo.dev/component=demo-app-web-service -o jsonpath='{.items[0].metadata.namespace}') -l openchoreo.dev/component=demo-app-web-service --tail=50
   ```

6. **Verify service endpoints:**
   ```bash
   kubectl get service -A -l openchoreo.dev/component=demo-app-web-service
   ```
