# Time Logger Scheduled Task
This is a simple program that logs the current time. This program can be deployed in Choreo to be executed periodically by creating a Scheduled Task.

The source code is available at:
https://github.com/wso2/choreo-samples/tree/main/docker-time-logger-schedule

## Deploy in Choreo
The following command will create the component, deployment track and the deployment in Choreo. It'll also trigger a build by creating a build resource. 

```bash
kubectl apply -f samples/deploying-applications/build-from-source/time-logger-task/time-logger.yaml
```
## Check the Build Workflow Status
You can check the logs of the workflow by running the following command.

```bash
choreoctl logs --type build --build time-logger-build-01 --organization default-org --project default-project --component time-logger
```

## Check the Deployment Status
You can check the deployment logs by running the following command.

```bash
choreoctl logs --type deployment --deployment time-logger-development-deployment-01 --organization default-org --project default-project --component time-logger
```

Note: You should see a k8s namespace created for your org, project and environment combination.
