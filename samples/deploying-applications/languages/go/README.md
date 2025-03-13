# Greeting Service

## Overview
This is a simple greeting service

The service exposes only one REST endpoint.

### Greet a user
**Endpoint:** `/greeter/greet`  
**Method:** `GET`  
**Functionality:** Sends a greeting to the user.

The source code is available at:
https://github.com/wso2/choreo-samples/tree/main/greeting-service-go

## Deploy in Choreo

```bash
kubectl apply -f samples/deploying-applications/languages/go/greeter-service.yaml
``` 

## Check the Build Workflow Status
You can check the logs of the workflow by running the following command.

```bash
choreoctl logs --type build --build greeting-service-go-build-01 --organization default-org --project default-project --component greeting-service-go
```

## Check the Deployment Status
You can check the deployment logs by running the following command.

```bash
choreoctl logs --type deployment --deployment greeting-service-go-development-deployment-01 --organization default-org --project default-project --component greeting-service-go
```

Note: You should see a k8s namespace created for your org, project and environment combination.

## Invoke the service
For this sample, we will use kubectl port-forward to access the service.

1. Run the following command to port-forward the gateway.

    ```bash
    kubectl port-forward svc/envoy-choreo-system-gateway-external-<hash> -n choreo-system 4430:443
    ```

   Note: You can find the <hash> part of the gateway name by running the following command:
    ```bash
    kubectl -n choreo-system get services
   ```
   
2. Invoke the service.

   Greet
   ```bash
    curl -k https://development.apis.choreo.localhost:4430/default-project/greeting-service-go/greeter/greet
   ```
   
   Greet with name
   ```bash
   curl -k https://development.apis.choreo.localhost:4430/default-project/greeting-service-go/greeter/greet?name="Alice"
   ```
   