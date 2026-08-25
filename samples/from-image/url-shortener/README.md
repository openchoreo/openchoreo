# Snip URL Shortener on OpenChoreo (From Image)

A sample application that demonstrates OpenChoreo's **tracing**, **alerting**, and **RCA agent** capabilities using a multi-service URL shortener. This variant uses pre-built container images so no build step is required.

> If you prefer to build from source, see the [from-source version](../../from-source/projects/url-shortener/README.md).

## Prerequisites

- An OpenChoreo cluster with the control plane and observability plane installed (and RCA agent setup for RCA)
- `kubectl` access to the cluster

## Deploy

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/url-shortener/alerting-demo/alert-notification-channels.yaml
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/url-shortener/project.yaml
```

`project.yaml` also includes the `ProjectReleaseBinding`s for each pipeline environment, which own the project's data-plane namespaces. Wait for the development one to become `Ready=True` before continuing, otherwise the Resource and Component steps below will fail to render with a `namespace ... not found` error:

```bash
kubectl get projectreleasebinding url-shortener-development -n default -w
```

Postgres is provisioned as a `Resource` from the shipped `postgres` `ClusterResourceType` rather than as a Component, so it needs an explicit `ResourceReleaseBinding` (Components get one automatically via `autoDeploy`). The `urls`/`clicks` schema — previously baked into a custom Postgres image — is now seeded via the CRT's `initSQL` parameter (see `resources/postgres.yaml`), so no separate migration step is needed.

`initSQL` support was added to the `postgres` CRT after the initial `getting-started/all.yaml` install most setups run — if your cluster already had the CRT installed, `resources/postgres.yaml`'s `initSQL` will silently do nothing until the CRT itself is updated. Re-apply it (idempotent, safe even if already up to date) before deploying the Resource:

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/getting-started/cluster-resource-types/postgres.yaml
```

Apply the Resource + binding, then promote the binding to the resource's latest release:

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/url-shortener/resources/postgres.yaml

for i in $(seq 1 150); do release=$(kubectl get resource snip-postgres -n default -o jsonpath='{.status.latestRelease.name}') && [ -n "$release" ] && break; sleep 2; done
[ -n "$release" ] || { echo "Timed out waiting for snip-postgres latestRelease name"; kubectl get resource snip-postgres -n default -o yaml; exit 1; }
kubectl patch resourcereleasebinding snip-postgres-development -n default \
  --type=merge -p "{\"spec\":{\"resourceRelease\":\"$release\"}}"
```

Wait for the binding to reach `Ready=True`:

```bash
kubectl wait --for=condition=Ready --timeout=5m resourcereleasebinding snip-postgres-development -n default
```

`initSQL` only runs on Postgres's first boot (initdb), so if you already had a running Postgres pod before promoting the binding above, delete it — and its PVC, since `resources/postgres.yaml` enables `persistenceEnabled` — to force a clean re-init against an empty data volume. Find the data-plane namespace and pod (named `r-snip-postgres-development-<hash>`, not `snip-postgres`) and delete both:

```bash
ns=$(kubectl get ns -o name | grep url-shortener-development | cut -d/ -f2)
kubectl delete -n "$ns" $(kubectl get pods -n "$ns" -o name | grep snip-postgres)
kubectl delete -n "$ns" $(kubectl get pvc -n "$ns" -o name | grep snip-postgres)
```

The frontend component has a log-based alert rule attached (`observability-alert-rule` trait, triggers when `status=500` appears more than 5 times within 1 minute). The trait's `enabled` defaults to `true`, and a notification channel is mandatory for any enabled alert rule — so `enable-alert.yaml` (which wires the trait to the `webhook-notification-channel-development` channel) must be applied *before* `frontend.yaml`. Applying it first means `autoDeploy` finds this `ReleaseBinding` already in place when the frontend Component is created and only patches in the release name, leaving the trait config untouched. Applying it after leaves the frontend's first render permanently failing validation (`A notification channel is mandatory for alert rules`) until you apply it:

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/url-shortener/alerting-demo/enable-alert.yaml
```

Now deploy the components:

```bash
kubectl apply \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/url-shortener/components/redis.yaml \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/url-shortener/components/api-service.yaml \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/url-shortener/components/analytics-service.yaml \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/url-shortener/components/frontend.yaml
```

This deploys the notification channel and the Postgres resource first, then four components (snip-redis, snip-api-service, snip-analytics-service, snip-frontend). The api-service and analytics-service consume Postgres via `dependencies.resources[]` — the CRT's `url` output (a full DSN) is injected as `POSTGRES_DSN`. Distributed tracing works out of the box once deployed.

For alerting and the RCA agent, see below.

## Alerting & RCA Agent

A log-based alert rule on the frontend triggers when `status=500` appears more than 5 times within 1 minute, and was already enabled and linked to the notification channel during Deploy above.

### Trigger the Alert

`failure-scenario.yaml` starves the Postgres StatefulSet of memory (via the `snip-postgres-development` ResourceReleaseBinding's `resourceTypeEnvironmentConfigs`), causing it to OOM-kill and crash-loop. The api-service stays up but every DB query fails while Postgres is down, returning 500s. This breaches the alert threshold. The RCA agent then traces from the frontend alert to api-service 500s to Postgres connection errors to the crash-looping Postgres pod.

`resources/postgres.yaml` enables `persistenceEnabled` on the Postgres Resource, so its data volume is a PVC (via `volumeClaimTemplates`) rather than `emptyDir`. Reverting the memory override below still causes the StatefulSet to roll the Postgres pod, but the PVC survives that pod recreate, so the `urls`/`clicks` data created before the incident is still there afterward.

Start generating traffic (auto-detects the frontend URL from the ReleaseBinding):

```bash
curl -sSL https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/url-shortener/alerting-demo/trigger-alerts.sh | bash
```

Starve Postgres of memory:

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/url-shortener/alerting-demo/failure-scenario.yaml
```

After the alert fires, revert by applying the fix from the UI if suggested, or manually via:

```bash
kubectl patch resourcereleasebinding snip-postgres-development -n default --type=json -p '[{"op":"remove","path":"/spec/resourceTypeEnvironmentConfigs/memory"}]'
```

Note this removes only the `memory` override, not the whole `resourceTypeEnvironmentConfigs` map — `persistenceEnabled: true` (set in `resources/postgres.yaml`) needs to stay in place, otherwise Postgres would fall back to `emptyDir` on the next pod recreate and the data would be lost anyway.

## Cleanup

None of these objects use Kubernetes owner references, so deleting the `Project` alone leaves the Resource, bindings, and notification channel behind. Delete everything explicitly instead — this also tears down Postgres's `StatefulSet` and its PVC (via `persistentVolumeClaimRetentionPolicy.whenDeleted: Delete`, set when `persistenceEnabled` is true) through the `ResourceReleaseBinding`'s finalizer:

```bash
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/url-shortener/components/frontend.yaml
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/url-shortener/components/analytics-service.yaml
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/url-shortener/components/api-service.yaml
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/url-shortener/components/redis.yaml
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/url-shortener/resources/postgres.yaml
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/url-shortener/project.yaml
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/url-shortener/alerting-demo/alert-notification-channels.yaml
```

(`enable-alert.yaml` and `failure-scenario.yaml` reuse the same `ReleaseBinding`/`ResourceReleaseBinding` names created above, so no separate delete is needed for those.)
