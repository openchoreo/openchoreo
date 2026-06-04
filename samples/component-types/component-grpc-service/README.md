# gRPC Service Component Sample

This sample demonstrates how to deploy a gRPC service component in OpenChoreo using a Gateway API `GRPCRoute`.

## Overview

The sample's YAML is self-contained: it defines a custom `ClusterComponentType` and a `Component` + `Workload` + `ReleaseBinding` that consume it.

### ClusterComponentType (`grpc-service`)

A reusable template for gRPC services. It:

- Sets the workload type to `deployment`
- Templates the underlying Kubernetes resources (`Deployment`, `Service`, `GRPCRoute`)
- Restricts endpoints to `type: gRPC`
- Emits one `GRPCRoute` per external endpoint and one per internal endpoint
- Gives each endpoint a **unique hostname** for multi-tenant isolation (see below)

### Component (`demo-app-grpc-service`)

References the `deployment/grpc-service` ClusterComponentType.

### Workload (`demo-app-grpc-service-workload`)

Specifies the container image ([`ghcr.io/openchoreo/samples/hello-world-grpc`](https://github.com/openchoreo/samples) — a public gRPC server that exposes `greeter.Greeter/SayHello` on port `9090` with reflection enabled) and a `grpc` endpoint. The endpoint carries an inline `schema` (`type: proto`) describing the `Greeter` service. The ClusterComponentType template opts into schema parsing via the `workload.toEndpointResources(<endpointName>)` macro; OpenChoreo then parses this proto at render time and exposes the extracted services/methods (keyed by endpoint name) so the generated `GRPCRoute` matches the exact `(service, method)` rather than routing all traffic. The macro is opt-in — schemas are only parsed when a template calls it — and if the schema is omitted the route falls back to a catch-all rule.

### ReleaseBinding (`demo-app-grpc-service-development`)

Deploys the component to the `development` environment with a single replica.

## How It Works

The OpenChoreo controller manager renders the ClusterComponentType templates into a `Deployment`, `Service`, and `GRPCRoute` for the data plane. The ReleaseBinding controller resolves the endpoint URLs from the rendered `GRPCRoute` and writes them to `.status.endpoints[].externalURLs`:

- `externalURLs.http` — cleartext gRPC over the gateway's `http` listener (scheme `grpc`)
- `externalURLs.https` — gRPC + TLS terminated at the gateway over the `https` listener (scheme `grpcs`)

### Hostname-based routing (multi-tenancy)

A gRPC request's HTTP/2 path **is** `/<package>.<Service>/<Method>`, so the gateway
routes by **hostname + service/method**. Unlike HTTP — where components on a shared host
are disambiguated by a unique path prefix (`/<component>-<endpoint>`) — gRPC has no base
path to attach a prefix to. So for multiple gRPC components to coexist on the same gateway
without colliding (e.g. two components both serving `greeter.Greeter`), each endpoint needs
its **own hostname**.

This ComponentType builds a unique subdomain per endpoint with `oc_dns_label(...)` (a
DNS-safe, ≤63-character label derived from component / endpoint / environment / namespace):

```
<oc_dns_label(component, endpoint, environment, namespace)>.openchoreoapis.localhost
# e.g. demo-app-grpc-grpc-development-default-3b9d1d04.openchoreoapis.localhost
```

The resolved `externalURLs` host (and the `:authority` clients dial) is this unique name.

> **Deployment note:** per-endpoint hostnames require wildcard DNS (`*.openchoreoapis.localhost`)
> and a matching wildcard gateway certificate in real environments. On k3d, `*.localhost`
> already resolves to `127.0.0.1`, so setting `grpcurl -authority <host>` is enough.

## Deploy the sample

```bash
kubectl apply --server-side -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/main/samples/component-types/component-grpc-service/grpc-service-component.yaml
```

## Check the ReleaseBinding status

```bash
kubectl get releasebinding demo-app-grpc-service-development -o jsonpath='{.status.endpoints}' | jq .
```

You should see something like:

```json
[
  {
    "name": "grpc",
    "type": "gRPC",
    "serviceURL": {
      "host": "demo-app-grpc-service.dp-default-default-development-f8e58905.svc.cluster.local",
      "port": 9090,
      "scheme": "grpc"
    },
    "externalURLs": {
      "http": {
        "host": "demo-app-grpc-grpc-development-default-3b9d1d04.openchoreoapis.localhost",
        "port": 19080,
        "scheme": "grpc"
      },
      "https": {
        "host": "demo-app-grpc-grpc-development-default-3b9d1d04.openchoreoapis.localhost",
        "port": 19443,
        "scheme": "grpcs"
      }
    }
  }
]
```

## Invoke the service

Because the generated `GRPCRoute` matches only the methods declared in the endpoint's proto schema (`greeter.Greeter/SayHello`), the gateway does **not** route reflection (`grpc.reflection.*`) or health (`grpc.health.v1.Health`) traffic. Supply the proto to `grpcurl` instead of relying on server reflection.

Create `greeter.proto`:

```proto
syntax = "proto3";
package greeter;
message HelloRequest { string name = 1; }
message HelloReply  { string message = 1; }
service Greeter {
  rpc SayHello (HelloRequest) returns (HelloReply);
}
```

Get the endpoint's unique hostname from the ReleaseBinding status, then invoke against the
cleartext gateway listener (the `:authority` selects the host the gateway routes on):

```bash
HOST=$(kubectl get releasebinding demo-app-grpc-service-development \
  -o jsonpath='{.status.endpoints[0].externalURLs.http.host}')

grpcurl -plaintext -import-path . -proto greeter.proto \
  -authority "$HOST" \
  -d '{"name":"OpenChoreo"}' \
  127.0.0.1:19080 \
  greeter.Greeter/SayHello
```

Expected response:

```json
{
  "message": "Hello, OpenChoreo!"
}
```

> `$HOST` is the per-endpoint subdomain (e.g. `demo-app-grpc-grpc-development-default-3b9d1d04.openchoreoapis.localhost`).
> Because `*.openchoreoapis.localhost` resolves to `127.0.0.1`, you can also `curl`/dial the
> hostname directly; `-authority` sets the gRPC `:authority` header the gateway matches on.
> The shared `development-default.openchoreoapis.localhost` host no longer routes to this
> service — that isolation is the point of the per-endpoint hostname.

## Cleanup

```bash
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/refs/heads/main/samples/component-types/component-grpc-service/grpc-service-component.yaml
```

## Troubleshooting

1. **Check ReleaseBinding conditions:**

   ```bash
   kubectl get releasebinding demo-app-grpc-service-development -o jsonpath='{.status.conditions}' | jq .
   ```

2. **Verify the GRPCRoute is rendered and accepted by the gateway:**

   ```bash
   kubectl get grpcroute -A -l openchoreo.dev/component=demo-app-grpc-service -o yaml
   ```

3. **Check deployment and pods:**

   ```bash
   kubectl get deployment,pods -A -l openchoreo.dev/component=demo-app-grpc-service
   ```

4. **Verify the rendered route carries the schema-derived method match:**

   ```bash
   kubectl get grpcroute -A -l openchoreo.dev/component=demo-app-grpc-service \
     -o jsonpath='{.items[0].spec.rules[0].matches}' | jq .
   ```

   Expect a match for `{service: greeter.Greeter, method: SayHello}`. If `matches` is absent, the route is a catch-all — the endpoint's proto schema was missing or failed to parse (the controller logs a `schemaextract` warning in that case). Note that listing services via reflection (`grpcurl ... list`) does **not** work through the gateway once explicit method matches are in effect, since reflection traffic is not routed.
