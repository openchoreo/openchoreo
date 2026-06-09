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
kubectl apply \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/projects/url-shortener/components/postgres.yaml \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/projects/url-shortener/components/redis.yaml \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/projects/url-shortener/components/api-service.yaml \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/projects/url-shortener/components/analytics-service.yaml \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/projects/url-shortener/components/frontend.yaml
```

This deploys the notification channel first, then five components (snip-postgres, snip-redis, snip-api-service, snip-analytics-service, snip-frontend). The alert trait is already attached to the frontend component. Distributed tracing works out of the box once deployed.

To set up alerting and the RCA agent, follow the steps in the [Alerting & RCA Agent](../../../from-image/url-shortener/README.md#alerting--rca-agent) section of the from-image README.

## Cleanup

```bash
kubectl delete -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-source/projects/url-shortener/project.yaml
```
