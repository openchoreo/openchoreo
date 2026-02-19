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
gcp-microservices-demo/
├── gcp-microservice-demo-project.yaml    # Project definition
├── components/                           # Component definitions
│   ├── ad-component.yaml                 # Ad service component
│   ├── cart-component.yaml               # Cart service component
│   ├── checkout-component.yaml           # Checkout service component
│   ├── currency-component.yaml           # Currency service component
│   ├── email-component.yaml              # Email service component
│   ├── frontend-component.yaml           # Frontend web application
│   ├── payment-component.yaml            # Payment service component
│   ├── productcatalog-component.yaml     # Product catalog service component
│   ├── recommendation-component.yaml     # Recommendation service component
│   ├── redis-component.yaml              # Redis cache component
│   └── shipping-component.yaml           # Shipping service component
└── README.md                             # This guide
```

## Step 1: Create the Project

First, create the project that will contain all the microservices:

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/gcp-microservice-demo-project.yaml
```

## Step 2: Deploy the Components

Deploy all the microservices components:

```bash
kubectl apply \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/ad-component.yaml \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/cart-component.yaml \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/checkout-component.yaml \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/currency-component.yaml \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/email-component.yaml \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/frontend-component.yaml \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/payment-component.yaml \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/productcatalog-component.yaml \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/recommendation-component.yaml \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/redis-component.yaml \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/shipping-component.yaml
```

This will deploy all the microservices using official Google Container Registry images.

## Step 3: Test the Application

Access the frontend application in your browser:

```
http://frontend-development-default.openchoreoapis.localhost:19080
```

> [!TIP]
> #### Verification
>
> You should see the Google Cloud Platform microservices demo store frontend with:
> - Product catalog
> - Shopping cart functionality
> - Checkout process

> [!NOTE]
> #### Startup Time
> All services may take 5-10 minutes to become fully operational. The URL will return errors or be unreachable until everything is ready.

## Clean Up

Remove all resources:

```bash
# Remove components
kubectl delete \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/ad-component.yaml \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/cart-component.yaml \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/checkout-component.yaml \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/currency-component.yaml \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/email-component.yaml \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/frontend-component.yaml \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/payment-component.yaml \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/productcatalog-component.yaml \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/recommendation-component.yaml \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/redis-component.yaml \
-f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/components/shipping-component.yaml

# Remove project
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/gcp-microservices-demo/gcp-microservice-demo-project.yaml
```
