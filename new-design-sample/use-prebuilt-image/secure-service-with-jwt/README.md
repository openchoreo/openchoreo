# Greeter Service Sample

This sample demonstrates how to deploy a basic service using OpenChoreo's new CRD design with ComponentV2, Workload, and
component-type-specific resources.

## Overview

This sample shows the separation between Platform Engineer (PE) and Developer resources:

- **Platform Engineer**: Sets up classes that define templates and policies
- **Developer**: Creates components, workloads, and services that use those classes

## Pre-requisites

- Kubernetes cluster with OpenChoreo installed
- The `kubectl` CLI tool installed

## File Structure

```
secure-service-with-jwt/
├── platform-classes.yaml     # PE resources (ServiceClass, APIClass)
├── greeter-service-with-jwt.yaml  # Developer resources (ComponentV2, Workload, Service, API)
└── README.md                 # This guide
```

## Step 1: Platform Engineer Setup

First, the Platform Engineer deploys the class templates that define deployment policies:

```bash
kubectl apply -f platform-classes.yaml
```

This creates:

- **ServiceClass**: Defines resource limits, replicas, and service templates
- **APIClass**: Defines rate limiting and CORS policies

## Step 2: Deploy Developer Resources

Deploy the greeter service application:

```bash
kubectl apply -f greeter-service-with-jwt.yaml
```

This creates:

- **ComponentV2**: Component metadata and type definition
- **Workload**: Container configuration and endpoint definitions
- **Service**: Runtime service configuration (automatically uses `className: default`)
- **API**: API definition with security requirements (automatically uses `className: default`)

## Step 3: Expose the API Gateway

Port forward the OpenChoreo gateway service to access it locally:

```bash
kubectl port-forward -n choreo-system svc/choreo-external-gateway 8443:443 &
```

## Step 4: Test the Service

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
# Remove developer resources
kubectl delete -f greeter-service-with-jwt.yaml

# Remove platform classes (optional, as they're shared)
kubectl delete -f platform-classes.yaml
```
