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

## Deploy in Choreo

Run the following command to deploy the service in Choreo. This will create the necessary resources in Choreo and deploy
the service including the build for the Go service.

```bash
choreoctl apply -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/deploying-applications/languages/go/greeter-service.yaml
``` 

## Check the Build Workflow Status

Run the following command to check the logs of the build workflow.

```bash
choreoctl logs --type build --build greeting-service-go-build-01 --organization default-org --project default-project --component greeting-service-go
```

> [!NOTE]
> The build will take around 5 minutes depending on the network speed.

## Check the Deployment Status

You can check the deployment logs by running the following command.

```bash
choreoctl logs --type deployment --deployment greeting-service-go-development-deployment-01 --organization default-org --project default-project --component greeting-service-go
```

You will see an output similar to the following:

```
=== Pod: greeting-service-go-greeting-service-go-main-8bad2f67-86bdf6cdt ===
2025/03/19 11:52:55 Starting HTTP Greeter on port 9090
```

## Invoke the Service

For this sample, we will use kubectl port-forward to access the service.

1. Run the following command to port-forward the gateway.

    ```bash
    kubectl -n choreo-system port-forward svc/choreo-external-gateway 8443:443
    ```
2. Invoke the service.
   Greet
   ```bash
    curl -k "https://dev.choreoapis.localhost:8443/default-project/greeting-service-go/greeter/greet"
   ```

   Greet with name
   ```bash
   curl -k "https://dev.choreoapis.localhost:8443/default-project/greeting-service-go/greeter/greet?name=Alice"
   ```

## Clean up

To clean up the resources created by this sample, run the following command.

```bash
choreoctl delete -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/deploying-applications/languages/go/greeter-service.yaml
```
