# React Starter Web Application - Container Image

This sample demonstrates how to deploy a web application as a container image in Choreo without the source code.

## Pre-requisites

- Kubernetes cluster with Choreo installed
- The `choreoctl` CLI tool installed

## 1. Deploy in Choreo

The following command will create the component, deployment track, deployable artifact and the deployment in Choreo.

```bash
choreoctl apply -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/deploying-applications/use-prebuilt-image/react-spa-webapp/react-starter.yaml
```

## 2. Retrieve the invocation URL

You can retrieve the invocation URL of the deployment by running the following command.

```bash
choreoctl get endpoint --organization default-org --project default-project --component  react-starter-image
```

This will output the endpoints in the `default-org` namespace. For this specific sample
you will see an endpoint with the name starting with `react-starter-image-deployment-webapp-<hash>`. 
It should have the address as `https://react-starter-image-development.choreoapps.localhost`

## 3. Access the web application

For this sample, we will use kubectl port-forward to access the web application.

I. Run the following command to port-forward the gateway.

    ```bash
    kubectl port-forward svc/choreo-external-gateway -n choreo-system 8443:443 &
    ```

II. Access the web application from your browser using the following URL.
    - https://react-starter-image-development.choreoapps.localhost:8443


## 4. Cleanup

To clean up the resources created by this sample, you can run the following command:

```bash
choreoctl delete -f https://raw.githubusercontent.com/choreo-idp/choreo/main/samples/deploying-applications/use-prebuilt-image/react-spa-webapp/react-starter.yaml
```
