# Hello World PHP Service

## Overview
The Hello World PHP Service is a simple REST service that responds with a friendly greeting message when accessed via a GET request. It demonstrates a basic PHP application with minimal configuration.

The service exposes one REST endpoint which responds with a "Hello, world!" message when accessed via a GET request.

### Get greeting message
**Endpoint:** `/`  
**Method:** `GET`  
**Functionality:** Get a greeting message.

The source code is available at:
https://github.com/wso2/choreo-samples/tree/main/hello-world-php-service

## Deploy in Choreo

```bash
choreoctl apply -f samples/deploying-applications/languages/php/hello-world-service.yaml
``` 

## Check the Build Workflow Status
You can check the logs of the workflow by running the following command.

```bash
choreoctl logs --type build --build hello-world-php-service-build-01 --organization default-org --project default-project --component hello-world-php-service
```

## Check the Deployment Status
You can check the deployment logs by running the following command.

```bash
choreoctl logs --type deployment --deployment hello-world-php-service-development-deployment-01 --organization default-org --project default-project --component hello-world-php-service
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
   
   Get a greeting message
   ```bash
   curl -k https://development.apis.choreo.localhost:4430/default-project/hello-world-php-service 
   ```
   