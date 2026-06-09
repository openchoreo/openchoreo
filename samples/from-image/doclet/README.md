# Doclet on OpenChoreo (From Image)

A sample application that demonstrates how an OpenChoreo Workload consumes managed infrastructure via the resource abstraction model. Doclet is a small collaborative document editor: a React frontend, two Go services, and dependencies on Postgres and NATS.

## What this sample shows

- A **Project** with three Components (frontend, two services) and two **Resources** (Postgres + NATS) provisioned from the shipped `ClusterResourceType`s.
- The document service consumes Postgres outputs via `dependencies.resources[].envBindings` — five individual env vars (`DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`) wired to the container.
- Both Go services consume NATS via a single envBinding to the composite `url` output. The full `nats://<token>@<host>:<port>` URL is composed on the data plane; the token never reaches the control plane.
- The frontend wires `dependencies.endpoints[]` to the two backend services, and nginx reverse-proxies `/api/document-svc` and `/ws/collab-svc` to them.
- `ResourceReleaseBinding`s ship as YAML under `bindings/development/`, demonstrating per-environment overrides via `resourceTypeEnvironmentConfigs`. The dev bindings enable the bundled admin UIs (Adminer for Postgres, NATS `/varz`).

## Architecture

```
                  external HTTP
                       │
                       ▼
              ┌────────────────┐
              │ doclet-frontend│ (nginx + React, port 80)
              └───────┬────────┘
                      │ reverse proxy
        ┌─────────────┴────────────────┐
        ▼                              ▼
┌────────────────┐            ┌────────────────┐
│ doclet-document│            │ doclet-collab  │
│  (HTTP :8080)  │            │  (WS :8090)    │
└───────┬────────┘            └────────┬───────┘
        │                              │
        │   ┌──────────────────────────┤
        ▼   ▼                          ▼
 ┌────────────────┐            ┌────────────────┐
 │ doclet-postgres│            │  doclet-nats   │
 │  (ResourceType)│            │ (ResourceType) │
 └────────────────┘            └────────────────┘
```

## Prerequisites

- An OpenChoreo cluster with the control plane installed
- `kubectl` access to the cluster

## Step 1: Enable WebSockets at the data-plane gateway

The doclet frontend reverse-proxies a WebSocket sub-path (`/ws/collab-svc`) to the collab service. kgateway disables WebSocket upgrades by default; enable them on the data-plane Gateway with a one-time `HTTPListenerPolicy`:

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

## Step 2: Deploy

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/doclet/project.yaml

kubectl apply \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/doclet/resources/postgres.yaml \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/doclet/resources/nats.yaml \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/doclet/components/service-document.yaml \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/doclet/components/service-collab.yaml \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/doclet/components/frontend.yaml

kubectl apply \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/doclet/bindings/development/postgres.yaml \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/doclet/bindings/development/nats.yaml
```

Components have `autoDeploy: true` and create their `ReleaseBinding`s automatically. The `ResourceReleaseBinding`s under `bindings/development/` are applied with `spec.resourceRelease` intentionally unset — they will stay pending until promoted in the next step.

## Step 3: Promote each resource to the development environment

Pin each Resource's binding to its latest release:

```bash
for r in doclet-postgres doclet-nats; do
  release=$(kubectl get resource $r -n default -o jsonpath='{.status.latestRelease.name}')
  kubectl patch resourcereleasebinding $r-development -n default \
    --type=merge -p "{\"spec\":{\"resourceRelease\":\"$release\"}}"
done
```

Wait for everything to reach `Ready=True`:

```bash
kubectl get resourcereleasebinding,releasebinding -n default | grep doclet
```

## Access the frontend

Find the gateway URL from the frontend's `ReleaseBinding`:

```bash
kubectl get releasebinding doclet-frontend-development -n default -o jsonpath='{.status.endpoints[0].url}'
```

Open the URL in a browser. The React app loads, lists documents (empty on first launch), and lets you create and edit documents with real-time collaboration.

You can also smoke-test the document service directly through the frontend's reverse proxy:

```bash
GATEWAY=$(kubectl get releasebinding doclet-frontend-development -n default -o jsonpath='{.status.endpoints[0].url}')
curl -sS "$GATEWAY/api/document-svc/documents"
curl -sS -X POST -H 'Content-Type: application/json' -d '{"title":"smoke test"}' "$GATEWAY/api/document-svc/documents"
```

## Access the admin UIs (dev only)

The dev bindings enable `adminEnabled: true`, which spins up an admin UI for each resource. The URLs surface on the `ResourceReleaseBinding.status.outputs[*].adminURL`:

```bash
kubectl get resourcereleasebinding doclet-postgres-development -n default -o jsonpath='{.status.outputs[?(@.name=="adminURL")].value}'
kubectl get resourcereleasebinding doclet-nats-development     -n default -o jsonpath='{.status.outputs[?(@.name=="adminURL")].value}'
```

- **Postgres → Adminer**. Server, database, and username (`demo`) are pre-filled via the query string. Password is `demo` (printed in `status.outputs[*].adminPassword`). The `demo` superuser is created by the CRT's bootstrap script and exists only when `adminEnabled: true`; the application user has an ESO-generated random password that never appears in any output.
- **NATS → built-in monitoring**. Unauthenticated read-only `/varz`, `/connz`, `/subsz` JSON pages. Useful for verifying that the document and collab services have connected (`/connz` shows two client connections).

## How the resource wiring works

The document service's `Workload` (in `components/service-document.yaml`):

```yaml
dependencies:
  resources:
    - ref: doclet-postgres
      envBindings:
        host: DB_HOST
        port: DB_PORT
        username: DB_USER
        password: DB_PASSWORD
        database: DB_NAME
    - ref: doclet-nats
      envBindings:
        url: DOCLET_NATS_URL
```

At runtime the container sees:

- `DB_HOST=r-doclet-postgres-development-<hash>.<dp-namespace>.svc.cluster.local`
- `DB_PORT=5432`
- `DB_USER=doclet-postgres-user`
- `DB_PASSWORD=<32-char random, resolved from a DP-side Secret>`
- `DB_NAME=doclet`
- `DOCLET_NATS_URL=nats://<token>@r-doclet-nats-development-<hash>.<dp-namespace>.svc.cluster.local:4222`

The Postgres outputs are atomic (one env var per output). The NATS `url` is a composite output: ESO templates the full URL on the data plane using the generated token, then the consumer reads a single env var. The token bytes never reach the control plane.

## Cleanup

```bash
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/doclet/project.yaml
```

Deleting the Project cascades through Components, Resources, and bindings. Postgres and NATS use `retainPolicy: Delete` (the CRT default), so DP-side state is removed along with the bindings.
