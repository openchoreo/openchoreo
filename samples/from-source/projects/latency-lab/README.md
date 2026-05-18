# Latency Lab on OpenChoreo

OpenChoreo manifests for the [latency-lab](https://github.com/openchoreo/sample-workloads/tree/main/project-latency-lab) sample — a notes microservice with conditional latency / fault injection via query params. The shape mirrors
[`samples/from-source/projects/url-shortener`](https://github.com/openchoreo/openchoreo/tree/main/samples/from-source/projects/url-shortener).

## Layout

```text
project-latency-lab/
├── project.yaml                       # Project: latency-lab
└── components/
    ├── postgres.yaml                  # lab-postgres          (deployment/service)
    ├── redis.yaml                     # lab-redis             (deployment/service)
    ├── auth-service.yaml              # lab-auth-service      (deployment/service)
    ├── api-service.yaml               # lab-api-service        (deployment/service)
    ├── api-service-broken.yaml        # lab-api-service-broken (deployment/service — intentionally fails to build)
    ├── analytics-service.yaml         # lab-analytics-service  (deployment/service)
    └── frontend.yaml                  # lab-frontend           (deployment/web-application + 5xx alert)
```

Each component pairs a `Component` resource with a one-shot `WorkflowRun` that builds the image via the cluster `dockerfile-builder` workflow.

## Prerequisites

- An OpenChoreo cluster (control plane + observability plane).
- `kubectl` access.
- Source is pulled from [`github.com/openchoreo/sample-workloads`](https://github.com/openchoreo/sample-workloads/tree/main/project-latency-lab) on `main`. If you're working off a fork, swap the URL in `project-latency-lab/components/*.yaml` first.

## Deploy

The frontend component declares an `observability-alert-rule` trait that is
not part of the default OpenChoreo install, and the trait requires a
notification channel to satisfy its CEL validation. Apply both before the
components:

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v1.1/samples/component-alerts/alert-rule-trait.yaml
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v1.1/samples/component-alerts/alert-notification-channels.yaml
```

Then apply the project and components directly from GitHub (no checkout
required):

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v1.1/samples/from-source/projects/latency-lab/project.yaml
kubectl apply \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v1.1/samples/from-source/projects/latency-lab/components/postgres.yaml \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v1.1/samples/from-source/projects/latency-lab/components/redis.yaml \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v1.1/samples/from-source/projects/latency-lab/components/auth-service.yaml \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v1.1/samples/from-source/projects/latency-lab/components/api-service.yaml \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v1.1/samples/from-source/projects/latency-lab/components/analytics-service.yaml \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v1.1/samples/from-source/projects/latency-lab/components/frontend.yaml
```

> `api-service-broken.yaml` is intentionally left out of the initial apply — it
> is an optional demo-only manifest used to show how OpenChoreo surfaces build
> failures. See the **Build-failure demo** section below for how to apply it
> separately.

The frontend ships with an `observability-alert-rule` trait that fires when
more than 5 HTTP-500s appear in the logs within a minute — easy to trigger
on demand by hitting any endpoint with `?fail_rate=1`.

## Build-failure demo

`api-service-broken.yaml` deploys a sibling component whose `main.go`
references an undefined symbol, so the `dockerfile-builder` `WorkflowRun`
fails at `go build`. Apply it on its own to demo how OpenChoreo surfaces
build failures:

```bash
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v1.1/samples/from-source/projects/latency-lab/components/api-service-broken.yaml
kubectl get workflowrun lab-api-service-broken-build-01 -o yaml
```

## Cleanup

```bash
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v1.1/samples/from-source/projects/latency-lab/project.yaml
```
