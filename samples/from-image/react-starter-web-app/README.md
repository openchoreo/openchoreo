# React Starter Web Application (From Image)

## Overview

This sample demonstrates how to deploy a React web application in OpenChoreo from a pre-built container image. The application uses a production-ready React SPA served on port 8080.

The application is deployed from the pre-built image:
`choreoanonymouspullable.azurecr.io/react-spa:v0.9`

## Step 1: Deploy the Application

The following command will create the relevant resources in OpenChoreo:

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/react-starter-web-app/react-starter.yaml
```

> [!NOTE]
> Since this uses a pre-built image, the deployment will be faster compared to building from source.

## Step 2: Port-forward the OpenChoreo Gateway

Port forward the OpenChoreo gateway service to access the web application locally:

```bash
kubectl port-forward -n openchoreo-data-plane svc/gateway-external 8443:443 &
```

## Step 3: Access the Application

Once the application is deployed and the port-forward is active, you can access the React application at:

```
https://react-starter-development.choreoapps.localhost:8443
```

> [!IMPORTANT]
> Since this uses a self-signed certificate, your browser will show a security warning. You need to:
> 1. Click "Advanced" when you see the security warning
> 2. Click "Proceed to react-starter-development.choreoapps.localhost (unsafe)"

## Troubleshooting Access Issues

If you cannot access the application:

1. Ensure the port-forward is running:
   ```bash
   ps aux | grep port-forward
   ```

2. Verify the web application URL:
   ```bash
   kubectl get webapplicationbindings.openchoreo.dev react-starter -o jsonpath='{.status.endpoints[0].public.uri}'
   ```

## Clean Up

Stop the port forwarding and remove all resources:

```bash
# Find and stop the specific port-forward process
pkill -f "port-forward.*gateway-external.*8443:443"

# Remove all resources
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/react-starter-web-app/react-starter.yaml
```
