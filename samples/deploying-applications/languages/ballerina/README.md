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

The source code is available at:
https://github.com/wso2/choreo-samples/tree/main/patient-management-service

## Deploy in Choreo

```bash
kubectl apply -f samples/deploying-applications/languages/ballerina/patient-management-service.yaml
``` 


## Checking the Build Workflow Status
You can check the logs of the workflow by running the following command.

```bash
choreoctl logs --type build --build patient-management-service-build-01 --organization default-org --project default-project --component patient-management-service
```

## Check the Deployment Status
You can check the deployment logs by running the following command.

```bash
choreoctl logs --type deployment --deployment patient-management-service-development-deployment-01 --organization default-org --project default-project --component patient-management-service
```

Note: You should see a k8s namespace created for your org, project and environment combination.

## Invoke the service
For this sample, we will use kubectl port-forward to access the web application.

1. Run the following command to port-forward the gateway.

    ```bash
    kubectl port-forward svc/envoy-choreo-system-gateway-external-<hash> -n choreo-system 4430:443
    ```

   Note: You can find the <hash> part of the gateway name by running the following command:
    ```bash
    kubectl -n choreo-system get services
   ```
   
2. Invoke the service.

   Health check
   ```bash
    curl -k https://development.apis.choreo.localhost:4430/default-project/patient-management-service/mediflow/health
   ```
   
   Add a new patient
   ```bash
   curl -k -X POST https://development.apis.choreo.localhost:4430/default-project/patient-management-service/mediflow \
   -H "Content-Type: application/json" \
   -d '{
   "name": "Alice",
   "age": 30,
   "condition": "Healthy"
   }'
   ```
   
   Retrieve a patient by name
   ```bash
    curl -k https://development.apis.choreo.localhost:4430/default-project/patient-management-service/mediflow/Alice
   ```
   
   List all patients
   ```bash
    curl -k https://development.apis.choreo.localhost:4430/default-project/patient-management-service/mediflow/patients
   ```
   