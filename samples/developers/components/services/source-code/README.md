# Reading List Service
This is a simple REST API to manage a reading list of books. The service supports adding, retrieving, updating, and deleting books. This program can be deployed in Choreo to manage your personal reading list.

## Deploy in Choreo
The following command will create the component, deployment track, and deployment in Choreo. It will also trigger a build by creating a build resource.

```bash
kubectl apply -f samples/reading-list/source-code/reading-list-service.yaml
```

## Check the Argo Workflow Status
The Argo Workflow will create three tasks for building and deploying the service:

NAMESPACE	            NAME
choreo-ci-default-org	reading-list-service-build-01-clone-step-2264035552
choreo-ci-default-org	reading-list-service-build-01-build-step-3433253592
choreo-ci-default-org	reading-list-service-build-01-push-step-3448493733

You can check the status of the workflow by running the following command:

```bash
kubectl get pods -n choreo-ci-default-org
```
## Check Build Logs
You can check the build logs of each step by running the following commands:

