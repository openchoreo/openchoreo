# Time Logger Scheduled Task

This is a simple program that logs the current time. This program can be deployed in Choreo to be executed periodically by creating a Scheduled Task.

The source code is available at:
https://github.com/wso2/choreo-samples/tree/main/docker-time-logger-schedule

## Deploy in Choreo

The following command will create the component, deployment track and the deployment in Choreo. It'll also trigger a build by creating a build resource. 

```bash
choreoctl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/deploying-applications/build-from-source/time-logger-task/time-logger.yaml
```

## Check the Build Workflow Status

You can check the build workflow status by running the following command.

```bash
choreoctl get build time-logger-build-01 --component time-logger
```

## Check the Build Workflow Logs

You can check the logs of the workflow by running the following command.

```bash
choreoctl logs --type build --build time-logger-build-01 --organization default-org --project default-project --component time-logger
```

> [!TIP]
> The build will take around 5 minutes depending on the network speed.
> You can check the status of the build by running the following command.
> `choreoctl get build time-logger-build-01 --component time-logger`

## Check the Deployment Status

You can check the deployment status by running the following command.

```shell
choreoctl get deployment --component time-logger
```

## Check the Deployment Logs

You can check the actual workload container's logs by running the following command.

```bash
choreoctl logs --type deployment --deployment time-logger-development-deployment --component time-logger
```

## Clean up

To clean up the resources created by this sample, run the following command.

```bash
choreoctl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/deploying-applications/build-from-source/time-logger-task/time-logger.yaml
```
