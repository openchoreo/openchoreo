# Snip URL Shortener on OpenChoreo

A sample application that demonstrates OpenChoreo's **tracing**, **alerting**, and **RCA agent** capabilities using a multi-service URL shortener. This variant builds from source using Dockerfile workflows.

> If you prefer to deploy with pre-built images (no build step), see the [from-image version](../../../from-image/url-shortener/README.md).

## Prerequisites

- An OpenChoreo cluster with the control plane and observability plane installed (and RCA agent setup for RCA)
- `kubectl` access to the cluster

## Deploy

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/url-shortener/alerting-demo/alert-notification-channels.yaml
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/projects/url-shortener/project.yaml
```

`project.yaml` also includes the `ProjectReleaseBinding`s for each pipeline environment, which own the project's data-plane namespaces. Wait for the development one to become `Ready=True` before continuing, otherwise the Resource and Component steps below will fail to render with a `namespace ... not found` error:

```bash
kubectl get projectreleasebinding url-shortener-development -n default -w
```

Postgres is provisioned as a `Resource` from the shipped `postgres` `ClusterResourceType` rather than as a Component, so it needs an explicit `ResourceReleaseBinding` (Components get one automatically via `autoDeploy`). The `urls`/`clicks` schema — previously baked into a custom Postgres image built from source — is now seeded via the CRT's `initSQL` parameter (see `resources/postgres.yaml`), so no separate build or migration step is needed.

`initSQL` support was added to the `postgres` CRT after the initial `getting-started/all.yaml` install that most setups run — if your cluster already had the CRT installed, `resources/postgres.yaml`'s `initSQL` will silently do nothing until the CRT itself is updated. Re-apply it (idempotent, safe even if already up to date) before deploying the Resource:

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/getting-started/cluster-resource-types/postgres.yaml
```

Apply the Resource + binding, then promote the binding to the resource's latest release:

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/projects/url-shortener/resources/postgres.yaml

for i in $(seq 1 150); do release=$(kubectl get resource snip-postgres -n default -o jsonpath='{.status.latestRelease.name}') && [ -n "$release" ] && break; sleep 2; done
[ -n "$release" ] || { echo "Timed out waiting for snip-postgres latestRelease name"; kubectl get resource snip-postgres -n default -o yaml; exit 1; }
kubectl patch resourcereleasebinding snip-postgres-development -n default \
  --type=merge -p "{\"spec\":{\"resourceRelease\":\"$release\"}}"
```

Wait for the binding to reach `Ready=True`:

```bash
kubectl wait --for=condition=Ready --timeout=5m resourcereleasebinding snip-postgres-development -n default
```

Now deploy the components:

```bash
kubectl apply \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/projects/url-shortener/components/redis.yaml \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/projects/url-shortener/components/api-service.yaml \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/projects/url-shortener/components/analytics-service.yaml \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/projects/url-shortener/components/frontend.yaml
```

This deploys four components (snip-redis, snip-api-service, snip-analytics-service, snip-frontend) built from source via Dockerfile workflows. The api-service and analytics-service consume Postgres via `dependencies.resources[]` — the CRT's `url` output (a full DSN) is injected as `POSTGRES_DSN`. The alert trait is already attached to the frontend component. Distributed tracing works out of the box once deployed.

To set up alerting and the RCA agent, follow the steps in the [Alerting & RCA Agent](../../../from-image/url-shortener/README.md#alerting--rca-agent) section of the from-image README.

## Cleanup

None of these objects use Kubernetes owner references, so deleting the `Project` alone leaves the Resource and bindings behind. Delete everything explicitly instead — this also tears down Postgres's `StatefulSet` and its PVC (via `persistentVolumeClaimRetentionPolicy.whenDeleted: Delete`, set when `persistenceEnabled` is true) through the `ResourceReleaseBinding`'s finalizer:

```bash
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/projects/url-shortener/components/frontend.yaml
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/projects/url-shortener/components/analytics-service.yaml
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/projects/url-shortener/components/api-service.yaml
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/projects/url-shortener/components/redis.yaml
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/projects/url-shortener/resources/postgres.yaml
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/projects/url-shortener/project.yaml
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/url-shortener/alerting-demo/alert-notification-channels.yaml
```
