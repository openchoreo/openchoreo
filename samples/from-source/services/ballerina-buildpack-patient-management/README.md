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


## Step 1: Deploy the Application

The following command will create the relevant resources in OpenChoreo. It will also trigger a build by creating a build resource.

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/services/ballerina-buildpack-patient-management/patient-management-service.yaml
```

> [!NOTE]
> The build will take around 8 minutes depending on the network speed.


## Step 2: Test the Application

First, get the service URL from the HTTPRoute:

```bash
# Get the hostname and path prefix from the HTTPRoute
HOSTNAME=$(kubectl get httproute -A -l openchoreo.org/component=patient-management-service -o jsonpath='{.items[0].spec.hostnames[0]}')
PATH_PREFIX=$(kubectl get httproute -A -l openchoreo.org/component=patient-management-service -o jsonpath='{.items[0].spec.rules[0].matches[0].path.value}')
```

### Health check
```bash
curl "http://${HOSTNAME}:9080${PATH_PREFIX}/mediflow/health"
```

```bash
curl http://patient-management-service-development-9d4355fb-development.openchoreoapis.localhost:9080/mediflow/health
```

### Add a new patient
```bash
curl -X POST "http://${HOSTNAME}:9080${PATH_PREFIX}/mediflow/patients" \
-H "Content-Type: application/json" \
-d '{
"name": "Alice",
"age": 30,
"condition": "Healthy"
}'
```

### Retrieve a patient by name
```bash
curl "http://${HOSTNAME}:9080${PATH_PREFIX}/mediflow/patients/Alice"
```

### List all patients
```bash
curl "http://${HOSTNAME}:9080${PATH_PREFIX}/mediflow/patients"
```

## Clean Up

```bash

# Remove all resources
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/services/ballerina-buildpack-patient-management/patient-management-service.yaml
```
