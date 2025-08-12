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

## Step 2: Port-forward the OpenChoreo Gateway

Port forward the OpenChoreo gateway service to access the service locally:

```bash
kubectl port-forward -n openchoreo-data-plane svc/gateway-external 8443:443 &
```

## Step 3: Test the Application

### Basic Greet
```bash
curl -k "$(kubectl get servicebinding greeter-service -o jsonpath='{.status.endpoints[0].public.uri}')/greet"
```

### Greet with name
```bash
curl -k "$(kubectl get servicebinding greeter-service -o jsonpath='{.status.endpoints[0].public.uri}')/greet?name=Alice"
```

## Troubleshooting Service Access Issues

If you cannot access the service:

1. Ensure the port-forward is running:
   ```bash
   ps aux | grep port-forward
   ```

2. Check if the service binding is ready:
   ```bash
   kubectl get servicebinding greeter-service -o yaml
   ```

## Clean Up

Stop the port forwarding and remove all resources:

```bash
# Find and stop the specific port-forward process
pkill -f "port-forward.*gateway-external.*8443:443"

# Remove all resources
kubectl delete -f greeter-service.yaml
```
