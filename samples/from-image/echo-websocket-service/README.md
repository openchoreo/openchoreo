# Echo WebSocket Service (From Image)

## Overview

This sample demonstrates how to deploy a WebSocket echo service in OpenChoreo from a pre-built container image. The service accepts WebSocket connections and echoes back any messages sent to it.

The service is deployed from the pre-built image:
`jmalloc/echo-server:latest`

### WebSocket Endpoint

**Endpoint:** `/.ws`
**Protocol:** WebSocket
**Functionality:** Echoes back any message sent by the client.

## Step 1: Enable WebSockets at the Ingress Gateway

Before deploying the WebSocket service, you need to enable WebSocket upgrades at the ingress gateway by applying the following HTTPListenerPolicy:

```bash
kubectl apply -f - <<EOF
apiVersion: gateway.kgateway.dev/v1alpha1
kind: HTTPListenerPolicy
metadata:
  name: default-httplistenerpolicy
  namespace: openchoreo-data-plane
spec:
  targetRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: gateway-default
  upgradeConfig:
    enabledUpgrades:
    - websocket
EOF
```

## Step 2: Deploy the Application

The following command will create the relevant resources in OpenChoreo:

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/from-image/echo-websocket-service/echo-websocket-service.yaml
```

> [!NOTE]
> Since this uses a pre-built image, the deployment will be faster compared to building from source.

## Step 3: Test the Application

You can test the WebSocket service using `wscat` (install via `npm install -g wscat`).

### Connect to the WebSocket

The service is exposed at the base path `/{component-name}`. For this sample, the component name is `echo-websocket-service`.

```bash
wscat -c "ws://localhost:19080/echo-websocket-service/.ws" --header "Host: development-default.openchoreoapis.localhost"
```

Once connected, type any message and press Enter. The server will echo back the same message.

### Example Session

```
Connected (press CTRL+C to quit)
> Hello, WebSocket!
< Hello, WebSocket!
> Test message
< Test message
```

## Troubleshooting Service Access Issues

If you cannot access the service:

1. Check if the ReleaseBinding is ready:
   ```bash
   kubectl get releasebinding echo-websocket-service-development -n default -o yaml
   ```

2. Check the ReleaseBinding status conditions:
   ```bash
   kubectl get releasebinding echo-websocket-service-development -n default -o jsonpath='{.status.conditions}' | jq .
   ```

3. Verify the HTTPRoute is configured correctly:
   ```bash
   kubectl get httproute -A -l openchoreo.dev/component=echo-websocket-service -o yaml
   ```

4. Check the deployment status:
   ```bash
   kubectl get deployment -A -l openchoreo.dev/component=echo-websocket-service
   ```

## Clean Up

Remove all resources:

```bash
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.15/samples/from-image/echo-websocket-service/echo-websocket-service.yaml
```
