# HTTP Service Component with OpenAPI-derived routes

This sample shows how to render a Gateway API `HTTPRoute` whose matches come from an
endpoint's **OpenAPI schema** — one match per `(path, method)` — instead of a single
catch-all rule. It deploys the public [`reading-list`](https://github.com/openchoreo/samples)
service (`ghcr.io/openchoreo/samples/reading-list:latest`), which serves a small Books
API under `/api/v1/reading-list`.

The sample's YAML is self-contained: it defines a custom `ClusterComponentType`
(`http-openapi-service`) plus a `Component` + `Workload` + `ComponentReleaseBinding` that use it.

## Two ways to route an HTTP endpoint

1. **Prefix routing** (see [`samples/getting-started/component-types/service.yaml`](../../getting-started/component-types/service.yaml)) —
   a single `PathPrefix` match on a unique per-endpoint prefix (`/<component>-<endpoint>`)
   with a `ReplacePrefixMatch` rewrite to the endpoint `basePath`. Simple and catch-all:
   every path/method under the prefix is forwarded. The ComponentReleaseBinding controller derives
   the invoke URL from the first (and only) route match.

2. **Schema-driven routing** (this sample) — the template parses the endpoint's OpenAPI
   schema and renders **one match per operation**, so only declared paths/methods are
   exposed. The invoke URL can no longer be inferred from "the first match" (it would be a
   single operation), so the route declares its base path via an annotation.

## How it works

### `ClusterComponentType` (`http-openapi-service`)

Emits a `Deployment`, `Service`, `HTTPRoute`, and `TrafficPolicy`. The HTTPRoute uses the
opt-in **`workload.toEndpointResources(<endpointName>)`** macro to read the parsed OpenAPI schema and
build one rule per `(path, method)`:

- Each match is the unique gateway prefix `/<component>-<endpoint>` + the OpenAPI path.
- Static paths (`/books`) use an `Exact` match; parameterized paths (`/books/{id}`) use a
  `RegularExpression` match with `{param}` → `[^/]+` (Gateway API forbids `{` `}` in
  `Exact`/`PathPrefix` values, so templated paths must be regex).
- The route carries `annotations: openchoreo.dev/endpoint-base-path: /<component>-<endpoint>`
  so the ComponentReleaseBinding controller can build the invoke URL.

A single **`TrafficPolicy`** per endpoint rewrites the gateway prefix back to the real
`basePath`, preserving everything after it (including the path parameter captured by the
regex match):

```yaml
urlRewrite:
  pathRegex:
    pattern: "^/<component>-<endpoint>(/.*)?$"
    substitution: "/api/v1/reading-list\\1"
```

One rewrite covers every operation of the endpoint — the per-operation precision lives in
the HTTPRoute matches, the rewrite is a uniform prefix-swap.

### `Workload` (`demo-app-reading-list-workload`)

Declares a `reading-list-api` HTTP endpoint on port `8080` with `basePath: /api/v1/reading-list`
and an inline OpenAPI `schema` (`type: openapi`). The controller parses this schema **only
because** the ComponentType template references `workload.toEndpointResources(<endpointName>)` — the macro
is opt-in, so endpoints whose templates don't use it incur no schema parsing.

## Prerequisites

`TrafficPolicy` is a [kgateway](https://kgateway.dev) CRD (`gateway.kgateway.dev`). The
data-plane cluster-agent must be allowed to manage it; the data-plane Helm chart grants
`trafficpolicies` to the cluster-agent ClusterRole. If you see a `forbidden` error applying
the TrafficPolicy, add `trafficpolicies` to that role:

```bash
kubectl get clusterrole cluster-agent-dataplane-openchoreo-data-plane \
  -o jsonpath='{range .rules[?(@.apiGroups[0]=="gateway.kgateway.dev")]}{.resources}{"\n"}{end}'
```

## Deploy

```bash
kubectl apply --server-side -f samples/component-types/component-http-openapi-service/http-openapi-service-component.yaml
```

## Inspect the rendered routing

```bash
# One HTTPRoute rule per (path, method) from the OpenAPI schema
kubectl get httproute -A -l openchoreo.dev/component=demo-app-reading-list \
  -o jsonpath='{.items[0].spec.rules}' | jq '.[] | {path: .matches[0].path, method: .matches[0].method}'

# The single prefix-stripping rewrite
kubectl get trafficpolicy -A -l openchoreo.dev/component=demo-app-reading-list \
  -o jsonpath='{.items[0].spec.urlRewrite.pathRegex}' | jq .
```

Expected matches:

```json
{"path":{"type":"Exact","value":"/demo-app-reading-list-reading-list-api/books"},"method":"GET"}
{"path":{"type":"Exact","value":"/demo-app-reading-list-reading-list-api/books"},"method":"POST"}
{"path":{"type":"RegularExpression","value":"/demo-app-reading-list-reading-list-api/books/[^/]+"},"method":"GET"}
{"path":{"type":"RegularExpression","value":"/demo-app-reading-list-reading-list-api/books/[^/]+"},"method":"PUT"}
{"path":{"type":"RegularExpression","value":"/demo-app-reading-list-reading-list-api/books/[^/]+"},"method":"DELETE"}
```

## Check the invoke URL

```bash
kubectl get componentreleasebinding demo-app-reading-list-development \
  -o jsonpath='{.status.endpoints[0].externalURLs.http}' | jq .
```

The path is the gateway base path from the annotation (not a specific operation):

```json
{
  "host": "development-default.openchoreoapis.localhost",
  "port": 19080,
  "path": "/demo-app-reading-list-reading-list-api",
  "scheme": "http"
}
```

## Invoke the service

Requests go to the unique gateway prefix; the TrafficPolicy rewrites it to the backend's
`/api/v1/reading-list` base path.

```bash
HOST=development-default.openchoreoapis.localhost
BASE=http://127.0.0.1:19080/demo-app-reading-list-reading-list-api

# POST /books  (Exact match)
curl -s -X POST -H "Host: $HOST" -H 'Content-Type: application/json' \
  -d '{"title":"Dune","author":"Frank Herbert","status":"to_read"}' "$BASE/books"

# GET /books  (Exact match)
curl -s -H "Host: $HOST" "$BASE/books"

# GET /books/{id}  (RegularExpression match; the id is preserved through the rewrite)
ID=$(curl -s -H "Host: $HOST" "$BASE/books" | jq -r '.[0].id')
curl -s -H "Host: $HOST" "$BASE/books/$ID"
```

Only declared `(path, method)` pairs route; undeclared paths/methods and deeper path
segments return `404` (e.g. `GET /books/$ID/extra`, `GET /authors`, `PATCH /books`).

> If `development-default.openchoreoapis.localhost` resolves to `127.0.0.1` on your machine,
> drop the `-H "Host: …"` and dial the hostname directly.

## Cleanup

```bash
kubectl delete -f samples/component-types/component-http-openapi-service/http-openapi-service-component.yaml
```

## Troubleshooting

```bash
# ComponentReleaseBinding conditions
kubectl get componentreleasebinding demo-app-reading-list-development -o jsonpath='{.status.conditions}' | jq .

# Route accepted by the gateway
kubectl get httproute -A -l openchoreo.dev/component=demo-app-reading-list -o yaml

# TrafficPolicy accepted + attached
kubectl get trafficpolicy -A -l openchoreo.dev/component=demo-app-reading-list

# Deployment / pods
kubectl get deployment,pods -A -l openchoreo.dev/component=demo-app-reading-list
```

- **HTTPRoute has no rules** — the endpoint has no parseable schema (this ComponentType
  requires one; there is no catch-all fallback). The controller logs a `schemaextract`
  warning when schema content fails to parse.
- **`TrafficPolicy ... is forbidden`** — the data-plane cluster-agent lacks RBAC for
  `trafficpolicies.gateway.kgateway.dev` (see [Prerequisites](#prerequisites)).
