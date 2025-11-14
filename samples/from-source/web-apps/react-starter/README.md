# React Starter Web Application

## Overview

This sample demonstrates how to deploy a React web application in OpenChoreo from source code. The application is built using Node.js 18 and served using nginx.

The source code is available at:
https://github.com/openchoreo/sample-workloads/tree/main/webapp-react-nginx

## Step 1: Deploy the Application

The following command will create the relevant resources in OpenChoreo. It will also trigger a build by creating a build resource.

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/web-apps/react-starter/react-web-app.yaml
```

> [!NOTE]
> The build will take around 5-8 minutes depending on the network speed and Node.js dependency installation.

## Step 2: Access the Application

Once the application is deployed and the port-forward is active, you can access the React application at:

```
http://react-starter-development-c37e66d8-development.openchoreoapis.localhost:9080
```

## Troubleshooting

### Build Issues
If the build fails, check the build status:

```bash
kubectl describe workflow react-starter-build-01
```

### Access Issues
If you cannot access the application:

1. Verify the web application URL:
   ```bash
   kubectl get httproute -A -l openchoreo.org/component=react-starter -o jsonpath='{.items[0].spec.hostnames[0]}'
   ```

## Clean Up

Stop the port forwarding and remove all resources:

```bash
# Remove all resources
kubectl delete https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/web-apps/react-starter/react-web-app.yaml
```
