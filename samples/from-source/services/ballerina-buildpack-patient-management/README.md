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

The following command will create the relevant resources in OpenChoreo. It will also trigger a workflow by creating a workflow resource.

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/from-source/services/ballerina-buildpack-patient-management/patient-management-service.yaml
```

> [!NOTE]
> The workflow will take around 5-8 minutes depending on the network speed and system resources.

## Step 2: Monitor the Workflow

After deploying, monitor the workflow progress:

```bash
# Check ComponentWorkflowRun status
kubectl get componentworkflowrun patient-management-service-build-01 -n default -o jsonpath='{.status.conditions}' | jq .

# Watch workflow pods (in openchoreo-ci-default namespace)
kubectl get pods -n openchoreo-ci-default | grep patient-management

# View workflow logs (replace <pod-name> with actual pod name)
kubectl logs -n openchoreo-ci-default <pod-name> -f
```

Wait for the ComponentWorkflowRun to complete successfully. You should see:
- `WorkflowCompleted: True`
- `WorkflowSucceeded: True`
- `WorkloadUpdated: True`

## Step 3: Verify Deployment

After the workflow completes, verify the deployment is ready:

```bash
# Check ReleaseBinding status
kubectl get releasebinding patient-management-service-development -n default -o jsonpath='{.status.conditions}' | jq .

# Verify deployment is ready
kubectl get deployment -A -l openchoreo.dev/component=patient-management-service
```

## Step 4: Test the Application

First, get the service URL from the HTTPRoute:

```bash
# Get the hostname and path prefix from the HTTPRoute
HOSTNAME=$(kubectl get httproute -A -l openchoreo.dev/component=patient-management-service -o jsonpath='{.items[0].spec.hostnames[0]}')
PATH_PREFIX=$(kubectl get httproute -A -l openchoreo.dev/component=patient-management-service -o jsonpath='{.items[0].spec.rules[0].matches[0].path.value}')
```

### Health check
```bash
curl "http://${HOSTNAME}:19080${PATH_PREFIX}/mediflow/health"
```

Example with direct URL (base path is `/{component-name}`). For this sample, the component name is `patient-management-service`:

```bash
curl http://development-default.openchoreoapis.localhost:19080/patient-management-service/mediflow/health
```

### Add a new patient
```bash
curl -X POST "http://${HOSTNAME}:19080${PATH_PREFIX}/mediflow/patients" \
-H "Content-Type: application/json" \
-d '{
"name": "Alice",
"age": 30,
"condition": "Healthy"
}'
```

### Retrieve a patient by name
```bash
curl "http://${HOSTNAME}:19080${PATH_PREFIX}/mediflow/patients/Alice"
```

### List all patients
```bash
curl "http://${HOSTNAME}:19080${PATH_PREFIX}/mediflow/patients"
```

## Troubleshooting

### Workflow Issues

If the workflow fails or takes too long:

1. **Check ComponentWorkflowRun status and conditions:**
   ```bash
   kubectl get componentworkflowrun patient-management-service-build-01 -n default -o jsonpath='{.status.conditions}' | jq .
   ```

2. **Check workflow pod status:**
   ```bash
   kubectl get pods -n openchoreo-ci-default | grep patient-management
   ```

3. **View workflow pod logs for errors:**
   ```bash
   # Get the pod name
   POD_NAME=$(kubectl get pods -n openchoreo-ci-default -l component-workflows.argoproj.io/workflow=patient-management-service-build-01 --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[-1].metadata.name}')

   # View logs
   kubectl logs -n openchoreo-ci-default $POD_NAME
   ```

4. **Check if Workload was created after workflow:**
   ```bash
   kubectl get workload -n default | grep patient-management
   ```

### Deployment Issues

If the application is not accessible:

1. **Check ReleaseBinding status:**
   ```bash
   kubectl get releasebinding patient-management-service-development -n default -o yaml
   ```

2. **Check ReleaseBinding conditions:**
   ```bash
   kubectl get releasebinding patient-management-service-development -n default -o jsonpath='{.status.conditions}' | jq .
   ```

3. **Verify HTTPRoute is configured:**
   ```bash
   kubectl get httproute -A -l openchoreo.dev/component=patient-management-service -o yaml
   ```

4. **Check deployment status:**
   ```bash
   kubectl get deployment -A -l openchoreo.dev/component=patient-management-service
   ```

5. **Check pod logs:**
   ```bash
   kubectl logs -n $(kubectl get pods -A -l openchoreo.dev/component=patient-management-service -o jsonpath='{.items[0].metadata.namespace}') -l openchoreo.dev/component=patient-management-service --tail=50
   ```

## Clean Up

Remove all resources:

```bash
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/from-source/services/ballerina-buildpack-patient-management/patient-management-service.yaml
```
