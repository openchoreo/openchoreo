# TLS Service Component Sample

This sample demonstrates how to deploy a service that is exposed via a Gateway API `TLSRoute` in **SNI passthrough** mode. The gateway routes encrypted traffic by SNI without terminating TLS; the workload terminates TLS itself using a self-signed certificate mounted via a ConfigMap.

## Overview

The sample's YAML is self-contained: it defines a custom `ClusterComponentType` and the `Component` + `Workload` + `ReleaseBinding` that consume it.

### ClusterComponentType (`tls-service`)

A reusable template for SNI-passthrough services. It:

- Sets the workload type to `deployment`
- Templates the underlying Kubernetes resources (`Deployment`, `Service`, `TLSRoute`, and one `ConfigMap` per workload file)
- Mounts the `Workload.container.files` entries into the pod via `configurations.toContainerVolumeMounts()` / `toVolumes()`
- Targets the gateway listener named `tls-passthrough` (overridable per environment)

### Component (`demo-app-tls-service`)

References the `deployment/tls-service` ClusterComponentType.

### Workload (`demo-app-tls-service-workload`)

Runs `nginx:1.27-alpine`, exposes an `https` endpoint on port `8443`, and mounts three files through `container.files`:

- `/etc/nginx/conf.d/default.conf` — an nginx server block that terminates TLS on `:8443`
- `/etc/nginx/certs/tls.crt` — the self-signed server certificate
- `/etc/nginx/certs/tls.key` — the matching private key

The cert and key inlined into the YAML are the same as the `tls.crt`/`tls.key` files committed to this directory, so you can re-use them as the CA bundle when invoking the service.

### ReleaseBinding (`demo-app-tls-service-development`)

Deploys the component to the `development` environment with a single replica.

## How It Works

The OpenChoreo controller manager renders the templates into a `Deployment`, `Service`, `TLSRoute`, and three `ConfigMap`s (one per mounted file). The ReleaseBinding controller resolves the endpoint URL from the rendered `TLSRoute` and writes it to `.status.endpoints[].externalURLs.tls`:

- `externalURLs.tls` — the URL served on the gateway's `tls-passthrough` listener. The scheme matches the application protocol the workload terminates (here `https`, because the workload endpoint type is `HTTP` over TLS).

## Deploy the sample

```bash
kubectl apply --server-side -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/release-v1.2.0-m.1/samples/component-types/component-tls-service/tls-service-component.yaml
```

## Check the ReleaseBinding status

```bash
kubectl get releasebinding demo-app-tls-service-development -o jsonpath='{.status.endpoints}' | jq .
```

Expected:

```json
[
  {
    "name": "https",
    "type": "HTTP",
    "serviceURL": {
      "host": "demo-app-tls-service.dp-default-default-development-f8e58905.svc.cluster.local",
      "port": 8443,
      "scheme": "http"
    },
    "externalURLs": {
      "tls": {
        "host": "development-default.openchoreoapis.localhost",
        "port": 19444,
        "scheme": "https"
      }
    }
  }
]
```

## Invoke the service

The gateway routes by SNI, so the request hostname must be `development-default.openchoreoapis.localhost`. If your machine doesn't resolve that hostname, use `--resolve` to point it at `127.0.0.1`.

### curl with the CA bundle

The `tls.crt` file in this directory is the self-signed CA for the workload cert — use it with `--cacert` so curl validates the chain instead of needing `-k`:

```bash
curl --cacert samples/component-types/component-tls-service/tls.crt \
  --resolve development-default.openchoreoapis.localhost:19444:127.0.0.1 \
  https://development-default.openchoreoapis.localhost:19444/
```

Expected output:

```text
Hello from the TLS-passthrough workload!
```

### Fetch the cert via openssl s_client

You can also pull the live cert that nginx is presenting through the passthrough listener:

```bash
openssl s_client -connect 127.0.0.1:19444 \
  -servername development-default.openchoreoapis.localhost \
  -showcerts </dev/null 2>/dev/null \
  | openssl x509 -noout -subject -issuer -ext subjectAltName
```

Expected:

```text
subject=CN=development-default.openchoreoapis.localhost
issuer=CN=development-default.openchoreoapis.localhost
X509v3 Subject Alternative Name:
    DNS:development-default.openchoreoapis.localhost, DNS:*.openchoreoapis.localhost
```

The `subject == issuer` confirms it's the self-signed workload cert, not a gateway-terminated cert — proof that TLS is being passed through.

## Regenerate the certs

If you need to refresh the cert (e.g. with a different hostname), regenerate it with `openssl` and update the inlined PEM blocks in `tls-service-component.yaml`:

```bash
openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
  -keyout tls.key -out tls.crt \
  -subj "/CN=development-default.openchoreoapis.localhost" \
  -addext "subjectAltName=DNS:development-default.openchoreoapis.localhost,DNS:*.openchoreoapis.localhost"
```

## Cleanup

```bash
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/release-v1.2.0-m.1/samples/component-types/component-tls-service/tls-service-component.yaml
```

## Troubleshooting

1. **Check ReleaseBinding conditions:**

   ```bash
   kubectl get releasebinding demo-app-tls-service-development -o jsonpath='{.status.conditions}' | jq .
   ```

2. **Verify the TLSRoute is rendered and accepted:**

   ```bash
   kubectl get tlsroute -A -l openchoreo.dev/component=demo-app-tls-service \
     -o jsonpath='{range .items[*]}{.metadata.name} sectionName={.spec.parentRefs[0].sectionName} reason={.status.parents[0].conditions[?(@.type=="Accepted")].reason}{"\n"}{end}'
   ```

3. **Check the pod and verify nginx is listening on `:8443` with the mounted cert:**

   ```bash
   POD=$(kubectl get pods -A -l openchoreo.dev/component=demo-app-tls-service -o jsonpath='{.items[0].metadata.name}')
   NS=$(kubectl get pods -A -l openchoreo.dev/component=demo-app-tls-service -o jsonpath='{.items[0].metadata.namespace}')
   kubectl exec -n "$NS" "$POD" -c main -- nginx -T 2>&1 | grep -E "listen|ssl_certificate"
   ```

4. **Another TLSRoute may be claiming the same SNI hostname.** TLS routing is SNI-only, so two TLSRoutes with the same hostname in the same gateway listener will collide. Inspect both:

   ```bash
   kubectl get tlsroute -A -o jsonpath='{range .items[*]}{.metadata.name}: hostnames={.spec.hostnames}{"\n"}{end}'
   ```
