# Go Greeter Service (From Image)

## Overview

This sample demonstrates how to deploy a Go REST service in OpenChoreo from a pre-built container image. The service exposes REST endpoints for greeting functionality and uses a containerized deployment approach.

The service is deployed from the pre-built image:
`ghcr.io/openchoreo/samples/greeter-service:latest`

Exposed REST endpoints:

### Greet a user

**Endpoint:** `/greeter/greet`
**Method:** `GET`
**Functionality:** Sends a greeting to the user.

## Step 1: Deploy the Application

The following command will create the relevant resources in OpenChoreo:

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/go-greeter-service/greeter-service.yaml
```

> [!NOTE]
> Since this uses a pre-built image, the deployment will be faster compared to building from source.

## Step 2: Test the Application

First, get the service URL from the HTTPRoute:

```bash
# Get the hostname and path prefix from the HTTPRoute
HOSTNAME=$(kubectl get httproute -A -l openchoreo.dev/component=greeter-service -o jsonpath='{.items[0].spec.hostnames[0]}')
PATH_PREFIX=$(kubectl get httproute -A -l openchoreo.dev/component=greeter-service -o jsonpath='{.items[0].spec.rules[0].matches[0].path.value}')
```

### Basic Greet
```bash
curl "http://${HOSTNAME}:19080${PATH_PREFIX}/greeter/greet"
```

### Greet with name
```bash
curl "http://${HOSTNAME}:19080${PATH_PREFIX}/greeter/greet?name=Alice"
```

### Generated URL
```bash
curl http://development-default.openchoreoapis.localhost:19080/greeter-service/greeter/greet
```

## Troubleshooting Service Access Issues

If you cannot access the service:

1. Check if the ReleaseBinding is ready:
   ```bash
   kubectl get releasebinding greeter-service-development -n default -o yaml
   ```

2. Check the ReleaseBinding status conditions:
   ```bash
   kubectl get releasebinding greeter-service-development -n default -o jsonpath='{.status.conditions}' | jq .
   ```

3. Verify the HTTPRoute is configured correctly:
   ```bash
   kubectl get httproute -A -l openchoreo.dev/component=greeter-service -o yaml
   ```

4. Check the deployment status:
   ```bash
   kubectl get deployment -A -l openchoreo.dev/component=greeter-service
   ```

## Clean Up

Remove all resources:

```bash
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/go-greeter-service/greeter-service.yaml
```
