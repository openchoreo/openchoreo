# React Starter Web Application

## Overview

This sample demonstrates how to deploy a React web application in OpenChoreo from source code. The application is built using Node.js 18 and served using nginx.

The source code is available at:
https://github.com/openchoreo/sample-workloads/tree/main/webapp-react-nginx

## Step 1: Deploy the Application

The following command will create the relevant resources in OpenChoreo. It will also trigger a build by creating a build resource.

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.3/samples/from-source/web-apps/react-starter/react-web-app.yaml
```

> [!NOTE]
> The build will take around 5-8 minutes depending on the network speed and Node.js dependency installation.

## Step 2: Port-forward the OpenChoreo Gateway

Port forward the OpenChoreo gateway service to access the web application locally after the build is completed:

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

## Troubleshooting

### Build Issues
If the build fails, check the build logs:

```bash
kubectl describe build react-starter-build-01
```

### Access Issues
If you cannot access the application:

1. Ensure the port-forward is running:
   ```bash
   ps aux | grep port-forward
   ```

2. Verify the web application URL:
   ```bash
   kubectl get webapplicationbindings.openchoreo.dev  react-starter -o jsonpath='{.status.endpoints[0].public.uri}'
   ```

## Clean Up

Stop the port forwarding and remove all resources:

```bash
# Find and stop the specific port-forward process
pkill -f "port-forward.*gateway-external.*8443:443"

# Remove all resources
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.3/samples/from-source/web-apps/react-starter/react-web-app.yaml
```
