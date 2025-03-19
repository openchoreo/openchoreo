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

Run the following command to deploy the service in Choreo.
This will create the necessary resources in Choreo and deploy the service including the build for the Ballerina service

```bash
choreoctl apply -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/deploying-applications/languages/ballerina/patient-management-service.yaml
``` 

## Check the Build Workflow Status

You can check the logs of the workflow by running the following command.

```bash
choreoctl logs --type build --build patient-management-service-build-01 --organization default-org --project default-project --component patient-management-service
```

> [!Note]
> The build will take around 5 minutes depending on the network speed.

## Check the Deployment Status

You can check the deployment logs by running the following command.

```bash
choreoctl logs --type deployment --deployment patient-management-service-development-deployment-01 --organization default-org --project default-project --component patient-management-service
```

You will see an output similar to the following:

Notice that you will only see any application logs after invoking the service.

```
=== Pod: patient-management-service-patient-management-service-main7hmf5 ===
```

## Invoke the service

For this sample, we will use kubectl port-forward to access the web application.

1. Run the following command to port-forward the gateway.

    ```bash
    kubectl -n choreo-system port-forward svc/choreo-external-gateway 8443:443
    ```

2. Invoke the service.

   Health check
   ```bash
    curl -k https://dev.choreoapis.localhost:8443/default-project/patient-management-service/mediflow/health
   ```

   Add a new patient
   ```bash
   curl -k -X POST https://dev.choreoapis.localhost:8443/default-project/patient-management-service/mediflow/patients \
   -H "Content-Type: application/json" \
   -d '{
   "name": "Alice",
   "age": 30,
   "condition": "Healthy"
   }'
   ```

   Retrieve a patient by name
   ```bash
    curl -k https://dev.choreoapis.localhost:8443/default-project/patient-management-service/mediflow/patients/Alice
   ```

   List all patients
   ```bash
    curl -k https://dev.choreoapis.localhost:8443/default-project/patient-management-service/mediflow/patients
   ```

## Clean up

To clean up the resources created by this sample, run the following command.

```bash
choreoctl delete -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/deploying-applications/languages/ballerina/patient-management-service.yaml
```
