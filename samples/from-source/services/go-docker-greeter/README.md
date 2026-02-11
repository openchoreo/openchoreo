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

The following command will create the relevant resources in OpenChoreo. It will also trigger a workflow by creating a workflow resource.

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/services/go-docker-greeter/greeting-service.yaml
```

> [!NOTE]
> The Docker workflow will take around 3-5 minutes depending on the network speed and system resources.

## Step 2: Monitor the Workflow

After deploying, monitor the workflow progress:

```bash
# Check ComponentWorkflowRun status
kubectl get componentworkflowrun greeting-service-build-01 -n default -o jsonpath='{.status.conditions}' | jq .

# Watch workflow pods (in openchoreo-ci-default namespace)
kubectl get pods -n openchoreo-ci-default | grep greeting-service

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
kubectl get releasebinding greeting-service-development -n default -o jsonpath='{.status.conditions}' | jq .

# Verify deployment is ready
kubectl get deployment -A -l openchoreo.dev/component=greeting-service
```

## Step 4: Test the Application

First, get the service URL from the HTTPRoute:

```bash
# Get the hostname and path prefix from the HTTPRoute
HOSTNAME=$(kubectl get httproute -A -l openchoreo.dev/component=greeting-service -o jsonpath='{.items[0].spec.hostnames[0]}')
PATH_PREFIX=$(kubectl get httproute -A -l openchoreo.dev/component=greeting-service -o jsonpath='{.items[0].spec.rules[0].matches[0].path.value}')
```

### Basic Greet
```bash
curl "http://${HOSTNAME}:19080${PATH_PREFIX}/greeter/greet"
```

### Greet with name
```bash
curl "http://${HOSTNAME}:19080${PATH_PREFIX}/greeter/greet?name=Alice"
```

### Example with direct URL
```bash
curl http://development-default.openchoreoapis.localhost:19080/greeting-service/greeter/greet
```

## Troubleshooting

### Workflow Issues

If the workflow fails or takes too long:

1. **Check ComponentWorkflowRun status and conditions:**
   ```bash
   kubectl get componentworkflowrun greeting-service-build-01 -n default -o jsonpath='{.status.conditions}' | jq .
   ```

2. **Check workflow pod status:**
   ```bash
   kubectl get pods -n openchoreo-ci-default | grep greeting-service
   ```

3. **View workflow pod logs for errors:**
   ```bash
   # Get the pod name
   POD_NAME=$(kubectl get pods -n openchoreo-ci-default -l component-workflows.argoproj.io/workflow=greeting-service-build-01 --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[-1].metadata.name}')

   # View logs
   kubectl logs -n openchoreo-ci-default $POD_NAME
   ```

4. **Check if Workload was created after workflow:**
   ```bash
   kubectl get workload -n default | grep greeting-service
   ```

### Deployment Issues

If the application is not accessible:

1. **Check ReleaseBinding status:**
   ```bash
   kubectl get releasebinding greeting-service-development -n default -o yaml
   ```

2. **Check ReleaseBinding conditions:**
   ```bash
   kubectl get releasebinding greeting-service-development -n default -o jsonpath='{.status.conditions}' | jq .
   ```

3. **Verify HTTPRoute is configured:**
   ```bash
   kubectl get httproute -A -l openchoreo.dev/component=greeting-service -o yaml
   ```

4. **Check deployment status:**
   ```bash
   kubectl get deployment -A -l openchoreo.dev/component=greeting-service
   ```

5. **Check pod logs:**
   ```bash
   kubectl logs -n $(kubectl get pods -A -l openchoreo.dev/component=greeting-service -o jsonpath='{.items[0].metadata.namespace}') -l openchoreo.dev/component=greeting-service --tail=50
   ```

## Clean Up

Remove all resources:

```bash
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/services/go-docker-greeter/greeting-service.yaml
```
