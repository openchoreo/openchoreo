# Time Logger Scheduled Task
This is a simple program that logs the current time. This program can be deployed in Choreo to be executed periodically by creating a Scheduled Task.

## Deploy in Choreo
The following command will create the component, deployment track and the deployment in Choreo. It'll also trigger a build by creating a build resource. 

```bash
kubectl apply -f samples/scheduled-tasks/source-code/time-logger.yaml
```

## Check the Argo Workflow Status
Argo workflow will create three tasks.

```
NAMESPACE                       NAME 
choreo-ci-default-org           time-logger-build-01-clone-step-2264035552      
choreo-ci-default-org           time-logger-build-01-build-step-3433253592                        
choreo-ci-default-org           time-logger-build-01-push-step-3448493733                  
```

You can check the status of the workflow by running the following commands.

```bash
kubectl get pods -n choreo-ci-default-org
```

You can check build logs of each step by running the following commands.

```bash
kubectl -n choreo-ci-default-org logs -l workflow=time-logger-build-01,step=clone-step --tail=-1
kubectl -n choreo-ci-default-org logs -l workflow=time-logger-build-01,step=build-step --tail=-1
kubectl -n choreo-ci-default-org logs -l workflow=time-logger-build-01,step=push-step --tail=-1
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
