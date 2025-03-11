# Patient Management Service (Mediflow)

## Overview
The **MediFlow** service provides functionalities to manage patient data, including:
- Adding a new patient
- Retrieving patient details by name
- Listing all patients

The service exposes several REST endpoints for performing these operations.

### Health Check
**Endpoint:** `/health`  
**Functionality:** Ensures the service is running.

### Add a new patient
**Endpoint:** `/patients`  
**Method:** `POST`  
**Functionality:** Adds a new patient by sending a JSON payload.

### Retrieve a patient by name
**Endpoint:** `/patients/{name}`  
**Method:** `GET`  
**Functionality:** Retrieves patient details by their name.

### List all patients
**Endpoint:** `/patients`  
**Method:** `GET`  
**Functionality:** Retrieves all patients.

## Deploy in Choreo

```bash
kubectl apply -f samples/languages/patient-management-service.yaml
``` 


## Checking the Argo Workflow Status
Argo workflow will create three tasks.

```
NAMESPACE                       NAME 
choreo-ci-default-org           patient-management-service-build-01-clone-step-2079439001     
choreo-ci-default-org           patient-management-service-build-01-build-step-3941607917                      
choreo-ci-default-org           patient-management-service-build-01-push-step-3448493733                  
```

You can check the status of the workflow by running the following commands.

```bash
kubectl get pods -n choreo-ci-default-org
```

You can check build logs of each step by running the following commands.

```bash
kubectl -n choreo-ci-default-org logs -l workflow=patient-management-service-build-01,step=clone-step --tail=-1
kubectl -n choreo-ci-default-org logs -l workflow=patient-management-service-build-01,step=build-step --tail=-1
kubectl -n choreo-ci-default-org logs -l workflow=patient-management-service-build-01,step=push-step --tail=-1
```

## Checking the Deployment Status
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

2. Add the following entry to your `/etc/hosts` file.

    ```bash
    echo "127.0.0.1 development.apis.choreo.local" | sudo tee -a /etc/hosts
    ```

   Now you can Invoke the endpoints using the following URL.
    ```bash
    https://development.apis.choreo.local:4430/default-project/patient-management-service/mediflow
   ```
   
3. Invoke the service

   Health check
   ```bash
    curl -k https://development.apis.choreo.local:4430/default-project/patient-management-service/mediflow/health
   ```
   
   Add a new patient
   ```bash
   curl -k -X POST https://development.apis.choreo.local:4430/default-project/patient-management-service/mediflow \
   -H "Content-Type: application/json" \
   -d '{
   "name": "Alice",
   "age": 30,
   "condition": "Healthy"
   }'
   ```
   
   Retrieve a patient by name
      ```bash
    curl -k https://development.apis.choreo.local:4430/default-project/patient-management-service/mediflow/Alice
   ```
   
   List all patients
   ```bash
    curl -k https://development.apis.choreo.local:4430/default-project/patient-management-service/mediflow/patients
   ```
   