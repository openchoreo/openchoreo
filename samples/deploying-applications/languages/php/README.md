# Reading List Php web app

## Overview
This is a simple hello world web application built with PHP.

The source code is available at:
https://github.com/wso2/choreo-samples/tree/main/hello-world-php-webapp

## Deploy in Choreo

```bash
choreoctl apply -f samples/deploying-applications/languages/php/hello-world-web-app.yaml
``` 

## Check the Build Workflow Status
You can check the logs of the workflow by running the following command.

```bash
choreoctl logs --type build --build hello-world-web-application-php-build-01 --organization default-org --project default-project --component hello-world-web-application-php
```

## Check the Deployment Status
You can check the deployment logs by running the following command.

```bash
choreoctl logs --type deployment --deployment hello-world-web-application-php-development-deployment-01 --organization default-org --project default-project --component hello-world-web-application-php
```

Note: You should see a k8s namespace created for your org, project and environment combination.

## Access the web application
For this sample, we will use kubectl port-forward to access the web application.

1. Run the following command to port-forward the gateway.

    ```bash
    kubectl port-forward svc/envoy-choreo-system-gateway-external-<hash> -n choreo-system 4430:443
    ```

   Note: You can find the <hash> part of the gateway name by running the following command:
    ```bash
    kubectl -n choreo-system get services
   ```

2. Access the web application from your browser using the following URL.
   - https://hello-world-web-application-php-development.choreo.localhost:4430
