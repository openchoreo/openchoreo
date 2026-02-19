# React Starter Web Application

## Overview

This sample demonstrates how to deploy a React web application in OpenChoreo from source code. The application is built using Node.js 20 and served using nginx.

The source code is available at:
https://github.com/openchoreo/sample-workloads/tree/main/webapp-react-nginx

## Step 1: Deploy the Application

The following command will create the relevant resources in OpenChoreo:

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/from-source/web-apps/react-starter/react-web-app.yaml
```

> [!NOTE]
> The React workflow will take around 4-6 minutes depending on the network speed and Node.js dependency installation.

## Step 2: Monitor the Workflow

After deploying, monitor the workflow progress:

```bash
# Check ComponentWorkflowRun status
kubectl get componentworkflowrun react-starter-build-01 -n default -o jsonpath='{.status.conditions}' | jq .

# Watch build pods (in openchoreo-ci-default namespace)
kubectl get pods -n openchoreo-ci-default | grep react-starter

# View build logs (replace <pod-name> with actual pod name)
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
kubectl get releasebinding react-starter-from-source-development -n default -o jsonpath='{.status.conditions}' | jq .

# Verify deployment is ready
kubectl get deployment -A -l openchoreo.dev/component=react-starter-from-source
```

## Step 4: Access the Application

Once the application is deployed, you can access the React application at:


http://react-starter-from-source-development-default.openchoreoapis.localhost:19080


You can also dynamically get the URL using:

```bash
HOSTNAME=$(kubectl get httproute -A -l openchoreo.dev/component=react-starter-from-source -o jsonpath='{.items[0].spec.hostnames[0]}')
echo "Access the application at: http://${HOSTNAME}:19080"
```

## Troubleshooting

### Workflow Issues

If the workflow fails or takes too long:

1. **Check ComponentWorkflowRun status and conditions:**
   ```bash
   kubectl get componentworkflowrun react-starter-build-01 -n default -o jsonpath='{.status.conditions}' | jq .
   ```

2. **Check workflow pod status:**
   ```bash
   kubectl get pods -n openchoreo-ci-default | grep react-starter
   ```

3. **View workflow pod logs for errors:**
   ```bash
   # Get the pod name
   POD_NAME=$(kubectl get pods -n openchoreo-ci-default -l component-workflows.argoproj.io/workflow=react-starter-build-01 --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[-1].metadata.name}')

   # View logs
   kubectl logs -n openchoreo-ci-default $POD_NAME
   ```

4. **Check if Workload was created after workflow:**
   ```bash
   kubectl get workload -n default | grep react-starter
   ```

### Deployment Issues

If the application is not accessible:

1. **Check ReleaseBinding status:**
   ```bash
   kubectl get releasebinding react-starter-from-source-development -n default -o yaml
   ```

2. **Check ReleaseBinding conditions:**
   ```bash
   kubectl get releasebinding react-starter-from-source-development -n default -o jsonpath='{.status.conditions}' | jq .
   ```

3. **Verify HTTPRoute is configured:**
   ```bash
   kubectl get httproute -A -l openchoreo.dev/component=react-starter-from-source -o yaml
   ```

4. **Check deployment status:**
   ```bash
   kubectl get deployment -A -l openchoreo.dev/component=react-starter-from-source
   ```

5. **Check pod logs:**
   ```bash
   kubectl logs -n $(kubectl get pods -A -l openchoreo.dev/component=react-starter-from-source -o jsonpath='{.items[0].metadata.namespace}') -l openchoreo.dev/component=react-starter-from-source --tail=50
   ```

6. **Verify the web application URL:**
   ```bash
   kubectl get httproute -A -l openchoreo.dev/component=react-starter-from-source -o jsonpath='{.items[0].spec.hostnames[0]}'
   ```

## Clean Up

Remove all resources:

```bash
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/from-source/web-apps/react-starter/react-web-app.yaml
```
