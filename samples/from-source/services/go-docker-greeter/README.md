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
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/services/go-docker-greeter/greeting-service.yaml
```

> [!NOTE]
> The build will take around 8 minutes depending on the network speed.

## Step 2: Test the Application

First, get the service URL from the HTTPRoute:

```bash
# Get the hostname and path prefix from the HTTPRoute
HOSTNAME=$(kubectl get httproute -A -l openchoreo.org/component=greeting-service -o jsonpath='{.items[0].spec.hostnames[0]}')
PATH_PREFIX=$(kubectl get httproute -A -l openchoreo.org/component=greeting-service -o jsonpath='{.items[0].spec.rules[0].matches[0].path.value}')
```

### Basic Greet
```bash
curl "http://${HOSTNAME}:9080${PATH_PREFIX}//greeter/greet"
```

### Greet with name
```bash
curl "http://${HOSTNAME}:9080${PATH_PREFIX}/greeter/greet?name=Alice"
```

### Generated curl
```bash
curl http://greeting-service-development-e6b7ae06-development.openchoreoapis.localhost:9080/greeting-service-development-e6b7ae06/greeter/greet
```
## Clean Up

Stop the port forwarding and remove all resources:

```bash
# Remove all resources
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/services/go-docker-greeter/greeting-service.yaml
```
