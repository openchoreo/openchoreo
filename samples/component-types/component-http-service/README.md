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

### Component (`demo-app-http-service`)

Defines the actual HTTP service component using the `http-service` type. It:

- References the `deployment/http-service` component type
- Specifies configuration parameters:
  - 2 replicas
  - Port 8080
  - CPU and memory resource requests/limits
- Belongs to the `default` project

### Workload (`demo-app-http-service-workload`)

Specifies the container image and configuration for the component:

- Links to the `demo-app-http-service` component
- Defines the container image (`ghcr.io/openchoreo/samples/greeter-service:latest`)
- Can specify multiple containers if needed

### ReleaseBinding (`demo-app-development`)

Represents a deployment of the component in a specific environment. It:

- Links to the `demo-app-http-service` component
- Targets the `development` environment
- Refers a ComponentRelease by `releaseName` (This will be overwritten with a generated ComponentRelease as the auto-deploy is enabled) 
- Provides environment-specific overrides (reduced resource limits for dev)

## How It Works

1. **ComponentType** acts as a template/blueprint with path-based routing rules
2. **Component** uses that template and provides base configuration
3. **Workload** specifies what container(s) to run (echo server for testing)
4. **ReleaseBinding** specifies the actual deployment of the component in a specific environment

The OpenChoreo controller manager uses these resources and generates the Kubernetes resources (Deployment, Service, HTTPRoute) based on the templates and parameters.

## Deploy the sample

Apply the sample:

```bash
kubectl apply --server-side -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/release-v0.15/samples/component-types/component-http-service/http-service-component.yaml
```

## Check the ReleaseBinding status

```bash
kubectl get releasebinding demo-app-http-service-development -o yaml | grep -A 50 "^status:"
```

## Test the Service by invoking

The service is exposed at the base path `/{component-name}`. For this sample, the component name is `demo-app-http-service`.

```bash
curl http://development-default.openchoreoapis.localhost:19080/demo-app-http-service/greeter/greet
```

Output:
```text
Hello, Stranger!
```

```bash
curl "http://development-default.openchoreoapis.localhost:19080/demo-app-http-service/greeter/greet?name=Alice"
```

Output:

```text
Hello, Alice!
```

## Cleanup

Remove all resources:

```bash
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/release-v0.15/samples/component-types/component-http-service/http-service-component.yaml
```

## Troubleshooting

If the application is not accessible:

1. **Check ReleaseBinding status:**
   ```bash
   kubectl get releasebinding demo-app-http-service-development -o yaml
   ```

2. **Check ReleaseBinding conditions:**
   ```bash
   kubectl get releasebinding demo-app-http-service-development -o jsonpath='{.status.conditions}' | jq .
   ```

3. **Verify HTTPRoute is configured:**
   ```bash
   kubectl get httproute -A -l openchoreo.dev/component=demo-app-http-service -o yaml
   ```

4. **Check deployment status:**
   ```bash
   kubectl get deployment -A -l openchoreo.dev/component=demo-app-http-service
   ```

5. **Check pod logs:**
   ```bash
   kubectl logs -n $(kubectl get pods -A -l openchoreo.dev/component=demo-app-http-service -o jsonpath='{.items[0].metadata.namespace}') -l openchoreo.dev/component=demo-app-http-service --tail=50
   ```

6. **Verify service endpoints:**
   ```bash
   kubectl get service -A -l openchoreo.dev/component=demo-app-http-service
   ```
