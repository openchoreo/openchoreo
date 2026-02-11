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

## Step 2: Access the Application

Once the application is deployed, you can access the React application at:

```
http://react-starter-development-default.openchoreoapis.localhost:19080
```

You can also dynamically get the URL using:

```bash
HOSTNAME=$(kubectl get httproute -A -l openchoreo.dev/component=react-starter -o jsonpath='{.items[0].spec.hostnames[0]}')
echo "Access the application at: http://${HOSTNAME}:19080"
```

## Troubleshooting Access Issues

If you cannot access the application:

1. **Check if the ReleaseBinding is ready:**
   ```bash
   kubectl get releasebinding react-starter-development -n default -o yaml
   ```

2. **Check the ReleaseBinding status conditions:**
   ```bash
   kubectl get releasebinding react-starter-development -n default -o jsonpath='{.status.conditions}' | jq .
   ```

3. **Verify the HTTPRoute is configured correctly:**
   ```bash
   kubectl get httproute -A -l openchoreo.dev/component=react-starter -o yaml
   ```

4. **Check the deployment status:**
   ```bash
   kubectl get deployment -A -l openchoreo.dev/component=react-starter
   ```

5. **Check pod logs for errors:**
   ```bash
   POD_UUID=$(kubectl get pods -A -l "$(kubectl get deploy -A -l openchoreo.dev/component=react-starter -o jsonpath='{.items[0].spec.selector.matchLabels}' | jq -r 'to_entries|map("\(.key)=\(.value)")|join(",")')" -o jsonpath='{range .items[*]}{.metadata.labels.openchoreo\.dev/component-uid}{"\n"}{end}' | sort -u | head -n1)
   POD_NAMESPACE=$(kubectl get deploy -A -l openchoreo.dev/component=react-starter -o jsonpath='{.items[0].metadata.namespace}')
   kubectl logs -n $POD_NAMESPACE -l openchoreo.dev/component-uid=$POD_UUID --tail=50
   ```

6. **Verify the web application URL:**
   ```bash
   kubectl get httproute -A -l openchoreo.dev/component=react-starter -o jsonpath='{.items[0].spec.hostnames[0]}'
   ```

## Clean Up

Remove all resources:

```bash
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/react-starter-web-app/react-starter.yaml
```
