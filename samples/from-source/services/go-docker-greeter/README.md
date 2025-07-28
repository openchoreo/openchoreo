# Greeting Service

## Overview

This sample demonstrates how to deploy a simple Go REST service in Choreo from the source code.
The service exposes one REST endpoint.

Exposed REST endpoints:

### Greet a user

**Endpoint:** `/greeter/greet`
**Method:** `GET`
**Functionality:** Sends a greeting to the user.

The source code is available at:
https://github.com/wso2/choreo-samples/tree/main/greeting-service-go

## Step 1: Deploy the Application

The following command will create the relevant resources in OpenChoreo. It will also trigger a build by creating a build resource.

```bash
kubectl apply -f new-design-sample/from-source/services/go-docker-greeter/greeter-service.yaml
```

> [!NOTE]
> The build will take around 8 minutes depending on the network speed.

## Step 2: Port-forward the OpenChoreo Gateway

Port forward the OpenChoreo gateway service to access the frontend locally:

```bash
kubectl port-forward -n openchoreo-data-plane svc/gateway-external 8443:443 &
```

## Step 3: Test the Application

   Greet
   ```bash
    curl -k "https://dev.openchoreoapis.localhost:8443/default/greeting-service/greeter/greet"
   ```

   Greet with name
   ```bash
   curl -k "https://dev.openchoreoapis.localhost:8443/default/greeting-service/greeter/greet?name=Alice"
   ```

## Clean Up

Stop the port forwarding and remove all resources:

```bash
# Find and stop the specific port-forward process
pkill -f "port-forward.*gateway-external.*8443:443"

# Remove all resources
kubectl delete -f new-design-sample/from-source/services/go-docker-greeter/greeter-service.yaml
```
