# Snip URL Shortener on OpenChoreo

A sample application that demonstrates OpenChoreo's **tracing**, **alerting**, and **RCA agent** capabilities using a multi-service URL shortener.

## Prerequisites

- An OpenChoreo cluster with the control plane and observability plane installed (and RCA agent setup for RCA)
- `kubectl` access to the cluster

## Deploy

```bash
kubectl apply -f samples/from-source/projects/url-shortener/project.yaml
kubectl apply -f samples/from-source/projects/url-shortener/components/
```

This deploys five components (snip-postgres, snip-redis, snip-api-service, snip-analytics-service, snip-frontend). Distributed tracing works out of the box once deployed.

For alerting and the RCA agent, follow the setup below.

## Alerting & RCA Agent

A log-based alert rule on the frontend triggers when `status=500` appears more than 5 times within 1 minute.

### Setup

The alert trait is already attached to the snip-frontend component. Set up the notification channel and enable the alert:

```bash
kubectl apply -f samples/from-source/projects/url-shortener/alerting-demo/alert-notification-channels.yaml
kubectl apply -f samples/from-source/projects/url-shortener/alerting-demo/enable-alert.yaml
```

### Trigger the Alert

`failure-scenario.yaml` misconfigures the api-service's `POSTGRES_DSN` to point to a non-existent host. The api-service starts but every DB query fails, returning 500s. This breaches the alert threshold. The RCA agent then traces from the frontend alert to api-service 500s to Postgres connection errors to the misconfigured DSN.

Start generating traffic (auto-detects the frontend URL from the ReleaseBinding):

```bash
chmod +x samples/from-source/projects/url-shortener/alerting-demo/trigger-alerts.sh
./samples/from-source/projects/url-shortener/alerting-demo/trigger-alerts.sh
```

Inject the misconfigured Postgres DSN:

```bash
kubectl apply -f samples/from-source/projects/url-shortener/alerting-demo/failure-scenario.yaml
```

After the alert fires, revert by applying the fix from the UI if suggested, or manually via:

```bash
kubectl patch releasebinding snip-api-service-development --type=json -p '[{"op":"remove","path":"/spec/workloadOverrides"}]'
```

## Cleanup

```bash
kubectl delete -f samples/from-source/projects/url-shortener/project.yaml
```
