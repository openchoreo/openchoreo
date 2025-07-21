# Google Microservices Demo Sample

This sample demonstrates how to deploy Google's microservices demo application using OpenChoreo.

## Overview

This sample demonstrates a complete microservices architecture deployment using the Google Cloud Platform's microservices demo application. It showcases multiple services working together using OpenChoreo.

## Pre-requisites

- Kubernetes cluster with OpenChoreo installed
- The `kubectl` CLI tool installed
- Docker runtime capable of running AMD64 images (see note below)

> [!NOTE]
> #### Architecture Compatibility
> This sample uses official Google Container Registry images built for AMD64 architecture. 
> If you're on Apple Silicon (M1/M2) or ARM-based systems, your container runtime may need 
> to emulate AMD64. To verify your setup can run AMD64 images:
> ```bash
> docker run --rm --platform linux/amd64 hello-world
> ```
> If this command fails, you may need to enable emulation support in your container runtime.

## File Structure

```
google-microservices-sample/
├── gcp-microservice-demo-project.yaml    # Project definition
├── ad-component.yaml                     # Ad service component
├── cart-component.yaml                   # Cart service component
├── checkout-component.yaml               # Checkout service component
├── currency-component.yaml               # Currency service component
├── email-component.yaml                  # Email service component
├── frontend-component.yaml               # Frontend web application
├── payment-component.yaml                # Payment service component
├── productcatalog-component.yaml         # Product catalog service component
├── recommendation-component.yaml         # Recommendation service component
├── redis-component.yaml                  # Redis cache component
├── shipping-component.yaml               # Shipping service component
└── README.md                             # This guide
```

## Step 1: Deploy the Application

From the repository root, deploy the project and all microservices components:

```bash
kubectl apply -f new-design-sample/google-microservices-sample/
```

This will create the project and deploy all the microservices using official Google Container Registry images.

## Step 2: Expose the Frontend

Port forward the OpenChoreo gateway service to access the frontend locally:

```bash
kubectl port-forward -n openchoreo-data-plane svc/gateway-external 8443:443 &
```

## Step 3: Test the Application

Access the frontend application in your browser:

```
https://frontend-development.choreoapps.localhost:8443
```

> [!TIP]
> #### Verification
>
> You should see the Google Cloud Platform microservices demo store frontend with:
> - Product catalog
> - Shopping cart functionality
> - Checkout process

## Clean Up

Stop the port forwarding and remove all resources:

```bash
# Find and stop the specific port-forward process
pkill -f "port-forward.*gateway-external.*8443:443"

# Remove all resources
kubectl delete -f new-design-sample/google-microservices-sample/
```
