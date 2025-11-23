# React Starter Web Application (From Image)

## Overview

This sample demonstrates how to deploy a React web application in OpenChoreo from a pre-built container image. The application uses a production-ready React SPA served on port 8080.

The application is deployed from the pre-built image:
`choreoanonymouspullable.azurecr.io/react-spa:v0.9`

## Step 1: Deploy the Application

The following command will create the relevant resources in OpenChoreo:

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.4/samples/from-image/react-starter-web-app/react-starter.yaml
```

> [!NOTE]
> Since this uses a pre-built image, the deployment will be faster compared to building from source.

## Step 2: Access the Application

Once the application is deployed and the port-forward is active, you can access the React application at:

```
http://react-starter-development.openchoreoapis.localhost:9080
```

## Troubleshooting Access Issues

If you cannot access the application:

1. Verify the web application URL:
   ```bash
   kubectl get httproute -A -l openchoreo.org/component=react-starter -o jsonpath='{.items[0].spec.hostnames[0]}'
   ```

## Clean Up

Stop the port forwarding and remove all resources:

```bash
# Remove all resources
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.4/samples/from-image/react-starter-web-app/react-starter.yaml
```
