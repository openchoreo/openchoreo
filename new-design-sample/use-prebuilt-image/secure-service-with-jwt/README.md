# Greeter Service Sample

This sample demonstrates how to deploy a basic service using OpenChoreo's new CRD design with ComponentV2, Workload, and
component-type-specific resources.

## Overview

This sample demonstrates a basic service deployment using the default classes provided by OpenChoreo.

## Pre-requisites

- Kubernetes cluster with OpenChoreo installed
- The `kubectl` CLI tool installed

## File Structure

```
secure-service-with-jwt/
├── greeter-service-with-jwt.yaml  # Developer resources (ComponentV2, Workload, Service, API)
└── README.md                 # This guide
```

## Step 1: Deploy the Service

Deploy the greeter service application:

```bash
kubectl apply -f greeter-service-with-jwt.yaml
```

This creates:

- **ComponentV2**: Component metadata and type definition
- **Workload**: Container configuration and endpoint definitions
- **Service**: Runtime service configuration (automatically uses `className: default`)
- **API**: API definition with security requirements (automatically uses `className: default`)

## Step 2: Expose the API Gateway

Port forward the OpenChoreo gateway service to access it locally:

```bash
kubectl port-forward -n choreo-system svc/choreo-external-gateway 8443:443 &
```

## Step 3: Test the Service

Test the greeter service:

```bash
curl -k https://development.choreoapis.localhost:8443/default/greeter-service/greeter/greet
```

> [!TIP]
> #### Verification
>
> You should receive a successful response from the greeter service:
> ```
> Hello, Stranger!
> ```

## Clean Up

Remove all resources:

```bash
# Remove all resources
kubectl delete -f greeter-service-with-jwt.yaml
```
