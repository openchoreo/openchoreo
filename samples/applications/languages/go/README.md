# Greeting Service

## Overview
This is a simple greeting service

The service exposes only one REST endpoint.

### Greet a user
**Endpoint:** `/greeter/greet`  
**Method:** `GET`  
**Functionality:** Sends a greeting to the user.

## Deploy in Choreo

```bash
kubectl apply -f samples/applications/languages/go/greeter-service.yaml
``` 

## Checking the Argo Workflow Status
Argo workflow will create three tasks.

```
NAMESPACE                       NAME 
choreo-ci-default-org           greeting-service-go-build-clone-step-2079439001     
choreo-ci-default-org           greeting-service-go-build-build-step-3941607917                      
choreo-ci-default-org           greeting-service-go-build-push-step-3448493733                  
```

You can check the status of the workflow by running the following commands.

```bash
kubectl get pods -n choreo-ci-default-org
```

You can check build logs of each step by running the following commands.

```bash
kubectl -n choreo-ci-default-org logs -l workflow=greeting-service-go-build,step=clone-step --tail=-1
kubectl -n choreo-ci-default-org logs -l workflow=greeting-service-go-build,step=build-step --tail=-1
kubectl -n choreo-ci-default-org logs -l workflow=greeting-service-go-build,step=push-step --tail=-1
```

## Check the Deployment Status
You should see a namespace created for your org, project and environment combination. In this sample it will have the prefix `dp-default-org-default-project-development-`.

List all the namespaces in the cluster to find the namespace created for the deployment.

```bash
kubectl get namespaces
``` 

You can check the status of the deployment by running the following commands.

```bash
kubectl get pods -n dp-default-org-default-project-development-39faf2d8
```

## Invoke the service
For this sample, we will use kubectl port-forward to access the web application.

1. Run the following command to port-forward the gateway.

    ```bash
    kubectl port-forward svc/envoy-choreo-system-gateway-external-<hash> -n choreo-system 4430:443
    ```

   Now you can Invoke the endpoints using the following URL.
    ```bash
    https://development.apis.choreo.localhost:4430/default-project/patient-management-service/mediflow
   ```
   
2. Invoke the service

   Greet
   ```bash
    curl -k https://development.apis.choreo.localhost:4430/default-project/greeting-service-go/greeter/greet
   ```
   
   Greet with name
   ```bash
   curl -k https://development.apis.choreo.localhost:4430/default-project/greeting-service-go/greeter/greet?name="Alice"
   ```
   